package customcommands

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

// automationInteractionKinds are step kinds that need a live Discord
// interaction to act on. A server event has none when it fires, so these are
// only valid AFTER a wait_for on a component (button/select) or modal — the
// click/submit IS the interaction. Used before any such wait they're rejected.
var automationInteractionKinds = map[string]string{
	KindReply:      "Reply needs an interaction to respond to — it's only valid after a Wait-for on a button/select click or a modal. To message on the event itself, use Send message.",
	KindEditReply:  "Edit reply needs the interaction from a button/modal Wait-for. Use Edit message to change an existing message.",
	KindDeferReply: "Defer only applies to an interaction, which an event doesn't have. It's valid after a button/modal Wait-for.",
	KindModalOpen:  "Open modal needs an interaction to attach to — add it after a Wait-for on a button/select click.",
}

// maxAutomationWait is the wait_for / modal window cap for automations (the
// runtime clamps to this; the validator warns past it).
const maxAutomationWait = time.Minute

// ValidateAutomation type-checks an automation's step program. It reuses the
// command step validator (the same tree walker, depth limits and per-kind spec
// checks) but skips the slash-surface checks (no command name, no options) and
// enforces event semantics: interaction-only steps are valid only inside a
// component/modal wait_for continuation, and waits are capped at one minute.
func ValidateAutomation(def Definition) ValidationResult {
	return ValidateEventFlow(def, false)
}

// ValidateEventFlow validates an event-triggered step program. rootInteraction
// says whether a live Discord interaction exists at the root: false for a flow
// that runs on the bare event (an automation, or Welcome's post-message tail),
// true for a button/select click program (the click IS the interaction, so
// reply / modal_open are valid from the first step). Everything else matches
// ValidateAutomation.
func ValidateEventFlow(def Definition, rootInteraction bool) ValidationResult {
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

	// Event semantics: interaction availability + wait-window cap.
	checkAutomationSteps(def.Steps, "steps", rootInteraction, &r)

	// Only errors block; warnings (e.g. a long wait that gets clamped) are advisory.
	r.OK = true
	for _, i := range r.Issues {
		if i.Severity == "error" {
			r.OK = false
			break
		}
	}
	sort.SliceStable(r.Issues, func(i, j int) bool {
		if r.Issues[i].Severity != r.Issues[j].Severity {
			return r.Issues[i].Severity == "error"
		}
		return r.Issues[i].Path < r.Issues[j].Path
	})
	return r
}

// checkAutomationSteps walks one branch left-to-right tracking whether a live
// interaction is available. It starts false (the event has none); a
// component/modal wait_for makes it available for that branch's continuation
// (the steps after it). A wait_for's on_timeout runs without the event, so it
// resets to false; parallel branches have no shared interaction.
func checkAutomationSteps(steps []Step, basePath string, interaction bool, r *ValidationResult) {
	for i := range steps {
		s := &steps[i]
		path := fmt.Sprintf("%s[%d]", basePath, i)

		if msg, needs := automationInteractionKinds[s.Kind]; needs && !interaction {
			r.fail(path+".kind", "needs_interaction", msg)
		}

		// Editing "the command's reply" needs the slash interaction, which an
		// event lacks — only valid inside a wait_for continuation. (Editing a
		// specific message is always fine.)
		if s.Kind == KindMessageEdit && !interaction && len(s.Spec) > 0 {
			var me SpecMessageEdit
			if json.Unmarshal(s.Spec, &me) == nil && me.Target == "reply" {
				r.fail(path+".spec.target", "needs_interaction",
					"Editing the reply needs an interaction; it's only valid after a button/select/modal Wait-for. Target a specific message instead.")
			}
		}

		// Recurse into the step's children with the interaction state as it
		// stands AT this step (a branch doesn't itself create an interaction).
		checkAutomationSteps(s.Then, path+".then", interaction, r)
		checkAutomationSteps(s.Else, path+".else", interaction, r)
		checkAutomationSteps(s.Default, path+".default", interaction, r)
		for ci, cse := range s.Cases {
			checkAutomationSteps(cse.Do, fmt.Sprintf("%s.cases[%d].do", path, ci), interaction, r)
		}
		checkAutomationSteps(s.OnError, path+".on_error", interaction, r)
		for ei, ec := range s.OnErrorCases {
			checkAutomationSteps(ec.Do, fmt.Sprintf("%s.on_error_cases[%d].do", path, ei), interaction, r)
		}
		if s.Kind == KindParallel && len(s.Spec) > 0 {
			var ps SpecParallel
			if json.Unmarshal(s.Spec, &ps) == nil {
				for bi, br := range ps.Branches {
					checkAutomationSteps(br, fmt.Sprintf("%s.branches[%d]", path, bi), false, r)
				}
			}
		}
		if s.Kind == KindWaitFor && len(s.Spec) > 0 {
			var ws SpecWaitFor
			if json.Unmarshal(s.Spec, &ws) == nil {
				warnWaitWindow(ws.Timeout, path+".spec.timeout", r)
				// The timeout path runs because the event did NOT arrive — no interaction.
				checkAutomationSteps(ws.OnTimeout, path+".on_timeout", false, r)
				// A component (button/select) wait yields an interaction for the
				// continuation — the click IS the interaction. A modal submission
				// can only follow Open a form, which itself needs a prior
				// interaction, so a modal wait is only valid when one already exists.
				if ws.Trigger == "component" {
					interaction = true
				} else if ws.Trigger == "modal" && !interaction {
					r.fail(path+".spec.trigger", "modal_wait_needs_interaction",
						"Waiting for a form submission only works after Open a form, which needs a button/select Wait-for first.")
				}
			}
		}
		if s.Kind == KindWait && len(s.Spec) > 0 {
			var sw SpecWait
			if json.Unmarshal(s.Spec, &sw) == nil {
				warnWaitWindow(sw.Duration, path+".spec.duration", r)
			}
		}
	}
}

func warnWaitWindow(dur, path string, r *ValidationResult) {
	if dur == "" {
		return
	}
	d, err := time.ParseDuration(dur)
	if err != nil {
		r.fail(path, "duration_invalid", "not a valid duration (e.g. 30s, 1m): "+dur)
		return
	}
	if d > maxAutomationWait {
		r.warn(path, "wait_too_long", "automations wait at most 1 minute; this will be clamped to 1m")
	}
}
