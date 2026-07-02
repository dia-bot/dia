package automations

import (
	"encoding/json"
	"fmt"
	"strings"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/leveling"
	"github.com/dia-bot/dia/internal/features/roles"
	"github.com/dia-bot/dia/internal/features/welcome"
	"github.com/dia-bot/dia/internal/store"
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
// JSONB config (nil/missing → that feature's defaults), featureEnabled maps a
// feature key to its top-level toggle, and menus is the guild's reaction-role
// menus (each posted menu contributes one built-in).
func BuildBuiltins(configs map[string]json.RawMessage, featureEnabled map[string]bool, menus []store.ReactionRoleMenu) []Builtin {
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

	// ── Leveling ─────────────────────────────────────────────────────────────
	lcfg := leveling.Default()
	if raw := configs[leveling.FeatureKey]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &lcfg)
	}
	lEnabled := featureEnabled[leveling.FeatureKey]
	out = append(out, Builtin{
		Key:         "leveling.levelup",
		Name:        "Announce level-ups",
		Description: "Posts a message (and optional buttons) when a member reaches a new level. Managed on the Leveling tab.",
		TriggerType: "level_up",
		FeatureKey:  leveling.FeatureKey,
		FeatureName: "Leveling",
		FeatureTab:  "leveling",
		Enabled:     lEnabled && lcfg.AnnounceLevelUp,
		Definition:  levelingFlow(lcfg),
	})

	// ── Auto-roles ───────────────────────────────────────────────────────────
	rcfg := roles.Default()
	if raw := configs[roles.FeatureKey]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &rcfg)
	}
	rEnabled := featureEnabled[roles.FeatureKey]
	out = append(out, Builtin{
		Key:         "autorole.join",
		Name:        "Grant roles on join",
		Description: "Grants the configured roles when a member joins (after screening, if enabled). Managed on the Auto-roles tab.",
		TriggerType: "member_join",
		FeatureKey:  roles.FeatureKey,
		FeatureName: "Roles",
		FeatureTab:  "auto-roles",
		Enabled:     rEnabled && len(rcfg.Roles) > 0,
		Definition:  autoroleFlow(rcfg),
	})

	// ── Reaction roles (one built-in per menu) ───────────────────────────────
	rrEnabled := featureEnabled[roles.ReactionRolesKey]
	for _, m := range menus {
		name := m.Title
		if name == "" {
			name = "Untitled menu"
		}
		var opts []roles.Option
		if len(m.Options) > 0 {
			_ = json.Unmarshal(m.Options, &opts)
		}
		posted := m.MessageID != 0 && m.ChannelID != 0
		out = append(out, Builtin{
			Key:         fmt.Sprintf("reactionroles.menu.%d", m.ID),
			Name:        "Reaction roles: " + name,
			Description: "Applies the roles a member picks from this menu, then runs your follow-up flow. Managed on the Reaction Roles tab.",
			TriggerType: "reaction_role_pick",
			FeatureKey:  roles.ReactionRolesKey,
			FeatureName: "Reaction Roles",
			FeatureTab:  "reaction-roles",
			Enabled:     rrEnabled && posted && len(opts) > 0,
			Definition:  menuFlow(m, opts, posted),
		})
	}

	return out
}

// menuFlow renders one reaction-role menu as a read-only step spine (a role_add
// step summarizing the pickable roles) followed by the editable follow-up tail.
// Mirrors autoroleFlow: the spine steps carry "builtin-" ids so the canvas
// treats them as generated/read-only, while the menu's tail is appended as
// real, persisted steps (their own non-"builtin-" ids) that render as ordinary
// draggable nodes off the spine's out handle.
func menuFlow(m store.ReactionRoleMenu, opts []roles.Option, posted bool) cc.Definition {
	var tail []cc.Step
	if len(m.Tail) > 0 {
		_ = json.Unmarshal(m.Tail, &tail)
	}

	if !posted || len(opts) == 0 {
		// Keep the tail behind the disabled spine so an authored follow-up flow
		// isn't dropped (and can't be silently wiped by a canvas round-trip) just
		// because the menu is momentarily unposted or empty.
		return cc.Definition{Steps: append([]cc.Step{{
			ID:   "builtin-disabled",
			Kind: cc.KindNoop,
		}}, tail...)}
	}

	// A single illustrative role_add naming every pickable role. The feature
	// applies the picked roles per the menu's mode (toggle / unique / verify);
	// the reason mirrors the runtime's "reaction role" reason. The options
	// collapse into one comma-joined summary so the spine reads as one honest
	// "apply picked roles" node.
	roleIDs := make([]string, 0, len(opts))
	for _, o := range opts {
		roleIDs = append(roleIDs, o.RoleID)
	}
	apply := cc.SpecRole{
		User:   cc.Expr{Src: "{{ .User.ID }}"},
		Role:   cc.Expr{Src: strings.Join(roleIDs, ", ")},
		Reason: "reaction role",
	}
	steps := []cc.Step{{
		ID:   "builtin-apply",
		Kind: cc.KindRoleAdd,
		Spec: mustSpec(apply),
	}}

	// The editable follow-up tail, wired after the apply spine on the canvas.
	steps = append(steps, tail...)

	return cc.Definition{Steps: steps}
}

