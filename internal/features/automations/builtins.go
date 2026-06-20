package automations

import (
	"encoding/json"
	"strings"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/welcome"
)

// Builtin is a read-only automation that Dia ships and a managed feature owns.
// It appears in the automations list so admins can SEE exactly how a feature
// like Welcome behaves as an event flow, but it can't be edited there — the
// FeatureTab deep link points at the feature's own settings page. The owning
// feature (not the automations engine) actually runs it at runtime, so the
// flow shown is an honest, generated mirror of the live config rather than a
// second executor.
type Builtin struct {
	Key         string        `json:"key"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	TriggerType string        `json:"trigger_type"`
	FeatureKey  string        `json:"feature_key"`  // owning feature key
	FeatureName string        `json:"feature_name"` // human label
	FeatureTab  string        `json:"feature_tab"`  // dashboard route segment to configure it
	Enabled     bool          `json:"enabled"`      // reflects the live feature/sub-toggle
	Definition  cc.Definition `json:"-"`            // generated, read-only step tree
}

// BuildBuiltins renders the catalogue of built-in automations for a guild from
// each owning feature's live config. configs maps a feature key to its raw
// JSONB config (nil/missing → that feature's defaults), and featureEnabled maps
// a feature key to its top-level toggle.
func BuildBuiltins(configs map[string]json.RawMessage, featureEnabled map[string]bool) []Builtin {
	var out []Builtin

	// ── Welcome ──────────────────────────────────────────────────────────────
	wcfg := welcome.Default()
	if raw := configs[welcome.FeatureKey]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &wcfg)
	}
	wEnabled := featureEnabled[welcome.FeatureKey]
	out = append(out,
		Builtin{
			Key:         "welcome.join",
			Name:        "Welcome new members",
			Description: "Greets members when they join, with a message, embed, and card image. Managed on the Welcome tab.",
			TriggerType: "member_join",
			FeatureKey:  welcome.FeatureKey,
			FeatureName: "Welcome",
			FeatureTab:  "welcome",
			Enabled:     wEnabled && wcfg.Welcome.Enabled,
			Definition:  welcomeFlow(wcfg.Welcome),
		},
		Builtin{
			Key:         "welcome.leave",
			Name:        "Farewell leaving members",
			Description: "Posts a goodbye when a member leaves. Managed on the Welcome tab.",
			TriggerType: "member_leave",
			FeatureKey:  welcome.FeatureKey,
			FeatureName: "Welcome",
			FeatureTab:  "welcome",
			Enabled:     wEnabled && wcfg.Goodbye.Enabled,
			Definition:  welcomeFlow(wcfg.Goodbye),
		},
	)

	return out
}

// welcomeFlow renders one welcome/goodbye MessageConfig as an illustrative
// read-only step tree: an optional DM, then the channel post (content + embeds,
// with a note when a card image is attached).
func welcomeFlow(mc welcome.MessageConfig) cc.Definition {
	if !mc.Enabled {
		return cc.Definition{Steps: []cc.Step{{
			ID:   "builtin-disabled",
			Kind: cc.KindNoop,
		}}}
	}

	var steps []cc.Step

	if mc.DM.Enabled && (mc.DM.Content != "" || len(mc.DM.Components) > 0 || len(mc.DM.Embeds) > 0) {
		dm := cc.SpecSendDM{
			User:    cc.Expr{Src: "{{ .User.ID }}"},
			Content: tokensToTmpl(mc.DM.Content),
		}
		for _, e := range mc.DM.Embeds {
			if !e.Enabled {
				continue
			}
			dm.Embeds = append(dm.Embeds, welcomeEmbed(e))
		}
		dm.Components = welcomeComponents(mc.DM.Components)
		steps = append(steps, cc.Step{ID: "builtin-dm", Kind: cc.KindSendDM, Spec: mustSpec(dm)})
		// The DM's own click-router, right after it, so the canvas fuses its
		// button dots independently of the channel message's.
		steps = append(steps, clickRouter("builtin-dm", mc.DM.Components, mc.DM.Actions)...)
	}

	if mc.Card.Enabled {
		// Illustrative: the Welcome feature renders the card image itself and
		// attaches it to the message below. Shown as its own step so the flow
		// reads truthfully (you can see the image is generated).
		steps = append(steps, cc.Step{
			ID:   "builtin-card",
			Kind: cc.KindImageRender,
			Spec: mustSpec(cc.SpecImageRender{Into: "welcome_card"}),
		})
	}

	send := cc.SpecSendMessage{
		Channel: cc.Expr{Src: channelExpr(mc.ChannelID)},
		Content: tokensToTmpl(mc.Content),
	}
	for _, e := range mc.Embeds {
		if !e.Enabled {
			continue
		}
		send.Embeds = append(send.Embeds, welcomeEmbed(e))
	}
	send.Components = welcomeComponents(mc.Components)
	if mc.Card.Enabled {
		send.Attachments = append(send.Attachments, cc.AttachmentRef{
			FromVar:  "welcome_card",
			Filename: "card.png",
		})
	}
	steps = append(steps, cc.Step{
		ID:   "builtin-send",
		Kind: cc.KindSendMessage,
		Spec: mustSpec(send),
	})

	steps = append(steps, clickRouter("builtin", mc.Components, mc.Actions)...)

	// The post-message tail: the editable steps the admin wired after the
	// channel message ("connect a new action after sending a message"). Unlike
	// the spine above (regenerated, read-only), these are real, persisted steps,
	// so they render as ordinary draggable nodes off the message's out handle.
	steps = append(steps, mc.Tail...)

	return cc.Definition{Steps: steps}
}

// welcomeComponents mirrors the configured button / select rows into the
// read-only flow, with token shorthands rendered to the canonical template
// form. Non-link components stay routable (they keep their suffix and no
// "none" on_click), so each shows a draggable dot on the canvas for wiring its
// click action; link buttons keep their URL.
func welcomeComponents(rows []cc.ComponentRow) []cc.ComponentRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]cc.ComponentRow, 0, len(rows))
	for _, row := range rows {
		comps := make([]cc.Component, 0, len(row.Components))
		for _, c := range row.Components {
			c.Label = tokensToTmpl(c.Label)
			c.Placeholder = tokensToTmpl(c.Placeholder)
			c.URL = tokensToTmpl(c.URL)
			comps = append(comps, c)
		}
		out = append(out, cc.ComponentRow{Components: comps})
	}
	return out
}

// clickRouter renders the per-component click programs (actions) for one surface
// as the canvas's click-router plumbing: one any-component wait_for plus a switch
// on the clicked id, with one case per wired (and still-live) suffix. idPrefix
// namespaces the two synthetic steps ("builtin" for the channel message,
// "builtin-dm" for the DM) so each surface gets its own router. The canvas fuses
// each pair into the per-button "on click" lines the user drags to edit.
//
// Skipping cases for a since-deleted component keeps the canvas fusion intact
// (the adapter requires every switch case to map to a live suffix on the
// preceding message) instead of surfacing the raw wait_for/switch as nodes.
func clickRouter(idPrefix string, comps []cc.ComponentRow, actions []welcome.ButtonAction) []cc.Step {
	live := make(map[string]bool)
	for _, row := range comps {
		for _, comp := range row.Components {
			if comp.CustomIDSuffix == "" || comp.OnClick == "none" || strings.EqualFold(comp.Style, "link") {
				continue
			}
			live[comp.CustomIDSuffix] = true
		}
	}
	var cases []cc.SwitchCase
	for _, a := range actions {
		if a.Suffix == "" || len(a.Steps) == 0 || !live[a.Suffix] {
			continue
		}
		cases = append(cases, cc.SwitchCase{
			When: cc.Expr{Lang: "tmpl", Src: a.Suffix},
			Do:   a.Steps,
		})
	}
	if len(cases) == 0 {
		return nil
	}
	return []cc.Step{
		{
			ID:   idPrefix + "-click-wait",
			Kind: cc.KindWaitFor,
			Spec: mustSpec(cc.SpecWaitFor{Trigger: "component", Into: "click", Timeout: "30s"}),
		},
		{
			ID:    idPrefix + "-click-switch",
			Kind:  cc.KindSwitch,
			Spec:  mustSpec(cc.SpecSwitch{On: cc.Expr{Lang: "tmpl", Src: "{{ .Vars.click.id }}"}}),
			Cases: cases,
		},
	}
}

func welcomeEmbed(e welcome.EmbedConfig) cc.EmbedSpec {
	out := cc.EmbedSpec{
		Title:       tokensToTmpl(e.Title),
		Description: tokensToTmpl(e.Description),
		URL:         e.URL,
		Color:       e.Color,
		AuthorName:  tokensToTmpl(e.AuthorName),
		AuthorIcon:  tokensToTmpl(e.AuthorIcon),
		Thumbnail:   tokensToTmpl(e.Thumbnail),
		ImageURL:    tokensToTmpl(e.ImageURL),
		FooterText:  tokensToTmpl(e.FooterText),
		FooterIcon:  tokensToTmpl(e.FooterIcon),
		Timestamp:   e.Timestamp,
	}
	for _, f := range e.Fields {
		out.Fields = append(out.Fields, cc.EmbedField{
			Name:   tokensToTmpl(f.Name),
			Value:  tokensToTmpl(f.Value),
			Inline: f.Inline,
		})
	}
	return out
}

func channelExpr(id string) string {
	if id == "" {
		return "{{ .Channel.ID }}"
	}
	return id
}

func mustSpec(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// tokensToTmpl rewrites the Welcome feature's legacy {brace} shorthands into Go
// template syntax so the built-in visualization only ever shows the canonical
// `{{ ... }}` form (the templating contract forbids surfacing legacy sugar).
var welcomeTokenReplacer = strings.NewReplacer(
	"{user.mention}", "{{ .User.Mention }}",
	"{user.id}", "{{ .User.ID }}",
	"{user.name}", "{{ .User.Username }}",
	"{user.username}", "{{ .User.Username }}",
	"{user.global}", "{{ .User.GlobalName }}",
	"{user.avatar}", "{{ .User.Avatar }}",
	"{user}", "{{ .User.GlobalName }}",
	"{server.name}", "{{ .Guild.Name }}",
	"{server.id}", "{{ .Guild.ID }}",
	"{server}", "{{ .Guild.Name }}",
	"{count}", "{{ .Guild.MemberCount }}",
	"{channel.id}", "{{ .Channel.ID }}",
	"{channel}", "{{ mentionChannel .Channel.ID }}",
)

func tokensToTmpl(s string) string {
	if s == "" {
		return ""
	}
	return welcomeTokenReplacer.Replace(s)
}
