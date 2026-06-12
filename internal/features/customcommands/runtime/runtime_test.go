package runtime

import (
	"encoding/json"
	"testing"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

func clickSwitch(t *testing.T, into string, cases map[string][]cc.Step, def []cc.Step) cc.Step {
	t.Helper()
	spec, _ := json.Marshal(cc.SpecSwitch{On: cc.Expr{Lang: "tmpl", Src: "{{ .Vars." + into + ".id }}"}})
	sw := cc.Step{ID: "sw", Kind: cc.KindSwitch, Spec: spec, Default: def}
	for when, do := range cases {
		sw.Cases = append(sw.Cases, cc.SwitchCase{When: cc.Expr{Lang: "tmpl", Src: when}, Do: do})
	}
	return sw
}

func TestClickContinuation(t *testing.T) {
	wait := cc.Step{ID: "w", Kind: cc.KindWaitFor}
	modal := cc.Step{ID: "m", Kind: cc.KindModalOpen}
	reply := cc.Step{ID: "r", Kind: cc.KindReply}

	t.Run("modal-first case answers with the form", func(t *testing.T) {
		branch := []cc.Step{wait, clickSwitch(t, "click", map[string][]cc.Step{"form": {modal, reply}}, nil)}
		mf, ur := clickContinuation(branch, 0, "click", "form")
		if !mf || ur {
			t.Fatalf("got modalFirst=%v unrouted=%v", mf, ur)
		}
	})

	t.Run("reply-first case keeps the configured ack", func(t *testing.T) {
		branch := []cc.Step{wait, clickSwitch(t, "click", map[string][]cc.Step{"go": {reply}}, nil)}
		mf, ur := clickContinuation(branch, 0, "click", "go")
		if mf || ur {
			t.Fatalf("got modalFirst=%v unrouted=%v", mf, ur)
		}
	})

	t.Run("unrouted suffix with static cases and empty default", func(t *testing.T) {
		branch := []cc.Step{wait, clickSwitch(t, "click", map[string][]cc.Step{"a": {reply}}, nil)}
		mf, ur := clickContinuation(branch, 0, "click", "other")
		if mf || !ur {
			t.Fatalf("got modalFirst=%v unrouted=%v", mf, ur)
		}
	})

	t.Run("templated case value disables the unrouted degradation", func(t *testing.T) {
		branch := []cc.Step{wait, clickSwitch(t, "click", map[string][]cc.Step{"vote_{{ .Vars.i }}": {reply}}, nil)}
		mf, ur := clickContinuation(branch, 0, "click", "other")
		if mf || ur {
			t.Fatalf("got modalFirst=%v unrouted=%v", mf, ur)
		}
	})

	t.Run("non-empty default still routes", func(t *testing.T) {
		branch := []cc.Step{wait, clickSwitch(t, "click", map[string][]cc.Step{"a": {reply}}, []cc.Step{reply})}
		mf, ur := clickContinuation(branch, 0, "click", "other")
		if mf || ur {
			t.Fatalf("got modalFirst=%v unrouted=%v", mf, ur)
		}
	})

	t.Run("legacy wait chains straight into a modal", func(t *testing.T) {
		branch := []cc.Step{wait, modal}
		mf, ur := clickContinuation(branch, 0, "", "anything")
		if !mf || ur {
			t.Fatalf("got modalFirst=%v unrouted=%v", mf, ur)
		}
	})

	t.Run("wait at the end of its branch", func(t *testing.T) {
		branch := []cc.Step{wait}
		mf, ur := clickContinuation(branch, 0, "click", "x")
		if mf || ur {
			t.Fatalf("got modalFirst=%v unrouted=%v", mf, ur)
		}
	})
}
