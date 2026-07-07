package giveaway

import (
	"context"
	"fmt"
	"strconv"
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

// managePermBits are the guild permissions that always allow managing giveaways
// (in addition to the guild owner and any configured manager roles).
const managePermBits = int64(discordgo.PermissionManageServer | discordgo.PermissionAdministrator)

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

// Init wires the /giveaway command, the Enter-button component handler, and the
// background sweeper that ends due giveaways.
func (p *Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	p.deps = d
	reg.Command(giveawayCommand(p))
	reg.Component(componentPrefix, func(c *interactions.Context) error { return p.handleComponent(c) })
	reg.Worker("giveaway-scheduler", func(ctx context.Context) { p.runScheduler(ctx) })
	return nil
}

func enterCustomID(giveawayID string) string { return componentPrefix + "enter:" + giveawayID }

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
	if !ok || action != "enter" || id == "" {
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

	cfg, _, err := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	if err != nil {
		return c.RespondEphemeral("Couldn't load the giveaway, try again in a moment.")
	}

	// Toggle: leave if already entered (no re-validation needed to leave).
	if has, _ := d.Store.Giveaways.HasEntry(c.Ctx, g.ID, uid); has {
		if _, err := d.Store.Giveaways.RemoveEntry(c.Ctx, g.ID, uid); err != nil {
			return c.RespondEphemeral("Couldn't update your entry, try again.")
		}
		_ = c.RespondEphemeral("You've left the giveaway for **" + g.Prize + "**.")
		p.refreshLiveMessage(c.Ctx, cfg, g)
		return nil
	}

	if c.User.Bot && !cfg.AllowBotsToWin {
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
	p.refreshLiveMessage(c.Ctx, cfg, g)
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

// refreshLiveMessage re-renders the giveaway embed with the current entry count.
// Best-effort: a failed edit (deleted message, missing perms) just leaves the
// count momentarily stale until the next change or the end draw.
func (p *Plugin) refreshLiveMessage(ctx context.Context, cfg Config, g store.Giveaway) {
	if g.MessageID == 0 {
		return
	}
	count, err := p.deps.Store.Giveaways.EntryCount(ctx, g.ID)
	if err != nil {
		return
	}
	name, memberCount := p.guildInfo(ctx, g.GuildID)
	em := buildLiveEmbed(ctx, cfg, g, scopeData(g, count, nil, name, memberCount), count)
	embeds := []*discordgo.MessageEmbed{em}
	comps := enterComponents(cfg, g.ID)
	_, err = p.deps.Discord.EditMessage(&discordgo.MessageEdit{
		Channel:    event.FormatID(g.ChannelID),
		ID:         event.FormatID(g.MessageID),
		Embeds:     &embeds,
		Components: &comps,
	})
	if err != nil {
		p.deps.Log.Debug("giveaway: refresh live message failed", "giveaway", g.ID, "err", err)
	}
}

// postGiveaway sends the live giveaway message (with an optional role ping above
// it) and returns the posted message so the caller can record its id.
func (p *Plugin) postGiveaway(ctx context.Context, cfg Config, g store.Giveaway, entryCount int) (*discordgo.Message, error) {
	name, memberCount := p.guildInfo(ctx, g.GuildID)
	send := buildLiveMessage(ctx, cfg, g, entryCount, name, memberCount)
	if cfg.PingRoleID != "" {
		send.Content = "<@&" + cfg.PingRoleID + ">"
		send.AllowedMentions = &discordgo.MessageAllowedMentions{Roles: []string{cfg.PingRoleID}}
	}
	return p.deps.Discord.SendMessage(event.FormatID(g.ChannelID), send)
}

// ── Guild + permission helpers ───────────────────────────────────────────────

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

// canManage reports whether the interaction actor may create/manage giveaways:
// the guild owner, anyone holding a role with Manage Server / Administrator, or
// anyone holding a configured manager role.
func (p *Plugin) canManage(ctx context.Context, c *interactions.Context, cfg Config) bool {
	if c.I.Member == nil {
		return false
	}
	roles := c.I.Member.Roles
	if p.deps.GuildState == nil {
		return false
	}
	snap, err := p.deps.GuildState.Snapshot(ctx, c.GuildID)
	if err != nil {
		return false
	}
	if snap.Meta.OwnerID != "" && snap.Meta.OwnerID == c.User.ID {
		return true
	}
	held := map[string]bool{}
	for _, r := range roles {
		held[r] = true
		if contains(cfg.ManagerRoles, r) {
			return true
		}
	}
	for _, role := range snap.Roles {
		if !held[role.ID] {
			continue
		}
		if bits, err := strconv.ParseInt(strings.TrimSpace(role.Permissions), 10, 64); err == nil && bits&managePermBits != 0 {
			return true
		}
	}
	return false
}
