package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// walk iterates a Step[] branch sequentially from index 0, recursing into
// control flow as needed. It is the synchronous core called by Run().
func (e *Engine) walk(ctx context.Context, run *RunState, scope *cc.Scope, steps []cc.Step, branch string) error {
	return e.walkFrame(ctx, run, scope, steps, cc.CursorFrame{Branch: branch})
}

// walkFrame is walk with a fully-specified cursor frame, for branches whose
// identity needs more than a name (a typed error case carries its index).
func (e *Engine) walkFrame(ctx context.Context, run *RunState, scope *cc.Scope, steps []cc.Step, frame cc.CursorFrame) error {
	run.push(frame)
	defer run.pop()

	for i, s := range steps {
		run.setIndex(i)
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := e.runOne(ctx, run, scope, s); err != nil {
			return err
		}
	}
	return nil
}

// runOne executes one step, dispatching into the control flow expansion when
// the kind is `if`/`switch`/`loop`/`parallel` — the handler dispatch only
// covers leaf and pause kinds.
func (e *Engine) runOne(ctx context.Context, run *RunState, scope *cc.Scope, s cc.Step) error {
	switch s.Kind {
	case cc.KindIf:
		return e.runIf(ctx, run, scope, s)
	case cc.KindSwitch:
		return e.runSwitch(ctx, run, scope, s)
	case cc.KindLoop:
		return e.runLoop(ctx, run, scope, s)
	case cc.KindParallel:
		return e.runParallel(ctx, run, scope, s)
	}
	return e.dispatch(ctx, run, scope, s)
}

// resumeAt continues a walk from a stored cursor. It performs the same
// recursion as walk() but starts at the cursor's index in each frame.
// timedOut marks a scheduler-driven resume of an event wait whose deadline
// passed: the on_timeout branch runs instead of the post-trigger path.
func (e *Engine) resumeAt(ctx context.Context, run *RunState, scope *cc.Scope, root []cc.Step, timedOut bool) error {
	cursor := run.Cursor()
	if len(cursor) == 0 {
		return e.walk(ctx, run, scope, root, "root")
	}
	// Reset the cursor; resumeBranch will rebuild it as it recurses.
	run.cursor = run.cursor[:0]
	return e.resumeBranch(ctx, run, scope, root, cursor, 0, timedOut)
}

