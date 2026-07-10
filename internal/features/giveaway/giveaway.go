package giveaway

import (
	"context"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// componentPrefix namespaces this feature's component clicks. The posted
// giveaway's Enter button mints "giveaway:enter:<giveawayID>" so a click routes
// back here and resolves its giveaway directly from the custom_id (no message-id
// round-trip, and it keeps working for the life of the message).
const componentPrefix = "giveaway:"

// Plugin implements the giveaway feature.
type Plugin struct {
	deps plugin.Deps
	// autoRunner fires a saved automation when a composed action button is
	// clicked (injected by the worker; nil until then). See automation_bridge.go.
	autoRunner AutomationRunner
}

// New returns the giveaway plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Giveaways",
		Description: "Host fully-customizable prize giveaways with entry requirements, bonus entries, and automatic weighted winner draws.",
		Category:    plugin.CategoryEngagement,
	}
}

// Init wires the Enter-button component handler and the background sweeper that
// posts scheduled giveaways and ends due ones. Giveaways are created and managed
// from the dashboard (and by the custom-command "Start giveaway" step), so there
// is deliberately no built-in slash command.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.deps = d
	reg.Component(componentPrefix, func(c *interactions.Context) error { return p.handleComponent(c) })
	reg.Worker("giveaway-scheduler", func(ctx context.Context) { p.runScheduler(ctx) })
	return nil
}

func enterCustomID(giveawayID string) string { return componentPrefix + "enter:" + giveawayID }

// actionCustomID routes a composed (non-entry, non-link) giveaway button back to
// this feature: giveaway:act:<giveawayID>:<suffix>. Wiring these buttons to a
// click flow is configured in Automations.
func actionCustomID(giveawayID, suffix string) string {
	return componentPrefix + "act:" + giveawayID + ":" + suffix
}

// ── Enter / leave button ─────────────────────────────────────────────────────

// handleComponent handles clicks on a posted giveaway's Enter button. It's a
// toggle: a member who hasn't entered is validated against the requirements and
// entered (with any role bonus entries); a member who has entered leaves. The
// reply is always ephemeral, and the live embed's entry count is refreshed after
// a successful change.
func (p *Plugin) handleComponent(c *interactions.Context) error {
	d := p.deps
	rest := strings.TrimPrefix(c.CustomID(), componentPrefix)
	action, id, ok := strings.Cut(rest, ":")
	if !ok {
		return c.DeferUpdate()
	}
	// Composed action buttons (giveaway:act:<id>:<suffix>) route here; a click
	// fires the saved automation the button points at.
	if action == "act" {
		gwID, suffix, ok2 := strings.Cut(id, ":")
		if !ok2 || suffix == "" {
			return c.DeferUpdate()
		}
		return p.handleActionButton(c, gwID, suffix)
	}
	if action != "enter" || id == "" {
		return c.DeferUpdate()
	}
	g, err := p.resolveGiveaway(c.Ctx, c.GuildID, id)
	if err != nil {
		return c.RespondEphemeral("This giveaway is no longer available.")
	}
	spec := decodeSpec(g.Spec)
	if g.Status != "running" {
		return p.respondEntry(c, g, spec, spec.Entry.Ended, defaultEnded, 0, "")
	}
	uid, _ := event.ParseID(c.User.ID)

	// Toggle: leave if already entered (no re-validation needed to leave).
	if has, _ := d.Store.Giveaways.HasEntry(c.Ctx, g.ID, uid); has {
		if _, err := d.Store.Giveaways.RemoveEntry(c.Ctx, g.ID, uid); err != nil {
			return c.RespondEphemeral("Couldn't update your entry, try again.")
		}
		_ = p.respondEntry(c, g, spec, spec.Entry.Left, defaultLeft, 0, "")
		p.refreshLiveMessage(c.Ctx, spec, g)
		p.publishEntered(c.Ctx, g, c.User, c.I.Member, "left", 0, "")
		return nil
	}

	if c.User.Bot && !spec.AllowBotsToWin {
		_ = p.respondEntry(c, g, spec, spec.Entry.BotsBlocked, defaultBotsBlocked, 0, "")
		p.publishEntered(c.Ctx, g, c.User, c.I.Member, "blocked", 0, "")
		return nil
	}

	req := decodeRequirements(g.Requirements)
	ent := p.entrantState(c.Ctx, d, g.GuildID, uid, c.User.ID, c.I.Member, req)
	eligible, reason, entries := evaluateEntry(req, ent)
	if !eligible {
		_ = p.respondEntry(c, g, spec, spec.Entry.NotEligible, defaultNotEligible, 0, reason)
		p.publishEntered(c.Ctx, g, c.User, c.I.Member, "denied", 0, reason)
		return nil
	}
	if _, err := d.Store.Giveaways.AddEntry(c.Ctx, g.ID, uid, entries); err != nil {
		return c.RespondEphemeral("Couldn't record your entry, try again.")
	}
	_ = p.respondEntry(c, g, spec, spec.Entry.Entered, defaultEntered, entries, "")
	p.refreshLiveMessage(c.Ctx, spec, g)
	p.publishEntered(c.Ctx, g, c.User, c.I.Member, "entered", entries, "")
	return nil
}

