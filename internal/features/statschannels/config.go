package statschannels

import (
	"encoding/json"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// FeatureKey is this feature's guild_feature_configs key.
const FeatureKey = "stats"

// Counter is one stats channel: a (usually locked voice) channel whose name is
// re-rendered from a Go template as the server changes. Values in scope:
// {{ .Members }}, {{ .Channels }}, {{ .Roles }}, {{ .Milestone }} and
// {{ .Guild.Name }}, mirrored by the dashboard's variable chips
// (web/src/lib/stats.ts).
type Counter struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	Template  string `json:"template"`
	Enabled   bool   `json:"enabled"`
}

// Config is the stats feature JSONB.
type Config struct {
	Counters []Counter `json:"counters,omitempty"`
	// MilestoneStep publishes a MEMBER_MILESTONE event every time the member
	// count crosses a multiple of this value (0 = off). Automations react via
	// the member_milestone trigger.
	MilestoneStep int `json:"milestone_step,omitempty"`
	// Tail is the editable follow-up flow of the built-in "Member milestone"
	// automation, run as a durable automation run on every MEMBER_MILESTONE.
	// Owned by the Automations canvas: settings saves pass through
	// MergeStoredTail and can't clobber a flow wired on the canvas.
	Tail []cc.Step `json:"tail,omitempty"`
}

// Default returns the default config.
func Default() Config { return Config{MilestoneStep: 100} }

// MergeStoredTail returns the incoming stats config JSON with its canvas-owned
// Tail replaced by the stored one. Mirrors giveaway.MergeStoredTail; on any
// decode/encode error the incoming bytes are returned unchanged.
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
