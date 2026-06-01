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

// CustomCommand is an admin-defined slash command.
type CustomCommand struct {
	ID          int64
	GuildID     int64
	Name        string
	Description string
	Response    json.RawMessage
	Enabled     bool
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
