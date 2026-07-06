// Package discord is a thin, REST-only wrapper over the vendored discordgo
// library. The Elixir gateway owns the WebSocket connections; the Go services
// never open a gateway here — they only make REST calls (respond to
// interactions, send messages, manage roles/members, register slash commands).
package discord

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/dia-bot/dia/pkg/discordgo"
)

// Client is a REST-only Discord client.
type Client struct {
	s     *discordgo.Session
	appID string
	log   *slog.Logger
}

// New constructs a REST-only client. It never calls Session.Open(), so no
// gateway connection is established.
func New(token, appID string, log *slog.Logger) (*Client, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("discordgo new: %w", err)
	}
	// We only use REST; keep the state cache off to save memory.
	s.StateEnabled = false
	return &Client{s: s, appID: appID, log: log}, nil
}

// AppID returns the application (client) ID.
func (c *Client) AppID() string { return c.appID }

// Session exposes the underlying discordgo session as an escape hatch for calls
// not yet wrapped here.
func (c *Client) Session() *discordgo.Session { return c.s }

// GuildEmojis lists a guild's custom emojis (dashboard emoji picker).
func (c *Client) GuildEmojis(guildID string) ([]*discordgo.Emoji, error) {
	return c.s.GuildEmojis(guildID)
}

// InteractionRef identifies an interaction for REST responses.
type InteractionRef struct {
	ID    string
	AppID string
	Token string
}

func (r InteractionRef) dg() *discordgo.Interaction {
	appID := r.AppID
	return &discordgo.Interaction{ID: r.ID, AppID: appID, Token: r.Token}
}

// Respond sends an initial interaction response.
func (c *Client) Respond(ref InteractionRef, resp *discordgo.InteractionResponse) error {
	return c.s.InteractionRespond(ref.dg(), resp)
}

// Defer acknowledges an interaction so a follow-up can be sent within 15 min.
func (c *Client) Defer(ref InteractionRef, ephemeral bool) error {
	data := &discordgo.InteractionResponseData{}
	if ephemeral {
		data.Flags = discordgo.MessageFlagsEphemeral
	}
	return c.s.InteractionRespond(ref.dg(), &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: data,
	})
}

// DeferUpdate acknowledges a component interaction with no visible response
// (DEFERRED_UPDATE_MESSAGE): the click stops spinning, the message stays as
// is, and the token remains usable for follow-ups or an @original edit.
func (c *Client) DeferUpdate(ref InteractionRef) error {
	return c.s.InteractionRespond(ref.dg(), &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredMessageUpdate,
	})
}

// Followup posts a follow-up message to a (deferred) interaction.
func (c *Client) Followup(ref InteractionRef, params *discordgo.WebhookParams) (*discordgo.Message, error) {
	return c.s.FollowupMessageCreate(ref.dg(), true, params)
}

// EditResponse edits the original interaction response.
func (c *Client) EditResponse(ref InteractionRef, edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
	return c.s.InteractionResponseEdit(ref.dg(), edit)
}

// ── Messaging ────────────────────────────────────────────────

// SendMessage sends a message to a channel.
func (c *Client) SendMessage(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
	return c.s.ChannelMessageSendComplex(channelID, data)
}

// DeleteMessage deletes a message (used by automod).
func (c *Client) DeleteMessage(channelID, messageID, reason string) error {
	return c.s.ChannelMessageDelete(channelID, messageID, discordgo.WithAuditLogReason(reason))
}

// SendDM opens a DM channel with a user and sends a message.
func (c *Client) SendDM(userID, content string) error {
	ch, err := c.s.UserChannelCreate(userID)
	if err != nil {
		return err
	}
	_, err = c.s.ChannelMessageSend(ch.ID, content)
	return err
}

// ── Roles ────────────────────────────────────────────────────

// AddRole grants a role to a member.
func (c *Client) AddRole(guildID, userID, roleID, reason string) error {
	return c.s.GuildMemberRoleAdd(guildID, userID, roleID, discordgo.WithAuditLogReason(reason))
}

