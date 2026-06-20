package exec

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// DiscordClient is the abstract surface step handlers call. The production
// binding wraps internal/discord.Client; tests inject a stub.
type DiscordClient interface {
	// Interactions
	Respond(ref Interaction, resp *discordgo.InteractionResponse) error
	Defer(ref Interaction, ephemeral bool) error
	Followup(ref Interaction, params *discordgo.WebhookParams) (*discordgo.Message, error)
	EditResponse(ref Interaction, edit *discordgo.WebhookEdit) (*discordgo.Message, error)

	// Messaging
	SendMessage(channelID string, data *discordgo.MessageSend) (*discordgo.Message, error)
	SendDM(userID string, data *discordgo.MessageSend) (*discordgo.Message, error)
	EditMessage(channelID, messageID string, edit *discordgo.MessageEdit) (*discordgo.Message, error)
	FetchMessage(channelID, messageID string) (*discordgo.Message, error)
	ListMessages(channelID string, limit int) ([]*discordgo.Message, error)
	BulkDeleteMessages(channelID string, messageIDs []string, reason string) error
	CrosspostMessage(channelID, messageID string) error
	DeleteMessage(channelID, messageID, reason string) error

	// Reactions / pins
	AddReaction(channelID, messageID, emoji string) error
	RemoveOwnReaction(channelID, messageID, emoji string) error
	RemoveUserReaction(channelID, messageID, emoji, userID string) error
	ClearReactions(channelID, messageID, emoji string) error // emoji "" = all
	PinMessage(channelID, messageID, reason string) error
	UnpinMessage(channelID, messageID, reason string) error

	// Roles / members
	AddRole(guildID, userID, roleID, reason string) error
	RemoveRole(guildID, userID, roleID, reason string) error
	SetNickname(guildID, userID, nickname, reason string) error
	Kick(guildID, userID, reason string) error
	Ban(guildID, userID, reason string, deleteMessageDays int) error
	Unban(guildID, userID, reason string) error
	Timeout(guildID, userID string, until *time.Time, reason string) error
	GetMember(guildID, userID string) (*discordgo.Member, error)

	// Channels / threads / voice
	CreateChannel(guildID string, data *discordgo.GuildChannelCreateData, reason string) (*discordgo.Channel, error)
	EditChannel(channelID string, data *discordgo.ChannelEdit, reason string) (*discordgo.Channel, error)
	DeleteChannel(channelID, reason string) error
	StartThread(channelID, messageID, name string, autoArchive int, private, invitable bool) (*discordgo.Channel, error)
	ArchiveThread(threadID string, locked bool) error
	ThreadMemberAdd(threadID, userID string) error
	ThreadMemberRemove(threadID, userID string) error
	CreateInvite(channelID string, maxAgeSeconds, maxUses int, temporary, unique bool, reason string) (*discordgo.Invite, error)
	MoveVoice(guildID, userID, channelID string) error
	SetVoiceState(guildID, userID string, mute, deafen *bool, reason string) error
}

// Interaction is the engine-local view of an interaction reference (the engine
// uses this thin type so it does not depend on internal/discord directly).
type Interaction struct {
	ID    string
	AppID string
	Token string
}

// StoreClient is what handlers read/write from the data layer.
type StoreClient interface {
	// KV
	KVGet(ctx context.Context, e store.FeatureKVEntry) (store.FeatureKVEntry, error)
	KVSet(ctx context.Context, e store.FeatureKVEntry) error
	KVDelete(ctx context.Context, e store.FeatureKVEntry) error
	// Image templates
	GetImageTemplate(ctx context.Context, guildID, id int64) (store.CommandImageTemplate, error)
	// Audit log
	AppendAudit(ctx context.Context, e store.AuditEntry) error
	// Commands (for run_command)
	GetCommandByName(ctx context.Context, guildID int64, name string) (store.CustomCommand, error)
	// Automations (for run_automation)
	GetAutomation(ctx context.Context, guildID int64, id string) (store.Automation, error)
}

// ImagingClient renders Studio layouts to PNG bytes.
type ImagingClient interface {
	RenderLayoutBytes(ctx context.Context, layout json.RawMessage, vars map[string]string, fonts map[string]string) ([]byte, error)
}

// HTTPClient performs SSRF-guarded outbound requests for the http_request step.
type HTTPClient interface {
	Do(ctx context.Context, req *http.Request) (*http.Response, error)
}

// ── internal helpers ──────────────────────────────────────────────────────

// refForRun extracts an Interaction reference from a run, or returns the
// zero value when the run has no interaction (e.g. scheduled triggers).
func refForRun(r *RunState, appID string) Interaction {
	return Interaction{ID: r.InteractionID, AppID: appID, Token: r.InteractionToken}
}

// declaredOption pulls a slash option value from input scope as any.
func declaredOption(scope *cc.Scope, name string) any {
	if scope == nil || scope.Data == nil {
		return nil
	}
	return scope.Data.Input[name]
}
