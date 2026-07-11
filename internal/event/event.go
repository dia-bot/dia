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
	TypeBanAdd            Type = "GUILD_BAN_ADD"
	TypeBanRemove         Type = "GUILD_BAN_REMOVE"
	TypeMessageCreate     Type = "MESSAGE_CREATE"
	TypeMessageUpdate     Type = "MESSAGE_UPDATE"
	TypeMessageDelete     Type = "MESSAGE_DELETE"
	TypeReactionAdd       Type = "MESSAGE_REACTION_ADD"
	TypeReactionRemove    Type = "MESSAGE_REACTION_REMOVE"
	TypeThreadCreate      Type = "THREAD_CREATE"
	TypeThreadDelete      Type = "THREAD_DELETE"
	TypeVoiceStateUpdate  Type = "VOICE_STATE_UPDATE"
	TypeInteractionCreate Type = "INTERACTION_CREATE"

	// TypeAutomodAction is NOT a gateway event: the worker publishes it on the
	// same stream when an automod rule fires, so the automations runtime can
	// trigger flows off moderation activity. It has no gateway/Elixir mapper.
	TypeAutomodAction Type = "AUTOMOD_ACTION"

	// These are worker-published like TypeAutomodAction (no gateway/Elixir
	// mapper): the verification, anti-raid and manual-moderation features emit
	// them so the automations runtime can trigger flows off safety activity.
	TypeVerificationPassed Type = "VERIFICATION_PASSED"
	TypeVerificationFailed Type = "VERIFICATION_FAILED"
	TypeRaidAlert          Type = "RAID_ALERT"
	TypeModerationAction   Type = "MODERATION_ACTION"

	// TypeLevelUp is NOT a gateway event: the worker publishes it on the same
	// stream when a member crosses into a new level, so the automations runtime
	// can trigger flows off leveling activity. Like TypeAutomodAction it has no
	// gateway/Elixir mapper.
	TypeLevelUp Type = "LEVEL_UP"

	// TypeReactionRolePick is NOT a gateway event: the worker publishes it on
	// the same stream when a member picks roles from a reaction-role menu, so
	// the automations runtime can trigger flows off role picks. Like
	// TypeAutomodAction it has no gateway/Elixir mapper.
	TypeReactionRolePick Type = "REACTION_ROLE_PICK"

	// These are worker-published like TypeAutomodAction (no gateway/Elixir
	// mapper): the ticketing feature emits them across a ticket's lifecycle so
	// the automations runtime can trigger flows off ticket activity.
	TypeTicketOpened  Type = "TICKET_OPENED"
	TypeTicketClaimed Type = "TICKET_CLAIMED"
	TypeTicketClosed  Type = "TICKET_CLOSED"
	TypeTicketRated   Type = "TICKET_RATED"

	// TypeGiveawayEnded is NOT a gateway event: the giveaway feature publishes it
	// on the same stream when a giveaway is drawn (natural end, manual end, or
	// reroll), so the automations runtime can trigger flows off giveaway results
	// (announce winners elsewhere, grant a role to each winner, log the draw).
	// Like TypeAutomodAction it has no gateway/Elixir mapper.
	TypeGiveawayEnded Type = "GIVEAWAY_ENDED"

	// TypeGiveawayEntered is NOT a gateway event: the giveaway feature publishes
	// it on the same stream whenever a member clicks a giveaway's Enter button and
	// the outcome is decided (entered, left, denied, or blocked), so the
	// automations runtime can trigger the built-in "on entry" flow (reward a role
	// on entry, log denials, DM a confirmation) branching on .Event.outcome. Like
	// TypeAutomodAction it has no gateway/Elixir mapper.
	TypeGiveawayEntered Type = "GIVEAWAY_ENTERED"
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
	User         User     `json:"user"`
	Nick         string   `json:"nick,omitempty"`
	Avatar       string   `json:"avatar,omitempty"`
	Roles        []string `json:"roles"`
	JoinedAt     string   `json:"joined_at,omitempty"`
	PremiumSince string   `json:"premium_since,omitempty"` // when the member started boosting; "" = not boosting
	Pending      bool     `json:"pending,omitempty"`
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