// resolveGiveaway resolves the giveaway a component click belongs to. In a
// guild the lookup stays guild-scoped; a click without a guild (a button on a
// DM'd entry reply) falls back to the id-only lookup — safe because the
// custom_id embedding the id only exists on bot-authored messages.
func (p *Plugin) resolveGiveaway(ctx context.Context, guildIDStr, id string) (store.Giveaway, error) {
	if gid, ok := event.ParseID(guildIDStr); ok && gid != 0 {
		return p.deps.Store.Giveaways.Get(ctx, gid, id)
	}
	return p.deps.Store.Giveaways.GetByID(ctx, id)
}

// respondEntry delivers one entry outcome's fully-composed reply (content +
// embeds + buttons) per its delivery mode: ephemeral (default), a public reply
// in the channel, a DM (the click is acknowledged silently), or nothing.
// Buttons route exactly like the giveaway message's own (enter / automation /
// link). A composition that renders empty falls back to the built-in default
// copy, so the member always gets a response.
func (p *Plugin) respondEntry(c *interactions.Context, g store.Giveaway, spec Spec, r EntryReply, def string, entries int, reason string) error {
	if r.Mode == "none" {
		return c.DeferUpdate()
	}
	ctx := c.Ctx
	data := p.entryScope(ctx, g, entries, reason)
	content := renderText(ctx, r.Content, data)
	embeds := renderComposedEmbeds(ctx, r.Embeds, data, g.Color)
	comps := renderComponentRows(ctx, r.Components, spec, g.ID, data)
	if content == "" && len(embeds) == 0 {
		content = renderText(ctx, def, data)
		if content == "" {
			content = "Done."
		}
	}
	if r.Mode == "dm" {
		// Acknowledge the click silently, then DM the composed reply.
		_ = c.DeferUpdate()
		send := &discordgo.MessageSend{Content: content, Embeds: embeds, Components: comps}
		if _, err := p.deps.Discord.SendDMComplex(c.User.ID, send); err != nil {
			p.deps.Log.Debug("giveaway: entry DM failed", "giveaway", g.ID, "user", c.User.ID, "err", err)
		}
		return nil
	}
	d := &discordgo.InteractionResponseData{Content: content, Embeds: embeds, Components: comps}
	if r.Mode != "public" {
		d.Flags = discordgo.MessageFlagsEphemeral
	}
	return c.RespondData(d)
}

// entryScope builds the template scope for an entry reply: the shared giveaway
// scope with the clicker's weighted ticket count and any denial reason folded in.
func (p *Plugin) entryScope(ctx context.Context, g store.Giveaway, entries int, reason string) map[string]any {
	name, memberCount := p.guildInfo(ctx, g.GuildID)
	count, _ := p.deps.Store.Giveaways.EntryCount(ctx, g.ID)
	data := scopeData(g, count, nil, name, memberCount)
	data["Entries"] = entries
	data["Reason"] = reason
	return data
}

