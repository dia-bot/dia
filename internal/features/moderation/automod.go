package moderation

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

var (
	inviteRe = regexp.MustCompile(`(?i)(discord\.gg/|discord(app)?\.com/invite/)`)
	linkRe   = regexp.MustCompile(`(?i)https?://`)
)

// handleAutomod screens an incoming message against the guild's automod config.
func handleAutomod(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	msg, err := plugin.DecodeData[event.Message](env)
	if err != nil {
		return err
	}
	if msg.Author.Bot || msg.GuildID == "" || strings.TrimSpace(msg.Content) == "" {
		return nil
	}

	gid, _ := event.ParseID(msg.GuildID)
	cfg, enabled, err := plugin.LoadConfig[AutomodConfig](ctx, d, gid, AutomodKey)
	if err != nil || !enabled {
		return err
	}

	if contains(cfg.IgnoredChannels, msg.ChannelID) {
		return nil
	}
	if msg.Member != nil && intersects(cfg.IgnoredRoles, msg.Member.Roles) {
		return nil
	}

	reason, violated := scan(cfg, msg)
	if !violated {
		return nil
	}

	// Always delete the offending message.
	if err := d.Discord.DeleteMessage(msg.ChannelID, msg.ID, reason); err != nil {
		d.Log.Warn("automod: delete failed", "channel", msg.ChannelID, "msg", msg.ID, "err", err)
	}

	switch cfg.Action {
	case "timeout":
		secs := cfg.TimeoutSeconds
		if secs <= 0 {
			secs = 600
		}
		until := time.Now().Add(time.Duration(secs) * time.Second)
		if err := d.Discord.Timeout(msg.GuildID, msg.Author.ID, &until, reason); err != nil {
			d.Log.Warn("automod: timeout failed", "user", msg.Author.ID, "err", err)
		}
		recordAutomodCase(ctx, d, gid, msg.Author, "timeout", reason, secs, &until)
	case "warn":
		recordAutomodCase(ctx, d, gid, msg.Author, "warn", reason, 0, nil)
	default: // "delete" or unknown: deletion already performed above.
	}

	logAutomod(ctx, d, gid, msg, cfg.Action, reason)
	return nil
}

// scan returns a human reason and whether the message violates the config.
func scan(cfg AutomodConfig, msg event.Message) (string, bool) {
	content := msg.Content

	if cfg.BlockInvites && inviteRe.MatchString(content) {
		return "Discord invite link", true
	}
	if cfg.BlockLinks && linkRe.MatchString(content) {
		return "Link not allowed", true
	}
	if w := matchBannedWord(cfg.BannedWords, content); w != "" {
		return "Banned word", true
	}
	if cfg.MaxMentions > 0 {
		count := len(msg.Mentions)
		if msg.MentionEveryone {
			count++
		}
		if count > cfg.MaxMentions {
			return fmt.Sprintf("Too many mentions (%d > %d)", count, cfg.MaxMentions), true
		}
	}
	return "", false
}

func matchBannedWord(words []string, content string) string {
	lc := strings.ToLower(content)
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" {
			continue
		}
		if strings.Contains(lc, strings.ToLower(w)) {
			return w
		}
	}
	return ""
}

func recordAutomodCase(ctx context.Context, d plugin.Deps, gid int64, target event.User, action, reason string, durSecs int, expiresAt *time.Time) {
	uid, _ := event.ParseID(target.ID)
	_, err := d.Store.Moderation.CreateCase(ctx, store.ModCase{
		GuildID:         gid,
		UserID:          uid,
		ModeratorID:     0, // automod has no moderator
		Action:          action,
		Reason:          "[Automod] " + reason,
		DurationSeconds: durSecs,
		ExpiresAt:       expiresAt,
		Active:          true,
	})
	if err != nil {
		d.Log.Warn("automod: create case failed", "err", err)
	}
}

func logAutomod(ctx context.Context, d plugin.Deps, gid int64, msg event.Message, action, reason string) {
	cfg, _, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || cfg.LogChannel == "" {
		return
	}
	embed := &discordgo.MessageEmbed{
		Title: "Automod — " + actionTitle(action),
		Color: 0xED4245,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "User", Value: mention(msg.Author.ID, userName(msg.Author)), Inline: true},
			{Name: "Channel", Value: "<#" + msg.ChannelID + ">", Inline: true},
			{Name: "Trigger", Value: reason, Inline: false},
			{Name: "Content", Value: truncate(msg.Content, 200), Inline: false},
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	_, _ = d.Discord.SendMessage(cfg.LogChannel, &discordgo.MessageSend{Embeds: []*discordgo.MessageEmbed{embed}})
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func intersects(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		set[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := set[s]; ok {
			return true
		}
	}
	return false
}
