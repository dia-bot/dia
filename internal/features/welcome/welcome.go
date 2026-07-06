// Package welcome posts configurable welcome/goodbye messages — plain content,
// a full embed, and/or a rendered card image — when members join or leave.
package welcome

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/features/automations/runner"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/dia-bot/dia/internal/tmpllookup"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// pid parses a snowflake string to int64 (0 on failure), for KV owner ids.
func pid(s string) int64 { id, _ := event.ParseID(s); return id }

// Plugin implements the welcome feature.
type Plugin struct {
	// runner runs the durable flows welcome owns — the per-button click-action
	// programs (welcome.Actions) and the post-message tail (welcome.Tail) — on
	// the shared automations machinery: it persists a parked run to
	// automation_runs and emits "auto:" components, so waits, modals and
	// follow-up clicks resume through the automations plugin's handlers + the
	// wait scheduler.
	runner *runner.Runner
}

// New returns the welcome plugin.
func New() *Plugin { return &Plugin{} }

const (
	// componentPrefix namespaces this feature's component clicks. A posted
	// welcome message mints custom_ids "welcome:<tab>:<suffix>" so a click
	// routes back here (and never collides with custom-command runs).
	componentPrefix = "welcome:"
	tabWelcome      = "welcome"
	tabGoodbye      = "goodbye"

	// dmTag marks a component posted in a DM. A channel click is
	// "welcome:<tab>:<suffix>"; a DM click is "welcome:dm:<tab>:<guildID>:<suffix>"
	// because a DM interaction carries no guild, so the guild id has to ride in
	// the custom_id for the click to find its way back to this guild's config.
	dmTag = "dm"

	// legacyRunPrefix is the prefix older deployments minted for components a
	// synchronous click action emitted. Click actions now run as durable
	// automation runs (their components mint the "auto:" prefix and resume
	// through the automations plugin), so this only survives to silently ack any
	// such button still live on an old message instead of erroring.
	legacyRunPrefix = "welcomerun:"
)

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Welcome",
		Description: "Greet joining members and bid farewell to leaving ones with custom messages, embeds and card images.",
		Category:    plugin.CategoryEngagement,
	}
}

// Init wires the join/leave handlers, the welcome:* component handler that runs
// per-button click actions, and the /welcome test command.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.runner = runner.New(d)

	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return p.handleJoin(ctx, d, env)
	})
	reg.OnEvent(event.TypeMemberRemove, func(ctx context.Context, env *event.Envelope) error {
		return p.handleLeave(ctx, d, env)
	})

	// Clicks on a posted welcome message route here (custom_id
	// "welcome:<tab>:<suffix>"); a wired Action runs as a durable flow, an
	// unwired one is acked silently. These buttons are persistent: the custom_id
	// carries no run reference, so they keep working for the life of the message,
	// and each click spins up a fresh run.
	reg.Component(componentPrefix, func(c *interactions.Context) error {
		return p.handleComponent(c, d)
	})
	// Back-compat: silently ack a component an old synchronous click action left
	// on a message (durable runs mint the "auto:" prefix and resume elsewhere).
	reg.Component(legacyRunPrefix, func(c *interactions.Context) error {
		return c.DeferUpdate()
	})

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("welcome",
			"Preview and manage the welcome message",
			interactions.SubCommand("test", "Send a test welcome message for yourself"),
		)),
		Handler: func(c *interactions.Context) error { return handleTest(c, d) },
	})
	return nil
}

