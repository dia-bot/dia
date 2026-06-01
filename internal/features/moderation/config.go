package moderation

// FeatureKey is the stable identifier for the core moderation feature (matches
// guild_feature_configs.feature_key and the dashboard route).
const FeatureKey = "moderation"

// AutomodKey is the stable identifier for the automod sub-feature. It is stored
// as a separate row so it can be toggled independently of manual moderation.
const AutomodKey = "automod"

// Config is the moderation feature's per-guild configuration (stored as JSONB
// and edited from the dashboard).
type Config struct {
	// LogChannel receives an embed for every moderation action (manual + automod).
	LogChannel string `json:"log_channel"`
	// DMOnAction DMs the affected user a notice when an action is taken.
	DMOnAction bool `json:"dm_on_action"`
}

// Default returns sensible moderation defaults.
func Default() Config {
	return Config{
		DMOnAction: true,
	}
}

// AutomodConfig is the automod sub-feature's per-guild configuration.
type AutomodConfig struct {
	// BannedWords are matched case-insensitively as substrings of message content.
	BannedWords []string `json:"banned_words"`
	// BlockInvites deletes messages containing Discord invite links.
	BlockInvites bool `json:"block_invites"`
	// BlockLinks deletes messages containing any http(s) link.
	BlockLinks bool `json:"block_links"`
	// MaxMentions is the maximum allowed user mentions per message (0 = unlimited).
	MaxMentions int `json:"max_mentions"`
	// Action is what to do on a violation: "delete" | "warn" | "timeout".
	Action string `json:"action"`
	// TimeoutSeconds is the timeout duration applied when Action == "timeout".
	TimeoutSeconds int `json:"timeout_seconds"`
	// IgnoredChannels are channel IDs exempt from automod.
	IgnoredChannels []string `json:"ignored_channels"`
	// IgnoredRoles are role IDs whose holders are exempt from automod.
	IgnoredRoles []string `json:"ignored_roles"`
}

// DefaultAutomod returns sensible automod defaults. (Go does not allow two
// Default functions in one package, so the automod default is named explicitly.)
func DefaultAutomod() AutomodConfig {
	return AutomodConfig{
		BlockInvites:   true,
		MaxMentions:    5,
		Action:         "delete",
		TimeoutSeconds: 600,
	}
}
