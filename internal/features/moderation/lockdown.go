package moderation

import (
	"errors"
	"fmt"
	"time"

	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── Server lockdown ──────────────────────────────────────────
//
// /lockdown denies SEND_MESSAGES for @everyone on every text-capable channel and
// snapshots each channel's prior @everyone overwrite to Redis so /unlock can
// restore the exact prior state. The @everyone role id equals the guild id.
//
// Redis key:
//   lockdown:<gid>  JSON snapshot {channelID -> {allow,deny,existed_before}}

// lockdownTTL keeps the snapshot around long enough that an /unlock weeks later
// still restores correctly.
const lockdownTTL = 30 * 24 * time.Hour

func lockdownKey(gidStr string) string { return "lockdown:" + gidStr }

// channelOverwriteSnapshot is the saved prior @everyone overwrite for one
// channel.
type channelOverwriteSnapshot struct {
	Allow         int64 `json:"allow"`
	Deny          int64 `json:"deny"`
	ExistedBefore bool  `json:"existed_before"`
}

// lockdownTextTypes are the channel types that have an @everyone send-messages
// overwrite worth toggling.
func isLockableChannel(t discordgo.ChannelType) bool {
	switch t {
	case discordgo.ChannelTypeGuildText, discordgo.ChannelTypeGuildNews, discordgo.ChannelTypeGuildForum:
		return true
	default:
		return false
	}
}

// handleLockdown denies @everyone SEND_MESSAGES across every text channel.
func handleLockdown(c *interactions.Context, d plugin.Deps) error {
	gidStr := c.GuildID
	reason := lockReason(c.Options().String("reason"))

	channels, err := d.Discord.GuildChannels(gidStr)
	if err != nil {
		return c.RespondEphemeral("Failed to list channels: " + err.Error())
	}

	snapshot := map[string]channelOverwriteSnapshot{}
	locked := 0
	for _, ch := range channels {
		if !isLockableChannel(ch.Type) {
			continue
		}
		priorAllow, priorDeny, existed := everyoneOverwrite(ch, gidStr)
		newDeny := priorDeny | int64(discordgo.PermissionSendMessages)
		if err := d.Discord.SetRolePermission(ch.ID, gidStr, priorAllow, newDeny, reason); err != nil {
			d.Log.Warn("lockdown: set overwrite failed", "channel", ch.ID, "err", err)
			continue
		}
		snapshot[ch.ID] = channelOverwriteSnapshot{Allow: priorAllow, Deny: priorDeny, ExistedBefore: existed}
		locked++
	}

	if err := d.Cache.SetJSON(c.Ctx, lockdownKey(gidStr), snapshot, lockdownTTL); err != nil {
		d.Log.Warn("lockdown: save snapshot failed", "guild", gidStr, "err", err)
	}

	return c.RespondEphemeral(fmt.Sprintf("Locked down %d channel(s). Run /unlock to restore.", locked))
}

// handleUnlock restores the pre-lockdown overwrites (or clears the
// send-messages deny best-effort if no snapshot survives) and lifts raid mode.
func handleUnlock(c *interactions.Context, d plugin.Deps) error {
	gidStr := c.GuildID
	reason := lockReason(c.Options().String("reason"))

	// Always clear any anti-raid raid mode on unlock.
	clearRaidMode(c.Ctx, d.Cache, gidStr)

	var snapshot map[string]channelOverwriteSnapshot
	err := d.Cache.GetJSON(c.Ctx, lockdownKey(gidStr), &snapshot)
	if errors.Is(err, cache.ErrMiss) || len(snapshot) == 0 {
		return c.RespondEphemeral(unlockNoSnapshot(c, d, gidStr, reason))
	}
	if err != nil {
		return c.RespondEphemeral("Failed to read lockdown snapshot: " + err.Error())
	}

	restored := 0
	for chID, snap := range snapshot {
		if snap.ExistedBefore {
			if err := d.Discord.SetRolePermission(chID, gidStr, snap.Allow, snap.Deny, reason); err != nil {
				d.Log.Warn("unlock: restore overwrite failed", "channel", chID, "err", err)
				continue
			}
		} else {
			if err := d.Discord.ClearRolePermission(chID, gidStr, reason); err != nil {
				d.Log.Warn("unlock: clear overwrite failed", "channel", chID, "err", err)
				continue
			}
		}
		restored++
	}
	_ = d.Cache.Delete(c.Ctx, lockdownKey(gidStr))

	return c.RespondEphemeral(fmt.Sprintf("Unlocked %d channel(s).", restored))
}

// unlockNoSnapshot lifts the @everyone SEND_MESSAGES deny on every text channel
// best-effort when no lockdown snapshot is available, returning a status line.
func unlockNoSnapshot(c *interactions.Context, d plugin.Deps, gidStr, reason string) string {
	channels, err := d.Discord.GuildChannels(gidStr)
	if err != nil {
		return "No lockdown snapshot found and failed to list channels: " + err.Error()
	}
	unlocked := 0
	for _, ch := range channels {
		if !isLockableChannel(ch.Type) {
			continue
		}
		priorAllow, priorDeny, existed := everyoneOverwrite(ch, gidStr)
		if !existed || priorDeny&int64(discordgo.PermissionSendMessages) == 0 {
			continue
		}
		newDeny := priorDeny &^ int64(discordgo.PermissionSendMessages)
		if newDeny == 0 && priorAllow == 0 {
			if err := d.Discord.ClearRolePermission(ch.ID, gidStr, reason); err != nil {
				d.Log.Warn("unlock: clear overwrite failed", "channel", ch.ID, "err", err)
				continue
			}
		} else if err := d.Discord.SetRolePermission(ch.ID, gidStr, priorAllow, newDeny, reason); err != nil {
			d.Log.Warn("unlock: set overwrite failed", "channel", ch.ID, "err", err)
			continue
		}
		unlocked++
	}
	return fmt.Sprintf("No lockdown snapshot found; cleared send-messages deny on %d channel(s).", unlocked)
}

// everyoneOverwrite returns the existing @everyone (role id == guild id)
// allow/deny overwrite for a channel, and whether one existed.
func everyoneOverwrite(ch *discordgo.Channel, guildID string) (allow, deny int64, existed bool) {
	for _, o := range ch.PermissionOverwrites {
		if o.Type == discordgo.PermissionOverwriteTypeRole && o.ID == guildID {
			return o.Allow, o.Deny, true
		}
	}
	return 0, 0, false
}

func lockReason(raw string) string {
	r := reasonOr(raw)
	return "[Lockdown] " + r
}
