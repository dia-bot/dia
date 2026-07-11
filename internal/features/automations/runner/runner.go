// Package runner is the shared "start a durable event/interaction flow" seam.
//
// It walks a Step program on the customcommands exec engine and, when a step
// parks the run (wait / wait_for / modal_open), persists it to automation_runs
// so the automations plugin's component / modal resume handlers and the wait
// scheduler carry it the rest of the way. A run that finishes without yielding
// leaves no row — only its side effects.
//
// The point is reuse: a managed feature (Welcome) can run the FULL step palette
// — branching, waits, modals, follow-up sends, durable button flows — without
// owning a scheduler or a resume router of its own. Every component this engine
// emits mints the "auto:" prefix, so its clicks land on the automations plugin's
// already-registered handlers, which resolve the parked run from its stored
// snapshot (they never need a row in `automations`).
package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/features/customcommands/exec"
	"github.com/dia-bot/dia/internal/features/giveaway"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
)

// RoutePrefix / NoopPrefix are the component custom_id namespaces the automations
// plugin listens on. A run started here mints these so its clicks resume through
// that plugin (distinct from custom commands' "ccmd:").
const (
	RoutePrefix = "auto:"
	NoopPrefix  = "auto:noop:"
)

// Runner starts durable runs on the shared automations machinery. Safe for
// concurrent use; construct once per feature.
type Runner struct {
	deps plugin.Deps
	eng  *exec.Engine
}

// New builds a runner whose engine routes components to the automations plugin
// and clamps wait windows to one minute (an event/click run has no interaction
// keeping it "live" longer).
func New(d plugin.Deps) *Runner {
	eng := exec.New(exec.Deps{
		Log:       d.Log,
		Discord:   &exec.DiscordAdapter{C: d.Discord},
		Store:     &exec.StoreAdapter{S: d.Store},
		Imaging:   &exec.ImagingAdapter{R: d.Imaging},
		HTTP:      &exec.HTTPAdapter{Client: &http.Client{Timeout: 10 * time.Second}},
		Giveaways: giveaway.NewManager(d),
	})
	eng.SetRouting(RoutePrefix, NoopPrefix)
	eng.SetMaxWaitFor(time.Minute)
	return &Runner{deps: d, eng: eng}
}

// Meta identifies a run for persistence and resume. AutomationID is a stable
// label (a real automation UUID, or a feature key like "welcome.join") used for
// KV scope and the Runs filter; it is not FK-bound. The Interaction* fields are
// set only for a click-started run, so its first reply / modal can answer the
// interaction.
type Meta struct {
	AutomationID       string
	Version            int
	GuildID            string
	InvokerID          string
	ActorID            string
	ChannelID          string
	TriggerKind        string
	InteractionID      string
	InteractionToken   string
	InteractionExpires *time.Time
}

// Result reports how a run ended so an interaction caller can finish
// acknowledging the click (e.g. a modal-first path's safety-net ack).
type Result struct {
	RunID   string
	Pause   *exec.PauseError // non-nil if the run parked (and was persisted)
	Outcome exec.Outcome
}

// Start walks def against scope. If a step parks the run, it is inserted into
// automation_runs ('waiting') with its step logs, so the automations resume
// handlers + scheduler continue it. A synchronous run persists nothing.
func (rn *Runner) Start(ctx context.Context, m Meta, def cc.Definition, scope *cc.Scope) Result {
	snapshot, _ := json.Marshal(def)
	run := &exec.RunState{
		ID:                 NewRunID(),
		CommandID:          m.AutomationID,
		CommandVersion:     m.Version,
		GuildID:            m.GuildID,
		InvokerID:          m.InvokerID,
		ActorID:            m.ActorID,
		ChannelID:          m.ChannelID,
		TriggerKind:        m.TriggerKind,
		InteractionID:      m.InteractionID,
		InteractionToken:   m.InteractionToken,
		InteractionExpires: m.InteractionExpires,
		DefinitionSnapshot: snapshot,
	}
	outcome, pause, err := rn.eng.Run(ctx, run, def, scope)
	if err != nil {
		rn.deps.Log.Warn("durable run error", "flow", m.AutomationID, "err", err)
	}
	if pause != nil {
		rn.persistParked(ctx, run, scope, pause)
	}
	return Result{RunID: run.ID, Pause: pause, Outcome: outcome}
}

// persistParked inserts the parked run row (so the resume router + scheduler can
// find it) plus its step logs. Best-effort: a write failure just means the wait
// can't resume, which is no worse than the old synchronous drop.
func (rn *Runner) persistParked(ctx context.Context, run *exec.RunState, scope *cc.Scope, pause *exec.PauseError) {
	gid, _ := event.ParseID(run.GuildID)
	uid, _ := event.ParseID(run.InvokerID)
	chID, _ := event.ParseID(run.ChannelID)
	scopeJSON, _ := scope.Marshal()
	cursorJSON, _ := json.Marshal(pause.Cursor)
	awUID, _ := event.ParseID(pause.AwaitingUserID)

	row := store.AutomationRun{
		ID:                 run.ID,
		AutomationID:       run.CommandID,
		AutomationVersion:  run.CommandVersion,
		GuildID:            gid,
		InvokerID:          uid,
		ChannelID:          chID,
		TriggerKind:        run.TriggerKind,
		InteractionID:      run.InteractionID,
		InteractionToken:   run.InteractionToken,
		InteractionExpires: run.InteractionExpires,
		Scope:              scopeJSON,
		Cursor:             cursorJSON,
		Status:             "waiting",
		ResumeAt:           pause.ResumeAt,
		AwaitingCustomID:   pause.AwaitingCustomID,
		AwaitingUserID:     awUID,
		AwaitingKind:       pause.AwaitingKind,
		DefinitionSnapshot: run.DefinitionSnapshot,
	}
	if err := rn.deps.Store.AutomationRuns.Insert(ctx, row); err != nil {
		rn.deps.Log.Warn("durable run persist", "flow", run.CommandID, "err", err)
		return
	}
	for _, l := range run.Logs() {
		if err := rn.deps.Store.AutomationRuns.AppendLog(ctx, store.AutomationRunLog{
			RunID:      run.ID,
			StepID:     l.StepID,
			StepKind:   l.StepKind,
			CursorPath: l.CursorPath,
			DurationMs: l.DurationMs,
			Status:     l.Status,
			Input:      l.Input,
			Output:     l.Output,
			Error:      l.Error,
		}); err != nil {
			rn.deps.Log.Debug("durable run log write", "err", err)
		}
	}
}

// NewRunID returns a sortable, opaque run id (mirrors the automations helper).
func NewRunID() string {
	ts := time.Now().UnixNano()
	return fmt.Sprintf("A%013xA%08x", ts/1_000_000, ts&0xFFFFFFFF)
}
