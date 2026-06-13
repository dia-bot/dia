package automations

import (
	"strings"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// Validate checks an automation end to end: a non-empty name, a known trigger,
// trigger-config sanity, and the step program (via the shared automation step
// validator, which forbids interaction-only steps). The returned result reuses
// the custom-command ValidationResult shape so the dashboard renders issues the
// same way it does for commands.
func Validate(name, triggerType string, cfg TriggerConfig, def cc.Definition) cc.ValidationResult {
	r := cc.ValidateAutomation(def)

	if strings.TrimSpace(name) == "" {
		addIssue(&r, "error", "name", "name_required", "give the automation a name")
	} else if len(name) > 80 {
		addIssue(&r, "error", "name", "name_too_long", "name must be 80 characters or fewer")
	}

	tk, ok := TriggerByKey(triggerType)
	if !ok {
		addIssue(&r, "error", "trigger_type", "trigger_invalid", "unknown trigger: "+triggerType)
		r.OK = len(errorsIn(r.Issues)) == 0
		return r
	}

	// Filter sanity, keyed on what the trigger actually supports.
	supports := func(f Filter) bool {
		for _, x := range tk.Filters {
			if x == f {
				return true
			}
		}
		return false
	}
	if supports(FilterRole) && cfg.Role == "" {
		addIssue(&r, "warning", "trigger_config.role", "role_unset",
			"no role chosen — this fires on any role change; pick a role to scope it")
	}
	if supports(FilterKeywords) && len(cfg.Keywords) > 0 && cfg.MatchMode == "regex" {
		// Regex matching isn't supported by the runtime (kept simple/safe).
		addIssue(&r, "error", "trigger_config.match_mode", "match_mode_unsupported",
			"regex matching isn't available; use contains, equals or word")
	}
	if cfg.Cooldown != nil && cfg.Cooldown.Seconds < 0 {
		addIssue(&r, "error", "trigger_config.cooldown", "cooldown_invalid",
			"cooldown seconds must be zero or positive")
	}

	r.OK = len(errorsIn(r.Issues)) == 0
	return r
}

func addIssue(r *cc.ValidationResult, severity, path, code, msg string) {
	r.Issues = append(r.Issues, cc.ValidationIssue{Severity: severity, Path: path, Code: code, Message: msg})
}

func errorsIn(issues []cc.ValidationIssue) []cc.ValidationIssue {
	var out []cc.ValidationIssue
	for _, i := range issues {
		if i.Severity == "error" {
			out = append(out, i)
		}
	}
	return out
}