func (p *Plugin) handleJoin(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	ma, err := plugin.DecodeData[event.MemberAdd](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(ma.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled || !cfg.Welcome.Enabled {
		return err
	}
	name, count, icon := guildInfo(ctx, d, gid, ma.MemberCount)
	v := Vars{user: ma.Member.User, guildID: ma.GuildID, server: name, serverIcon: discord.GuildIconURL(ma.GuildID, icon, 256), count: count, lookup: tmpllookup.New(ctx, d.GuildState, ma.GuildID), fonts: guildFonts(ctx, d, gid), kv: d.Store.FeatureKV.CardLookup(gid, pid(ma.Member.User.ID))}
	if err := sendConfigured(ctx, d, cfg.Welcome, v, tabWelcome); err != nil {
		return err
	}
	p.runTail(ctx, d, cfg.Welcome, v, &ma.Member, name, count, "welcome.join", "member_join",
		map[string]any{"member_count": ma.MemberCount, "pending": ma.Member.Pending})
	return nil
}

func (p *Plugin) handleLeave(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	mr, err := plugin.DecodeData[event.MemberRemove](env)
	if err != nil {
		return err
	}
	gid, _ := event.ParseID(mr.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled || !cfg.Goodbye.Enabled {
		return err
	}
	name, count, icon := guildInfo(ctx, d, gid, mr.MemberCount)
	v := Vars{user: mr.User, guildID: mr.GuildID, server: name, serverIcon: discord.GuildIconURL(mr.GuildID, icon, 256), count: count, lookup: tmpllookup.New(ctx, d.GuildState, mr.GuildID), fonts: guildFonts(ctx, d, gid), kv: d.Store.FeatureKV.CardLookup(gid, pid(mr.User.ID))}
	if err := sendConfigured(ctx, d, cfg.Goodbye, v, tabGoodbye); err != nil {
		return err
	}
	p.runTail(ctx, d, cfg.Goodbye, v, nil, name, count, "welcome.leave", "member_leave",
		map[string]any{"member_count": mr.MemberCount})
	return nil
}

// runTail runs the post-message flow (mc.Tail) as a durable automation run once
// the join/leave message has been posted. The scope mirrors an automations
// member_join / member_leave run — same .User / .Guild / .Event vars — so a tail
// authored on the canvas behaves exactly like a hand-built automation. Nothing
// runs (and nothing persists) when the tail is empty.
func (p *Plugin) runTail(ctx context.Context, d plugin.Deps, mc MessageConfig, v Vars, member *event.Member, guildName string, count int, label, triggerKind string, eventMap map[string]any) {
	if len(mc.Tail) == 0 || p.runner == nil {
		return
	}
	guildCtx := cc.ContextGuild{ID: v.guildID, Name: guildName, MemberCount: count}
	ctxVars := cc.BuildContext(v.guildID, mc.ChannelID, v.user, member, guildCtx, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, v.guildID, ctxVars, nil, nil)
	scope.SetEvent(eventMap)
	p.runner.Start(ctx, runner.Meta{
		AutomationID: label,
		Version:      1,
		GuildID:      v.guildID,
		InvokerID:    v.user.ID,
		ActorID:      v.user.ID,
		ChannelID:    mc.ChannelID,
		TriggerKind:  triggerKind,
	}, cc.Definition{Steps: mc.Tail}, scope)
}

func handleTest(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil {
		return err
	}
	if !enabled || !cfg.Welcome.Enabled || cfg.Welcome.ChannelID == "" {
		return c.RespondEphemeral("Welcome is disabled or has no channel set. Configure it on the dashboard first.")
	}
	if err := c.Defer(true); err != nil {
		return err
	}
	name, count, icon := guildInfo(c.Ctx, d, gid, 0)
	v := Vars{user: c.User, guildID: c.GuildID, server: name, serverIcon: discord.GuildIconURL(c.GuildID, icon, 256), count: count, lookup: tmpllookup.New(c.Ctx, d.GuildState, c.GuildID), fonts: guildFonts(c.Ctx, d, gid), kv: d.Store.FeatureKV.CardLookup(gid, pid(c.User.ID))}
	if err := sendConfigured(c.Ctx, d, cfg.Welcome, v, tabWelcome); err != nil {
		_, e := c.FollowupContent("Failed to send test welcome: " + err.Error())
		return e
	}
	_, err = c.FollowupContent("✅ Sent a test welcome to <#" + cfg.Welcome.ChannelID + ">.")
	return err
}

// sendConfigured posts one configured message (optionally DMing the member),
// then sending the composed channel message. tab ("welcome" / "goodbye")
// namespaces the component custom_ids so clicks route to the right per-button
// actions.
func sendConfigured(ctx context.Context, d plugin.Deps, mc MessageConfig, v Vars, tab string) error {
	// Render the card once per event and share the PNG across surfaces: the
	// channel message attaches it whenever the card is on, and the DM attaches
	// the same image when DM.AttachCard is on (there is no separate DM card).
	var card []byte
	dmWantsCard := mc.DM.Enabled && mc.DM.AttachCard
	if mc.Card.Enabled && d.Imaging != nil && (mc.ChannelID != "" || dmWantsCard) {
		if png, err := renderCard(ctx, d.Imaging, mc.Card, v); err == nil {
			card = png
		}
	}
	if mc.DM.Enabled {
		// Build first, then send only if the DM actually rendered to something:
		// a config can pass the "has content/components/embeds" test yet render
		// empty (a template that yields "", a disabled embed, a zero-option select),
		// and an empty message is a 400 from Discord.
		dm := BuildDM(mc, v, tab, v.guildID)
		if dmWantsCard {
			attachCard(dm, card)
		}
		if dm.Content != "" || len(dm.Embeds) > 0 || len(dm.Components) > 0 || len(dm.Files) > 0 {
			if ch, err := d.Discord.Session().UserChannelCreate(v.user.ID); err == nil {
				if _, err := d.Discord.SendMessage(ch.ID, dm); err != nil {
					d.Log.Warn("welcome DM send failed", "tab", tab, "err", err)
				}
			}
		}
	}
	if mc.ChannelID == "" {
		return nil
	}
	_, err := d.Discord.SendMessage(mc.ChannelID, composeMessage(mc, v, tab, card))
	return err
}

// BuildMessage composes the channel message (content + optional embed + optional
// card image + components) for one MessageConfig, rendering the card itself. tab
// namespaces component custom_ids ("welcome:<tab>:<suffix>"). Exported so the
// dashboard's Test endpoint reuses the exact same rendering the bot uses at
// runtime.
func BuildMessage(ctx context.Context, img *imaging.Renderer, mc MessageConfig, v Vars, tab string) (*discordgo.MessageSend, error) {
	var card []byte
	if mc.Card.Enabled && img != nil {
		if png, err := renderCard(ctx, img, mc.Card, v); err == nil {
			card = png
		}
	}
	return composeMessage(mc, v, tab, card), nil
}

// composeMessage assembles the channel message from pre-rendered card bytes
// (nil = card off or its render failed): content + embeds (whose {card} image
// token resolves against the attachment) + components.
func composeMessage(mc MessageConfig, v Vars, tab string, card []byte) *discordgo.MessageSend {
	send := &discordgo.MessageSend{}
	cardAttached := attachCard(send, card)

	if c := v.render(mc.Content); c != "" {
		send.Content = c
	}
	for _, e := range mc.Embeds {
		if !e.Enabled {
			continue
		}
		send.Embeds = append(send.Embeds, buildEmbed(e, v, cardAttached))
	}
	if len(mc.Components) > 0 {
		send.Components = buildComponents(mc.Components, v, channelRoutePrefix(tab))
	}
	if !mc.PingUser {
		// Render mentions as text without pinging anyone.
		send.AllowedMentions = &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	}
	return send
}

// attachCard attaches pre-rendered card bytes to a message as "card.png",
// reporting whether it did; nil/empty bytes attach nothing.
func attachCard(send *discordgo.MessageSend, png []byte) bool {
	if len(png) == 0 {
		return false
	}
	send.Files = append(send.Files, &discordgo.File{Name: "card.png", ContentType: "image/png", Reader: bytes.NewReader(png)})
	return true
}

// BuildDM composes the private DM (content + optional embeds + optional
// components) for one MessageConfig's DM. Its components route to this feature's
// handler with the DM custom_id scheme (the guild id is embedded so a guild-less
// DM click can still be resolved). The card image is not rendered here: when
// DM.AttachCard is on, sendConfigured attaches the channel message's
// pre-rendered card after building, so the PNG is rendered once per event.
func BuildDM(mc MessageConfig, v Vars, tab, guildID string) *discordgo.MessageSend {
	send := &discordgo.MessageSend{}
	if c := v.render(mc.DM.Content); c != "" {
		send.Content = c
	}
	for _, e := range mc.DM.Embeds {
		if !e.Enabled {
			continue
		}
		send.Embeds = append(send.Embeds, buildEmbed(e, v, false))
	}
	if len(mc.DM.Components) > 0 {
		send.Components = buildComponents(mc.DM.Components, v, dmRoutePrefix(tab, guildID))
	}
	return send
}

// channelRoutePrefix / dmRoutePrefix mint the custom_id prefix (everything up to
// the per-component suffix) for each surface. Channel clicks carry the guild
// implicitly; DM clicks must embed it.
func channelRoutePrefix(tab string) string { return componentPrefix + tab + ":" }
func dmRoutePrefix(tab, guildID string) string {
	return componentPrefix + dmTag + ":" + tab + ":" + guildID + ":"
}

func buildEmbed(e EmbedConfig, v Vars, cardAttached bool) *discordgo.MessageEmbed {
	em := &discordgo.MessageEmbed{
		Title:       v.render(e.Title),
		URL:         e.URL,
		Description: v.render(e.Description),
		Color:       colorInt(e.Color, 0xB244FC),
	}
	if e.AuthorName != "" {
		em.Author = &discordgo.MessageEmbedAuthor{Name: v.render(e.AuthorName), IconURL: v.apply(e.AuthorIcon)}
	}
	if t := v.apply(e.Thumbnail); t != "" {
		em.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: t}
	}
	if e.FooterText != "" {
		em.Footer = &discordgo.MessageEmbedFooter{Text: v.render(e.FooterText), IconURL: v.apply(e.FooterIcon)}
	}
	for _, f := range e.Fields {
		if f.Name == "" && f.Value == "" {
			continue
		}
		em.Fields = append(em.Fields, &discordgo.MessageEmbedField{
			Name: v.render(f.Name), Value: v.render(f.Value), Inline: f.Inline,
		})
	}
	// Image: the literal token {card} embeds the generated card; any other value
	// is treated as a URL. (A card with no embed referencing it shows standalone.)
	if u := strings.TrimSpace(e.ImageURL); u == "{card}" {
		if cardAttached {
			em.Image = &discordgo.MessageEmbedImage{URL: "attachment://card.png"}
		}
	} else if u != "" {
		em.Image = &discordgo.MessageEmbedImage{URL: v.apply(u)}
	}
	if e.Timestamp {
		em.Timestamp = time.Now().Format(time.RFC3339)
	}
	return em
}

// buildComponents renders the configured button / select rows into Discord
// message components. A non-link component routes its click back to this
// feature's handler via custom_id routePrefix+suffix (the channel and DM
// surfaces pass different prefixes); the handler runs the wired Action (if any)
// or acks silently. Link buttons carry their URL instead. Labels, placeholders
// and option text are templated.
func buildComponents(rows []cc.ComponentRow, v Vars, routePrefix string) []discordgo.MessageComponent {
	out := make([]discordgo.MessageComponent, 0, len(rows))
	for _, row := range rows {
		comps := make([]discordgo.MessageComponent, 0, len(row.Components))
		for _, c := range row.Components {
			if mc := buildComponent(c, v, routePrefix); mc != nil {
				comps = append(comps, mc)
			}
		}
		if len(comps) > 0 {
			out = append(out, discordgo.ActionsRow{Components: comps})
		}
	}
	return out
}

// buildComponent renders one component, or returns nil to skip it when it would
// make Discord reject the whole message (e.g. a string select with no options).
func buildComponent(c cc.Component, v Vars, routePrefix string) discordgo.MessageComponent {
	// Routed custom_id: clicks land on this feature's handler, which finds the
	// matching Action by suffix (or acks silently). The suffix also keeps ids
	// unique within the message.
	routeID := routePrefix + c.CustomIDSuffix
	switch c.Type {
	case "select_string":
		if len(c.Options) == 0 {
			return nil
		}
		opts := make([]discordgo.SelectMenuOption, 0, len(c.Options))
		for _, o := range c.Options {
			so := discordgo.SelectMenuOption{
				Label:       v.render(o.Label),
				Value:       o.Value,
				Description: v.render(o.Description),
				Default:     o.Default,
			}
			if o.Emoji != "" {
				so.Emoji = componentEmoji(o.Emoji)
			}
			opts = append(opts, so)
		}
		return discordgo.SelectMenu{
			MenuType:    discordgo.StringSelectMenu,
			CustomID:    routeID,
			Placeholder: v.render(c.Placeholder),
			Options:     opts,
			MinValues:   c.MinValues,
			MaxValues:   intOrZero(c.MaxValues),
			Disabled:    c.Disabled,
		}
	case "select_user":
		return discordgo.SelectMenu{MenuType: discordgo.UserSelectMenu, CustomID: routeID, Placeholder: v.render(c.Placeholder), Disabled: c.Disabled}
	case "select_role":
		return discordgo.SelectMenu{MenuType: discordgo.RoleSelectMenu, CustomID: routeID, Placeholder: v.render(c.Placeholder), Disabled: c.Disabled}
	case "select_channel":
		return discordgo.SelectMenu{MenuType: discordgo.ChannelSelectMenu, CustomID: routeID, Placeholder: v.render(c.Placeholder), Disabled: c.Disabled}
	default: // button
		style := buttonStyle(c.Style)
		btn := discordgo.Button{Label: v.render(c.Label), Style: style, Disabled: c.Disabled}
		// Discord rejects a button that carries both a URL and a custom_id: a
		// link button is its URL, anything else routes its click to the handler.
		if style == discordgo.LinkButton {
			btn.URL = v.render(c.URL)
		} else {
			btn.CustomID = routeID
		}
		if c.Emoji != "" {
			btn.Emoji = componentEmoji(c.Emoji)
		}
		return btn
	}
}

func buttonStyle(s string) discordgo.ButtonStyle {
	switch strings.ToLower(s) {
	case "primary":
		return discordgo.PrimaryButton
	case "success":
		return discordgo.SuccessButton
	case "danger":
		return discordgo.DangerButton
	case "link":
		return discordgo.LinkButton
	}
	return discordgo.SecondaryButton
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// componentEmoji turns the editor's emoji string into Discord's shape: a
// unicode glyph passes through as Name; a custom emoji arrives as "name:id"
// (also tolerated: "a:name:id" for animated, or the full "<a:name:id>" paste)
// and splits into Name + ID + Animated.
func componentEmoji(s string) *discordgo.ComponentEmoji {
	s = strings.Trim(strings.TrimSpace(s), "<>")
	parts := strings.Split(s, ":")
	last := parts[len(parts)-1]
	if len(parts) >= 2 && isSnowflakeID(last) {
		e := &discordgo.ComponentEmoji{Name: parts[len(parts)-2], ID: last}
		if len(parts) >= 3 && parts[0] == "a" {
			e.Animated = true
		}
		return e
	}
	return &discordgo.ComponentEmoji{Name: s}
}

// isSnowflakeID reports whether s looks like a Discord id (long enough that a
// short select value can't be mistaken for one).
func isSnowflakeID(s string) bool {
	if len(s) < 15 || len(s) > 21 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// handleComponent runs the click-action wired to a button / select on a posted
// welcome message or DM. Channel clicks are "welcome:<tab>:<suffix>" (guild from
// the interaction); DM clicks are "welcome:dm:<tab>:<guildID>:<suffix>" (guild
// embedded). An unwired component (or any decode / lookup miss) is acked
// silently. A wired program runs as a fresh durable flow: it can open a modal,
// wait for the member's reply, branch and send follow-ups, all resuming through
// the automations plugin's "auto:" handlers + the wait scheduler.
func (p *Plugin) handleComponent(c *interactions.Context, d plugin.Deps) error {
	rest := strings.TrimPrefix(c.CustomID(), componentPrefix)
	tab, suffix, guildID, isDM, ok := parseComponentID(rest, c.GuildID)
	if !ok || guildID == "" {
		return c.DeferUpdate()
	}
	gid, _ := event.ParseID(guildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return c.DeferUpdate()
	}
	mc := cfg.Welcome
	if tab == tabGoodbye {
		mc = cfg.Goodbye
	}
	actions := mc.Actions
	if isDM {
		actions = mc.DM.Actions
	}
	steps := actionSteps(actions, suffix)
	if len(steps) == 0 || p.runner == nil {
		return c.DeferUpdate() // decorative / unwired
	}

	// A program that opens a modal must answer the click with the form itself (a
	// modal is the interaction's first response, so no up-front ack). Every other
	// program is acked silently, claiming the 3s window, and its steps post fresh
	// output to the channel / DM.
	scope := p.clickScope(c, d, suffix, guildID)
	modalFirst := stepsOpenModalFirst(steps)
	if modalFirst {
		scope.MarkDeferred(false)
		scope.MarkReplied(false)
	} else {
		_ = c.DeferUpdate()
		scope.MarkDeferred(true)
		scope.MarkReplied(true)
	}

	res := p.runner.Start(c.Ctx, runner.Meta{
		AutomationID:     "welcome." + tabKind(tab),
		Version:          1,
		GuildID:          guildID,
		InvokerID:        c.User.ID,
		ActorID:          c.User.ID,
		ChannelID:        c.I.ChannelID,
		TriggerKind:      "welcome_click",
		InteractionID:    c.I.ID,
		InteractionToken: c.I.Token,
	}, cc.Definition{Steps: steps}, scope)

	// Safety net: if a modal-first program didn't actually open the modal (a
	// failing step, or a since-edited config), the click still needs SOME ack
	// within the deadline or Discord shows "interaction failed".
	if modalFirst && !c.Responded() && (res.Pause == nil || res.Pause.AwaitingKind != "modal") {
		_ = c.DeferUpdate()
	}
	return nil
}

// stepsOpenModalFirst reports whether a click program opens a modal as its first
// Discord-facing step (pure data steps may precede it). When it does, the click
// must be answered with the modal rather than pre-deferred, which Discord forbids
// before a modal response.
func stepsOpenModalFirst(steps []cc.Step) bool {
	for _, s := range steps {
		switch s.Kind {
		case cc.KindModalOpen:
			return true
		case cc.KindSetVar, cc.KindIncrVar, cc.KindKVGet, cc.KindKVSet, cc.KindKVDelete,
			cc.KindJSONParse, cc.KindPickRandom, cc.KindMemberFetch:
			continue // pure data steps don't touch the interaction
		default:
			return false
		}
	}
	return false
}

// tabKind maps a config tab to its event kind, used as the durable run's label
// ("welcome.join" / "welcome.leave") so click and tail runs share the flow's KV
// scope and Runs filter.
func tabKind(tab string) string {
	if tab == tabGoodbye {
		return "leave"
	}
	return "join"
}

// MergeStoredActions returns the incoming welcome config JSON with each tab's
// canvas-owned fields (per-button click Actions and the post-message Tail)
// replaced by the stored config's. The composer page owns the message; the
// button click actions and the follow-up flow are owned by the automation
// canvas (saved via /welcome/actions), so a composer save must not overwrite
// them with its (possibly stale, or absent) copy. On any decode/encode error the
// incoming config is returned unchanged.
func MergeStoredActions(incoming, stored []byte) []byte {
	var in, st Config
	if json.Unmarshal(incoming, &in) != nil || json.Unmarshal(stored, &st) != nil {
		return incoming
	}
	in.Welcome.Actions = st.Welcome.Actions
	in.Welcome.DM.Actions = st.Welcome.DM.Actions
	in.Welcome.Tail = st.Welcome.Tail
	in.Goodbye.Actions = st.Goodbye.Actions
	in.Goodbye.DM.Actions = st.Goodbye.DM.Actions
	in.Goodbye.Tail = st.Goodbye.Tail
	out, err := json.Marshal(in)
	if err != nil {
		return incoming
	}
	return out
}

// actionSteps returns the click program wired to a component suffix, or nil.
func actionSteps(actions []ButtonAction, suffix string) []cc.Step {
	for _, a := range actions {
		if a.Suffix == suffix {
			return a.Steps
		}
	}
	return nil
}

// parseComponentID splits a welcome component custom_id (already stripped of the
// "welcome:" prefix) into its tab, suffix, resolved guild id, and whether the
// click came from a DM. Channel form: "<tab>:<suffix>" (guild from the
// interaction). DM form: "dm:<tab>:<guildID>:<suffix>" (guild embedded, since a
// DM interaction has none). ok is false on a malformed id.
func parseComponentID(rest, interactionGuildID string) (tab, suffix, guildID string, isDM, ok bool) {
	if strings.HasPrefix(rest, dmTag+":") {
		parts := strings.SplitN(strings.TrimPrefix(rest, dmTag+":"), ":", 3)
		if len(parts) != 3 {
			return "", "", "", false, false
		}
		return parts[0], parts[2], parts[1], true, true
	}
	t, s, found := strings.Cut(rest, ":")
	if !found {
		return "", "", "", false, false
	}
	return t, s, interactionGuildID, false, true
}

// clickScope builds the run scope for a click action: the clicker is the user,
// and the click's id plus any selected values are exposed under .Vars.click. The
// caller sets the interaction-ack flags (deferred / replied) based on whether the
// program opens a modal first.
func (p *Plugin) clickScope(c *interactions.Context, d plugin.Deps, suffix, guildID string) *cc.Scope {
	gid, _ := event.ParseID(guildID)
	guildName := "the server"
	memberCount := 0
	if g, err := d.Store.Guilds.Get(c.Ctx, gid); err == nil {
		if g.Name != "" {
			guildName = g.Name
		}
		memberCount = g.MemberCount
	}
	// A DM click carries no member object (c.I.Member is nil); BuildContext
	// tolerates that, leaving the .Member.* template vars empty.
	ctxVars := cc.BuildContext(guildID, c.I.ChannelID, c.User, c.I.Member, cc.ContextGuild{
		ID: guildID, Name: guildName, MemberCount: memberCount,
	}, time.Now().UnixMilli())
	scope := cc.NewScope(d.GuildState, guildID, ctxVars, nil, nil)
	scope.Set("click", map[string]any{"id": suffix, "values": c.ComponentValues()})
	return scope
}

// guildFonts loads a guild's custom (premium) fonts for the card renderer; a
// failure is non-fatal (the card just falls back to the bundled fonts).
func guildFonts(ctx context.Context, d plugin.Deps, gid int64) map[string]string {
	m, _ := d.Store.Uploads.FontMap(ctx, gid)
	return m
}

func renderCard(ctx context.Context, img *imaging.Renderer, card CardConfig, v Vars) ([]byte, error) {
	// Card Studio layout is the primary path; the legacy preset model only
	// renders for configs created before the studio existed.
	if card.Layout != nil {
		return img.RenderLayout(templating.WithCardKV(ctx, v.kv), *card.Layout, v.Map(), v.fonts)
	}
	return img.RenderWelcome(ctx, imaging.WelcomeInput{
		Background:   card.Background,
		AccentColor:  card.AccentColor,
		TextColor:    card.TextColor,
		SubTextColor: card.SubTextColor,
		AvatarURL:    discord.AvatarURL(v.user.ID, v.user.Avatar, 256),
		Title:        v.apply(card.Title),
		Subtitle:     v.apply(card.Subtitle),
		Footer:       v.apply(card.Footer),
	})
}

func guildInfo(ctx context.Context, d plugin.Deps, guildID int64, fallbackCount int) (name string, count int, icon string) {
	if g, err := d.Store.Guilds.Get(ctx, guildID); err == nil {
		name, count, icon = g.Name, g.MemberCount, g.Icon
	}
	if fallbackCount > 0 {
		count = fallbackCount
	}
	if name == "" {
		name = "the server"
	}
	return name, count, icon
}

// colorInt converts a #RRGGBB string to a Discord embed color int.
func colorInt(hex string, fallback int) int {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return fallback
	}
	n, err := strconv.ParseInt(hex, 16, 32)
	if err != nil {
		return fallback
	}
	return int(n)
}