// resumeBranch recurses through frames[depth..] to land at the paused step
// (one past the cursor's saved index, the next step we should execute).
func (e *Engine) resumeBranch(ctx context.Context, run *RunState, scope *cc.Scope, steps []cc.Step, frames []cc.CursorFrame, depth int, timedOut bool) error {
	frame := frames[depth]
	run.push(cc.CursorFrame{Branch: frame.Branch, Case: frame.Case, Iter: frame.Iter, Total: frame.Total})

	startIdx := frame.Index
	// The paused step is at frame.Index; resume executes the NEXT step, except
	// when the paused step is a loop iteration that hasn't yet finished its body.
	if depth == len(frames)-1 {
		// A timed-out event wait takes its on_timeout branch and the run
		// drains: the post-trigger continuation is the path that never
		// happened. (Documented in exec.go as the wait_for timeout drain.)
		if timedOut && startIdx >= 0 && startIdx < len(steps) {
			s := steps[startIdx]
			run.setIndex(startIdx)
			if s.Kind == cc.KindWaitFor {
				var spec cc.SpecWaitFor
				_ = json.Unmarshal(s.Spec, &spec)
				if len(spec.OnTimeout) > 0 {
					if err := e.walkFrame(ctx, run, scope, spec.OnTimeout, cc.CursorFrame{Branch: "on_timeout"}); err != nil {
						run.pop()
						return err
					}
				}
				run.pop()
				return errExit
			}
			if s.Kind == cc.KindModalOpen {
				run.pop()
				return errExit
			}
			// Not an event wait (legacy cursor): fall through to the normal
			// continuation below.
		}
		// Resume from the paused step's next sibling.
		// Re-execute the paused step itself NOT directly, but conceptually it
		// has already completed (the trigger payload has been written to scope
		// by the caller before invoking Resume).
		for i := startIdx + 1; i < len(steps); i++ {
			run.setIndex(i)
			if err := e.runOne(ctx, run, scope, steps[i]); err != nil {
				run.pop()
				return err
			}
		}
		run.pop()
		return nil
	}

	// Mid-tree frame: find the matching step container, recurse, then
	// continue executing siblings after it returns. A frame describes the
	// steps array it walks, so the branch to descend INTO is named by the
	// NEXT frame, while this frame's Index says which step holds it.
	if startIdx < 0 || startIdx >= len(steps) {
		run.pop()
		return fmt.Errorf("resume: cursor index %d out of range (%d steps)", startIdx, len(steps))
	}
	s := steps[startIdx]
	run.setIndex(startIdx)

	child := frames[depth+1]
	switch {
	case child.Branch == "body" && s.Kind == cc.KindLoop:
		// Resume the paused iteration's remaining body, then run the
		// iterations the pause cut off, then continue with siblings.
		if err := e.resumeBranch(ctx, run, scope, s.Then, frames, depth+1, timedOut); err != nil {
			run.pop()
			return err
		}
		if err := e.runLoopFrom(ctx, run, scope, s, child.Iter+1); err != nil {
			run.pop()
			return err
		}
	case child.Branch == "parallel" && s.Kind == cc.KindParallel:
		var spec cc.SpecParallel
		_ = json.Unmarshal(s.Spec, &spec)
		var br []cc.Step
		if child.Case >= 0 && child.Case < len(spec.Branches) {
			br = spec.Branches[child.Case]
		}
		if err := e.resumeBranch(ctx, run, scope, br, frames, depth+1, timedOut); err != nil {
			run.pop()
			return err
		}
		// The resumed branch completing satisfies a race join; otherwise the
		// remaining branches still owe their work.
		if spec.Join != "race" {
			if err := e.runParallelFrom(ctx, run, scope, spec, child.Case+1); err != nil {
				run.pop()
				return err
			}
		}
	default:
		var childSteps []cc.Step
		switch child.Branch {
		case "then", "body":
			childSteps = s.Then
		case "else":
			childSteps = s.Else
		case "default":
			childSteps = s.Default
		case "case":
			if child.Case >= 0 && child.Case < len(s.Cases) {
				childSteps = s.Cases[child.Case].Do
			}
		case "on_error":
			childSteps = s.OnError
		case "on_error_case":
			if child.Case >= 0 && child.Case < len(s.OnErrorCases) {
				childSteps = s.OnErrorCases[child.Case].Do
			}
		case "on_timeout":
			// wait_for timeout branch lives in the spec.
			if s.Kind == cc.KindWaitFor {
				var spec cc.SpecWaitFor
				_ = json.Unmarshal(s.Spec, &spec)
				childSteps = spec.OnTimeout
			}
		}
		if err := e.resumeBranch(ctx, run, scope, childSteps, frames, depth+1, timedOut); err != nil {
			run.pop()
			return err
		}
	}
	// Continue with the parent's siblings after the resumed container returns.
	for i := startIdx + 1; i < len(steps); i++ {
		run.setIndex(i)
		if err := e.runOne(ctx, run, scope, steps[i]); err != nil {
			run.pop()
			return err
		}
	}
	run.pop()
	return nil
}

// StepAtCursor returns the step a persisted cursor points at (the one that
// paused the run), or nil if the cursor no longer resolves.
func StepAtCursor(steps []cc.Step, frames []cc.CursorFrame) *cc.Step {
	branch, idx := BranchAtCursor(steps, frames)
	if branch == nil {
		return nil
	}
	return &branch[idx]
}

