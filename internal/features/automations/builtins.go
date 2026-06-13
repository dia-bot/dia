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

	if mc.DM.Enabled && mc.DM.Content != "" {
		steps = append(steps, cc.Step{
			ID:   "builtin-dm",
			Kind: cc.KindSendDM,
			Spec: mustSpec(cc.SpecSendDM{
				User:    cc.Expr{Src: "{{ .User.ID }}"},
				Content: tokensToTmpl(mc.DM.Content),
			}),
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
	if mc.Card.Enabled {
		// The card is rendered by the Welcome feature itself; represent it as an
		// attached image so the flow reads truthfully.
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

	return cc.Definition{Steps: steps}
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
