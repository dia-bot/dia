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

// ReactionRoleMenu is a self-assign role menu (buttons/select).
type ReactionRoleMenu struct {
	ID        int64
	GuildID   int64
	ChannelID int64
	MessageID int64
	Title     string
	Mode      string
	Options   json.RawMessage
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CustomCommand is one admin-defined programmable slash command. Definition
// holds the full JSONB program (slash params + Step[] tree); see
// internal/features/customcommands/config.go for the typed shape.
type CustomCommand struct {
	ID            int64
	GuildID       int64
	Name          string
	Description   string
	Enabled       bool
	Status        string // draft | published | archived
	Version       int
	RequiresDefer bool
	Definition    json.RawMessage
	CreatedBy     int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// CustomCommandVersion is an immutable snapshot of the Definition at publish.
type CustomCommandVersion struct {
	CommandID   int64
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
	CommandID          int64
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
	CommandID int64
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

// AuditEntry is a dashboard audit-log row.
type AuditEntry struct {
	ID        int64
	GuildID   int64
	UserID    int64
	Action    string
	Detail    json.RawMessage
	CreatedAt time.Time
}
