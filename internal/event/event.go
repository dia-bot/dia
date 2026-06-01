// Package event defines the normalized event contract exchanged between the
// Elixir gateway (producer) and the Go services (consumers) over NATS JetStream.
//
// The gateway holds the Discord WebSocket connections and re-serializes each
// relevant gateway event into one of the payload types below, wrapped in an
// Envelope, and publishes it to:
//
//	discord.events.<type>.<guild_id>
//
// All snowflake IDs are encoded as decimal strings (Discord's wire format) so
// that the contract is independent of both Nostrum's and discordgo's internal
// representations. This package is the single source of truth for that schema.
package event

import "encoding/json"

// Type is the discriminator carried in an Envelope and the NATS subject.
type Type string

const (
	TypeGuildCreate       Type = "GUILD_CREATE"
	TypeGuildUpdate       Type = "GUILD_UPDATE"
	TypeGuildDelete       Type = "GUILD_DELETE"
	TypeChannelCreate     Type = "CHANNEL_CREATE"
	TypeChannelUpdate     Type = "CHANNEL_UPDATE"
	TypeChannelDelete     Type = "CHANNEL_DELETE"
	TypeRoleCreate        Type = "GUILD_ROLE_CREATE"
	TypeRoleUpdate        Type = "GUILD_ROLE_UPDATE"
	TypeRoleDelete        Type = "GUILD_ROLE_DELETE"
	TypeMemberAdd         Type = "GUILD_MEMBER_ADD"
	TypeMemberRemove      Type = "GUILD_MEMBER_REMOVE"
	TypeMemberUpdate      Type = "GUILD_MEMBER_UPDATE"
	TypeMessageCreate     Type = "MESSAGE_CREATE"
	TypeInteractionCreate Type = "INTERACTION_CREATE"
)

// SubjectPrefix is the JetStream subject root for forwarded gateway events.
const SubjectPrefix = "discord.events"

// Subject returns the NATS subject for an event of the given type and guild.
// A guild ID of "" (e.g. a DM interaction) maps to the "0" token.
func Subject(t Type, guildID string) string {
	if guildID == "" {
		guildID = "0"
	}
	return SubjectPrefix + "." + string(t) + "." + guildID
}

// Envelope is the outer message published to JetStream for every event.
type Envelope struct {
	Type    Type            `json:"type"`
	GuildID string          `json:"guild_id"`
	ShardID int             `json:"shard_id"`
	TS      int64           `json:"ts"` // unix milliseconds when forwarded
	Data    json.RawMessage `json:"data"`
}

// ── Shared sub-objects ───────────────────────────────────────

// User is a Discord user (normalized subset).
type User struct {
	ID            string `json:"id"`
	Username      string `json:"username"`
	GlobalName    string `json:"global_name,omitempty"`
	Discriminator string `json:"discriminator,omitempty"`
	Avatar        string `json:"avatar,omitempty"`
	Bot           bool   `json:"bot,omitempty"`
}

// Member is a guild member (normalized subset).
type Member struct {
	User     User     `json:"user"`
	Nick     string   `json:"nick,omitempty"`
	Avatar   string   `json:"avatar,omitempty"`
	Roles    []string `json:"roles"`
	JoinedAt string   `json:"joined_at,omitempty"`
	Pending  bool     `json:"pending,omitempty"`
}

// Channel is a guild channel (normalized subset).
type Channel struct {
	ID       string `json:"id"`
	GuildID  string `json:"guild_id,omitempty"`
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Position int    `json:"position"`
	ParentID string `json:"parent_id,omitempty"`
	Topic    string `json:"topic,omitempty"`
	NSFW     bool   `json:"nsfw,omitempty"`
}

// Role is a guild role (normalized subset).
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       int    `json:"color"`
	Position    int    `json:"position"`
	Permissions string `json:"permissions"` // decimal-string bitfield
	Hoist       bool   `json:"hoist,omitempty"`
	Managed     bool   `json:"managed,omitempty"`
	Mentionable bool   `json:"mentionable,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

// ── Payloads (json.Unmarshal Envelope.Data into the matching type) ──

// Guild is the full snapshot delivered on GUILD_CREATE / GUILD_UPDATE.
type Guild struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Icon        string    `json:"icon,omitempty"`
	OwnerID     string    `json:"owner_id"`
	MemberCount int       `json:"member_count"`
	Channels    []Channel `json:"channels,omitempty"`
	Roles       []Role    `json:"roles,omitempty"`
	Unavailable bool      `json:"unavailable,omitempty"`
}

// GuildDelete is delivered when the bot leaves / guild becomes unavailable.
type GuildDelete struct {
	ID          string `json:"id"`
	Unavailable bool   `json:"unavailable,omitempty"`
}

// ChannelEvent wraps CHANNEL_CREATE/UPDATE/DELETE.
type ChannelEvent struct {
	Channel
}

// RoleEvent wraps GUILD_ROLE_CREATE/UPDATE; for delete only RoleID is set.
type RoleEvent struct {
	GuildID string `json:"guild_id"`
	Role    Role   `json:"role"`
	RoleID  string `json:"role_id,omitempty"` // set on GUILD_ROLE_DELETE
}

// MemberAdd is delivered on GUILD_MEMBER_ADD.
type MemberAdd struct {
	GuildID     string `json:"guild_id"`
	Member      Member `json:"member"`
	MemberCount int    `json:"member_count,omitempty"`
}

// MemberRemove is delivered on GUILD_MEMBER_REMOVE.
type MemberRemove struct {
	GuildID     string `json:"guild_id"`
	User        User   `json:"user"`
	MemberCount int    `json:"member_count,omitempty"`
}

// MemberUpdate is delivered on GUILD_MEMBER_UPDATE.
type MemberUpdate struct {
	GuildID string `json:"guild_id"`
	Member  Member `json:"member"`
}

// Message is delivered on MESSAGE_CREATE (subset sufficient for XP + automod).
type Message struct {
	ID              string   `json:"id"`
	GuildID         string   `json:"guild_id"`
	ChannelID       string   `json:"channel_id"`
	Content         string   `json:"content"`
	Author          User     `json:"author"`
	Member          *Member  `json:"member,omitempty"`
	MentionEveryone bool     `json:"mention_everyone,omitempty"`
	MentionRoles    []string `json:"mention_roles,omitempty"`
	Mentions        []User   `json:"mentions,omitempty"`
	AttachmentCount int      `json:"attachment_count,omitempty"`
}
