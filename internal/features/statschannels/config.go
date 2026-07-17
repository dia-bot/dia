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

// Milestone kinds.
const (
	// MilestoneEvery fires each time the member count crosses a multiple of
	// Value (every 100 members, every 500, ...).
	MilestoneEvery = "every"
	// MilestoneAt fires once, when the member count first reaches Value
	// (the road-to-10k target).
	MilestoneAt = "at"
)

// Milestone is one configured member-count milestone. Each crossing publishes
// a MEMBER_MILESTONE event carrying this milestone's ID, so automations can
// scope to exactly this milestone via TriggerConfig.Milestones.
type Milestone struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"` // every | at
	Value   int    `json:"value"`
	Enabled bool   `json:"enabled"`
}

// Fires reports whether a join that moved the member count to now (from
// now-1) crosses this milestone, and the milestone value reached.
func (m Milestone) Fires(now int) (reached int, ok bool) {
	if !m.Enabled || m.Value <= 0 || now <= 0 {
		return 0, false
	}
	switch m.Kind {
	case MilestoneAt:
		if now == m.Value {
			return m.Value, true
		}
	default: // every
		if (now / m.Value) > ((now - 1) / m.Value) {
			return (now / m.Value) * m.Value, true
		}
	}
	return 0, false
}

// Config is the stats feature JSONB.
type Config struct {
	Counters []Counter `json:"counters,omitempty"`
	// Milestones publish MEMBER_MILESTONE events as the member count grows.
	// Automations react via the member_milestone trigger, scoped per milestone.
	Milestones []Milestone `json:"milestones,omitempty"`
	// MilestoneStep is the legacy single recurring milestone; decoded so
	// configs saved before Milestones existed keep firing (see Normalize).
	MilestoneStep int `json:"milestone_step,omitempty"`
	// Tail is the editable follow-up flow of the built-in "Member milestone"
	// automation, run as a durable automation run on every MEMBER_MILESTONE.
	// Owned by the Automations canvas: settings saves pass through
	// MergeStoredTail and can't clobber a flow wired on the canvas.
	Tail []cc.Step `json:"tail,omitempty"`
}

// Normalize folds the legacy MilestoneStep field into the Milestones list so
// the rest of the code (and the dashboard) only ever sees the list form.
func (c Config) Normalize() Config {
	if c.MilestoneStep > 0 && len(c.Milestones) == 0 {
		c.Milestones = []Milestone{{ID: "legacy-step", Kind: MilestoneEvery, Value: c.MilestoneStep, Enabled: true}}
	}
	c.MilestoneStep = 0
	return c
}

// Default returns the default config.
func Default() Config {
	return Config{Milestones: []Milestone{{ID: "every-100", Kind: MilestoneEvery, Value: 100, Enabled: true}}}
}

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