// MemberUpdate is delivered on GUILD_MEMBER_UPDATE. OldRoles carries the
// member's role set before the change when the gateway can recover it (it may
// be empty under the thin/NoOp cache), letting consumers diff added/removed
// roles and detect boosts (premium_since transitions).
type MemberUpdate struct {
	GuildID  string   `json:"guild_id"`
	Member   Member   `json:"member"`
	OldRoles []string `json:"old_roles,omitempty"`
}

// BanEvent is delivered on GUILD_BAN_ADD / GUILD_BAN_REMOVE.
type BanEvent struct {
	GuildID string `json:"guild_id"`
	User    User   `json:"user"`
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

// MessageUpdate is delivered on MESSAGE_UPDATE. Discord sends the full updated
// message under the thin cache, so it carries the same shape as Message.
type MessageUpdate struct {
	Message
}

// MessageDelete is delivered on MESSAGE_DELETE (only the ids survive).
type MessageDelete struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	GuildID   string `json:"guild_id,omitempty"`
}

// Emoji is the reaction emoji (custom emojis carry an id; unicode ones only a name).
type Emoji struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Animated bool   `json:"animated,omitempty"`
}

// Reaction is delivered on MESSAGE_REACTION_ADD / MESSAGE_REACTION_REMOVE.
type Reaction struct {
	UserID    string  `json:"user_id"`
	ChannelID string  `json:"channel_id"`
	MessageID string  `json:"message_id"`
	GuildID   string  `json:"guild_id,omitempty"`
	Emoji     Emoji   `json:"emoji"`
	Member    *Member `json:"member,omitempty"` // present on REACTION_ADD in guilds
}

// AutomodAction is published by the worker (not the gateway) when an automod
// rule fires, on subject discord.events.AUTOMOD_ACTION.<guild>. The automations
// runtime consumes it as the "automod_action" trigger, exposing these fields to
// flows as .Event.* alongside the offending member as .User / .Member.
type AutomodAction struct {
	GuildID     string   `json:"guild_id"`
	RuleID      string   `json:"rule_id"`
	RuleName    string   `json:"rule_name"`
	TriggerType string   `json:"trigger_type"`        // one of the automod Trigger* keys
	Reason      string   `json:"reason"`              // human description of the hit
	Actions     []string `json:"actions"`             // action types applied, in order
	Points      int      `json:"points"`              // points added by this hit
	TotalPoints int      `json:"total_points"`        // user's active infraction total after
	Escalated   string   `json:"escalated,omitempty"` // escalation action fired ("" = none)
	User        User     `json:"user"`                // the offending member
	Member      *Member  `json:"member,omitempty"`
	ChannelID   string   `json:"channel_id,omitempty"`
	MessageID   string   `json:"message_id,omitempty"`
	Content     string   `json:"content,omitempty"` // offending message content (truncated)
}

// LevelUp is published by the worker (not the gateway) when a member crosses
// into a new level, on subject discord.events.LEVEL_UP.<guild>. The automations
// runtime consumes it as the "level_up" trigger, exposing these fields to flows
// as .Event.* alongside the member as .User / .Member.
type LevelUp struct {
	GuildID   string  `json:"guild_id"`
	User      User    `json:"user"`
	Member    *Member `json:"member,omitempty"`
	ChannelID string  `json:"channel_id,omitempty"` // where the leveling message was posted
	Level     int     `json:"level"`                // the level just reached
	NewLevel  int     `json:"new_level"`            // same as Level (mirrors the runtime var name)
	XP        int64   `json:"xp"`                   // the member's total XP after the gain
	Rank      int     `json:"rank"`                 // leaderboard position (1-based; 0 if unknown)
}

