package moderation

import (
	"context"
	"fmt"
	"time"

	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── Anti-raid join guard ─────────────────────────────────────
//
// raidCheck watches the rate of member joins. When more than Threshold members
// join inside Window seconds the guild enters "raid mode": each subsequent
// joiner (optionally only new accounts) is actioned per the config, until Window
// passes with no new joins (the key TTL lapses) or a moderator runs /unlock.
//
// Redis keys (all suffixed with the decimal guild id):
//   automod:raidjoins:<gid>  fixed-window join counter (TTL = Window)
//   automod:raidmode:<gid>   present while raid mode is active (refreshed each join)
//   automod:raidalert:<gid>  one-shot reservation so the alert fires once per spell

func raidJoinsKey(gid string) string { return "automod:raidjoins:" + gid }
func raidModeKey(gid string) string  { return "automod:raidmode:" + gid }
func raidAlertKey(gid string) string { return "automod:raidalert:" + gid }

// raidModeActive reports whether the guild is currently in raid mode.
func raidModeActive(ctx context.Context, c *cache.Store, gidStr string) bool {
	if c == nil {
		return false
	}
	var v string
	err := c.GetJSON(ctx, raidModeKey(gidStr), &v)
	return err == nil
}

// clearRaidMode lifts raid mode for a guild (used by /unlock and auto-calm).
func clearRaidMode(ctx context.Context, c *cache.Store, gidStr string) {
	if c == nil {
		return
	}
	_ = c.Delete(ctx, raidModeKey(gidStr), raidAlertKey(gidStr), raidJoinsKey(gidStr))
}

// raidCheck runs the join-velocity guard for a single member add.
func raidCheck(ctx context.Context, d plugin.Deps, automodCfg AutomodConfig, gid int64, gidStr string, member event.Member, rc RaidConfig) {
	if !rc.Enabled || d.Cache == nil {
		return
	}
	window := rc.Window
	if window <= 0 {
		window = 10
	}
	threshold := rc.Threshold
	if threshold <= 0 {
		threshold = 8
	}

	n, err := d.Cache.Incr(ctx, raidJoinsKey(gidStr), time.Duration(window)*time.Second)
	if err != nil {
		d.Log.Warn("anti-raid: join counter failed", "guild", gidStr, "err", err)
		return
	}

	tripped := int(n) >= threshold
	active := raidModeActive(ctx, d.Cache, gidStr)

	if tripped && !active {
		// Enter raid mode. Refresh the mode key each join so it survives at least
		// Window (min 60s) past the last join, then auto-calms.
		ttl := time.Duration(maxInt(window, 60)) * time.Second
		if err := d.Cache.SetJSON(ctx, raidModeKey(gidStr), "1", ttl); err == nil {
			active = true
		}
		// Fire the alert + automation event exactly once per raid spell.
		if first, _ := d.Cache.Reserve(ctx, raidAlertKey(gidStr), 5*time.Minute); first {
			postRaidAlert(d, automodCfg, gidStr, rc, int(n), window)
			act := rc.Action
			if act == "" {
				act = "kick"
			}
			publishEvent(ctx, d, event.TypeRaidAlert, gidStr, event.RaidAlert{
				GuildID:   gidStr,
				Active:    true,
				Joins:     int(n),
				Threshold: threshold,
				Window:    window,
				Action:    act,
			})
		}
	} else if active {
		// Keep raid mode alive while joins keep arriving.
		ttl := time.Duration(maxInt(window, 60)) * time.Second
		_ = d.Cache.SetJSON(ctx, raidModeKey(gidStr), "1", ttl)
	}

	if !active {
		return
	}

	// Optionally only action new accounts so established members caught in a
	// join spike are left alone.
	if rc.OnlyNewAccounts {
		hours := rc.NewAccountHours
		if hours <= 0 {
			hours = 72
		}
		if created, ok := accountCreated(member.User.ID); ok {
			if time.Since(created) >= time.Duration(hours)*time.Hour {
				return
			}
		}
	}

	actionRaidJoiner(ctx, d, gid, gidStr, member, rc)
}

// actionRaidJoiner applies the configured raid action to a single joiner and
// records a mod case (moderator 0 = automod).
func actionRaidJoiner(ctx context.Context, d plugin.Deps, gid int64, gidStr string, member event.Member, rc RaidConfig) {
	reason := "[Anti-raid] join during raid"
	uid := member.User.ID
	action := rc.Action
	if action == "" {
		action = "kick"
	}

	var (
		durSecs   int
		expiresAt *time.Time
	)
	switch action {
	case "ban":
		if err := d.Discord.Ban(gidStr, uid, reason, 1); err != nil {
			d.Log.Warn("anti-raid: ban failed", "user", uid, "err", err)
			return
		}
	case "timeout":
		secs := rc.TimeoutSeconds
		if secs <= 0 {
			secs = 3600
		}
		until := time.Now().Add(time.Duration(secs) * time.Second)
		if err := d.Discord.Timeout(gidStr, uid, &until, reason); err != nil {
			d.Log.Warn("anti-raid: timeout failed", "user", uid, "err", err)
			return
		}
		durSecs = secs
		expiresAt = &until
	default: // kick
		action = "kick"
		if err := d.Discord.Kick(gidStr, uid, reason); err != nil {
			d.Log.Warn("anti-raid: kick failed", "user", uid, "err", err)
			return
		}
	}

	userID, _ := event.ParseID(uid)
	_, err := d.Store.Moderation.CreateCase(ctx, store.ModCase{
		GuildID:         gid,
		UserID:          userID,
		ModeratorID:     0,
		Action:          action,
		Reason:          reason,
		DurationSeconds: durSecs,
		ExpiresAt:       expiresAt,
		Active:          true,
	})
	if err != nil {
		d.Log.Warn("anti-raid: create case failed", "user", uid, "err", err)
	}
}

// postRaidAlert posts the "raid mode engaged" embed to the raid alert channel,
// the automod alert channel, or the mod-log (in that order).
func postRaidAlert(d plugin.Deps, automodCfg AutomodConfig, gidStr string, rc RaidConfig, joins, window int) {
	channel := firstNonEmpty(rc.AlertChannel, automodCfg.AlertChannel)
	if channel == "" {
		channel = loadModLogChannel(d, gidStr)
	}
	if channel == "" {
		return
	}
	action := rc.Action
	if action == "" {
		action = "kick"
	}
	embed := &discordgo.MessageEmbed{
		Title: "Anti-raid: raid mode engaged",
		Color: 0xED4245,
		Description: fmt.Sprintf(
			"Detected %d joins within %ds. New members will be auto-%sed until the spike subsides.",
			joins, window, action,
		),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	_, _ = d.Discord.SendMessage(channel, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}})
}

// loadModLogChannel resolves the moderation log channel for a guild id string.
func loadModLogChannel(d plugin.Deps, gidStr string) string {
	gid, ok := event.ParseID(gidStr)
	if !ok {
		return ""
	}
	return loadModConfig(context.Background(), d, gid).LogChannel
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
