package schedmessages

import (
	"encoding/json"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// FeatureKey is this feature's guild_feature_configs key.
const FeatureKey = "scheduler"

// Config is the feature-level JSONB. Schedules live on scheduled_messages
// rows; the config carries the master toggle plus the built-in automation's
// follow-up flow.
type Config struct {
	// Tail is the editable follow-up flow of the built-in "Scheduled message
	// sent" automation, run as a durable automation run after every post.
	// Owned by the Automations canvas: settings saves pass through
	// MergeStoredTail and can't clobber a flow wired on the canvas.
	Tail []cc.Step `json:"tail,omitempty"`
}

// Default returns the default config.
func Default() Config { return Config{} }

// MergeStoredTail returns the incoming scheduler config JSON with its
// canvas-owned Tail replaced by the stored one. Mirrors
// giveaway.MergeStoredTail; on any decode/encode error the incoming bytes are
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
