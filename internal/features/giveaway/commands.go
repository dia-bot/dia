package giveaway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// giveawayCommand builds the /giveaway command tree. It is intentionally NOT
// permission-gated at the Discord level (no default_member_permissions) so a
// configured manager role can use it; every handler runs canManage first.
func giveawayCommand(p *Plugin) *interactions.Command {
	return &interactions.Command{
		Def: interactions.Slash("giveaway", "Create and manage giveaways",
			interactions.SubCommand("start", "Start a new giveaway",
				interactions.StringOpt("prize", "What you're giving away", true),
				interactions.StringOpt("duration", "How long it runs (e.g. 30m, 2h, 3d, 1w)", false),
				interactions.IntOpt("winners", "Number of winners (default 1)", false),
				interactions.ChannelOpt("channel", "Where to post it (default: here)", false),
				interactions.StringOpt("description", "Extra text shown on the embed", false),
				interactions.RoleOpt("required_role", "Only members with this role can enter", false),
				interactions.StringOpt("image", "Image URL to show on the embed", false),
				interactions.StringOpt("starts_in", "Schedule the start (e.g. 1h) instead of now", false),
			),
			interactions.SubCommand("end", "End a giveaway now and draw winners",
				interactions.StringOpt("giveaway", "Message link/id (default: latest here)", false),
			),
			interactions.SubCommand("reroll", "Reroll winners for an ended giveaway",
				interactions.StringOpt("giveaway", "Message link/id (default: latest ended here)", false),
				interactions.IntOpt("winners", "How many new winners to draw", false),
			),
			interactions.SubCommand("cancel", "Cancel a giveaway without drawing winners",
				interactions.StringOpt("giveaway", "Message link/id (default: latest here)", false),
			),
			interactions.SubCommand("list", "List active giveaways in this server"),
		),
		Handler: func(c *interactions.Context) error { return p.handleCommand(c) },
	}
}

func (p *Plugin) handleCommand(c *interactions.Context) error {
	if c.GuildID == "" {
		return c.RespondEphemeral("Giveaways can only be managed in a server.")
	}
	gid, _ := event.ParseID(c.GuildID)
	cfg, enabled, err := plugin.LoadConfig[Config](c.Ctx, p.deps, gid, FeatureKey)
	if err != nil {
		return c.RespondEphemeral("Couldn't load giveaway settings.")
	}
	if !p.canManage(c.Ctx, c, cfg) {
		return c.RespondEphemeral("You don't have permission to manage giveaways.")
	}
	sub := ""
	if path := c.Subcommand(); len(path) > 0 {
		sub = path[0]
	}
	switch sub {
	case "start":
		if !enabled {
			return c.RespondEphemeral("Giveaways are disabled. Enable the feature on the dashboard first.")
		}
		return p.cmdStart(c, cfg, gid)
	case "end":
		return p.cmdEnd(c, cfg, gid)
	case "reroll":
		return p.cmdReroll(c, cfg, gid)
	case "cancel":
		return p.cmdCancel(c, cfg, gid)
	case "list":
		return p.cmdList(c, gid)
	}
	return c.RespondEphemeral("Unknown giveaway command.")
}