// autoroleFlow renders the auto-roles grant as a read-only step spine (a
// role_add step summarizing the configured roles) followed by the editable
// post-grant tail. Mirrors welcomeFlow: the spine steps carry "builtin-" ids so
// the canvas treats them as generated/read-only, while cfg.Tail is appended as
// real, persisted steps (their own non-"builtin-" ids) that render as ordinary
// draggable nodes off the spine's out handle.
func autoroleFlow(cfg roles.Config) cc.Definition {
	if len(cfg.Roles) == 0 {
		// Keep the tail behind the disabled spine so an authored follow-up flow
		// isn't dropped (and can't be silently wiped by a canvas round-trip) just
		// because the grant list is momentarily empty.
		return cc.Definition{Steps: append([]cc.Step{{
			ID:   "builtin-disabled",
			Kind: cc.KindNoop,
		}}, cfg.Tail...)}
	}

	// A single illustrative role_add naming every configured role. Auto-roles
	// grants each role to the joining member; the reason mirrors the runtime's
	// "autorole" reason. Multiple roles collapse into one comma-joined,
	// newline-separated summary rather than a per-role fan-out, so the spine reads
	// as one honest "grant roles" node.
	grant := cc.SpecRole{
		User:   cc.Expr{Src: "{{ .User.ID }}"},
		Role:   cc.Expr{Src: strings.Join(cfg.Roles, ", ")},
		Reason: "autorole",
	}
	steps := []cc.Step{{
		ID:   "builtin-grant",
		Kind: cc.KindRoleAdd,
		Spec: mustSpec(grant),
	}}

	// The editable post-grant tail, wired after the grant spine on the canvas.
	steps = append(steps, cfg.Tail...)

	return cc.Definition{Steps: steps}
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
		steps = append(steps, clickRouter("builtin-dm", mc.DM.Components, welcomeClickActions(mc.DM.Actions))...)
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

	steps = append(steps, clickRouter("builtin", mc.Components, welcomeClickActions(mc.Actions))...)

	// The post-message tail: the editable steps the admin wired after the
	// channel message ("connect a new action after sending a message"). Unlike
	// the spine above (regenerated, read-only), these are real, persisted steps,
	// so they render as ordinary draggable nodes off the message's out handle.
	steps = append(steps, mc.Tail...)

	return cc.Definition{Steps: steps}
}

