// Package customcommands is Dia's programmable per-guild slash command engine.
//
// A custom command is a JSONB program: slash params + declared variables +
// a nested tree of typed Steps. Control-flow steps (if / switch / loop /
// parallel / wait / wait_for) embed their child Step arrays inline — no DAG,
// no edges, no orphans. The runtime is a tree-walking interpreter; durable
// steps (wait, wait_for, scheduled triggers) persist a resumable cursor in
// command_runs so they survive worker restarts.
//
// Expressions and user-facing strings both run through internal/templating
// (Go text/template + the dia funcmap), sandboxed: 500 ms wall-clock budget,
// 4 KiB output cap, no I/O, no reflection, no shelling out. One mental model
// for the admin, two surfaces: a condition is a template that produces
// "true" / non-empty for truthy; a value is a template that produces a string
// (parsed into number / bool / snowflake by the step that reads it).
package customcommands

import (
	"encoding/json"
	"time"
)

// FeatureKey is the stable identifier used in guild_feature_configs and as the
// dashboard route segment.
const FeatureKey = "customcommands"

// Status is the publish state of a command.
type Status string

const (
	StatusDraft     Status = "draft"
	StatusPublished Status = "published"
	StatusArchived  Status = "archived"
)

// Config is the feature-level toggle (per-guild). Per-command enabled lives on
// each custom_commands row.
type Config struct {
	Enabled bool `json:"enabled"`
}

// Default returns sensible defaults.
func Default() Config { return Config{Enabled: true} }

// ── The command program ──────────────────────────────────────────────────────

// Definition is the publishable command program. It is what the dashboard
// edits, what the validator type-checks, and what the executor walks.
type Definition struct {
	Options     []CommandOption `json:"options,omitempty"`
	Permissions string          `json:"permissions,omitempty"` // Discord permission bitfield string
	Cooldown    *Cooldown       `json:"cooldown,omitempty"`
	Variables   []VarDecl       `json:"variables,omitempty"`
	Triggers    []Trigger       `json:"triggers,omitempty"`
	Steps       []Step          `json:"steps,omitempty"`
	// Scratch holds disconnected step chains: islands on the canvas the
	// runtime never executes. Detaching a line parks the downstream chain
	// here (steps survive); reconnecting a chain moves it back into Steps.
	Scratch [][]Step        `json:"scratch,omitempty"`
	UIHints json.RawMessage `json:"ui_hints,omitempty"` // graph layout / flow editor state; never read by runtime
}

// Cooldown gates re-invocation by scope.
type Cooldown struct {
	Scope   string `json:"scope"` // user | channel | guild
	Seconds int    `json:"seconds"`
}

// CommandOption is one typed slash command parameter.
type CommandOption struct {
	Kind        string   `json:"kind"` // string | int | bool | user | role | channel | mentionable | attachment | number
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Required    bool     `json:"required,omitempty"`
	Choices     []Choice `json:"choices,omitempty"`
	// Numeric (int/number) bounds.
	MinValue *float64 `json:"min_value,omitempty"`
	MaxValue *float64 `json:"max_value,omitempty"`
	// String length bounds.
	MinLength *int `json:"min_length,omitempty"`
	MaxLength *int `json:"max_length,omitempty"`
	// Autocomplete — runtime answers suggestion requests; mutually exclusive
	// with Choices. Only valid on string / int / number.
	Autocomplete bool `json:"autocomplete,omitempty"`
	// ChannelTypes filters which channel kinds Discord lets the user pick.
	// Discord channel type IDs (0 text, 2 voice, 4 category, 5 announce, 10
	// announce-thread, 11 public-thread, 12 private-thread, 13 stage, 15 forum,
	// 16 media). Only valid on `channel` options.
	ChannelTypes []int `json:"channel_types,omitempty"`
}

// Choice is a fixed choice attached to a string/int option.
type Choice struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// VarDecl declares a runtime variable that step expressions can read & write.
type VarDecl struct {
	Name    string          `json:"name"`
	Type    string          `json:"type"` // string | int | float | bool | list | object
	Default json.RawMessage `json:"default,omitempty"`
	Scope   string          `json:"scope"` // run (ephemeral) — member/guild use kv_* steps instead
}

// Trigger lets a command fire from sources other than its own slash surface.
type Trigger struct {
	Kind     string `json:"kind"` // slash | component | modal | event | schedule
	Prefix   string `json:"prefix,omitempty"`
	Event    string `json:"event,omitempty"`
	Cron     string `json:"cron,omitempty"`
	Timezone string `json:"timezone,omitempty"`
	Channel  string `json:"channel,omitempty"` // default channel for scheduled / event triggers with no reply context
}

// ── Steps (the program) ──────────────────────────────────────────────────────

// Step is a discriminated union keyed on Kind; the per-kind Spec is decoded
// lazily by the step handler. Control-flow embeds children inline.
type Step struct {
	ID           string          `json:"id"`   // ULID — stable across edits so logs & cursors stay coherent
	Kind         string          `json:"kind"` // see kinds.go
	Spec         json.RawMessage `json:"spec,omitempty"`
	Then         []Step          `json:"then,omitempty"`           // if / loop body
	Else         []Step          `json:"else,omitempty"`           // if else branch
	Cases        []SwitchCase    `json:"cases,omitempty"`          // switch
	Default      []Step          `json:"default,omitempty"`        // switch default
	OnError      []Step          `json:"on_error,omitempty"`       // default recovery handler (no kind dispatch)
	OnErrorCases []ErrorCase     `json:"on_error_cases,omitempty"` // typed-error dispatch: first matching `When` runs
}

