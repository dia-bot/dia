// Package leveling awards members XP for chatting, ranks them on a classic polynomial
// curve, renders rank cards, and grants configurable level-role rewards.
package leveling

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the leveling feature.
type Plugin struct{}

// New returns the leveling plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Leveling",
		Description: "Award XP for chatting, rank members, and grant level-role rewards.",
		Category:    plugin.CategoryEngagement,
	}
}

// Init wires the message XP handler and the /rank, /leaderboard and
// /level-rewards commands.
func (*Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	reg.OnEvent(event.TypeMessageCreate, func(ctx context.Context, env *event.Envelope) error {
		return handleMessage(ctx, d, env)
	})

	reg.Command(&interactions.Command{
		Def: interactions.Slash("rank",
			"Show your (or another member's) rank card",
			interactions.UserOpt("user", "The member to look up", false),
		),
		Handler: func(c *interactions.Context) error { return handleRank(c, d) },
	})

	reg.Command(&interactions.Command{
		Def:     interactions.Slash("leaderboard", "Show the server XP leaderboard"),
		Handler: func(c *interactions.Context) error { return handleLeaderboard(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.AdminOnly(interactions.Slash("level-rewards",
			"Manage level-role rewards",
			interactions.SubCommand("add", "Grant a role when a member reaches a level",
				interactions.IntOpt("level", "The level to reward", true),
				interactions.RoleOpt("role", "The role to grant", true),
			),
			interactions.SubCommand("remove", "Remove the reward at a level",
				interactions.IntOpt("level", "The level whose reward to remove", true),
			),
			interactions.SubCommand("list", "List configured level-role rewards"),
		)),
		Handler: func(c *interactions.Context) error { return handleRewards(c, d) },
	})
	return nil
}

// ── XP earning ───────────────────────────────────────────────

func handleMessage(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	msg, err := plugin.DecodeData[event.Message](env)
	if err != nil {
		return err
	}
	if msg.Author.Bot || msg.GuildID == "" {
		return nil
	}

	gid, ok := event.ParseID(msg.GuildID)
	if !ok {
		return nil
	}
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return err
	}

	// Channel / role exclusions.
	if contains(cfg.NoXPChannels, msg.ChannelID) {
		return nil
	}
	if msg.Member != nil {
		for _, rid := range msg.Member.Roles {
			if contains(cfg.NoXPRoles, rid) {
				return nil
			}
		}
	}

	// Anti-spam cooldown: only earn once per cooldown window.
	if d.Cache != nil && cfg.CooldownSeconds > 0 {
		key := "lvl:cd:" + msg.GuildID + ":" + msg.Author.ID
		set, rerr := d.Cache.Reserve(ctx, key, time.Duration(cfg.CooldownSeconds)*time.Second)
		if rerr == nil && !set {
			return nil // still on cooldown
		}
	}

	uid, ok := event.ParseID(msg.Author.ID)
	if !ok {
		return nil
	}

	delta := xpDelta(cfg)
	updated, err := d.Store.Levels.AddXP(ctx, gid, uid, delta, time.Now())
	if err != nil {
		return err
	}

	prevLevel := updated.Level // the level stored before this message
	newLevel := LevelFromXP(updated.XP)
	if newLevel <= prevLevel {
		return nil
	}

	if err := d.Store.Levels.SetLevel(ctx, gid, uid, newLevel); err != nil {
		d.Log.Warn("leveling: set level failed", "err", err)
	}

	grantRewards(ctx, d, cfg, msg.GuildID, msg.Author.ID, newLevel)

	if cfg.AnnounceLevelUp {
		announce(ctx, d, cfg, msg, newLevel)
	}
	return nil
}

// xpDelta picks a random XP amount in [XPMin, XPMax] scaled by Multiplier.
func xpDelta(cfg Config) int64 {
	min, max := cfg.XPMin, cfg.XPMax
	if min <= 0 {
		min = 1
	}
	if max < min {
		max = min
	}
	base := min
	if max > min {
		base = min + rand.IntN(max-min+1)
	}
	mult := cfg.Multiplier
	if mult <= 0 {
		mult = 1
	}
	delta := int64(float64(base) * mult)
	if delta < 1 {
		delta = 1
	}
	return delta
}

// grantRewards adds the roles the member should now hold and, when not stacking,
// removes superseded reward roles.
func grantRewards(ctx context.Context, d plugin.Deps, cfg Config, guildID, userID string, level int) {
	rewards, err := d.Store.Levels.RewardsUpTo(ctx, mustID(guildID), level)
	if err != nil || len(rewards) == 0 {
		return
	}
	// rewards is ordered by level ascending; the last entry is the highest
	// reward the member qualifies for.
	for i, rw := range rewards {
		roleID := event.FormatID(rw.RoleID)
		isHighest := i == len(rewards)-1
		// When not stacking, only the highest-level reward role should be held;
		// also honour a per-reward RemovePrevious flag.
		if (!cfg.StackRewards || rw.RemovePrevious) && !isHighest {
			_ = d.Discord.RemoveRole(guildID, userID, roleID, "leveling: reward superseded")
			continue
		}
		if err := d.Discord.AddRole(guildID, userID, roleID, "leveling: reached level "+strconv.Itoa(level)); err != nil {
			d.Log.Warn("leveling: add reward role failed", "role", roleID, "err", err)
		}
	}
}

// announce posts the level-up message to the configured destination.
func announce(ctx context.Context, d plugin.Deps, cfg Config, msg event.Message, level int) {
	name, _ := guildInfo(ctx, d, msg.GuildID)
	text := applyVars(cfg.LevelUpMessage, msg.Author, name, level)
	if text == "" {
		return
	}

	switch cfg.AnnounceChannel {
	case "dm":
		_ = d.Discord.SendDM(msg.Author.ID, text)
	case "":
		_, _ = d.Discord.SendMessage(msg.ChannelID, &discordgo.MessageSend{Content: text})
	default:
		_, _ = d.Discord.SendMessage(cfg.AnnounceChannel, &discordgo.MessageSend{Content: text})
	}
}

// ── /rank ────────────────────────────────────────────────────

func handleRank(c *interactions.Context, d plugin.Deps) error {
	if err := c.Defer(false); err != nil {
		return err
	}

	target := c.User
	if u, ok := c.Options().User("user"); ok {
		target = u
	}

	gid, _ := event.ParseID(c.GuildID)
	uid, _ := event.ParseID(target.ID)

	lu, err := d.Store.Levels.Get(c.Ctx, gid, uid)
	if errors.Is(err, store.ErrNotFound) {
		_, e := c.Followup(&discordgo.WebhookParams{
			Content: "No XP yet for " + displayName(target) + ".",
			Flags:   discordgo.MessageFlagsEphemeral,
		})
		return e
	}
	if err != nil {
		return err
	}

	rank, err := d.Store.Levels.Rank(c.Ctx, gid, uid)
	if err != nil {
		return err
	}

	level := LevelFromXP(lu.XP)
	into, span := Progress(lu.XP)

	cfg, _, _ := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)
	card := cfg.RankCard

	png, err := d.Imaging.RenderRank(c.Ctx, imaging.RankInput{
		Background:   card.Background,
		AccentColor:  card.AccentColor,
		TextColor:    card.TextColor,
		SubTextColor: card.SubTextColor,
		BarColor:     card.BarColor,
		BarBgColor:   card.BarBgColor,
		AvatarURL:    discord.AvatarURL(target.ID, target.Avatar, 256),
		Username:     displayName(target),
		Rank:         rank,
		Level:        level,
		LevelXP:      into,
		NeededXP:     span,
		TotalXP:      lu.XP,
	})
	if err != nil {
		_, e := c.FollowupContent("Failed to render rank card: " + err.Error())
		return e
	}

	_, err = c.Followup(&discordgo.WebhookParams{
		Files: []*discordgo.File{{Name: "rank.png", ContentType: "image/png", Reader: bytes.NewReader(png)}},
	})
	return err
}

