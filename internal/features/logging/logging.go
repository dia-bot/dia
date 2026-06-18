package logging

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// Embed colours per category.
const (
	colorDelete = 0xED4245 // red
	colorEdit   = 0xFAA61A // amber
	colorJoin   = 0x57F287 // green
	colorLeave  = 0x95A5A6 // grey
	colorBan    = 0x992D22 // dark red
	colorUnban  = 0x57F287 // green
	colorMod    = 0x5865F2 // blurple
)

// msgCacheTTL is how long a created message's content is retained so a later
// edit/delete can recover the "before" text.
const msgCacheTTL = 12 * time.Hour

// msgCacheKey is the Redis key for a cached message record.
func msgCacheKey(messageID string) string { return "logmsg:" + messageID }

// cachedMessage is the compact record stored on MESSAGE_CREATE.
type cachedMessage struct {
	AuthorID   string `json:"a"`
	AuthorName string `json:"n"`
	Content    string `json:"c"`
	ChannelID  string `json:"ch"`
}

// Plugin implements the server-logging feature.
type Plugin struct{}

// New returns the logging plugin.
func New() *Plugin { return &Plugin{} }

// Info identifies the plugin.
func (*Plugin) Info() plugin.Info {
	return plugin.Info{
		Key:         FeatureKey,
		Name:        "Server Logs",
		Description: "Audit trail of message edits/deletes, member joins, leaves, bans and role changes, posted as embeds to a log channel.",
		Category:    plugin.CategoryModeration,
	}
}

// Init wires the event handlers.
func (*Plugin) Init(ctx context.Context, d plugin.Deps, reg *plugin.Registrar) error {
	reg.OnEvent(event.TypeMessageCreate, func(ctx context.Context, env *event.Envelope) error {
		return handleMessageCreate(ctx, d, env)
	})
	reg.OnEvent(event.TypeMessageUpdate, func(ctx context.Context, env *event.Envelope) error {
		return handleMessageUpdate(ctx, d, env)
	})
	reg.OnEvent(event.TypeMessageDelete, func(ctx context.Context, env *event.Envelope) error {
		return handleMessageDelete(ctx, d, env)
	})
	reg.OnEvent(event.TypeMemberAdd, func(ctx context.Context, env *event.Envelope) error {
		return handleMemberAdd(ctx, d, env)
	})
	reg.OnEvent(event.TypeMemberRemove, func(ctx context.Context, env *event.Envelope) error {
		return handleMemberRemove(ctx, d, env)
	})
	reg.OnEvent(event.TypeBanAdd, func(ctx context.Context, env *event.Envelope) error {
		return handleBan(ctx, d, env, true)
	})
	reg.OnEvent(event.TypeBanRemove, func(ctx context.Context, env *event.Envelope) error {
		return handleBan(ctx, d, env, false)
	})
	reg.OnEvent(event.TypeMemberUpdate, func(ctx context.Context, env *event.Envelope) error {
		return handleMemberUpdate(ctx, d, env)
	})
	reg.OnEvent(event.TypeAutomodAction, func(ctx context.Context, env *event.Envelope) error {
		return handleAutomodAction(ctx, d, env)
	})
	return nil
}

// loadEnabled loads the config, returning ok=false if logging is disabled.
func loadEnabled(ctx context.Context, d plugin.Deps, guildID string) (Config, int64, bool) {
	gid, ok := event.ParseID(guildID)
	if !ok {
		return Config{}, 0, false
	}
	cfg, enabled, err := plugin.LoadConfig[Config](ctx, d, gid, FeatureKey)
	if err != nil || !enabled {
		return Config{}, gid, false
	}
	return cfg, gid, true
}

// messageChannel resolves the destination for message-category logs.
func (c Config) messageChannel() string {
	if c.MessageChannel != "" {
		return c.MessageChannel
	}
	return c.Channel
}

// memberChannel resolves the destination for member-category logs.
func (c Config) memberChannel() string {
	if c.MemberChannel != "" {
		return c.MemberChannel
	}
	return c.Channel
}

func (c Config) ignored(channelID string) bool {
	for _, id := range c.IgnoredChannels {
		if id == channelID {
			return true
		}
	}
	return false
}

