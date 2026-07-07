// Package exec is the runtime engine that walks a custom command's Step tree.
//
// The Engine is dependency-injected (Discord client, store, imaging renderer,
// guild state) and owns the handler registry — one handler per Step.Kind. It
// is reused across many runs and is safe for concurrent calls. A single
// invocation is a Run() call that builds a Scope, walks the Step[] tree, and
// emits a structured log row per step.
//
// Two yield sentinels drive control flow:
//
//   - errExit terminates the run successfully (the `exit` step / wait_for
//     timeout drain after on_timeout / loop break).
//   - PauseError suspends the run for durable resume (wait / wait_for). The
//     top-level Run() catches it and persists scope+cursor; the scheduler /
//     interaction router calls Resume() later with the matching payload.
package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// Deps are the runtime services every step handler may need. The Engine owns
// one of these; handlers read from it without mutating.
type Deps struct {
	Log       *slog.Logger
	Discord   DiscordClient
	Store     StoreClient
	Imaging   ImagingClient
	HTTP      HTTPClient
	Giveaways GiveawayStarter // optional; nil disables the giveaway_start step
}

// Engine is the per-worker command runtime. Construct once via New(); call
// Run() per invocation.
type Engine struct {
	deps     Deps
	handlers map[string]Handler
	// routePrefix / noopPrefix namespace the component custom_ids this engine
	// mints so a feature's clicks route back to its own resume handler. Custom
	// commands use "ccmd:"; automations use "auto:" (set via SetRouting).
	routePrefix string
	noopPrefix  string
	// maxWaitFor caps wait_for / modal listening windows. Custom commands allow
	// up to the interaction-token lifetime (~10 min); automations clamp tighter
	// (1 min) since there's no interaction keeping the run "live" (SetMaxWaitFor).
	maxWaitFor time.Duration
}

// New builds an engine and registers the standard step handlers.
func New(d Deps) *Engine {
	e := &Engine{
		deps:        d,
		handlers:    map[string]Handler{},
		routePrefix: "ccmd:",
		noopPrefix:  cc.NoopCustomIDPrefix,
		maxWaitFor:  10 * time.Minute,
	}
	registerStdHandlers(e)
	return e
}

// SetMaxWaitFor overrides the cap on wait_for / modal listening windows (e.g.
// 1 minute for automations). A non-positive value is ignored.
func (e *Engine) SetMaxWaitFor(d time.Duration) {
	if d > 0 {
		e.maxWaitFor = d
	}
}

// MaxWaitFor returns the configured wait_for / modal window cap.
func (e *Engine) MaxWaitFor() time.Duration { return e.maxWaitFor }

// SetRouting overrides the component custom_id prefixes (routed + decorative).
// Pass e.g. ("auto:", "auto:noop:") for the automations engine so component
// clicks on automation-sent messages resume the right runs.
func (e *Engine) SetRouting(routePrefix, noopPrefix string) {
	if routePrefix != "" {
		e.routePrefix = routePrefix
	}
	if noopPrefix != "" {
		e.noopPrefix = noopPrefix
	}
}

// Register adds (or replaces) a handler for a step kind.
func (e *Engine) Register(kind string, h Handler) { e.handlers[kind] = h }

// Handler is the contract every step kind implements. It reads/writes scope
// via H.Scope, side-effects via H.Deps, and returns its output (logged) or an
// error. Returning errExit unwinds successfully; returning a PauseError
// suspends the run for durable resume.
type Handler func(ctx context.Context, h *Halt) error

// Halt is the per-step argument passed to handlers — a focused view over the
// engine, the current scope, the decoded spec, the run, and the cursor path.
// "Halt" is a nod to its dual role: it carries everything a step needs to
// either complete or halt the walker.
type Halt struct {
	Engine *Engine
	Deps   *Deps
	Scope  *cc.Scope
	Step   cc.Step
	Run    *RunState
	Path   string // dotted cursor path for logging
}

// SetOutput records the step's logged output value (any).
func (h *Halt) SetOutput(v any) {
	b, _ := json.Marshal(v)
	h.Run.lastOutput = b
}

