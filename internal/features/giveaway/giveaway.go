package giveaway

import (
	"context"
	"fmt"
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
	// Composed action buttons (giveaway:act:<id>:<suffix>) route here; their
	// click flow is wired in Automations. Until wired, acknowledge gracefully.
	if action == "act" {
		return c.RespondEphemeral("This button isn't set up yet.")
	}
	if action != "enter" || id == "" {
		return c.DeferUpdate()
	}
	gid, _ := event.ParseID(c.GuildID)
	g, err := d.Store.Giveaways.Get(c.Ctx, gid, id)
	if err != nil {
		return c.RespondEphemeral("This giveaway is no longer available.")
	}
	if g.Status != "running" {
		return c.RespondEphemeral("This giveaway has already ended.")
	}
	uid, _ := event.ParseID(c.User.ID)
	spec := decodeSpec(g.Spec)

	// Toggle: leave if already entered (no re-validation needed to leave).
	if has, _ := d.Store.Giveaways.HasEntry(c.Ctx, g.ID, uid); has {
		if _, err := d.Store.Giveaways.RemoveEntry(c.Ctx, g.ID, uid); err != nil {
			return c.RespondEphemeral("Couldn't update your entry, try again.")
		}
		_ = c.RespondEphemeral("You've left the giveaway for **" + g.Prize + "**.")
		p.refreshLiveMessage(c.Ctx, spec, g)
		return nil
	}

	if c.User.Bot && !spec.AllowBotsToWin {
		return c.RespondEphemeral("Bots can't enter this giveaway.")
	}

	req := decodeRequirements(g.Requirements)
	ent := p.entrantState(c.Ctx, d, gid, uid, c.User.ID, c.I.Member, req)
	eligible, reason, entries := evaluateEntry(req, ent)
	if !eligible {
		return c.RespondEphemeral("❌ " + reason)
	}
	if _, err := d.Store.Giveaways.AddEntry(c.Ctx, g.ID, uid, entries); err != nil {
		return c.RespondEphemeral("Couldn't record your entry, try again.")
	}
	msg := "🎉 You're entered into the giveaway for **" + g.Prize + "**!"
	if entries > 1 {
		msg += fmt.Sprintf(" You have **%d** entries.", entries)
	}
	_ = c.RespondEphemeral(msg)
	p.refreshLiveMessage(c.Ctx, spec, g)
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