func post(d plugin.Deps, channel string, embed *discordgo.MessageEmbed) {
	if channel == "" {
		return
	}
	_, _ = d.Discord.SendMessage(channel, &discordgo.MessageSend{
		Embeds:          []*discordgo.MessageEmbed{embed},
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
	})
}

// ── Message handlers ─────────────────────────────────────────

func handleMessageCreate(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	cfg, _, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok {
		return nil
	}
	// Only cache when an edit/delete log could later use it.
	if !cfg.MessageDelete && !cfg.MessageEdit {
		return nil
	}
	m, err := plugin.DecodeData[event.Message](env)
	if err != nil {
		return err
	}
	if m.Author.Bot || strings.TrimSpace(m.Content) == "" {
		return nil
	}
	rec := cachedMessage{
		AuthorID:   m.Author.ID,
		AuthorName: userTag(m.Author),
		Content:    m.Content,
		ChannelID:  m.ChannelID,
	}
	return d.Cache.SetJSON(ctx, msgCacheKey(m.ID), rec, msgCacheTTL)
}

func handleMessageDelete(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	cfg, _, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok || !cfg.MessageDelete {
		return nil
	}
	md, err := plugin.DecodeData[event.MessageDelete](env)
	if err != nil {
		return err
	}
	if cfg.ignored(md.ChannelID) {
		return nil
	}

	var rec cachedMessage
	have := d.Cache.GetJSON(ctx, msgCacheKey(md.ID), &rec) == nil
	_ = d.Cache.Delete(ctx, msgCacheKey(md.ID))

	fields := []*discordgo.MessageEmbedField{
		{Name: "Channel", Value: channelMention(md.ChannelID), Inline: true},
		{Name: "Message ID", Value: "`" + md.ID + "`", Inline: true},
	}
	embed := &discordgo.MessageEmbed{
		Title:     "Message Deleted",
		Color:     colorDelete,
		Fields:    fields,
		Timestamp: nowRFC3339(),
	}
	if have {
		embed.Author = &discordgo.MessageEmbedAuthor{Name: rec.AuthorName}
		embed.Description = "**Content**\n" + truncate(rec.Content, 1024)
		embed.Footer = &discordgo.MessageEmbedFooter{Text: "User ID: " + rec.AuthorID}
	} else {
		embed.Description = "Content unavailable (message was not cached)."
	}
	post(d, cfg.messageChannel(), embed)
	return nil
}

func handleMessageUpdate(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	cfg, _, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok || !cfg.MessageEdit {
		return nil
	}
	mu, err := plugin.DecodeData[event.MessageUpdate](env)
	if err != nil {
		return err
	}
	m := mu.Message
	if m.Author.Bot {
		return nil
	}
	after := strings.TrimSpace(m.Content)
	if after == "" {
		// Embed-only or otherwise contentless edit; nothing to show.
		return nil
	}
	if cfg.ignored(m.ChannelID) {
		return nil
	}

	var rec cachedMessage
	haveBefore := d.Cache.GetJSON(ctx, msgCacheKey(m.ID), &rec) == nil
	before := ""
	if haveBefore {
		before = rec.Content
	}
	if haveBefore && before == m.Content {
		return nil // unchanged content (e.g. embed render); skip
	}

	// Refresh the cache with the new content for subsequent edits/deletes.
	newRec := cachedMessage{
		AuthorID:   m.Author.ID,
		AuthorName: userTag(m.Author),
		Content:    m.Content,
		ChannelID:  m.ChannelID,
	}
	_ = d.Cache.SetJSON(ctx, msgCacheKey(m.ID), newRec, msgCacheTTL)

	fields := []*discordgo.MessageEmbedField{}
	if haveBefore {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Before", Value: truncate(before, 1024)})
	} else {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Before", Value: "Content unavailable (not cached)."})
	}
	fields = append(fields,
		&discordgo.MessageEmbedField{Name: "After", Value: truncate(m.Content, 1024)},
		&discordgo.MessageEmbedField{Name: "Channel", Value: channelMention(m.ChannelID), Inline: true},
		&discordgo.MessageEmbedField{Name: "Message ID", Value: "`" + m.ID + "`", Inline: true},
	)
	embed := &discordgo.MessageEmbed{
		Title:     "Message Edited",
		Color:     colorEdit,
		Author:    &discordgo.MessageEmbedAuthor{Name: userTag(m.Author)},
		Fields:    fields,
		Footer:    &discordgo.MessageEmbedFooter{Text: "User ID: " + m.Author.ID},
		Timestamp: nowRFC3339(),
	}
	post(d, cfg.messageChannel(), embed)
	return nil
}

