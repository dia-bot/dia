package customcommands

import (
	"encoding/json"
	"fmt"
	"sort"
)

// automationForbiddenKinds are interaction-only step kinds: a server event has
// no interaction to respond to, so these can never run inside an automation.
// The author is steered to the channel-message equivalents instead.
var automationForbiddenKinds = map[string]string{
	KindReply:      "Reply needs a slash interaction — use Send message in an automation",
	KindEditReply:  "Edit reply needs a slash interaction — use Edit message in an automation",
	KindDeferReply: "Defer needs a slash interaction and has no effect in an automation",
	KindModalOpen:  "Modals need a slash or component interaction — not available from an event trigger",
}

// ValidateAutomation type-checks an automation's step program. It reuses the
// command step validator (the same tree walker, depth limits and per-kind spec
// checks) but skips the slash-surface checks (no command name, no options) and
// forbids interaction-only steps that have nothing to act on when an event
// fires. The returned RequiresDefer is always false (automations never defer).
func ValidateAutomation(def Definition) ValidationResult {
	r := ValidationResult{OK: true}

	// Declared variable shape + uniqueness — identical to the command path.
	seenVars := map[string]bool{}
	for i, v := range def.Variables {
		path := fmt.Sprintf("variables[%d]", i)
		if v.Name == "" {
			r.fail(path+".name", "var_name_empty", "variable name required")
			continue
		}
		if seenVars[v.Name] {
			r.fail(path+".name", "var_name_duplicate", "variable names must be unique")
		}
		seenVars[v.Name] = true
		if !validVarType(v.Type) {
			r.fail(path+".type", "var_type_invalid", "unknown variable type: "+v.Type)
		}
	}

	depth := newStackDepth()
	stepIDs := map[string]bool{}
	r.StepCount = walkSteps(def.Steps, "steps", stepIDs, depth, &r)

	// Disconnected chains: validated for shape, flagged as unreachable.
	for i, ch := range def.Scratch {
		base := fmt.Sprintf("scratch[%d]", i)
		r.StepCount += walkSteps(ch, base, stepIDs, depth, &r)
		if len(ch) > 0 {
			r.warn(base, "scratch_unreachable",
				"disconnected steps never run; reconnect them or delete them")
		}
	}

	// Forbid interaction-only kinds anywhere in the executed tree.
	forbidInteractionSteps(def.Steps, "steps", &r)

	r.OK = len(r.Issues) == 0
	sort.SliceStable(r.Issues, func(i, j int) bool {
		if r.Issues[i].Severity != r.Issues[j].Severity {
			return r.Issues[i].Severity == "error"
		}
		return r.Issues[i].Path < r.Issues[j].Path
	})
	return r
}

// forbidInteractionSteps walks the full step tree and fails on any
// interaction-only kind, recursing through every control branch.
func forbidInteractionSteps(steps []Step, basePath string, r *ValidationResult) {
	for i := range steps {
		s := &steps[i]
		path := fmt.Sprintf("%s[%d]", basePath, i)
		if msg, bad := automationForbiddenKinds[s.Kind]; bad {
			r.fail(path+".kind", "kind_not_in_automation", msg)
		}
		forbidInteractionSteps(s.Then, path+".then", r)
		forbidInteractionSteps(s.Else, path+".else", r)
		forbidInteractionSteps(s.Default, path+".default", r)
		forbidInteractionSteps(s.OnError, path+".on_error", r)
		for ci, cse := range s.Cases {
			forbidInteractionSteps(cse.Do, fmt.Sprintf("%s.cases[%d].do", path, ci), r)
		}
		for ei, ec := range s.OnErrorCases {
			forbidInteractionSteps(ec.Do, fmt.Sprintf("%s.on_error_cases[%d].do", path, ei), r)
		}
		if s.Kind == KindParallel && len(s.Spec) > 0 {
			var ps SpecParallel
			if json.Unmarshal(s.Spec, &ps) == nil {
				for bi, br := range ps.Branches {
					forbidInteractionSteps(br, fmt.Sprintf("%s.branches[%d]", path, bi), r)
				}
			}
		}
		if s.Kind == KindWaitFor && len(s.Spec) > 0 {
			var ws SpecWaitFor
			if json.Unmarshal(s.Spec, &ws) == nil {
				forbidInteractionSteps(ws.OnTimeout, path+".on_timeout", r)
			}
		}
	}
}
