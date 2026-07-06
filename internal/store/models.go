package store

import (
	"encoding/json"
	"time"
)

// Guild is a row of the guilds table.
type Guild struct {
	ID          int64
	Name        string
	Icon        string
	OwnerID     int64
	MemberCount int
	JoinedAt    time.Time
	LeftAt      *time.Time
	UpdatedAt   time.Time
}

// FeatureConfig is a per-guild feature configuration (config is opaque JSONB).
type FeatureConfig struct {
	GuildID   int64
	Feature   string
	Enabled   bool
	Config    json.RawMessage
	UpdatedAt time.Time
}

// LevelUser is a member's leveling state.
type LevelUser struct {
	GuildID       int64
	UserID        int64
	XP            int64
	Level         int
	Messages      int64
	LastMessageAt *time.Time
}

// LevelReward maps a level to a role grant.
type LevelReward struct {
	GuildID        int64
	Level          int
	RoleID         int64
	RemovePrevious bool
}

// ModCase is a moderation action record.
type ModCase struct {
	ID              int64
	GuildID         int64
	CaseNumber      int
	UserID          int64
	ModeratorID     int64
	Action          string
	Reason          string
	DurationSeconds int
	CreatedAt       time.Time
	ExpiresAt       *time.Time
	Active          bool
}

// AutomodInfraction is one automod rule hit that awarded escalation points.
type AutomodInfraction struct {
	ID          int64
	GuildID     int64
	UserID      int64
	RuleID      string
	RuleName    string
	TriggerType string
	Points      int
	Reason      string
	ChannelID   *int64
	CreatedAt   time.Time
	ExpiresAt   *time.Time
}

// Offender is an aggregate row for the automod leaderboard: a user with their
// total active points and hit count over a window.
type Offender struct {
	UserID      int64
	TotalPoints int
	Hits        int
	LastAt      time.Time
}

// ReactionRoleMenu is a self-assign role menu (buttons/select). Tail is the
// canvas-owned follow-up flow (a []cc.Step JSONB array) run after a member
// picks roles; it is saved via SetTail only, so the dashboard's menu upsert
// (Create/Update) can never clobber it.
type ReactionRoleMenu struct {
	ID        int64
	GuildID   int64
	ChannelID int64
	MessageID int64
	Title     string
	Mode      string
	Options   json.RawMessage
	Tail      json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CustomCommand is one admin-defined programmable slash command. Definition
// holds the full JSONB program (slash params + Step[] tree); see
// internal/features/customcommands/config.go for the typed shape.
type CustomCommand struct {
	ID            string // UUID
	GuildID       int64
	Name          string
	Description   string
	Enabled       bool
	Status        string // draft | published | archived
	Version       int
	RequiresDefer bool
	Definition    json.RawMessage
	GroupID       *string // UUID of the group; nil = ungrouped
	CreatedBy     int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CommandGroup is a dashboard organizational folder for custom commands.
type CommandGroup struct {
	ID        string // UUID
	GuildID   int64
	Name      string
	Position  int
	CreatedAt time.Time
}

// CustomCommandVersion is an immutable snapshot of the Definition at publish.
type CustomCommandVersion struct {
	CommandID   string // UUID
	Version     int
	Definition  json.RawMessage
	PublishedBy int64
	PublishedAt time.Time
}

// CommandRun is a persisted in-flight (or completed) command execution. Runs
// exist only for durable steps (wait, wait_for, scheduled, parallel); pure
// synchronous runs leave only their RunLog rows.
type CommandRun struct {
	ID                 string
	CommandID          string // UUID
	CommandVersion     int
	GuildID            int64
	InvokerID          int64
	ChannelID          int64
	TriggerKind        string
	InteractionID      string
	InteractionToken   string
	InteractionExpires *time.Time
	Scope              json.RawMessage
	Cursor             json.RawMessage
	Status             string
	ResumeAt           *time.Time
	AwaitingCustomID   string
	AwaitingUserID     int64
	AwaitingKind       string
	DefinitionSnapshot json.RawMessage
	StartedAt          time.Time
	CompletedAt        *time.Time
	Error              string
}

// CommandRunLog is one structured log row per executed step.
type CommandRunLog struct {
	ID         int64
	RunID      string
	StepID     string
	StepKind   string
	CursorPath string
	StartedAt  time.Time
	DurationMs int
	Status     string
	Input      json.RawMessage
	Output     json.RawMessage
	Error      string
}

// FeatureKVEntry is one durable key/value pair backing kv_get / kv_set steps.
type FeatureKVEntry struct {
	GuildID   int64
	CommandID string // UUID; "" or zero-UUID = guild-shared
	Scope     string
	OwnerID   int64
	Key       string
	Value     json.RawMessage
	ExpiresAt *time.Time
	UpdatedAt time.Time
}

// CommandImageTemplate is one stored Card Studio layout referenced by
// image_render steps.
type CommandImageTemplate struct {
	ID          int64
	GuildID     int64
	Name        string
	Description string
	Layout      json.RawMessage
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ── Automations (server-event step programs) ─────────────────

// Automation is one admin-defined server-event automation: a trigger (a gateway
// event + filters) paired with the same JSONB Step[] program custom commands
// use. EventType is the resolved gateway event the TriggerType maps to (stored
// so per-event dispatch is an indexed lookup).
type Automation struct {
	ID            string // UUID
	GuildID       int64
	Name          string
	Description   string
	Enabled       bool
	Status        string // draft | published | archived
	Version       int
	TriggerType   string // catalogue key (member_join, message_create, ...)
	EventType     string // gateway event (GUILD_MEMBER_ADD, MESSAGE_CREATE, ...)
	TriggerConfig json.RawMessage
	Definition    json.RawMessage
	CreatedBy     int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AutomationVersion is an immutable snapshot of an automation at publish.
type AutomationVersion struct {
	AutomationID  string
	Version       int
	Definition    json.RawMessage
	TriggerType   string
	TriggerConfig json.RawMessage
	PublishedBy   int64
	PublishedAt   time.Time
}

// AutomationRun is a persisted in-flight (or completed) automation execution.
// Like CommandRun, it exists only for durable steps (wait, wait_for, parallel).
type AutomationRun struct {
	ID                 string
	AutomationID       string // UUID
	AutomationVersion  int
	GuildID            int64
	InvokerID          int64 // the event actor
	ChannelID          int64
	TriggerKind        string
	InteractionID      string
	InteractionToken   string
	InteractionExpires *time.Time
	Scope              json.RawMessage
	Cursor             json.RawMessage
	Status             string
	ResumeAt           *time.Time
	AwaitingCustomID   string
	AwaitingUserID     int64
	AwaitingKind       string
	DefinitionSnapshot json.RawMessage
	StartedAt          time.Time
	CompletedAt        *time.Time
	Error              string
}

// AutomationRunLog is one structured log row per executed automation step.
type AutomationRunLog struct {
	ID         int64
	RunID      string
	StepID     string
	StepKind   string
	CursorPath string
	StartedAt  time.Time
	DurationMs int
	Status     string
	Input      json.RawMessage
	Output     json.RawMessage
	Error      string
}

// AuditEntry is a dashboard audit-log row.
type AuditEntry struct {
	ID        int64
	GuildID   int64
	UserID    int64
	Action    string
	Detail    json.RawMessage
	CreatedAt time.Time
}
