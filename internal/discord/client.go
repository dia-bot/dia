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

// ── Slash command registration ───────────────────────────────

// BulkOverwriteGuildCommands replaces a guild's command set (instant update).
func (c *Client) BulkOverwriteGuildCommands(guildID string, cmds []*discordgo.ApplicationCommand) ([]*discordgo.ApplicationCommand, error) {
	return c.s.ApplicationCommandBulkOverwrite(c.appID, guildID, cmds)
}

// BulkOverwriteGlobalCommands replaces the global command set (propagates over ~1h).
func (c *Client) BulkOverwriteGlobalCommands(cmds []*discordgo.ApplicationCommand) ([]*discordgo.ApplicationCommand, error) {
	return c.s.ApplicationCommandBulkOverwrite(c.appID, "", cmds)
}
