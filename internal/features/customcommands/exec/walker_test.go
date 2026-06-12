package exec

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// testEngine returns an engine with a "probe" handler that records execution
// order, so tests can assert exactly which steps ran.
func testEngine(t *testing.T) (*Engine, *[]string) {
	t.Helper()
	e := New(Deps{})
	ran := &[]string{}
	e.Register("probe", func(ctx context.Context, h *Halt) error {
		*ran = append(*ran, h.Step.ID)
		return nil
	})
	return e, ran
}

func probe(id string) cc.Step { return cc.Step{ID: id, Kind: "probe"} }

func waitFor(id string) cc.Step {
	spec, _ := json.Marshal(cc.SpecWaitFor{Trigger: "component", Timeout: "5m", Into: "click"})
	return cc.Step{ID: id, Kind: cc.KindWaitFor, Spec: spec}
}

func ifTrue(id string, then ...cc.Step) cc.Step {
	spec, _ := json.Marshal(cc.SpecIf{Cond: cc.Expr{Lang: "tmpl", Src: "yes"}})
	return cc.Step{ID: id, Kind: cc.KindIf, Spec: spec, Then: then}
}

func newTestScope() *cc.Scope {
	return cc.NewScope(nil, "1", cc.ContextVars{}, map[string]any{}, map[string]any{})
}

// A pause at root level must persist a cursor pointing at the wait step, and
// resuming must run only the steps after it.
func TestPauseAndResumeAtRoot(t *testing.T) {
	e, ran := testEngine(t)
	def := cc.Definition{Steps: []cc.Step{probe("a"), waitFor("w"), probe("b"), probe("c")}}

	run := &RunState{ID: "r1"}
	out, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, got out=%+v pause=%v err=%v", out, pause, err)
	}
	if len(pause.Cursor) != 1 || pause.Cursor[0].Branch != "root" || pause.Cursor[0].Index != 1 {
		t.Fatalf("pause cursor should point at the wait step, got %+v", pause.Cursor)
	}

	*ran = (*ran)[:0]
	resumed := &RunState{ID: "r1"}
	out, pause2, err := e.Resume(context.Background(), resumed, def, newTestScope(), pause.Cursor)
	if err != nil || pause2 != nil {
		t.Fatalf("resume: out=%+v pause=%v err=%v", out, pause2, err)
	}
	if got := *ran; len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("resume should run only b,c, got %v", got)
	}
}

// A pause inside an if-branch (where every dashboard click cluster lands when
// the message lives in a branch) must resume the REST of that branch, then
// continue with the parent's siblings.
func TestPauseAndResumeInsideBranch(t *testing.T) {
	e, ran := testEngine(t)
	def := cc.Definition{Steps: []cc.Step{
		ifTrue("if1", probe("a"), waitFor("w"), probe("b"), probe("c")),
		probe("d"),
	}}

	run := &RunState{ID: "r2"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, err=%v", err)
	}
	want := []cc.CursorFrame{{Branch: "root", Index: 0}, {Branch: "then", Index: 1}}
	if len(pause.Cursor) != 2 || pause.Cursor[0] != want[0] || pause.Cursor[1] != want[1] {
		t.Fatalf("cursor: want %+v got %+v", want, pause.Cursor)
	}

	*ran = (*ran)[:0]
	resumed := &RunState{ID: "r2"}
	_, pause2, err := e.Resume(context.Background(), resumed, def, newTestScope(), pause.Cursor)
	if err != nil || pause2 != nil {
		t.Fatalf("resume: pause=%v err=%v", pause2, err)
	}
	if got := *ran; len(got) != 3 || got[0] != "b" || got[1] != "c" || got[2] != "d" {
		t.Fatalf("resume should run b,c then d, got %v", got)
	}
}

// A timed-out event wait runs its on_timeout branch and drains; the
// post-click continuation must NOT execute.
func TestTimedOutWaitRunsOnTimeoutAndExits(t *testing.T) {
	e, ran := testEngine(t)
	w := waitFor("w")
	spec, _ := json.Marshal(cc.SpecWaitFor{
		Trigger: "component", Timeout: "5m", Into: "click",
		OnTimeout: []cc.Step{probe("t")},
	})
	w.Spec = spec
	def := cc.Definition{Steps: []cc.Step{w, probe("b")}}

	run := &RunState{ID: "r4"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, err=%v", err)
	}

	*ran = (*ran)[:0]
	resumed := &RunState{ID: "r4"}
	out, pause2, err := e.ResumeTimedOut(context.Background(), resumed, def, newTestScope(), pause.Cursor)
	if err != nil || pause2 != nil {
		t.Fatalf("timeout resume: pause=%v err=%v", pause2, err)
	}
	if out.Status != "exited" {
		t.Fatalf("timed-out run should drain as exited, got %q", out.Status)
	}
	if got := *ran; len(got) != 1 || got[0] != "t" {
		t.Fatalf("only on_timeout should run, got %v", got)
	}
}