// RemoveRole revokes a role from a member.
func (c *Client) RemoveRole(guildID, userID, roleID, reason string) error {
	return c.s.GuildMemberRoleRemove(guildID, userID, roleID, discordgo.WithAuditLogReason(reason))
}

// ── Moderation ───────────────────────────────────────────────

// Timeout times out (or clears, with until=nil) a member.
func (c *Client) Timeout(guildID, userID string, until *time.Time, reason string) error {
	return c.s.GuildMemberTimeout(guildID, userID, until, discordgo.WithAuditLogReason(reason))
}

// Kick removes a member from a guild.
func (c *Client) Kick(guildID, userID, reason string) error {
	return c.s.GuildMemberDeleteWithReason(guildID, userID, reason)
}

// Ban bans a user, optionally deleting up to deleteMessageDays of their messages.
func (c *Client) Ban(guildID, userID, reason string, deleteMessageDays int) error {
	return c.s.GuildBanCreateWithReason(guildID, userID, reason, deleteMessageDays)
}

// Unban lifts a ban.
func (c *Client) Unban(guildID, userID, reason string) error {
	return c.s.GuildBanDelete(guildID, userID, discordgo.WithAuditLogReason(reason))
}

// GuildMember fetches a single member (used by verification / raid scoring when
// the gateway event lacks the field).
func (c *Client) GuildMember(guildID, userID string) (*discordgo.Member, error) {
	return c.s.GuildMember(guildID, userID)
}

// ── Channels & lockdown ──────────────────────────────────────

// GuildChannels lists a guild's channels, including their permission overwrites
// (used by lockdown to read and restore @everyone send permission).
func (c *Client) GuildChannels(guildID string) ([]*discordgo.Channel, error) {
	return c.s.GuildChannels(guildID)
}

// SetRolePermission writes a role-targeted permission overwrite on a channel
// (allow/deny are permission bitfields). Used by lockdown to deny SEND_MESSAGES
// for @everyone, and to restore the prior overwrite afterwards.
func (c *Client) SetRolePermission(channelID, roleID string, allow, deny int64, reason string) error {
	return c.s.ChannelPermissionSet(channelID, roleID, discordgo.PermissionOverwriteTypeRole, allow, deny,
		discordgo.WithAuditLogReason(reason))
}

// ClearRolePermission removes a role's permission overwrite from a channel
// (used to restore a channel that had no @everyone overwrite before lockdown).
func (c *Client) ClearRolePermission(channelID, roleID, reason string) error {
	return c.s.ChannelPermissionDelete(channelID, roleID, discordgo.WithAuditLogReason(reason))
}

// CreateChannel creates a guild channel (text, category, ...) in one call,
// including any parent category and permission overwrites (used by ticketing to
// open a private ticket channel visible only to the opener + support roles).
func (c *Client) CreateChannel(guildID string, data discordgo.GuildChannelCreateData, reason string) (*discordgo.Channel, error) {
	return c.s.GuildChannelCreateComplex(guildID, data, discordgo.WithAuditLogReason(reason))
}

// EditChannel edits a channel (rename, move to a new parent, lock, ...).
func (c *Client) EditChannel(channelID string, edit *discordgo.ChannelEdit, reason string) (*discordgo.Channel, error) {
	return c.s.ChannelEdit(channelID, edit, discordgo.WithAuditLogReason(reason))
}

// DeleteChannel deletes a channel (or thread).
func (c *Client) DeleteChannel(channelID, reason string) error {
	_, err := c.s.ChannelDelete(channelID, discordgo.WithAuditLogReason(reason))
	return err
}

// SetMemberPermission writes a member-targeted permission overwrite on a channel
// (grant/revoke a single user's access to a ticket channel).
func (c *Client) SetMemberPermission(channelID, userID string, allow, deny int64, reason string) error {
	return c.s.ChannelPermissionSet(channelID, userID, discordgo.PermissionOverwriteTypeMember, allow, deny,
		discordgo.WithAuditLogReason(reason))
}

