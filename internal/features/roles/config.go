package roles

import cc "github.com/dia-bot/dia/internal/features/customcommands"

// FeatureKey is the stable identifier for the autorole feature (matches
// guild_feature_configs.feature_key and the dashboard route).
//
// Reaction-role menus are NOT stored in feature config — they live in the
// reaction_role_menus table via store.ReactionRoles. This config covers only
// autorole (roles automatically granted on join).
const FeatureKey = "autorole"

// Config is the autorole feature's per-guild configuration (stored as JSONB and
// edited from the dashboard).
type Config struct {
	// Roles is the set of role IDs (snowflake strings) granted on join.
	Roles []string `json:"roles"`
	// IncludeBots grants the roles to bot accounts too (default: skip bots).
	IncludeBots bool `json:"include_bots"`
	// WaitForScreening defers granting until a member passes membership
	// screening (the join arrives with member.Pending = true, and the roles are
	// applied on the later MemberUpdate once they're no longer pending).
	WaitForScreening bool `json:"wait_for_screening"`
	// Tail is the optional post-grant follow-up flow: the editable steps the
	// admin wired after the "grant roles" spine on the autorole.join automation
	// canvas. It runs as a durable automation run once the configured roles have
	// been granted (see roles.Plugin.handleMemberAdd). Owned by the automation
	// canvas (saved via /autorole/actions), not the auto-roles settings page.
	Tail []cc.Step `json:"tail,omitempty"`
}

// Default returns sensible defaults (no roles configured; bots excluded).
func Default() Config {
	return Config{
		Roles:            []string{},
		IncludeBots:      false,
		WaitForScreening: false,
	}
}