// No on_timeout branch: a timed-out wait still drains without running the
// continuation.
func TestTimedOutWaitWithoutOnTimeoutExits(t *testing.T) {
	e, ran := testEngine(t)
	def := cc.Definition{Steps: []cc.Step{waitFor("w"), probe("b")}}

	run := &RunState{ID: "r5"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, err=%v", err)
	}

	*ran = (*ran)[:0]
	out, _, err := e.ResumeTimedOut(context.Background(), &RunState{ID: "r5"}, def, newTestScope(), pause.Cursor)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if out.Status != "exited" || len(*ran) != 0 {
		t.Fatalf("want exited with nothing run, got %q ran=%v", out.Status, *ran)
	}
}

// A pause inside a loop body resumes the body remainder AND the remaining
// iterations (each pausing again at its own wait), then the after-chain.
func TestPauseAndResumeInsideLoopContinuesIterations(t *testing.T) {
	e, ran := testEngine(t)
	loopSpec, _ := json.Marshal(cc.SpecLoop{
		Over: cc.Expr{Lang: "tmpl", Src: "x,y,z"}, As: "item",
	})
	loop := cc.Step{ID: "lp", Kind: cc.KindLoop, Spec: loopSpec, Then: []cc.Step{waitFor("w"), probe("b")}}
	def := cc.Definition{Steps: []cc.Step{loop, probe("d")}}

	run := &RunState{ID: "r6"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, err=%v", err)
	}
	scope := newTestScope()
	for iter := 0; iter < 2; iter++ {
		*ran = (*ran)[:0]
		_, pause2, err := e.Resume(context.Background(), &RunState{ID: "r6"}, def, scope, pause.Cursor)
		if err != nil || pause2 == nil {
			t.Fatalf("iter %d: want next pause, err=%v", iter, err)
		}
		if got := *ran; len(got) != 1 || got[0] != "b" {
			t.Fatalf("iter %d: body remainder should run once, got %v", iter, got)
		}
		if pause2.Cursor[1].Iter != iter+1 {
			t.Fatalf("iter %d: next pause should be iteration %d, got %+v", iter, iter+1, pause2.Cursor)
		}
		pause = pause2
	}
	*ran = (*ran)[:0]
	_, pause3, err := e.Resume(context.Background(), &RunState{ID: "r6"}, def, scope, pause.Cursor)
	if err != nil || pause3 != nil {
		t.Fatalf("final resume: pause=%v err=%v", pause3, err)
	}
	if got := *ran; len(got) != 2 || got[0] != "b" || got[1] != "d" {
		t.Fatalf("final resume should run b then d, got %v", got)
	}
}

// A pause inside a parallel branch resumes that branch's remainder, runs the
// remaining branches, then continues after the block.
func TestPauseAndResumeInsideParallel(t *testing.T) {
	e, ran := testEngine(t)
	parSpec, _ := json.Marshal(cc.SpecParallel{Branches: [][]cc.Step{
		{waitFor("w"), probe("b")},
		{probe("c")},
	}})
	par := cc.Step{ID: "par", Kind: cc.KindParallel, Spec: parSpec}
	def := cc.Definition{Steps: []cc.Step{par, probe("d")}}

	run := &RunState{ID: "r7"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, err=%v", err)
	}
	if st := StepAtCursor(def.Steps, pause.Cursor); st == nil || st.ID != "w" {
		t.Fatalf("StepAtCursor should find the wait inside the branch, got %+v (cursor %+v)", st, pause.Cursor)
	}

	*ran = (*ran)[:0]
	_, pause2, err := e.Resume(context.Background(), &RunState{ID: "r7"}, def, newTestScope(), pause.Cursor)
	if err != nil || pause2 != nil {
		t.Fatalf("resume: pause=%v err=%v", pause2, err)
	}
	if got := *ran; len(got) != 3 || got[0] != "b" || got[1] != "c" || got[2] != "d" {
		t.Fatalf("resume should run b, c then d, got %v", got)
	}
}