// ClearMemberPermission removes a member's permission overwrite from a channel.
func (c *Client) ClearMemberPermission(channelID, userID, reason string) error {
	return c.s.ChannelPermissionDelete(channelID, userID, discordgo.WithAuditLogReason(reason))
}

// ChannelMessages fetches up to limit (max 100) messages before beforeID
// (newest-first). Paginate a full history by passing the oldest returned id.
func (c *Client) ChannelMessages(channelID string, limit int, beforeID string) ([]*discordgo.Message, error) {
	return c.s.ChannelMessages(channelID, limit, beforeID, "", "")
}

// StartThread starts a thread in a channel (ticketing thread mode).
func (c *Client) StartThread(channelID string, data *discordgo.ThreadStart, reason string) (*discordgo.Channel, error) {
	return c.s.ThreadStartComplex(channelID, data, discordgo.WithAuditLogReason(reason))
}

// ThreadAddMember adds a member to a (private) thread.
func (c *Client) ThreadAddMember(threadID, userID string) error {
	return c.s.ThreadMemberAdd(threadID, userID)
}

// Guild fetches a guild (name / owner) for template scopes.
func (c *Client) Guild(guildID string) (*discordgo.Guild, error) {
	return c.s.Guild(guildID)
}

// SendDMComplex opens a DM channel with a user and sends a rich message (embeds,
// components). Returns the sent message so its channel can be reused.
func (c *Client) SendDMComplex(userID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
	ch, err := c.s.UserChannelCreate(userID)
	if err != nil {
		return nil, err
	}
	return c.s.ChannelMessageSendComplex(ch.ID, data)
}

// ── Native Discord AutoMod (REST) ────────────────────────────
//
// These wrap Discord's built-in AutoMod so the dashboard can manage native
// rules (Block Mention Spam, keyword/preset filters) alongside Dia's own engine.
// They require the MANAGE_GUILD permission.

// AutoModRules lists a guild's native AutoMod rules.
func (c *Client) AutoModRules(guildID string) ([]*discordgo.AutoModerationRule, error) {
	return c.s.AutoModerationRules(guildID)
}

// AutoModRuleCreate creates a native AutoMod rule.
func (c *Client) AutoModRuleCreate(guildID string, rule *discordgo.AutoModerationRule) (*discordgo.AutoModerationRule, error) {
	return c.s.AutoModerationRuleCreate(guildID, rule)
}

// AutoModRuleEdit updates a native AutoMod rule.
func (c *Client) AutoModRuleEdit(guildID, ruleID string, rule *discordgo.AutoModerationRule) (*discordgo.AutoModerationRule, error) {
	return c.s.AutoModerationRuleEdit(guildID, ruleID, rule)
}

// AutoModRuleDelete deletes a native AutoMod rule.
func (c *Client) AutoModRuleDelete(guildID, ruleID, reason string) error {
	return c.s.AutoModerationRuleDelete(guildID, ruleID, discordgo.WithAuditLogReason(reason))
}

// ── Slash command registration ───────────────────────────────

// BulkOverwriteGuildCommands replaces a guild's command set (instant update).
func (c *Client) BulkOverwriteGuildCommands(guildID string, cmds []*discordgo.ApplicationCommand) ([]*discordgo.ApplicationCommand, error) {
	return c.s.ApplicationCommandBulkOverwrite(c.appID, guildID, cmds)
}

// BulkOverwriteGlobalCommands replaces the global command set (propagates over ~1h).
func (c *Client) BulkOverwriteGlobalCommands(cmds []*discordgo.ApplicationCommand) ([]*discordgo.ApplicationCommand, error) {
	return c.s.ApplicationCommandBulkOverwrite(c.appID, "", cmds)
}