// RunState is the per-invocation state the walker mutates: cursor, scope,
// pending logs, statistics. It is NOT shared between concurrent runs.
type RunState struct {
	ID             string
	CommandID      string // UUID
	CommandVersion int
	GuildID        string
	InvokerID      string
	// ActorID is who drives the CURRENT interaction: the invoker on a slash
	// run, the clicker on a component resume. Steps that gate a follow-up
	// interaction (a modal shown to whoever clicked) must await the actor,
	// not the invoker.
	ActorID            string
	ChannelID          string
	TriggerKind        string
	InteractionID      string
	InteractionToken   string
	InteractionExpires *time.Time
	DefinitionSnapshot json.RawMessage
	StartedAt          time.Time

	cursor     []cc.CursorFrame
	stepsRun   atomic.Int64
	httpCalls  atomic.Int64
	imgRenders atomic.Int64

	lastOutput json.RawMessage
	logs       []cc.RunLog

	// durable indicates this run has yielded at least once and must persist.
	durable bool
}

// Logs returns the in-memory log buffer (flushed by the caller).
func (r *RunState) Logs() []cc.RunLog { return r.logs }

// Durable reports whether the run yielded (wait/wait_for) and so requires
// persistence.
func (r *RunState) Durable() bool { return r.durable }

// markDurable flips the durability flag (set when a wait/wait_for is hit).
func (r *RunState) markDurable() { r.durable = true }

// Cursor returns the current path through the tree (copied; safe to persist).
func (r *RunState) Cursor() []cc.CursorFrame {
	out := make([]cc.CursorFrame, len(r.cursor))
	copy(out, r.cursor)
	return out
}

// SetCursor restores a cursor on resume.
func (r *RunState) SetCursor(c []cc.CursorFrame) {
	r.cursor = append(r.cursor[:0], c...)
}

// ── Run limits (per-invocation, walker-enforced) ────────────────────────────

const (
	maxStepsPerRun        = 500
	maxHTTPCallsPerRun    = 10
	maxImageRendersPerRun = 20
	maxLoopIter           = 1000
	defaultLoopIter       = 100
	maxParallelBranches   = 8
)

// ── Errors / sentinels ──────────────────────────────────────────────────────

// errExit is returned by the `exit` step (and by wait_for-on-timeout) to
// terminate the run successfully without an error. The walker unwinds without
// touching on_error chains.
var errExit = errors.New("exit")

// errFail is returned by the `fail` step.
type errFail struct{ msg string }

func (e *errFail) Error() string { return "fail: " + e.msg }

// PauseError is the yield sentinel returned by wait / wait_for. The walker
// records the cursor + scope + resume conditions and stops walking.
type PauseError struct {
	Kind             string // "wait" | "wait_for"
	ResumeAt         *time.Time
	AwaitingCustomID string
	AwaitingUserID   string
	AwaitingKind     string // "component" | "modal" | "message" | "reaction" | ""

	// Cursor is the path to the paused step, snapshotted by dispatch() the
	// moment the handler yields. It MUST be captured here: as the pause
	// unwinds, every walk frame defer-pops, so the RunState cursor is empty
	// again by the time the caller persists the run.
	Cursor []cc.CursorFrame
}

// Error implements error.
func (p *PauseError) Error() string {
	if p.Kind == "wait" && p.ResumeAt != nil {
		return "wait until " + p.ResumeAt.Format(time.RFC3339)
	}
	return "wait_for " + p.AwaitingKind
}

// IsPause reports whether err is a PauseError.
func IsPause(err error) (*PauseError, bool) {
	var p *PauseError
	if errors.As(err, &p) {
		return p, true
	}
	return nil, false
}

// IsExit reports whether err is the exit sentinel.
func IsExit(err error) bool { return errors.Is(err, errExit) }

// IsFail extracts the message from a fail sentinel.
func IsFail(err error) (string, bool) {
	var f *errFail
	if errors.As(err, &f) {
		return f.msg, true
	}
	return "", false
}