// A pause inside a TYPED error case must resume against that case's steps,
// not the default on_error chain.
func TestPauseAndResumeInsideTypedErrorCase(t *testing.T) {
	e, ran := testEngine(t)
	e.Register("boom", func(ctx context.Context, h *Halt) error {
		return errors.New("kaboom")
	})
	boom := cc.Step{ID: "x", Kind: "boom", OnErrorCases: []cc.ErrorCase{
		{When: []string{"*"}, Do: []cc.Step{waitFor("w"), probe("r")}},
	}}
	def := cc.Definition{Steps: []cc.Step{boom, probe("d")}}

	run := &RunState{ID: "r8"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, err=%v", err)
	}
	if pause.Cursor[1].Branch != "on_error_case" || pause.Cursor[1].Case != 0 {
		t.Fatalf("cursor should record the typed case, got %+v", pause.Cursor)
	}
	if st := StepAtCursor(def.Steps, pause.Cursor); st == nil || st.ID != "w" {
		t.Fatalf("StepAtCursor should resolve through the typed case, got %+v", st)
	}

	*ran = (*ran)[:0]
	_, pause2, err := e.Resume(context.Background(), &RunState{ID: "r8"}, def, newTestScope(), pause.Cursor)
	if err != nil || pause2 != nil {
		t.Fatalf("resume: pause=%v err=%v", pause2, err)
	}
	if got := *ran; len(got) != 2 || got[0] != "r" || got[1] != "d" {
		t.Fatalf("resume should run r then d, got %v", got)
	}
}

// fakeDiscord satisfies DiscordClient via interface embedding; only the
// methods a test exercises are implemented.
type fakeDiscord struct{ DiscordClient }

func (fakeDiscord) Respond(ref Interaction, resp *discordgo.InteractionResponse) error { return nil }

// A modal shown on a component resume belongs to whoever clicked (the
// actor), not the run's original invoker; the submit gate must await them.
func TestModalOpenAwaitsTheActor(t *testing.T) {
	e := New(Deps{Discord: fakeDiscord{}})
	spec, _ := json.Marshal(cc.SpecModalOpen{Title: "Form", CustomIDSuffix: "f"})
	def := cc.Definition{Steps: []cc.Step{{ID: "m", Kind: cc.KindModalOpen, Spec: spec}}}

	run := &RunState{ID: "r10", InvokerID: "111", ActorID: "222", InteractionToken: "tok"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want modal pause, err=%v", err)
	}
	if pause.AwaitingKind != "modal" || pause.AwaitingUserID != "222" {
		t.Fatalf("modal must await the actor: %+v", pause)
	}

	// Slash-invoked runs have no separate actor; the invoker still gates.
	run2 := &RunState{ID: "r11", InvokerID: "111", InteractionToken: "tok"}
	_, pause2, err := e.Run(context.Background(), run2, def, newTestScope())
	if err != nil || pause2 == nil || pause2.AwaitingUserID != "111" {
		t.Fatalf("invoker fallback: %+v err=%v", pause2, err)
	}
}

// Same through a switch case, two levels deep.
func TestPauseAndResumeInsideCase(t *testing.T) {
	e, ran := testEngine(t)
	swSpec, _ := json.Marshal(cc.SpecSwitch{On: cc.Expr{Lang: "tmpl", Src: "x"}})
	sw := cc.Step{ID: "sw", Kind: cc.KindSwitch, Spec: swSpec, Cases: []cc.SwitchCase{
		{When: cc.Expr{Lang: "tmpl", Src: "nope"}, Do: []cc.Step{probe("n")}},
		{When: cc.Expr{Lang: "tmpl", Src: "x"}, Do: []cc.Step{waitFor("w"), probe("b")}},
	}}
	def := cc.Definition{Steps: []cc.Step{sw, probe("d")}}

	run := &RunState{ID: "r3"}
	_, pause, err := e.Run(context.Background(), run, def, newTestScope())
	if err != nil || pause == nil {
		t.Fatalf("want pause, err=%v", err)
	}
	if len(pause.Cursor) != 2 || pause.Cursor[1].Branch != "case" || pause.Cursor[1].Case != 1 || pause.Cursor[1].Index != 0 {
		t.Fatalf("cursor: got %+v", pause.Cursor)
	}

	*ran = (*ran)[:0]
	resumed := &RunState{ID: "r3"}
	_, _, err = e.Resume(context.Background(), resumed, def, newTestScope(), pause.Cursor)
	if err != nil {
		t.Fatalf("resume err=%v", err)
	}
	if got := *ran; len(got) != 2 || got[0] != "b" || got[1] != "d" {
		t.Fatalf("resume should run b then d, got %v", got)
	}
}