// ── /leaderboard ─────────────────────────────────────────────

func handleLeaderboard(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	top, err := d.Store.Levels.Leaderboard(c.Ctx, gid, 10, 0)
	if err != nil {
		return err
	}
	if len(top) == 0 {
		return c.RespondEphemeral("No one has earned any XP yet.")
	}

	name, _ := guildInfo(c.Ctx, d, c.GuildID)
	var b strings.Builder
	for i, lu := range top {
		level := LevelFromXP(lu.XP)
		fmt.Fprintf(&b, "**%d.** <@%s> — Level %d (%s XP)\n",
			i+1, event.FormatID(lu.UserID), level, formatInt(lu.XP))
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🏆 " + name + " Leaderboard",
		Description: b.String(),
		Color:       0xB244FC,
	}
	return c.RespondEmbed(false, embed)
}

// ── /level-rewards ───────────────────────────────────────────

func handleRewards(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	sub := c.Subcommand()
	if len(sub) == 0 {
		return c.RespondEphemeral("Unknown subcommand.")
	}

	switch sub[0] {
	case "add":
		opts := c.Options()
		level := int(opts.Int("level"))
		roleID, _ := event.ParseID(opts.Snowflake("role"))
		if level <= 0 {
			return c.RespondEphemeral("Level must be 1 or higher.")
		}
		if roleID == 0 {
			return c.RespondEphemeral("Please pick a valid role.")
		}
		if err := d.Store.Levels.SetReward(c.Ctx, store.LevelReward{
			GuildID: gid,
			Level:   level,
			RoleID:  roleID,
		}); err != nil {
			return err
		}
		return c.RespondEphemeral(fmt.Sprintf("✅ Members reaching **level %d** will now get <@&%s>.", level, opts.Snowflake("role")))

	case "remove":
		level := int(c.Options().Int("level"))
		if err := d.Store.Levels.DeleteReward(c.Ctx, gid, level); err != nil {
			return err
		}
		return c.RespondEphemeral(fmt.Sprintf("✅ Removed the reward at **level %d**.", level))

	case "list":
		rewards, err := d.Store.Levels.ListRewards(c.Ctx, gid)
		if err != nil {
			return err
		}
		if len(rewards) == 0 {
			return c.RespondEphemeral("No level rewards configured yet. Add one with `/level-rewards add`.")
		}
		var b strings.Builder
		for _, rw := range rewards {
			fmt.Fprintf(&b, "**Level %d** → <@&%s>\n", rw.Level, event.FormatID(rw.RoleID))
		}
		embed := &discordgo.MessageEmbed{
			Title:       "Level Rewards",
			Description: b.String(),
			Color:       0xB244FC,
		}
		return c.RespondEmbed(true, embed)
	}
	return c.RespondEphemeral("Unknown subcommand.")
}

// ── helpers ──────────────────────────────────────────────────

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func mustID(s string) int64 {
	id, _ := event.ParseID(s)
	return id
}

func displayName(u event.User) string {
	if u.GlobalName != "" {
		return u.GlobalName
	}
	return u.Username
}

func guildInfo(ctx context.Context, d plugin.Deps, guildID string) (name string, count int) {
	if id, ok := event.ParseID(guildID); ok {
		if g, err := d.Store.Guilds.Get(ctx, id); err == nil {
			name, count = g.Name, g.MemberCount
		}
	}
	if name == "" {
		name = "the server"
	}
	return name, count
}

// applyVars substitutes the level-up message placeholders.
func applyVars(s string, user event.User, server string, level int) string {
	if s == "" {
		return ""
	}
	return strings.NewReplacer(
		"{user.mention}", "<@"+user.ID+">",
		"{username}", user.Username,
		"{user}", displayName(user),
		"{server}", server,
		"{level}", strconv.Itoa(level),
	).Replace(s)
}

// formatInt adds thousands separators to an int64.
func formatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	var out []byte
	for i := 0; i < len(s); i++ {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