func (p *Plugin) cmdStart(c *interactions.Context, cfg Config, gid int64) error {
	o := c.Options()
	prize := strings.TrimSpace(o.String("prize"))
	if prize == "" {
		return c.RespondEphemeral("You must specify a prize.")
	}
	dur, err := parseGiveawayDuration(firstNonEmpty(o.String("duration"), cfg.DefaultDuration, "24h"))
	if err != nil {
		return c.RespondEphemeral("Invalid duration. Try something like 30m, 2h, 3d or 1w.")
	}
	winners := int(o.Int("winners"))
	if winners <= 0 {
		winners = cfg.DefaultWinnerCount
	}
	if winners <= 0 {
		winners = 1
	}
	if winners > 20 {
		winners = 20
	}
	channelID := firstNonEmpty(o.Snowflake("channel"), cfg.DefaultChannelID, c.I.ChannelID)
	chIDInt, _ := event.ParseID(channelID)

	// A quick required-role override; the full requirement set lives on the
	// dashboard (and is inherited from the feature config here).
	req := cfg.Requirements
	if rid := o.Snowflake("required_role"); rid != "" {
		req.RequiredRoles = []string{rid}
	}
	reqJSON, _ := json.Marshal(req)

	hostID, _ := event.ParseID(c.User.ID)
	now := time.Now()
	startsAt, status, scheduled := now, "running", false
	if si := strings.TrimSpace(o.String("starts_in")); si != "" {
		sd, err := parseGiveawayDuration(si)
		if err != nil {
			return c.RespondEphemeral("Invalid start delay. Try something like 1h or 2d.")
		}
		startsAt, status, scheduled = now.Add(sd), "scheduled", true
	}

	if err := c.Defer(true); err != nil {
		return err
	}

	g := store.Giveaway{
		GuildID:      gid,
		ChannelID:    chIDInt,
		Prize:        prize,
		Description:  strings.TrimSpace(o.String("description")),
		WinnerCount:  winners,
		HostID:       hostID,
		Status:       status,
		Requirements: reqJSON,
		ImageURL:     strings.TrimSpace(o.String("image")),
		StartsAt:     startsAt,
		EndsAt:       startsAt.Add(dur),
		CreatedBy:    hostID,
	}
	created, err := p.deps.Store.Giveaways.Create(c.Ctx, g)
	if err != nil {
		p.deps.Log.Warn("giveaway: create failed", "err", err)
		_, e := c.FollowupContent("Couldn't create the giveaway.")
		return e
	}

	if scheduled {
		_, e := c.FollowupContent(fmt.Sprintf("🕒 Giveaway for **%s** scheduled to start %s in <#%s>.", prize, discordTS(startsAt, "R"), channelID))
		return e
	}

	msg, err := p.postGiveaway(c.Ctx, cfg, created, 0)
	if err != nil {
		// Roll back so a failed post doesn't leave an invisible running giveaway.
		_ = p.deps.Store.Giveaways.Delete(c.Ctx, gid, created.ID)
		p.deps.Log.Warn("giveaway: post failed", "err", err)
		_, e := c.FollowupContent("Couldn't post the giveaway. Do I have permission to post in <#" + channelID + ">?")
		return e
	}
	mid, _ := event.ParseID(msg.ID)
	if err := p.deps.Store.Giveaways.SetMessageID(c.Ctx, created.ID, mid); err != nil {
		p.deps.Log.Warn("giveaway: set message id", "err", err)
	}
	_, e := c.FollowupContent(fmt.Sprintf("🎉 Giveaway for **%s** started in <#%s>, ending %s.", prize, channelID, discordTS(created.EndsAt, "R")))
	return e
}

func (p *Plugin) cmdEnd(c *interactions.Context, cfg Config, gid int64) error {
	if err := c.Defer(true); err != nil {
		return err
	}
	g, err := p.resolveGiveaway(c.Ctx, gid, c.I.ChannelID, c.Options().String("giveaway"), "running")
	if err != nil {
		_, e := c.FollowupContent("Couldn't find a running giveaway to end (" + err.Error() + ").")
		return e
	}
	// An explicit id/link can resolve a non-running giveaway; the status guard
	// (and finishGiveaway's atomic claim) stop a double draw.
	if g.Status != "running" {
		_, e := c.FollowupContent("That giveaway isn't running anymore.")
		return e
	}
	if !p.finishGiveaway(c.Ctx, cfg, g) {
		_, e := c.FollowupContent("That giveaway isn't running anymore.")
		return e
	}
	_, e := c.FollowupContent("✅ Ended the giveaway for **" + g.Prize + "** and drew the winners.")
	return e
}

func (p *Plugin) cmdReroll(c *interactions.Context, cfg Config, gid int64) error {
	if err := c.Defer(true); err != nil {
		return err
	}
	g, err := p.resolveGiveaway(c.Ctx, gid, c.I.ChannelID, c.Options().String("giveaway"), "ended")
	if err != nil {
		_, e := c.FollowupContent("Couldn't find an ended giveaway to reroll (" + err.Error() + ").")
		return e
	}
	// Only an already-ended giveaway can be rerolled; an explicit id/link could
	// otherwise resolve a still-running one and draw it prematurely.
	if g.Status != "ended" {
		_, e := c.FollowupContent("You can only reroll a giveaway that has already ended.")
		return e
	}
	winners := p.rerollGiveaway(c.Ctx, cfg, g, int(c.Options().Int("winners")))
	if len(winners) == 0 {
		_, e := c.FollowupContent("No eligible entrants left to reroll for **" + g.Prize + "**.")
		return e
	}
	_, e := c.FollowupContent("🎲 Rerolled **" + g.Prize + "**: " + strings.Join(userMentions(winners), ", "))
	return e
}

