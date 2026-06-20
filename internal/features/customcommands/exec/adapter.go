package exec

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/layout"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// ── Discord adapter ─────────────────────────────────────────────────────────

// DiscordAdapter wraps internal/discord.Client to satisfy DiscordClient.
type DiscordAdapter struct {
	C *discord.Client
}

func (a *DiscordAdapter) ref(r Interaction) discord.InteractionRef {
	return discord.InteractionRef{ID: r.ID, AppID: a.C.AppID(), Token: r.Token}
}

func (a *DiscordAdapter) Respond(ref Interaction, resp *discordgo.InteractionResponse) error {
	return a.C.Respond(a.ref(ref), resp)
}

func (a *DiscordAdapter) Defer(ref Interaction, ephemeral bool) error {
	return a.C.Defer(a.ref(ref), ephemeral)
}

func (a *DiscordAdapter) Followup(ref Interaction, params *discordgo.WebhookParams) (*discordgo.Message, error) {
	return a.C.Followup(a.ref(ref), params)
}

func (a *DiscordAdapter) EditResponse(ref Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error) {
	return a.C.EditResponse(a.ref(ref), edit)
}

func (a *DiscordAdapter) SendMessage(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
	return a.C.SendMessage(channelID, data)
}

func (a *DiscordAdapter) SendDM(userID string, data *discordgo.MessageSend) (*discordgo.Message, error) {
	ch, err := a.C.Session().UserChannelCreate(userID)
	if err != nil {
		return nil, err
	}
	return a.C.SendMessage(ch.ID, data)
}

func (a *DiscordAdapter) DeleteMessage(channelID, messageID, reason string) error {
	return a.C.DeleteMessage(channelID, messageID, reason)
}

func (a *DiscordAdapter) EditMessage(channelID, messageID string, edit *discordgo.MessageEdit) (*discordgo.Message, error) {
	edit.Channel = channelID
	edit.ID = messageID
	return a.C.Session().ChannelMessageEditComplex(edit)
}

func (a *DiscordAdapter) FetchMessage(channelID, messageID string) (*discordgo.Message, error) {
	return a.C.Session().ChannelMessage(channelID, messageID)
}

func (a *DiscordAdapter) ListMessages(channelID string, limit int) ([]*discordgo.Message, error) {
	return a.C.Session().ChannelMessages(channelID, limit, "", "", "")
}

func (a *DiscordAdapter) BulkDeleteMessages(channelID string, messageIDs []string, reason string) error {
	if len(messageIDs) == 1 {
		return a.C.DeleteMessage(channelID, messageIDs[0], reason)
	}
	return a.C.Session().ChannelMessagesBulkDelete(channelID, messageIDs, discordgo.WithAuditLogReason(reason))
}

func (a *DiscordAdapter) CrosspostMessage(channelID, messageID string) error {
	_, err := a.C.Session().ChannelMessageCrosspost(channelID, messageID)
	return err
}

func (a *DiscordAdapter) AddReaction(channelID, messageID, emoji string) error {
	return a.C.Session().MessageReactionAdd(channelID, messageID, emoji)
}
func (a *DiscordAdapter) RemoveOwnReaction(channelID, messageID, emoji string) error {
	return a.C.Session().MessageReactionRemove(channelID, messageID, emoji, "@me")
}
func (a *DiscordAdapter) RemoveUserReaction(channelID, messageID, emoji, userID string) error {
	return a.C.Session().MessageReactionRemove(channelID, messageID, emoji, userID)
}

func (a *DiscordAdapter) ClearReactions(channelID, messageID, emoji string) error {
	if emoji == "" {
		return a.C.Session().MessageReactionsRemoveAll(channelID, messageID)
	}
	return a.C.Session().MessageReactionsRemoveEmoji(channelID, messageID, emoji)
}

func (a *DiscordAdapter) PinMessage(channelID, messageID, reason string) error {
	_ = reason
	return a.C.Session().ChannelMessagePin(channelID, messageID)
}
func (a *DiscordAdapter) UnpinMessage(channelID, messageID, reason string) error {
	_ = reason
	return a.C.Session().ChannelMessageUnpin(channelID, messageID)
}

func (a *DiscordAdapter) AddRole(guildID, userID, roleID, reason string) error {
	return a.C.AddRole(guildID, userID, roleID, reason)
}
func (a *DiscordAdapter) RemoveRole(guildID, userID, roleID, reason string) error {
	return a.C.RemoveRole(guildID, userID, roleID, reason)
}

func (a *DiscordAdapter) SetNickname(guildID, userID, nickname, reason string) error {
	return a.C.Session().GuildMemberNickname(guildID, userID, nickname, discordgo.WithAuditLogReason(reason))
}

func (a *DiscordAdapter) Kick(guildID, userID, reason string) error {
	return a.C.Kick(guildID, userID, reason)
}
func (a *DiscordAdapter) Ban(guildID, userID, reason string, deleteMessageDays int) error {
	return a.C.Ban(guildID, userID, reason, deleteMessageDays)
}
func (a *DiscordAdapter) Unban(guildID, userID, reason string) error {
	return a.C.Unban(guildID, userID, reason)
}
func (a *DiscordAdapter) Timeout(guildID, userID string, until *time.Time, reason string) error {
	return a.C.Timeout(guildID, userID, until, reason)
}

func (a *DiscordAdapter) GetMember(guildID, userID string) (*discordgo.Member, error) {
	return a.C.Session().GuildMember(guildID, userID)
}

