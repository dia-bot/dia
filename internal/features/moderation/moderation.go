// Package moderation provides manual moderation slash commands (ban, kick,
// timeout, warn, …) backed by a per-guild case log, plus a configurable automod
// that screens incoming messages for invites, links, banned words and mention
// spam.
package moderation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/interactions"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Plugin implements the moderation + automod feature.
type Plugin struct{}

// New returns the moderation plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Moderation",
		Description: "Ban, kick, timeout and warn members with a case log, plus a rule-based automod: ordered detection rules (words, regex, invites, links, spam, mentions, caps, emoji, new accounts, names, and more), per-rule actions, and a cross-rule escalation ladder driven by infraction points.",
		Category:    plugin.CategoryModeration,
	}
}

// Init registers the moderation slash commands, the automod message handler and
// the background expiry worker.
func (*Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	banPerm := int64(discordgo.PermissionBanMembers)
	kickPerm := int64(discordgo.PermissionKickMembers)
	modPerm := int64(discordgo.PermissionModerateMembers)
	chanPerm := int64(discordgo.PermissionManageChannels)

	// autocompleteReason is the shared reason-template autocomplete handler wired
	// onto every command whose reason/text option opts into autocomplete.
	autocompleteReason := func(c *interactions.Context) error { return reasonAutocomplete(c, d) }

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("ban", "Ban a member from the server",
			interactions.UserOpt("user", "The member to ban", true),
			interactions.WithAutocomplete(interactions.StringOpt("reason", "Reason for the ban", false)),
			interactions.IntOpt("delete_days", "Delete this many days of their messages (0-7)", false),
			interactions.StringOpt("duration", "Temp-ban duration, e.g. 30s, 10m, 2h, 7d", false),
		), banPerm),
		Handler:      func(c *interactions.Context) error { return handleBan(c, d) },
		Autocomplete: autocompleteReason,
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("kick", "Kick a member from the server",
			interactions.UserOpt("user", "The member to kick", true),
			interactions.WithAutocomplete(interactions.StringOpt("reason", "Reason for the kick", false)),
		), kickPerm),
		Handler:      func(c *interactions.Context) error { return handleKick(c, d) },
		Autocomplete: autocompleteReason,
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("timeout", "Timeout a member",
			interactions.UserOpt("user", "The member to timeout", true),
			interactions.StringOpt("duration", "Timeout duration, e.g. 30s, 10m, 2h, 7d", true),
			interactions.WithAutocomplete(interactions.StringOpt("reason", "Reason for the timeout", false)),
		), modPerm),
		Handler:      func(c *interactions.Context) error { return handleTimeout(c, d) },
		Autocomplete: autocompleteReason,
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("untimeout", "Clear a member's timeout",
			interactions.UserOpt("user", "The member to untimeout", true),
			interactions.StringOpt("reason", "Reason", false),
		), modPerm),
		Handler: func(c *interactions.Context) error { return handleUntimeout(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("unban", "Lift a ban by user ID",
			interactions.StringOpt("user_id", "The ID of the user to unban", true),
			interactions.StringOpt("reason", "Reason", false),
		), banPerm),
		Handler: func(c *interactions.Context) error { return handleUnban(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("warn", "Warn a member",
			interactions.UserOpt("user", "The member to warn", true),
			interactions.WithAutocomplete(interactions.StringOpt("reason", "Reason for the warning", false)),
		), modPerm),
		Handler:      func(c *interactions.Context) error { return handleWarn(c, d) },
		Autocomplete: autocompleteReason,
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("case", "Show a moderation case",
			interactions.IntOpt("number", "The case number", true),
		), modPerm),
		Handler: func(c *interactions.Context) error { return handleCase(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("cases", "List recent moderation cases",
			interactions.UserOpt("user", "Only show cases for this member", false),
		), modPerm),
		Handler: func(c *interactions.Context) error { return handleCases(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("note", "Attach a private staff note to a member",
			interactions.UserOpt("user", "The member to note", true),
			interactions.WithAutocomplete(interactions.StringOpt("text", "The note text", true)),
		), modPerm),
		Handler:      func(c *interactions.Context) error { return handleNote(c, d) },
		Autocomplete: autocompleteReason,
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("notes", "List a member's staff notes",
			interactions.UserOpt("user", "The member whose notes to list", true),
		), modPerm),
		Handler: func(c *interactions.Context) error { return handleNotes(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("lockdown", "Deny @everyone send-messages on every text channel",
			interactions.StringOpt("reason", "Reason for the lockdown", false),
		), chanPerm),
		Handler: func(c *interactions.Context) error { return handleLockdown(c, d) },
	})

	reg.Command(&interactions.Command{
		Def: interactions.RequirePerms(interactions.Slash("unlock", "Restore channels after a lockdown and lift raid mode",
			interactions.StringOpt("reason", "Reason for the unlock", false),
		), chanPerm),
		Handler: func(c *interactions.Context) error { return handleUnlock(c, d) },
	})

	reg.OnEvent(event.TypeMessageCreate, func(ctx context.Context, env *event.Envelope) error {
		return handleAutomodMessage(ctx, d, env, false)
	})
	reg.OnEvent(event.TypeMessageUpdate, func(ctx context.Context, env *event.Envelope) error {
		return handleAutomodMessage(ctx, d, env, true)
	})
	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return handleAutomodMember(ctx, d, env)
	})
	reg.OnEvent(event.TypeMemberUpdate, func(ctx context.Context, env *event.Envelope) error {
		return handleAutomodMember(ctx, d, env)
	})

	reg.Worker("mod-expiry", func(ctx context.Context) { runExpiry(ctx, d) })
	reg.Worker("automod-threatfeed", func(ctx context.Context) { runThreatFeed(ctx, d) })

	return nil
}