// levelingFlow renders the leveling level-up announcement as a read-only step
// tree mirroring welcomeFlow: the channel (or DM) post with content, embeds and
// buttons, then the per-button click router and the editable post-message tail.
// The read-only visualization uses the leveling token replacer so the extra
// {level}/{rank}/{xp}/{progress} shorthands surface in canonical `{{ }}` form.
func levelingFlow(cfg leveling.Config) cc.Definition {
	if !cfg.AnnounceLevelUp {
		return cc.Definition{Steps: []cc.Step{{
			ID:   "builtin-disabled",
			Kind: cc.KindNoop,
		}}}
	}

	// The rich announcement body, falling back to the legacy single-line message
	// when the composer content is empty (mirrors announce()'s hasLevelUp path).
	content := cfg.LevelUp.Content
	if content == "" {
		content = cfg.LevelUpMessage
	}

	var steps []cc.Step

	if cfg.AnnounceChannel == "dm" {
		// DM announcements are content-only at runtime; components have no DM
		// route for level-ups, so the visualization omits them too.
		dm := cc.SpecSendDM{
			User:    cc.Expr{Src: "{{ .User.ID }}"},
			Content: levelingTokensToTmpl(content),
		}
		for _, e := range cfg.LevelUp.Embeds {
			dm.Embeds = append(dm.Embeds, levelingEmbed(e))
		}
		steps = append(steps, cc.Step{ID: "builtin-send", Kind: cc.KindSendDM, Spec: mustSpec(dm)})
		steps = append(steps, cfg.Tail...)
		return cc.Definition{Steps: steps}
	}

	send := cc.SpecSendMessage{
		Channel: cc.Expr{Src: channelExpr(cfg.AnnounceChannel)},
		Content: levelingTokensToTmpl(content),
	}
	for _, e := range cfg.LevelUp.Embeds {
		send.Embeds = append(send.Embeds, levelingEmbed(e))
	}
	send.Components = levelingComponents(cfg.LevelUp.Components)
	steps = append(steps, cc.Step{
		ID:   "builtin-send",
		Kind: cc.KindSendMessage,
		Spec: mustSpec(send),
	})

	steps = append(steps, clickRouter("builtin", cfg.LevelUp.Components, levelingClickActions(cfg.Actions))...)

	// The editable post-announce tail, wired after the message on the canvas.
	steps = append(steps, cfg.Tail...)

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

// clickAction is the feature-agnostic shape clickRouter consumes: a component's
// custom_id suffix and the durable steps its click runs. Each owning feature's
// local ButtonAction (welcome.ButtonAction, leveling.ButtonAction, …) maps onto
// it so the same router serves every builtin flow.
type clickAction struct {
	Suffix string
	Steps  []cc.Step
}

// welcomeClickActions adapts welcome's ButtonActions to the router shape.
func welcomeClickActions(actions []welcome.ButtonAction) []clickAction {
	out := make([]clickAction, len(actions))
	for i, a := range actions {
		out[i] = clickAction{Suffix: a.Suffix, Steps: a.Steps}
	}
	return out
}

// levelingClickActions adapts leveling's ButtonActions to the router shape.
func levelingClickActions(actions []leveling.ButtonAction) []clickAction {
	out := make([]clickAction, len(actions))
	for i, a := range actions {
		out[i] = clickAction{Suffix: a.Suffix, Steps: a.Steps}
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
//
// The actions are passed as a feature-agnostic []clickAction so each owning
// feature (welcome, leveling, …) can reuse the router with its own local
// ButtonAction type.
func clickRouter(idPrefix string, comps []cc.ComponentRow, actions []clickAction) []cc.Step {
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

// levelingTokenReplacer extends the shared shorthands with the leveling-specific
// {level}/{rank}/{xp}/{progress} tokens the level-up composer documents, so the
// read-only builtin flow renders them in canonical `{{ }}` form.
var levelingTokenReplacer = strings.NewReplacer(
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
	"{level}", "{{ .Event.level }}",
	"{rank}", "{{ .Event.rank }}",
	"{xp}", "{{ .Event.xp }}",
	"{progress}", "{{ .Event.progress }}",
)

func levelingTokensToTmpl(s string) string {
	if s == "" {
		return ""
	}
	return levelingTokenReplacer.Replace(s)
}

// levelingComponents mirrors the level-up button / select rows into the
// read-only flow (leveling token shorthands rendered to canonical form),
// keeping non-link components routable so each shows a draggable click dot.
func levelingComponents(rows []cc.ComponentRow) []cc.ComponentRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]cc.ComponentRow, 0, len(rows))
	for _, row := range rows {
		comps := make([]cc.Component, 0, len(row.Components))
		for _, c := range row.Components {
			c.Label = levelingTokensToTmpl(c.Label)
			c.Placeholder = levelingTokensToTmpl(c.Placeholder)
			c.URL = levelingTokensToTmpl(c.URL)
			comps = append(comps, c)
		}
		out = append(out, cc.ComponentRow{Components: comps})
	}
	return out
}

// levelingEmbed token-maps one composer embed (already a cc.EmbedSpec) into the
// read-only flow using the leveling replacer.
func levelingEmbed(e cc.EmbedSpec) cc.EmbedSpec {
	out := cc.EmbedSpec{
		Title:       levelingTokensToTmpl(e.Title),
		Description: levelingTokensToTmpl(e.Description),
		URL:         e.URL,
		Color:       e.Color,
		AuthorName:  levelingTokensToTmpl(e.AuthorName),
		AuthorIcon:  levelingTokensToTmpl(e.AuthorIcon),
		AuthorURL:   e.AuthorURL,
		Thumbnail:   levelingTokensToTmpl(e.Thumbnail),
		ImageURL:    levelingTokensToTmpl(e.ImageURL),
		FooterText:  levelingTokensToTmpl(e.FooterText),
		FooterIcon:  levelingTokensToTmpl(e.FooterIcon),
		Timestamp:   e.Timestamp,
	}
	for _, f := range e.Fields {
		out.Fields = append(out.Fields, cc.EmbedField{
			Name:   levelingTokensToTmpl(f.Name),
			Value:  levelingTokensToTmpl(f.Value),
			Inline: f.Inline,
		})
	}
	return out
}