// ── Member handlers ──────────────────────────────────────────

func handleMemberAdd(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	cfg, _, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok || !cfg.MemberJoin {
		return nil
	}
	ma, err := plugin.DecodeData[event.MemberAdd](env)
	if err != nil {
		return err
	}
	u := ma.Member.User
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: mention(u.ID) + " (" + userTag(u) + ")", Inline: false},
		{Name: "Account Age", Value: accountAge(u.ID), Inline: true},
	}
	if ma.MemberCount > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Member Count", Value: strconv.Itoa(ma.MemberCount), Inline: true})
	}
	embed := &discordgo.MessageEmbed{
		Title:     "Member Joined",
		Color:     colorJoin,
		Fields:    fields,
		Footer:    &discordgo.MessageEmbedFooter{Text: "User ID: " + u.ID},
		Timestamp: nowRFC3339(),
	}
	post(d, cfg.memberChannel(), embed)
	return nil
}

func handleMemberRemove(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	cfg, gid, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok || !cfg.MemberLeave {
		return nil
	}
	mr, err := plugin.DecodeData[event.MemberRemove](env)
	if err != nil {
		return err
	}
	u := mr.User
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: mention(u.ID) + " (" + userTag(u) + ")", Inline: false},
	}
	if roles := memberRoles(ctx, d, gid, u.ID); roles != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Roles", Value: truncate(roles, 1024), Inline: false})
	}
	if mr.MemberCount > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Member Count", Value: strconv.Itoa(mr.MemberCount), Inline: true})
	}
	embed := &discordgo.MessageEmbed{
		Title:     "Member Left",
		Color:     colorLeave,
		Fields:    fields,
		Footer:    &discordgo.MessageEmbedFooter{Text: "User ID: " + u.ID},
		Timestamp: nowRFC3339(),
	}
	post(d, cfg.memberChannel(), embed)
	return nil
}

func handleBan(ctx context.Context, d plugin.Deps, env *event.Envelope, banned bool) error {
	cfg, _, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok {
		return nil
	}
	if banned && !cfg.MemberBan {
		return nil
	}
	if !banned && !cfg.MemberUnban {
		return nil
	}
	be, err := plugin.DecodeData[event.BanEvent](env)
	if err != nil {
		return err
	}
	u := be.User
	embed := &discordgo.MessageEmbed{
		Fields: []*discordgo.MessageEmbedField{
			{Name: "User", Value: mention(u.ID) + " (" + userTag(u) + ")", Inline: false},
		},
		Footer:    &discordgo.MessageEmbedFooter{Text: "User ID: " + u.ID},
		Timestamp: nowRFC3339(),
	}
	if banned {
		embed.Title = "Member Banned"
		embed.Color = colorBan
	} else {
		embed.Title = "Member Unbanned"
		embed.Color = colorUnban
	}
	post(d, cfg.memberChannel(), embed)
	return nil
}

func handleMemberUpdate(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	cfg, _, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok || !cfg.RoleChanges {
		return nil
	}
	mu, err := plugin.DecodeData[event.MemberUpdate](env)
	if err != nil {
		return err
	}
	if len(mu.OldRoles) == 0 {
		// Without a previous role set we cannot reliably diff; skip quietly.
		return nil
	}
	added, removed := diffRoles(mu.OldRoles, mu.Member.Roles)
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	u := mu.Member.User
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: mention(u.ID) + " (" + userTag(u) + ")", Inline: false},
	}
	if len(added) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Roles Added", Value: truncate(roleMentions(added), 1024), Inline: false})
	}
	if len(removed) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Roles Removed", Value: truncate(roleMentions(removed), 1024), Inline: false})
	}
	embed := &discordgo.MessageEmbed{
		Title:     "Roles Updated",
		Color:     colorEdit,
		Fields:    fields,
		Footer:    &discordgo.MessageEmbedFooter{Text: "User ID: " + u.ID},
		Timestamp: nowRFC3339(),
	}
	post(d, cfg.memberChannel(), embed)
	return nil
}