// ── Slash command handlers ───────────────────────────────────

func handleBan(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	target, ok := opts.User("user")
	if !ok {
		return c.RespondEphemeral("You must specify a user to ban.")
	}
	reason := reasonOr(opts.String("reason"))
	deleteDays := int(opts.Int("delete_days"))
	if deleteDays < 0 {
		deleteDays = 0
	}
	if deleteDays > 7 {
		deleteDays = 7
	}

	var expiresAt *time.Time
	var durSecs int
	if raw := opts.String("duration"); raw != "" {
		dur, err := parseDuration(raw)
		if err != nil || dur <= 0 {
			return c.RespondEphemeral("Invalid duration. Use e.g. `30s`, `10m`, `2h`, `7d`.")
		}
		t := time.Now().Add(dur)
		expiresAt = &t
		durSecs = int(dur / time.Second)
	}

	if err := d.Discord.Ban(c.GuildID, target.ID, reason, deleteDays); err != nil {
		return c.RespondEphemeral("Failed to ban: " + err.Error())
	}
	return finishAction(c, d, target, "ban", reason, durSecs, expiresAt)
}

func handleKick(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	target, ok := opts.User("user")
	if !ok {
		return c.RespondEphemeral("You must specify a user to kick.")
	}
	reason := reasonOr(opts.String("reason"))
	if err := d.Discord.Kick(c.GuildID, target.ID, reason); err != nil {
		return c.RespondEphemeral("Failed to kick: " + err.Error())
	}
	return finishAction(c, d, target, "kick", reason, 0, nil)
}

func handleTimeout(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	target, ok := opts.User("user")
	if !ok {
		return c.RespondEphemeral("You must specify a user to timeout.")
	}
	dur, err := parseDuration(opts.String("duration"))
	if err != nil || dur <= 0 {
		return c.RespondEphemeral("Invalid duration. Use e.g. `30s`, `10m`, `2h`, `7d`.")
	}
	reason := reasonOr(opts.String("reason"))
	until := time.Now().Add(dur)
	if err := d.Discord.Timeout(c.GuildID, target.ID, &until, reason); err != nil {
		return c.RespondEphemeral("Failed to timeout: " + err.Error())
	}
	return finishAction(c, d, target, "timeout", reason, int(dur/time.Second), &until)
}

func handleUntimeout(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	target, ok := opts.User("user")
	if !ok {
		return c.RespondEphemeral("You must specify a user to untimeout.")
	}
	reason := reasonOr(opts.String("reason"))
	if err := d.Discord.Timeout(c.GuildID, target.ID, nil, reason); err != nil {
		return c.RespondEphemeral("Failed to clear timeout: " + err.Error())
	}
	return finishAction(c, d, target, "untimeout", reason, 0, nil)
}

func handleUnban(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	userID := strings.TrimSpace(opts.String("user_id"))
	if _, ok := event.ParseID(userID); !ok {
		return c.RespondEphemeral("Provide a valid user ID to unban.")
	}
	reason := reasonOr(opts.String("reason"))
	if err := d.Discord.Unban(c.GuildID, userID, reason); err != nil {
		return c.RespondEphemeral("Failed to unban: " + err.Error())
	}
	return finishAction(c, d, event.User{ID: userID, Username: userID}, "unban", reason, 0, nil)
}

func handleWarn(c *interactions.Context, d plugin.Deps) error {
	opts := c.Options()
	target, ok := opts.User("user")
	if !ok {
		return c.RespondEphemeral("You must specify a user to warn.")
	}
	reason := reasonOr(opts.String("reason"))
	return finishAction(c, d, target, "warn", reason, 0, nil)
}

func handleCase(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	number := int(c.Options().Int("number"))
	mc, err := d.Store.Moderation.GetCase(c.Ctx, gid, number)
	if errors.Is(err, store.ErrNotFound) {
		return c.RespondEphemeral(fmt.Sprintf("No case #%d found.", number))
	}
	if err != nil {
		return err
	}
	return c.RespondEmbed(true, caseEmbed(mc))
}