// BranchAtCursor returns the steps array containing the paused step plus its
// index in it, or (nil, -1). It mirrors resumeBranch's traversal, so the two
// must stay in lockstep.
func BranchAtCursor(steps []cc.Step, frames []cc.CursorFrame) ([]cc.Step, int) {
	for depth := 0; depth < len(frames); depth++ {
		frame := frames[depth]
		if frame.Index < 0 || frame.Index >= len(steps) {
			return nil, -1
		}
		s := &steps[frame.Index]
		if depth == len(frames)-1 {
			return steps, frame.Index
		}
		child := frames[depth+1]
		switch child.Branch {
		case "then", "body":
			steps = s.Then
		case "else":
			steps = s.Else
		case "default":
			steps = s.Default
		case "case":
			if child.Case < 0 || child.Case >= len(s.Cases) {
				return nil, -1
			}
			steps = s.Cases[child.Case].Do
		case "on_error":
			steps = s.OnError
		case "on_error_case":
			if child.Case < 0 || child.Case >= len(s.OnErrorCases) {
				return nil, -1
			}
			steps = s.OnErrorCases[child.Case].Do
		case "parallel":
			if s.Kind != cc.KindParallel {
				return nil, -1
			}
			var spec cc.SpecParallel
			if json.Unmarshal(s.Spec, &spec) != nil {
				return nil, -1
			}
			if child.Case < 0 || child.Case >= len(spec.Branches) {
				return nil, -1
			}
			steps = spec.Branches[child.Case]
		case "on_timeout":
			if s.Kind != cc.KindWaitFor {
				return nil, -1
			}
			var spec cc.SpecWaitFor
			if json.Unmarshal(s.Spec, &spec) != nil {
				return nil, -1
			}
			steps = spec.OnTimeout
		default:
			return nil, -1
		}
	}
	return nil, -1
}

// ── if ───────────────────────────────────────────────────────────────────────

func (e *Engine) runIf(ctx context.Context, run *RunState, scope *cc.Scope, s cc.Step) error {
	var spec cc.SpecIf
	_ = json.Unmarshal(s.Spec, &spec)
	t0 := time.Now()
	ok, err := cc.EvalBool(ctx, spec.Cond, scope)
	branch := "then"
	if !ok {
		branch = "else"
	}
	// Log the predicate evaluation as a step row so the Runs tab shows the result.
	out, _ := json.Marshal(map[string]any{"branch": branch, "value": ok})
	run.logs = append(run.logs, cc.RunLog{
		RunID:      run.ID,
		StepID:     s.ID,
		StepKind:   s.Kind,
		CursorPath: run.cursorPath(),
		StartedAt:  t0,
		DurationMs: int(time.Since(t0) / time.Millisecond),
		Status:     "ok",
		Input:      s.Spec,
		Output:     out,
	})
	if err != nil {
		return fmt.Errorf("if cond: %w", err)
	}
	if ok {
		return e.walk(ctx, run, scope, s.Then, "then")
	}
	if len(s.Else) > 0 {
		return e.walk(ctx, run, scope, s.Else, "else")
	}
	return nil
}

// ── switch ───────────────────────────────────────────────────────────────────

func (e *Engine) runSwitch(ctx context.Context, run *RunState, scope *cc.Scope, s cc.Step) error {
	var spec cc.SpecSwitch
	_ = json.Unmarshal(s.Spec, &spec)
	t0 := time.Now()
	target, err := cc.EvalString(ctx, spec.On, scope)
	if err != nil {
		return fmt.Errorf("switch on: %w", err)
	}
	for ci, c := range s.Cases {
		want, _ := cc.EvalString(ctx, c.When, scope)
		if want == target {
			run.logs = append(run.logs, cc.RunLog{
				RunID:      run.ID,
				StepID:     s.ID,
				StepKind:   s.Kind,
				CursorPath: run.cursorPath(),
				StartedAt:  t0,
				DurationMs: int(time.Since(t0) / time.Millisecond),
				Status:     "ok",
				Output:     mustJSON(map[string]any{"case": ci, "match": target}),
			})
			run.push(cc.CursorFrame{Branch: "case", Case: ci})
			for j, st := range c.Do {
				run.setIndex(j)
				if err := e.runOne(ctx, run, scope, st); err != nil {
					run.pop()
					return err
				}
			}
			run.pop()
			return nil
		}
	}
	run.logs = append(run.logs, cc.RunLog{
		RunID:      run.ID,
		StepID:     s.ID,
		StepKind:   s.Kind,
		CursorPath: run.cursorPath(),
		StartedAt:  t0,
		DurationMs: int(time.Since(t0) / time.Millisecond),
		Status:     "ok",
		Output:     mustJSON(map[string]any{"case": "default", "match": target}),
	})
	if len(s.Default) > 0 {
		return e.walk(ctx, run, scope, s.Default, "default")
	}
	return nil
}