// ReactionRolePick is published by the worker (not the gateway) when a member
// picks roles from a reaction-role menu (button or select), on subject
// discord.events.REACTION_ROLE_PICK.<guild>. The automations runtime consumes
// it as the "reaction_role_pick" trigger, exposing these fields to flows as
// .Event.* alongside the picking member as .User / .Member. A no-op pick (the
// member's roles already matched) still publishes, with empty Added/Removed.
type ReactionRolePick struct {
	GuildID   string   `json:"guild_id"`
	ChannelID string   `json:"channel_id,omitempty"` // where the menu message lives
	MessageID string   `json:"message_id,omitempty"` // the posted menu message
	MenuID    string   `json:"menu_id"`              // reaction_role_menus.id, decimal string
	MenuTitle string   `json:"menu_title,omitempty"`
	Mode      string   `json:"mode"`    // toggle | unique | verify
	Values    []string `json:"values"`  // role IDs the member chose
	Added     []string `json:"added"`   // role IDs actually granted
	Removed   []string `json:"removed"` // role IDs actually removed
	Member    Member   `json:"member"`  // the picking member
}

// VerificationPassed is published when a member clears verification (button or
// captcha). The automations runtime consumes it as the "verification_passed"
// trigger, with the member as .User / .Member.
type VerificationPassed struct {
	GuildID   string  `json:"guild_id"`
	User      User    `json:"user"`
	Member    *Member `json:"member,omitempty"`
	Mode      string  `json:"mode"`                 // "button" | "captcha"
	ChannelID string  `json:"channel_id,omitempty"` // the gate channel
}

// VerificationFailed is published when a member fails the captcha too many times
// or is removed for not verifying in time. Consumed as "verification_failed".
type VerificationFailed struct {
	GuildID string  `json:"guild_id"`
	User    User    `json:"user"`
	Member  *Member `json:"member,omitempty"`
	Reason  string  `json:"reason"` // "failed_captcha" | "timed_out"
	Kicked  bool    `json:"kicked"` // whether the member was removed
}

// RaidAlert is published when anti-raid mode is entered or lifted. Consumed as
// the "raid_alert" trigger; flows branch on .Event.active.
type RaidAlert struct {
	GuildID   string `json:"guild_id"`
	Active    bool   `json:"active"`    // true = raid mode entered, false = lifted
	Joins     int    `json:"joins"`     // joins counted in the window (on trip)
	Threshold int    `json:"threshold"` // the configured trip threshold
	Window    int    `json:"window"`    // the rolling window, seconds
	Action    string `json:"action"`    // action applied to joiners (kick/ban/timeout)
}

// ModerationAction is published when a moderator runs a manual mod command
// (/ban /kick /timeout /untimeout /unban /warn /note). Consumed as the
// "moderation_action" trigger; .User / .Member is the actioned member.
type ModerationAction struct {
	GuildID         string `json:"guild_id"`
	Action          string `json:"action"` // ban|kick|timeout|untimeout|unban|warn|note
	Reason          string `json:"reason"`
	User            User   `json:"user"`      // the actioned member
	Moderator       User   `json:"moderator"` // the moderator who ran the command
	CaseNumber      int    `json:"case_number"`
	DurationSeconds int    `json:"duration_seconds,omitempty"`
}

// TicketEvent is published by the worker (not the gateway) across a support
// ticket's lifecycle, on subject discord.events.TICKET_*.<guild>. The
// automations runtime consumes it as the "ticket_opened" / "ticket_claimed" /
// "ticket_closed" / "ticket_rated" triggers, exposing these fields to flows as
// .Event.* with the ticket OPENER as .User / .Member and the ticket channel as
// .Channel. Like TypeAutomodAction it has no gateway/Elixir mapper.
type TicketEvent struct {
	GuildID       string  `json:"guild_id"`
	TicketID      string  `json:"ticket_id"`
	Number        int     `json:"number"`
	PanelID       string  `json:"panel_id,omitempty"`
	CategoryID    string  `json:"category_id,omitempty"`
	CategoryLabel string  `json:"category_label,omitempty"`
	ChannelID     string  `json:"channel_id,omitempty"`
	Subject       string  `json:"subject,omitempty"`
	User          User    `json:"user"` // the ticket opener
	Member        *Member `json:"member,omitempty"`
	ActorID       string  `json:"actor_id,omitempty"`   // who claimed/closed (may differ from opener)
	ClaimedBy     string  `json:"claimed_by,omitempty"` // set on TICKET_CLAIMED
	ClosedBy      string  `json:"closed_by,omitempty"`  // set on TICKET_CLOSED
	Reason        string  `json:"reason,omitempty"`     // close reason
	Rating        int     `json:"rating,omitempty"`     // set on TICKET_RATED (1..5)
}

