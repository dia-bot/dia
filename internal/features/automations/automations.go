// Package automations is Dia's server-event automation engine: "when X happens
// on the server, run these steps". An automation pairs a trigger (a gateway
// event plus optional filters) with the very same JSONB step program custom
// commands use (internal/features/customcommands) — branching, loops, waits,
// image rendering, member/role/channel actions, KV, HTTP, the lot. The only
// differences from a custom command are the entry point (an event, not a slash
// invocation) and the run scope (`.Event.*` carries the trigger payload; there
// are no slash `.Input` options and no interaction to reply to).
//
// Presence is deliberately never a trigger — Dia does not track presence.
//
// This package holds the type-only model (Config, TriggerConfig) and the
// trigger catalogue. The execution glue (event subscription, filter matching,
// scope building, the exec engine, durable run persistence) lives in
// automations/runtime, mirroring the customcommands/{exec,runtime} split.
package automations

import "encoding/json"

// FeatureKey is the stable identifier used in guild_feature_configs and as the
// dashboard route segment.
const FeatureKey = "automations"

// Status is the publish state of an automation (parity with custom commands so
// durable runs can pin to an immutable version snapshot).
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

// Config is the feature-level per-guild toggle. Per-automation enabled lives on
// each automations row.
type Config struct {
	Enabled bool `json:"enabled"`
}

// Default returns sensible defaults.
func Default() Config { return Config{Enabled: true} }

// TriggerConfig is the per-automation filter set. Which fields apply depends on
// the trigger kind (see the Filters list on each TriggerKind); the dashboard
// only shows the relevant ones. Every id is a decimal snowflake string.
type TriggerConfig struct {
	// Channels, when non-empty, restricts the trigger to these channels
	// (message / reaction / voice events). IgnoreChannels excludes channels.
	Channels       []string `json:"channels,omitempty"`
	IgnoreChannels []string `json:"ignore_channels,omitempty"`
	// Roles, when non-empty, requires the acting member to hold one of these
	// roles. IgnoreRoles excludes members holding any of these roles.
	Roles       []string `json:"roles,omitempty"`
	IgnoreRoles []string `json:"ignore_roles,omitempty"`
	// IgnoreBots drops events whose actor is a bot (default off).
	IgnoreBots bool `json:"ignore_bots,omitempty"`
	// Keywords + MatchMode filter message content (message triggers).
	Keywords  []string `json:"keywords,omitempty"`
	MatchMode string   `json:"match_mode,omitempty"` // contains (default) | equals | word | regex
	// Emojis, when non-empty, restricts reaction triggers to these emojis
	// (unicode glyph, custom-emoji name, or custom-emoji id).
	Emojis []string `json:"emojis,omitempty"`
	// Role is the single watched role for role_added / role_removed triggers
	// (empty = any role change fires).
	Role string `json:"role,omitempty"`
	// Cooldown, when set, rate-limits the automation per scope so a burst of
	// events doesn't fan out a burst of runs.
	Cooldown *Cooldown `json:"cooldown,omitempty"`
}

// Cooldown gates how often an automation may fire, per scope.
type Cooldown struct {
	Scope   string `json:"scope"` // user | channel | guild
	Seconds int    `json:"seconds"`
}

// DecodeTriggerConfig parses a stored TriggerConfig JSONB blob (nil/empty → zero).
func DecodeTriggerConfig(raw json.RawMessage) TriggerConfig {
	var tc TriggerConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &tc)
	}
	return tc
}