func handleAutomodAction(ctx context.Context, d plugin.Deps, env *event.Envelope) error {
	cfg, _, ok := loadEnabled(ctx, d, env.GuildID)
	if !ok || !cfg.ModActions {
		return nil
	}
	a, err := plugin.DecodeData[event.AutomodAction](env)
	if err != nil {
		return err
	}
	fields := []*discordgo.MessageEmbedField{
		{Name: "User", Value: mention(a.User.ID) + " (" + userTag(a.User) + ")", Inline: true},
		{Name: "Rule", Value: nonEmpty(a.RuleName), Inline: true},
	}
	if len(a.Actions) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Actions", Value: strings.Join(a.Actions, ", "), Inline: true})
	}
	if a.Reason != "" {
		fields = append(fields, &discordgo.MessageEmbedField{Name: "Reason", Value: truncate(a.Reason, 1024), Inline: false})
	}
	embed := &discordgo.MessageEmbed{
		Title:     "Automod Action",
		Color:     colorMod,
		Fields:    fields,
		Footer:    &discordgo.MessageEmbedFooter{Text: "User ID: " + a.User.ID},
		Timestamp: nowRFC3339(),
	}
	post(d, cfg.memberChannel(), embed)
	return nil
}

// ── Helpers ──────────────────────────────────────────────────

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

func mention(userID string) string { return "<@" + userID + ">" }

func channelMention(channelID string) string { return "<#" + channelID + ">" }

func userTag(u event.User) string {
	if u.GlobalName != "" {
		return u.GlobalName + " (@" + u.Username + ")"
	}
	if u.Discriminator != "" && u.Discriminator != "0" {
		return u.Username + "#" + u.Discriminator
	}
	return "@" + u.Username
}

func nonEmpty(s string) string {
	if strings.TrimSpace(s) == "" {
		return "unknown"
	}
	return s
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

// accountAge returns a human age plus a Discord relative timestamp derived from
// the user's snowflake (created_ms = (id>>22)+1420070400000).
func accountAge(userID string) string {
	id, ok := event.ParseID(userID)
	if !ok {
		return "unknown"
	}
	createdMs := (id >> 22) + 1420070400000
	created := time.UnixMilli(createdMs)
	d := time.Since(created)
	days := int(d.Hours() / 24)
	var human string
	switch {
	case days >= 365:
		human = fmt.Sprintf("%d year(s)", days/365)
	case days >= 30:
		human = fmt.Sprintf("%d month(s)", days/30)
	case days >= 1:
		human = fmt.Sprintf("%d day(s)", days)
	default:
		human = fmt.Sprintf("%d hour(s)", int(d.Hours()))
	}
	return fmt.Sprintf("%s old (<t:%d:R>)", human, created.Unix())
}

// diffRoles returns roles present in after but not before (added), and roles in
// before but not after (removed).
func diffRoles(before, after []string) (added, removed []string) {
	beforeSet := make(map[string]struct{}, len(before))
	for _, r := range before {
		beforeSet[r] = struct{}{}
	}
	afterSet := make(map[string]struct{}, len(after))
	for _, r := range after {
		afterSet[r] = struct{}{}
	}
	for _, r := range after {
		if _, ok := beforeSet[r]; !ok {
			added = append(added, r)
		}
	}
	for _, r := range before {
		if _, ok := afterSet[r]; !ok {
			removed = append(removed, r)
		}
	}
	return added, removed
}

func roleMentions(roleIDs []string) string {
	parts := make([]string, 0, len(roleIDs))
	for _, id := range roleIDs {
		parts = append(parts, "<@&"+id+">")
	}
	return strings.Join(parts, ", ")
}

// memberRoles returns the role mentions for a member from the guild-state
// snapshot if the member can be resolved; "" otherwise. It uses a best-effort
// Discord lookup since the leave event carries no roles.
func memberRoles(ctx context.Context, d plugin.Deps, guildID int64, userID string) string {
	if d.Discord == nil {
		return ""
	}
	m, err := d.Discord.GuildMember(strconv.FormatInt(guildID, 10), userID)
	if err != nil || m == nil || len(m.Roles) == 0 {
		return ""
	}
	return roleMentions(m.Roles)
}