func handleCases(c *interactions.Context, d plugin.Deps) error {
	gid, _ := event.ParseID(c.GuildID)
	var userID *int64
	if target, ok := c.Options().User("user"); ok {
		if id, ok := event.ParseID(target.ID); ok {
			userID = &id
		}
	}
	cases, err := d.Store.Moderation.ListCases(c.Ctx, gid, userID, 15, 0)
	if err != nil {
		return err
	}
	if len(cases) == 0 {
		return c.RespondEphemeral("No moderation cases found.")
	}
	return c.RespondEmbed(true, casesEmbed(cases))
}

// ── Shared action plumbing ───────────────────────────────────

// finishAction records a case, optionally DMs the target and logs to the mod
// channel, then replies ephemerally to the moderator with the case number.
func finishAction(c *interactions.Context, d plugin.Deps, target event.User, action, reason string, durSecs int, expiresAt *time.Time) error {
	gid, _ := event.ParseID(c.GuildID)
	cfg, _, _ := plugin.LoadConfig[Config](c.Ctx, d, gid, FeatureKey)

	uid, _ := event.ParseID(target.ID)
	modID, _ := event.ParseID(c.User.ID)
	mc := store.ModCase{
		GuildID:         gid,
		UserID:          uid,
		ModeratorID:     modID,
		Action:          action,
		Reason:          reason,
		DurationSeconds: durSecs,
		ExpiresAt:       expiresAt,
		Active:          true,
	}
	created, err := d.Store.Moderation.CreateCase(c.Ctx, mc)
	if err != nil {
		return c.RespondEphemeral("Action applied but failed to record case: " + err.Error())
	}

	if cfg.DMOnAction && target.ID != "" {
		notice := dmNotice(action, reason)
		if notice != "" {
			_ = d.Discord.SendDM(target.ID, notice)
		}
	}

	if cfg.LogChannel != "" {
		embed := actionLogEmbed(created, target, c.User, expiresAt)
		_, _ = d.Discord.SendMessage(cfg.LogChannel, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}})
	}

	return c.RespondEphemeral(fmt.Sprintf("✅ %s applied — case #%d.", actionTitle(action), created.CaseNumber))
}

func dmNotice(action, reason string) string {
	verb := map[string]string{
		"ban":       "You have been banned.",
		"kick":      "You have been kicked.",
		"timeout":   "You have been timed out.",
		"untimeout": "Your timeout has been cleared.",
		"unban":     "Your ban has been lifted.",
		"warn":      "You have been warned.",
	}[action]
	if verb == "" {
		return ""
	}
	if reason != "" {
		return verb + " Reason: " + reason
	}
	return verb
}

// ── Background expiry worker ──────────────────────────────────

func runExpiry(ctx context.Context, d plugin.Deps) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweepExpired(ctx, d)
		}
	}
}

func sweepExpired(ctx context.Context, d plugin.Deps) {
	cases, err := d.Store.Moderation.ListExpired(ctx, time.Now(), 50)
	if err != nil {
		d.Log.Warn("mod-expiry: list expired failed", "err", err)
		return
	}
	for _, mc := range cases {
		if ctx.Err() != nil {
			return
		}
		guildID := event.FormatID(mc.GuildID)
		userID := event.FormatID(mc.UserID)
		switch mc.Action {
		case "ban":
			if err := d.Discord.Unban(guildID, userID, "Temp-ban expired"); err != nil {
				d.Log.Warn("mod-expiry: auto-unban failed", "guild", guildID, "user", userID, "err", err)
				continue
			}
			if err := d.Store.Moderation.Deactivate(ctx, mc.ID); err != nil {
				d.Log.Warn("mod-expiry: deactivate failed", "case", mc.ID, "err", err)
			}
		case "timeout":
			// Discord auto-clears the timeout; just mark the case resolved.
			if err := d.Store.Moderation.Deactivate(ctx, mc.ID); err != nil {
				d.Log.Warn("mod-expiry: deactivate failed", "case", mc.ID, "err", err)
			}
		default:
			// Nothing to reverse; resolve so we stop re-scanning it.
			_ = d.Store.Moderation.Deactivate(ctx, mc.ID)
		}
	}
}

// ── Helpers ──────────────────────────────────────────────────

func reasonOr(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "No reason provided"
	}
	return reason
}

func actionTitle(action string) string {
	switch action {
	case "ban":
		return "Ban"
	case "kick":
		return "Kick"
	case "timeout":
		return "Timeout"
	case "untimeout":
		return "Untimeout"
	case "unban":
		return "Unban"
	case "warn":
		return "Warn"
	case "note":
		return "Note"
	default:
		if action == "" {
			return "Action"
		}
		return strings.ToUpper(action[:1]) + action[1:]
	}
}

func actionColor(action string) int {
	switch action {
	case "ban":
		return 0xED4245
	case "kick":
		return 0xFAA61A
	case "timeout":
		return 0xF1C40F
	case "warn":
		return 0xFEE75C
	case "unban", "untimeout":
		return 0x57F287
	case "note":
		return 0x99AAB5 // neutral grey
	default:
		return 0x5865F2
	}
}