// GiveawayEnded is published by the giveaway feature (not the gateway) when a
// giveaway is drawn, on subject discord.events.GIVEAWAY_ENDED.<guild>. The
// automations runtime consumes it as the "giveaway_ended" trigger, exposing
// these fields to flows as .Event.* alongside the (first) winner as .User /
// .Member. A giveaway that ended with no eligible entrants still publishes, with
// empty Winners and WinnerCount 0.
type GiveawayEnded struct {
	GuildID     string   `json:"guild_id"`
	GiveawayID  string   `json:"giveaway_id"`
	ChannelID   string   `json:"channel_id,omitempty"` // where the giveaway lives
	MessageID   string   `json:"message_id,omitempty"`
	Prize       string   `json:"prize"`
	HostID      string   `json:"host_id,omitempty"`
	WinnerCount int      `json:"winner_count"`     // number of winners actually drawn
	WinnerIDs   []string `json:"winner_ids"`       // decimal snowflakes of the winners
	EntryCount  int      `json:"entry_count"`      // distinct entrants
	Rerolled    bool     `json:"rerolled"`         // true when this draw was a reroll
	User        User     `json:"user"`             // the first winner (zero value if none)
	Member      *Member  `json:"member,omitempty"` // first winner's member, when available
}

// GiveawayEntered is published by the giveaway feature (not the gateway) when a
// member clicks a giveaway's Enter button and the outcome is decided, on subject
// discord.events.GIVEAWAY_ENTERED.<guild>. The automations runtime consumes it
// as the "giveaway_entry" trigger, exposing these fields as .Event.* alongside
// the clicking member as .User / .Member. Outcome is one of "entered", "left",
// "denied" (failed a requirement) or "blocked" (a bot). Entries is the member's
// weighted ticket count on a successful entry (0 otherwise); Reason carries the
// denial explanation for outcome=="denied".
type GiveawayEntered struct {
	GuildID    string  `json:"guild_id"`
	GiveawayID string  `json:"giveaway_id"`
	ChannelID  string  `json:"channel_id,omitempty"`
	MessageID  string  `json:"message_id,omitempty"`
	Prize      string  `json:"prize"`
	HostID     string  `json:"host_id,omitempty"`
	Outcome    string  `json:"outcome"`          // entered | left | denied | blocked
	Entries    int     `json:"entries"`          // weighted tickets on a successful entry
	Reason     string  `json:"reason,omitempty"` // denial reason (outcome=="denied")
	EntryCount int     `json:"entry_count"`      // distinct entrants after this click
	User       User    `json:"user"`             // the member who clicked Enter
	Member     *Member `json:"member,omitempty"`
}

// VoiceState is delivered on VOICE_STATE_UPDATE. ChannelID == "" means the
// member disconnected. The gateway can't recover the previous channel under the
// thin cache; consumers that need join/leave/move transitions diff against
// their own last-known state.
type VoiceState struct {
	GuildID   string  `json:"guild_id"`
	ChannelID string  `json:"channel_id,omitempty"`
	UserID    string  `json:"user_id"`
	Member    *Member `json:"member,omitempty"`
	SessionID string  `json:"session_id,omitempty"`
	Deaf      bool    `json:"deaf,omitempty"`
	Mute      bool    `json:"mute,omitempty"`
	SelfDeaf  bool    `json:"self_deaf,omitempty"`
	SelfMute  bool    `json:"self_mute,omitempty"`
	SelfVideo bool    `json:"self_video,omitempty"`
	Stream    bool    `json:"self_stream,omitempty"`
}