func (a *DiscordAdapter) CreateChannel(guildID string, data *discordgo.GuildChannelCreateData, reason string) (*discordgo.Channel, error) {
	if data == nil {
		return nil, errors.New("create channel: missing data")
	}
	return a.C.Session().GuildChannelCreateComplex(guildID, *data, discordgo.WithAuditLogReason(reason))
}

func (a *DiscordAdapter) EditChannel(channelID string, data *discordgo.ChannelEdit, reason string) (*discordgo.Channel, error) {
	return a.C.Session().ChannelEditComplex(channelID, data, discordgo.WithAuditLogReason(reason))
}

func (a *DiscordAdapter) DeleteChannel(channelID, reason string) error {
	_, err := a.C.Session().ChannelDelete(channelID, discordgo.WithAuditLogReason(reason))
	return err
}

func (a *DiscordAdapter) StartThread(channelID, messageID, name string, autoArchive int, private, invitable bool) (*discordgo.Channel, error) {
	if messageID != "" {
		return a.C.Session().MessageThreadStart(channelID, messageID, name, autoArchive)
	}
	typ := discordgo.ChannelTypeGuildPublicThread
	if private {
		typ = discordgo.ChannelTypeGuildPrivateThread
	}
	return a.C.Session().ThreadStartComplex(channelID, &discordgo.ThreadStart{
		Name:                name,
		Type:                typ,
		AutoArchiveDuration: autoArchive,
		Invitable:           invitable,
	})
}

func (a *DiscordAdapter) ArchiveThread(threadID string, locked bool) error {
	archived := true
	edit := &discordgo.ChannelEdit{Archived: &archived}
	if locked {
		edit.Locked = &locked
	}
	_, err := a.C.Session().ChannelEditComplex(threadID, edit)
	return err
}

func (a *DiscordAdapter) ThreadMemberAdd(threadID, userID string) error {
	return a.C.Session().ThreadMemberAdd(threadID, userID)
}
func (a *DiscordAdapter) ThreadMemberRemove(threadID, userID string) error {
	return a.C.Session().ThreadMemberRemove(threadID, userID)
}

func (a *DiscordAdapter) CreateInvite(channelID string, maxAgeSeconds, maxUses int, temporary, unique bool, reason string) (*discordgo.Invite, error) {
	return a.C.Session().ChannelInviteCreate(channelID, discordgo.Invite{
		MaxAge:    maxAgeSeconds,
		MaxUses:   maxUses,
		Temporary: temporary,
		Unique:    unique,
	}, discordgo.WithAuditLogReason(reason))
}

func (a *DiscordAdapter) MoveVoice(guildID, userID, channelID string) error {
	var ch *string
	if channelID != "" {
		ch = &channelID
	}
	return a.C.Session().GuildMemberMove(guildID, userID, ch)
}

func (a *DiscordAdapter) SetVoiceState(guildID, userID string, mute, deafen *bool, reason string) error {
	if mute != nil {
		if err := a.C.Session().GuildMemberMute(guildID, userID, *mute, discordgo.WithAuditLogReason(reason)); err != nil {
			return err
		}
	}
	if deafen != nil {
		if err := a.C.Session().GuildMemberDeafen(guildID, userID, *deafen, discordgo.WithAuditLogReason(reason)); err != nil {
			return err
		}
	}
	return nil
}

// ── Store adapter ──────────────────────────────────────────────────────────

// StoreAdapter wraps the typed repos behind the StoreClient surface.
type StoreAdapter struct {
	S *store.Store
}

func (a *StoreAdapter) KVGet(ctx context.Context, e store.FeatureKVEntry) (store.FeatureKVEntry, error) {
	return a.S.FeatureKV.Get(ctx, e)
}
func (a *StoreAdapter) KVSet(ctx context.Context, e store.FeatureKVEntry) error {
	return a.S.FeatureKV.Set(ctx, e)
}
func (a *StoreAdapter) KVDelete(ctx context.Context, e store.FeatureKVEntry) error {
	return a.S.FeatureKV.Delete(ctx, e)
}
func (a *StoreAdapter) GetImageTemplate(ctx context.Context, guildID, id int64) (store.CommandImageTemplate, error) {
	return a.S.ImageTemplates.Get(ctx, guildID, id)
}
func (a *StoreAdapter) AppendAudit(ctx context.Context, e store.AuditEntry) error {
	return a.S.Audit.Add(ctx, e)
}
func (a *StoreAdapter) GetCommandByName(ctx context.Context, guildID int64, name string) (store.CustomCommand, error) {
	return a.S.CustomCommands.GetByName(ctx, guildID, name)
}
func (a *StoreAdapter) GetAutomation(ctx context.Context, guildID int64, id string) (store.Automation, error) {
	return a.S.Automations.Get(ctx, guildID, id)
}

// ── Imaging adapter ────────────────────────────────────────────────────────

// ImagingAdapter wraps internal/imaging.Renderer for RenderLayoutBytes.
type ImagingAdapter struct {
	R *imaging.Renderer
}

func (a *ImagingAdapter) RenderLayoutBytes(ctx context.Context, raw json.RawMessage, vars map[string]string, fonts map[string]string) ([]byte, error) {
	var layoutDoc layout.Layout
	if err := json.Unmarshal(raw, &layoutDoc); err != nil {
		return nil, err
	}
	return a.R.RenderLayout(ctx, layoutDoc, vars, fonts)
}

// ── HTTP adapter ───────────────────────────────────────────────────────────

// HTTPAdapter wraps an *http.Client (caller-supplied with the SSRF guard).
type HTTPAdapter struct {
	Client *http.Client
}

func (a *HTTPAdapter) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	return a.Client.Do(req)
}