// SwitchCase is one arm of a `switch` step.
type SwitchCase struct {
	When Expr   `json:"when"`
	Do   []Step `json:"do"`
}

// ErrorCase is one arm of a typed on_error dispatch. When the wrapped
// step fails, the runtime walks the OnErrorCases in order and runs the
// first one whose `When` patterns match the failure's kind. Patterns are
// segment-globs: `discord.*`, `*.timeout`, `discord.permission_denied`,
// or `*` (catch-all). Empty / unmatched falls back to the default OnError.
type ErrorCase struct {
	When []string `json:"when"` // one or more kind patterns
	Do   []Step   `json:"do"`
}

// ErrorInfo is the public shape exposed to expressions inside an on_error
// branch as `.Error.*` — Kind ("discord.permission_denied"), the raw
// Message, and which step failed. Lives in the cc package (not exec) so
// Scope can carry it without a circular import.
type ErrorInfo struct {
	Kind      string `json:"kind"`
	Message   string `json:"message"`
	Step      string `json:"step"`
	StepID    string `json:"step_id"`
	Retryable bool   `json:"retryable"`
}

// Expr is either a templated source ("cel" — using Go templates as the engine,
// see expr.go) or a JSON literal already parsed into Value.
type Expr struct {
	Lang  string          `json:"lang,omitempty"`  // "tmpl" | "literal" (default: tmpl when Src set, literal when Value set)
	Src   string          `json:"src,omitempty"`   // template source for lang=tmpl
	Value json.RawMessage `json:"value,omitempty"` // already-typed JSON for lang=literal
}

// ── Runs (durable execution state) ───────────────────────────────────────────

// Run is one in-progress or completed command execution. Persisted only for
// durable runs (wait / wait_for / parallel / scheduled).
type Run struct {
	ID                 string          `json:"id"` // ULID
	CommandID          int64           `json:"command_id"`
	CommandVersion     int             `json:"command_version"`
	GuildID            string          `json:"guild_id"` // decimal snowflake
	InvokerID          string          `json:"invoker_id"`
	ChannelID          string          `json:"channel_id"`
	TriggerKind        string          `json:"trigger_kind"`
	InteractionID      string          `json:"interaction_id,omitempty"`
	InteractionToken   string          `json:"interaction_token,omitempty"`
	InteractionExpires *time.Time      `json:"interaction_expires,omitempty"`
	Scope              json.RawMessage `json:"scope"`  // marshalled ScopeData
	Cursor             []CursorFrame   `json:"cursor"` // path in the tree
	Status             string          `json:"status"`
	ResumeAt           *time.Time      `json:"resume_at,omitempty"`
	AwaitingCustomID   string          `json:"awaiting_custom_id,omitempty"`
	AwaitingUserID     string          `json:"awaiting_user_id,omitempty"`
	AwaitingKind       string          `json:"awaiting_kind,omitempty"`
	DefinitionSnapshot json.RawMessage `json:"definition_snapshot"`
	StartedAt          time.Time       `json:"started_at"`
	CompletedAt        *time.Time      `json:"completed_at,omitempty"`
	Error              string          `json:"error,omitempty"`
}

// CursorFrame is one level of the path through the nested Step tree.
// Branch names: "root" / "then" / "else" / "case" / "default" / "body" / "on_error".
type CursorFrame struct {
	Branch string `json:"branch"`
	Index  int    `json:"index"`           // position within the branch
	Case   int    `json:"case,omitempty"`  // arm index for switch.Cases[Case].Do
	Iter   int    `json:"iter,omitempty"`  // loop iteration counter
	Total  int    `json:"total,omitempty"` // loop iteration count (cached)
}

// RunLog is one structured log row per executed step.
type RunLog struct {
	ID         int64           `json:"id"`
	RunID      string          `json:"run_id"`
	StepID     string          `json:"step_id"`
	StepKind   string          `json:"step_kind"`
	CursorPath string          `json:"cursor_path"`
	StartedAt  time.Time       `json:"started_at"`
	DurationMs int             `json:"duration_ms"`
	Status     string          `json:"status"` // ok | error | skipped
	Input      json.RawMessage `json:"input,omitempty"`
	Output     json.RawMessage `json:"output,omitempty"`
	Error      string          `json:"error,omitempty"`
}

// ── Image templates (reusable Card Studio layouts) ───────────────────────────

// ImageTemplate is one stored layout an image_render step can reference.
type ImageTemplate struct {
	ID          int64           `json:"id"`
	GuildID     string          `json:"guild_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Layout      json.RawMessage `json:"layout"` // layout.Layout
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
}

// ── KV (durable per-guild / per-member store for kv_get / kv_set) ────────────

// KVEntry is one key/value pair in the durable store.
type KVEntry struct {
	GuildID   string          `json:"guild_id"`
	CommandID int64           `json:"command_id"`
	Scope     string          `json:"scope"` // guild | member
	OwnerID   string          `json:"owner_id"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	UpdatedAt time.Time       `json:"updated_at"`
}
