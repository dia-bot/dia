// Package logging relays server events (message edits/deletes, member joins and
// leaves, bans, role changes, and moderation actions) to a log channel as
// embeds, giving moderators an audit trail. Deleted/edited message content is
// recovered from a short-lived message cache the feature keeps in Redis.
package logging

// FeatureKey is the stable identifier for the logging feature (matches
// guild_feature_configs.feature_key and the dashboard route).
const FeatureKey = "logging"

// Config is the logging feature's per-guild configuration (JSONB). Channel is
// the default destination; per-category overrides let a server split, say,
// message logs from member logs into different channels ("" => use Channel).
type Config struct {
	// Channel is the default log channel for any enabled category without an
	// override.
	Channel string `json:"channel"`

	// Categories (each can be toggled independently).
	MessageDelete bool `json:"message_delete"`
	MessageEdit   bool `json:"message_edit"`
	MemberJoin    bool `json:"member_join"`
	MemberLeave   bool `json:"member_leave"`
	MemberBan     bool `json:"member_ban"`
	MemberUnban   bool `json:"member_unban"`
	RoleChanges   bool `json:"role_changes"`
	// ModActions relays moderation cases (manual + automod) to the log.
	ModActions bool `json:"mod_actions"`

	// Optional per-category channel overrides ("" => Channel).
	MessageChannel string `json:"message_channel,omitempty"`
	MemberChannel  string `json:"member_channel,omitempty"`

	// IgnoredChannels are channels whose messages are never message-logged
	// (e.g. spam or bot channels).
	IgnoredChannels []string `json:"ignored_channels,omitempty"`
}

// Default returns sensible logging defaults (the common categories on).
func Default() Config {
	return Config{
		MessageDelete: true,
		MessageEdit:   true,
		MemberJoin:    true,
		MemberLeave:   true,
		MemberBan:     true,
		MemberUnban:   true,
		ModActions:    true,
	}
}