// handleActionButton fires the saved automation a composed action button points
// at. It resolves the giveaway (for the click's event scope), looks up the
// button's automation and runs it via the injected bridge. Missing wiring (no
// mapping, or no bridge injected) reports an ephemeral notice rather than
// silently doing nothing. The automation runs detached (context.WithoutCancel)
// so a slow flow doesn't fail the interaction ack.
func (p *Plugin) handleActionButton(c *interactions.Context, giveawayID, suffix string) error {
	g, err := p.resolveGiveaway(c.Ctx, c.GuildID, giveawayID)
	if err != nil {
		return c.RespondEphemeral("This giveaway is no longer available.")
	}
	autoID := decodeSpec(g.Spec).ButtonActions[suffix]
	if autoID == "" || p.autoRunner == nil {
		return c.RespondEphemeral("This button isn't set up yet.")
	}
	_ = c.DeferUpdate()
	ev := map[string]any{
		"giveaway_id": g.ID,
		"prize":       g.Prize,
		"button":      suffix,
		"channel_id":  c.I.ChannelID,
	}
	// The giveaway row's own guild drives the automation lookup, so a click on a
	// DM'd entry reply (no guild on the interaction) still resolves.
	if err := p.autoRunner.RunAutomation(context.WithoutCancel(c.Ctx), event.FormatID(g.GuildID), autoID, c.User, c.I.Member, c.I.ChannelID, ev); err != nil {
		p.deps.Log.Warn("giveaway: action button automation", "giveaway", g.ID, "automation", autoID, "err", err)
	}
	return nil
}

// entrantState resolves the inputs the requirement check needs for a clicker:
// their roles, account age, time in the server, and (only when a min-level rule
// exists) their leveling level. The interaction member (a guild button click)
// carries the roles + join time; userID drives the account-age snowflake.
func (p *Plugin) entrantState(ctx context.Context, d plugin.Deps, guildID, userID int64, userIDStr string, member *event.Member, req RequirementConfig) entrant {
	e := entrant{memberAge: -1}
	if created, ok := accountCreated(userIDStr); ok {
		e.accountAge = time.Since(created)
	}
	if member != nil {
		e.roles = member.Roles
		if member.JoinedAt != "" {
			if t, err := time.Parse(time.RFC3339, member.JoinedAt); err == nil {
				e.memberAge = time.Since(t)
			}
		}
	}
	if req.requiresLevelLookup() {
		if lu, err := d.Store.Levels.Get(ctx, guildID, userID); err == nil {
			e.level = lu.Level
		}
	}
	return e
}

// refreshLiveMessage re-renders the giveaway message with the current entry
// count. Best-effort: a failed edit (deleted message, missing perms) just leaves
// the count momentarily stale until the next change or the end draw. Only the
// embeds + button are edited so a role ping in the original content isn't re-sent.
func (p *Plugin) refreshLiveMessage(ctx context.Context, spec Spec, g store.Giveaway) {
	if g.MessageID == 0 {
		return
	}
	count, err := p.deps.Store.Giveaways.EntryCount(ctx, g.ID)
	if err != nil {
		return
	}
	name, memberCount := p.guildInfo(ctx, g.GuildID)
	send := buildLiveMessage(ctx, spec, g, count, name, memberCount)
	_, err = p.deps.Discord.EditMessage(&discordgo.MessageEdit{
		Channel:    event.FormatID(g.ChannelID),
		ID:         event.FormatID(g.MessageID),
		Embeds:     &send.Embeds,
		Components: &send.Components,
	})
	if err != nil {
		p.deps.Log.Debug("giveaway: refresh live message failed", "giveaway", g.ID, "err", err)
	}
}

// postGiveaway sends the live giveaway message (with an optional role ping above
// it) and returns the posted message so the caller can record its id.
func (p *Plugin) postGiveaway(ctx context.Context, spec Spec, g store.Giveaway, entryCount int) (*discordgo.Message, error) {
	name, memberCount := p.guildInfo(ctx, g.GuildID)
	send := buildLiveMessage(ctx, spec, g, entryCount, name, memberCount)
	if spec.PingRoleID != "" {
		// The role ping rides above the giveaway as the message content; the
		// composed spec.Content (if any) is folded in beneath it.
		content := "<@&" + spec.PingRoleID + ">"
		if send.Content != "" {
			content += "\n" + send.Content
		}
		send.Content = content
		send.AllowedMentions = &discordgo.MessageAllowedMentions{Roles: []string{spec.PingRoleID}}
	}
	return p.deps.Discord.SendMessage(event.FormatID(g.ChannelID), send)
}

// ── Guild helpers ────────────────────────────────────────────────────────────

// guildInfo returns the guild's display name and member count from the cached
// snapshot (falls back to sensible defaults when unavailable).
func (p *Plugin) guildInfo(ctx context.Context, guildID int64) (name string, memberCount int) {
	name = "the server"
	if p.deps.GuildState == nil {
		return name, 0
	}
	snap, err := p.deps.GuildState.Snapshot(ctx, event.FormatID(guildID))
	if err != nil {
		return name, 0
	}
	if snap.Meta.Name != "" {
		name = snap.Meta.Name
	}
	return name, snap.Meta.MemberCount
}
