package socialnotifications

// FeatureKey is this feature's guild_feature_configs key.
const FeatureKey = "social"

// Config is the feature-level JSONB config. The per-account settings (channel,
// template, ping role, embed) live on the social_subscriptions rows; the
// feature config only carries the master toggle via the enabled flag.
type Config struct{}

// Default returns the default config.
func Default() Config { return Config{} }

// Plan limits: how many accounts a guild can follow.
const (
	FreeSubscriptionLimit    = 3
	PremiumSubscriptionLimit = 25
)