// ── Entry point ─────────────────────────────────────────────────────────────

// Run walks the definition's Step[] tree against scope. Returns (terminal,
// pause, err). Exactly one will be non-nil on a non-error return:
//   - terminal != nil: run completed (success/failure recorded inside);
//   - pause != nil:    run yielded (caller persists run + logs);
//   - err  != nil:     internal engine error (caller surfaces it).
type Outcome struct {
	Status string // done | failed | exited | waiting
	Error  string
}

// Run starts a fresh execution at the root.
func (e *Engine) Run(ctx context.Context, run *RunState, def cc.Definition, scope *cc.Scope) (out Outcome, pause *PauseError, err error) {
	defer e.recoverPanic(run, &out, &pause, &err)
	run.StartedAt = nowUTC()
	werr := e.walk(ctx, run, scope, def.Steps, "root")
	return e.classifyOutcome(werr)
}

// Resume continues an in-flight run from a stored cursor. The caller must
// have already injected the trigger payload into scope (e.g. via
// ScopeData.Vars[into] = matched modal/component values).
func (e *Engine) Resume(ctx context.Context, run *RunState, def cc.Definition, scope *cc.Scope, cursor []cc.CursorFrame) (Outcome, *PauseError, error) {
	return e.resumeWith(ctx, run, def, scope, cursor, false)
}

// ResumeTimedOut continues a run whose event wait expired: the wait's
// on_timeout branch runs (if any) and the run drains, instead of executing
// the post-trigger continuation as if the event had arrived.
func (e *Engine) ResumeTimedOut(ctx context.Context, run *RunState, def cc.Definition, scope *cc.Scope, cursor []cc.CursorFrame) (Outcome, *PauseError, error) {
	return e.resumeWith(ctx, run, def, scope, cursor, true)
}

func (e *Engine) resumeWith(ctx context.Context, run *RunState, def cc.Definition, scope *cc.Scope, cursor []cc.CursorFrame, timedOut bool) (out Outcome, pause *PauseError, err error) {
	defer e.recoverPanic(run, &out, &pause, &err)
	run.SetCursor(cursor)
	werr := e.resumeAt(ctx, run, scope, def.Steps, timedOut)
	return e.classifyOutcome(werr)
}

// recoverPanic converts a step-handler panic into a failed outcome instead of
// crashing the worker. Custom commands are authored by server admins on a
// public bot; a malformed spec must never take the process down.
func (e *Engine) recoverPanic(run *RunState, out *Outcome, pause **PauseError, err *error) {
	if r := recover(); r != nil {
		if e.deps.Log != nil {
			e.deps.Log.Error("ccmd panic recovered",
				"run", run.ID, "panic", fmt.Sprint(r), "stack", string(debug.Stack()))
		}
		*out = Outcome{Status: "failed", Error: "internal error while running the command"}
		*pause = nil
		*err = nil
	}
}

func (e *Engine) classifyOutcome(err error) (Outcome, *PauseError, error) {
	if err == nil {
		return Outcome{Status: "done"}, nil, nil
	}
	if IsExit(err) {
		return Outcome{Status: "exited"}, nil, nil
	}
	if msg, ok := IsFail(err); ok {
		return Outcome{Status: "failed", Error: msg}, nil, nil
	}
	if p, ok := IsPause(err); ok {
		return Outcome{Status: "waiting"}, p, nil
	}
	return Outcome{Status: "failed", Error: err.Error()}, nil, err
}

// ── Cursor helpers ──────────────────────────────────────────────────────────

func (r *RunState) push(f cc.CursorFrame) { r.cursor = append(r.cursor, f) }
func (r *RunState) pop()                  { r.cursor = r.cursor[:len(r.cursor)-1] }

func (r *RunState) setIndex(i int) {
	if len(r.cursor) == 0 {
		return
	}
	r.cursor[len(r.cursor)-1].Index = i
}

func (r *RunState) setIter(i int) {
	if len(r.cursor) == 0 {
		return
	}
	r.cursor[len(r.cursor)-1].Iter = i
}

