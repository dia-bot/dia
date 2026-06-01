package roles

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
}

// Default returns sensible defaults (no roles configured; bots excluded).
func Default() Config {
	return Config{
		Roles:            []string{},
		IncludeBots:      false,
		WaitForScreening: false,
	}
}
