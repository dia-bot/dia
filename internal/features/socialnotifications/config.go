package socialnotifications

import (
	"encoding/json"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// FeatureKey is this feature's guild_feature_configs key.
const FeatureKey = "social"

// Config is the feature-level JSONB config. The per-account settings (channel,
// message, ping role, embed) live on the social_subscriptions rows; the
// feature config carries the master toggle (enabled flag) and the built-in
// automation's follow-up flow.
type Config struct {
	// Tail is the editable follow-up flow of the built-in "Announce social
	// updates" automation, run as a durable automation run on every
	// SOCIAL_UPDATE. Owned by the Automations canvas: settings saves pass
	// through MergeStoredTail and can't clobber a flow wired on the canvas.
	Tail []cc.Step `json:"tail,omitempty"`
}

// Default returns the default config.
func Default() Config { return Config{} }

// MergeStoredTail returns the incoming social config JSON with its
// canvas-owned Tail replaced by the stored one, so a save from the Social
// settings page (which doesn't know about the built-in automation's follow-up
// flow) can't wipe a flow wired on the Automations canvas. Mirrors
// giveaway.MergeStoredTail. On any decode/encode error the incoming bytes are
// returned unchanged.
func MergeStoredTail(incoming, stored []byte) []byte {
	var in, st Config
	if err := json.Unmarshal(incoming, &in); err != nil {
		return incoming
	}
	if err := json.Unmarshal(stored, &st); err != nil {
		return incoming
	}
	in.Tail = st.Tail
	out, err := json.Marshal(in)
	if err != nil {
		return incoming
	}
	return out
}

// Plan limits: how many accounts a guild can follow.
const (
	FreeSubscriptionLimit    = 3
	PremiumSubscriptionLimit = 25
)