func (r *RunState) cursorPath() string {
	if len(r.cursor) == 0 {
		return ""
	}
	var b strings.Builder
	for i, f := range r.cursor {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(f.Branch)
		b.WriteByte('[')
		b.WriteString(strconv.Itoa(f.Index))
		b.WriteByte(']')
	}
	return b.String()
}

// ── walker dispatch ─────────────────────────────────────────────────────────

// dispatch runs one step (after the per-kind handler resolves). Errors from
// the handler are caught by on_error if present (inline recovery).
func (e *Engine) dispatch(ctx context.Context, run *RunState, scope *cc.Scope, s cc.Step) error {
	if run.stepsRun.Add(1) > maxStepsPerRun {
		return fmt.Errorf("step budget exceeded (%d)", maxStepsPerRun)
	}
	h, ok := e.handlers[s.Kind]
	if !ok {
		return fmt.Errorf("no handler for kind %q", s.Kind)
	}
	halt := &Halt{
		Engine: e,
		Deps:   &e.deps,
		Scope:  scope,
		Step:   s,
		Run:    run,
		Path:   run.cursorPath(),
	}
	t0 := time.Now()
	run.lastOutput = nil
	err := h(ctx, halt)
	dur := int(time.Since(t0) / time.Millisecond)

	// Always log — even on error, so the Runs tab shows what happened.
	status := "ok"
	errMsg := ""
	if err != nil {
		if p, isPause := IsPause(err); isPause {
			status = "ok" // a pause completed the step normally
			if p.Cursor == nil {
				p.Cursor = run.Cursor()
			}
		} else if IsExit(err) {
			status = "ok"
		} else {
			status = "error"
			errMsg = err.Error()
		}
	}
	specCopy := s.Spec
	run.logs = append(run.logs, cc.RunLog{
		RunID:      run.ID,
		StepID:     s.ID,
		StepKind:   s.Kind,
		CursorPath: halt.Path,
		StartedAt:  t0,
		DurationMs: dur,
		Status:     status,
		Input:      specCopy,
		Output:     run.lastOutput,
		Error:      errMsg,
	})

	// Inline error recovery: if on_error / on_error_cases are set and the
	// failure is recoverable (not a Pause/Exit), dispatch to the first
	// matching typed case, then fall back to the default OnError handler.
	if err != nil && (len(s.OnError) > 0 || len(s.OnErrorCases) > 0) {
		if _, isPause := IsPause(err); isPause {
			return err
		}
		if IsExit(err) {
			return err
		}
		// Build the .Error.* view that templates inside the recovery branch
		// can read (kind, message, retryable, …).
		info := ErrorInfoFrom(s.Kind, s.ID, err)
		scope.SetErrorInfo(info)
		defer scope.ClearErrorInfo()

		// Walk typed cases first, first match wins. The cursor frame must
		// say WHICH array the recovery steps live in, or a pause inside a
		// typed case would resume against the default on_error chain.
		handler, caseIdx := pickErrorHandler(info.Kind, s.OnErrorCases)
		if handler == nil {
			handler = s.OnError
			caseIdx = -1
		}
		if len(handler) > 0 {
			frame := cc.CursorFrame{Branch: "on_error"}
			if caseIdx >= 0 {
				frame = cc.CursorFrame{Branch: "on_error_case", Case: caseIdx}
			}
			recover := e.walkFrame(ctx, run, scope, handler, frame)
			if recover != nil {
				return recover
			}
			return nil
		}
	}
	return err
}

// pickErrorHandler returns the first OnErrorCases arm whose `When` patterns
// match the given error kind plus its index, or (nil, -1) for "no match,
// fall back".
func pickErrorHandler(kind string, cases []cc.ErrorCase) ([]cc.Step, int) {
	for ci, c := range cases {
		for _, p := range c.When {
			if MatchKind(kind, p) {
				return c.Do, ci
			}
		}
	}
	return nil, -1
}

// ── Misc helpers ────────────────────────────────────────────────────────────

func nowUTC() time.Time { return time.Now().UTC() }