func (p *Plugin) cmdCancel(c *interactions.Context, cfg Config, gid int64) error {
	if err := c.Defer(true); err != nil {
		return err
	}
	g, err := p.resolveGiveaway(c.Ctx, gid, c.I.ChannelID, c.Options().String("giveaway"), "running", "scheduled")
	if err != nil {
		_, e := c.FollowupContent("Couldn't find a giveaway to cancel (" + err.Error() + ").")
		return e
	}
	cancelled, ok, err := p.deps.Store.Giveaways.Cancel(c.Ctx, gid, g.ID)
	if err != nil {
		_, e := c.FollowupContent("Couldn't cancel that giveaway.")
		return e
	}
	if !ok {
		_, e := c.FollowupContent("That giveaway can't be cancelled (it already ended).")
		return e
	}
	p.markCancelled(c.Ctx, cfg, cancelled)
	_, e := c.FollowupContent("🚫 Cancelled the giveaway for **" + cancelled.Prize + "**.")
	return e
}

func (p *Plugin) cmdList(c *interactions.Context, gid int64) error {
	gws, err := p.deps.Store.Giveaways.ListByGuild(c.Ctx, gid, "active", 25)
	if err != nil {
		return c.RespondEphemeral("Couldn't load giveaways.")
	}
	if len(gws) == 0 {
		return c.RespondEphemeral("There are no active giveaways right now. Start one with `/giveaway start`.")
	}
	var b strings.Builder
	b.WriteString("**Active giveaways**\n")
	for _, g := range gws {
		when := "ends " + discordTS(g.EndsAt, "R")
		if g.Status == "scheduled" {
			when = "starts " + discordTS(g.StartsAt, "R")
		}
		b.WriteString(fmt.Sprintf("• **%s** — %s in <#%s>", g.Prize, when, event.FormatID(g.ChannelID)))
		if g.MessageID != 0 {
			b.WriteString(fmt.Sprintf(" · [jump](https://discord.com/channels/%s/%s/%s)",
				event.FormatID(g.GuildID), event.FormatID(g.ChannelID), event.FormatID(g.MessageID)))
		}
		b.WriteString("\n")
	}
	return c.RespondEphemeral(b.String())
}

// resolveGiveaway locates the giveaway a command targets. An explicit arg (a
// giveaway id, a message id, or a message link) is resolved directly; an empty
// arg falls back to the most recent giveaway in the invoking channel whose
// status is in `statuses` (statuses only gate the fallback, not an explicit id).
func (p *Plugin) resolveGiveaway(ctx context.Context, gid int64, channelID, arg string, statuses ...string) (store.Giveaway, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		list, err := p.deps.Store.Giveaways.ListByGuild(ctx, gid, "", 100)
		if err != nil {
			return store.Giveaway{}, errors.New("lookup failed")
		}
		chIDInt, _ := event.ParseID(channelID)
		for _, g := range list { // newest first
			if g.ChannelID == chIDInt && statusIn(g.Status, statuses) {
				return g, nil
			}
		}
		return store.Giveaway{}, errors.New("none found here — pass the giveaway's message link")
	}
	// A message link: take the trailing message-id segment.
	if strings.Contains(arg, "/channels/") {
		if i := strings.LastIndex(arg, "/"); i >= 0 {
			arg = arg[i+1:]
		}
	}
	// A numeric snowflake is a message id (giveaway ids are non-numeric UUIDs).
	if id, ok := event.ParseID(arg); ok {
		if g, err := p.deps.Store.Giveaways.GetByMessage(ctx, id); err == nil && g.GuildID == gid {
			return g, nil
		}
	}
	g, err := p.deps.Store.Giveaways.Get(ctx, gid, arg)
	if err != nil {
		return store.Giveaway{}, errors.New("not found")
	}
	return g, nil
}

func statusIn(status string, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, w := range want {
		if status == w {
			return true
		}
	}
	return false
}

// parseGiveawayDuration parses a compact duration like "30m", "2h", "3d", "1w"
// or a combination ("1d12h"). Result is clamped to [10s, 28d].
func parseGiveawayDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, errors.New("empty duration")
	}
	var total time.Duration
	num := ""
	sawUnit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			num += string(r)
			continue
		}
		if num == "" {
			return 0, errors.New("malformed duration")
		}
		n, _ := strconv.Atoi(num)
		var unit time.Duration
		switch r {
		case 's':
			unit = time.Second
		case 'm':
			unit = time.Minute
		case 'h':
			unit = time.Hour
		case 'd':
			unit = 24 * time.Hour
		case 'w':
			unit = 7 * 24 * time.Hour
		default:
			return 0, errors.New("unknown unit")
		}
		total += time.Duration(n) * unit
		num, sawUnit = "", true
	}
	if num != "" {
		return 0, errors.New("number without a unit")
	}
	if !sawUnit || total <= 0 {
		return 0, errors.New("malformed duration")
	}
	if total < 10*time.Second {
		total = 10 * time.Second
	}
	if total > 28*24*time.Hour {
		total = 28 * 24 * time.Hour
	}
	return total, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
