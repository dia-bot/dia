package exec

import (
	"context"
	"encoding/json"
	"testing"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
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
		t.Fatalf("resume should run only b,c — got %v", got)
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
		t.Fatalf("resume should run b,c then d — got %v", got)
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
		t.Fatalf("resume should run b then d — got %v", got)
	}
}