// ── loop ─────────────────────────────────────────────────────────────────────

func (e *Engine) runLoop(ctx context.Context, run *RunState, scope *cc.Scope, s cc.Step) error {
	return e.runLoopFrom(ctx, run, scope, s, 0)
}

// runLoopFrom runs a loop's iterations starting at startIter; resume uses it
// to finish the iterations a mid-body pause cut off. The item list is
// re-evaluated from the restored scope, so a source that changed while the
// run was parked yields the fresh data.
func (e *Engine) runLoopFrom(ctx context.Context, run *RunState, scope *cc.Scope, s cc.Step, startIter int) error {
	var spec cc.SpecLoop
	_ = json.Unmarshal(s.Spec, &spec)
	if spec.As == "" {
		return errors.New("loop missing 'as' variable name")
	}
	maxIter := spec.MaxIter
	if maxIter <= 0 {
		maxIter = defaultLoopIter
	}
	if maxIter > maxLoopIter {
		maxIter = maxLoopIter
	}
	items, err := cc.EvalList(ctx, spec.Over, scope)
	if err != nil {
		return fmt.Errorf("loop over: %w", err)
	}
	t0 := time.Now()
	total := len(items)
	if total > maxIter {
		total = maxIter
	}
	if startIter == 0 {
		run.logs = append(run.logs, cc.RunLog{
			RunID:      run.ID,
			StepID:     s.ID,
			StepKind:   s.Kind,
			CursorPath: run.cursorPath(),
			StartedAt:  t0,
			DurationMs: int(time.Since(t0) / time.Millisecond),
			Status:     "ok",
			Output:     mustJSON(map[string]any{"total": total, "as": spec.As}),
		})
	}
	for i := startIter; i < total; i++ {
		scope.Set(spec.As, items[i])
		if spec.IndexAs != "" {
			scope.Set(spec.IndexAs, i)
		}
		run.push(cc.CursorFrame{Branch: "body", Iter: i, Total: total})
		for j, st := range s.Then {
			run.setIndex(j)
			if err := e.runOne(ctx, run, scope, st); err != nil {
				run.pop()
				return err
			}
		}
		run.pop()
	}
	return nil
}

// ── parallel ─────────────────────────────────────────────────────────────────

func (e *Engine) runParallel(ctx context.Context, run *RunState, scope *cc.Scope, s cc.Step) error {
	var spec cc.SpecParallel
	_ = json.Unmarshal(s.Spec, &spec)
	if len(spec.Branches) == 0 {
		return nil
	}
	if len(spec.Branches) > maxParallelBranches {
		return fmt.Errorf("parallel: too many branches (max %d)", maxParallelBranches)
	}
	// We execute sequentially inside one goroutine for now (Discord REST ops
	// are I/O-bound; goroutine soup adds complexity to scope merge that the
	// MVP doesn't need). The semantic is "join=all": every branch runs to
	// completion; first error stops further branches.
	return e.runParallelFrom(ctx, run, scope, spec, 0)
}

// runParallelFrom runs branches starting at startBranch; resume uses it to
// finish the branches a mid-branch pause cut off. The frame carries the
// branch number in Case (Index tracks the step within the branch).
func (e *Engine) runParallelFrom(ctx context.Context, run *RunState, scope *cc.Scope, spec cc.SpecParallel, startBranch int) error {
	for bi := startBranch; bi < len(spec.Branches); bi++ {
		run.push(cc.CursorFrame{Branch: "parallel", Case: bi})
		for j, st := range spec.Branches[bi] {
			run.setIndex(j)
			if err := e.runOne(ctx, run, scope, st); err != nil {
				run.pop()
				return err
			}
		}
		run.pop()
		if spec.Join == "race" {
			return nil
		}
	}
	return nil
}

// mustJSON marshals v or returns []byte("null") on failure (logging never blocks).
func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
