package customcommands

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ValidationIssue is one structured problem found during publish-time validation.
type ValidationIssue struct {
	Severity string `json:"severity"` // error | warning
	Path     string `json:"path"`     // e.g. steps[2].then[0].spec.cond
	Code     string `json:"code"`
	Message  string `json:"message"`
}

// ValidationResult is the outcome of a Validate() call.
type ValidationResult struct {
	OK            bool              `json:"ok"`
	Issues        []ValidationIssue `json:"issues"`
	RequiresDefer bool              `json:"requires_defer"`
	StepCount     int               `json:"step_count"`
}

// commandNamePattern matches Discord's allowed command names (a subset that
// also keeps our routing simple).
var commandNamePattern = regexp.MustCompile(`^[a-z0-9_-]{1,32}$`)

// Validate checks the command name + definition shape and computes the
// `requires_defer` flag the runtime uses to decide whether to auto-Defer.
func Validate(name string, def Definition) ValidationResult {
	r := ValidationResult{OK: true}

	if !commandNamePattern.MatchString(name) {
		r.fail("name", "name_invalid", "name must be 1-32 chars, lowercase letters/numbers/-/_")
	}

	// Option name uniqueness + kind validation.
	seenOpts := map[string]bool{}
	for i, o := range def.Options {
		path := fmt.Sprintf("options[%d]", i)
		if !commandNamePattern.MatchString(o.Name) {
			r.fail(path+".name", "option_name_invalid", "option name must be 1-32 chars [a-z0-9_-]")
		}
		if seenOpts[o.Name] {
			r.fail(path+".name", "option_name_duplicate", "option names must be unique")
		}
		seenOpts[o.Name] = true
		if !validOptionKind(o.Kind) {
			r.fail(path+".kind", "option_kind_invalid", "unknown option kind: "+o.Kind)
			continue
		}
		validateOptionFields(o, path, &r)
	}

	// Declared variable shape + uniqueness.
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

	// Step tree.
	depth := newStackDepth()
	stepIDs := map[string]bool{}
	r.StepCount = walkSteps(def.Steps, "steps", stepIDs, depth, &r)

	// Disconnected chains: validated for shape (so reconnecting can't surprise)
	// but never executed; remind the author they're parked.
	for i, ch := range def.Scratch {
		base := fmt.Sprintf("scratch[%d]", i)
		r.StepCount += walkSteps(ch, base, stepIDs, depth, &r)
		if len(ch) > 0 {
			r.warn(base, "scratch_unreachable",
				"disconnected steps never run; reconnect them or delete them")
		}
	}

	// Latency analysis: any slow/defer kind on the worst-case path from root
	// to the first user-visible reply means we must Defer.
	r.RequiresDefer = requiresDefer(def.Steps)

	// Trigger sanity.
	for i, t := range def.Triggers {
		path := fmt.Sprintf("triggers[%d]", i)
		if t.Kind == "" {
			continue
		}
		if !validTriggerKind(t.Kind) {
			r.fail(path+".kind", "trigger_kind_invalid", "unknown trigger kind: "+t.Kind)
		}
		if t.Kind == "schedule" && t.Cron == "" {
			r.fail(path+".cron", "schedule_cron_required", "schedule triggers need a cron expression")
		}
		if t.Kind == "event" && t.Event == "" {
			r.fail(path+".event", "event_required", "event triggers need an event name")
		}
	}

	r.OK = len(r.Issues) == 0
	sort.SliceStable(r.Issues, func(i, j int) bool { return r.Issues[i].Path < r.Issues[j].Path })
	return r
}

func (r *ValidationResult) fail(path, code, msg string) {
	r.Issues = append(r.Issues, ValidationIssue{Severity: "error", Path: path, Code: code, Message: msg})
}

func (r *ValidationResult) warn(path, code, msg string) {
	r.Issues = append(r.Issues, ValidationIssue{Severity: "warning", Path: path, Code: code, Message: msg})
}

// walkSteps validates each step in a branch and recurses into control flow.
// Returns the total step count (including nested).
func walkSteps(steps []Step, basePath string, ids map[string]bool, depth *stackDepth, r *ValidationResult) int {
	if depth.enter() {
		r.fail(basePath, "depth_exceeded", "step nesting too deep (max 16 levels)")
		depth.leave()
		return 0
	}
	defer depth.leave()

	count := 0
	for i, s := range steps {
		count++
		path := fmt.Sprintf("%s[%d]", basePath, i)
		if s.Kind == "" {
			r.fail(path+".kind", "kind_required", "step kind is required")
			continue
		}
		if !validStepKind(s.Kind) {
			r.fail(path+".kind", "kind_unknown", "unknown step kind: "+s.Kind)
			continue
		}
		if s.ID == "" {
			r.warn(path+".id", "id_missing", "step is missing a stable id (the editor should assign one)")
		} else if ids[s.ID] {
			r.fail(path+".id", "id_duplicate", "duplicate step id: "+s.ID)
		} else {
			ids[s.ID] = true
		}
		validateSpec(s, path, r)

		if IsControl(s.Kind) {
			switch s.Kind {
			case KindIf:
				count += walkSteps(s.Then, path+".then", ids, depth, r)
				count += walkSteps(s.Else, path+".else", ids, depth, r)
			case KindSwitch:
				for ci, c := range s.Cases {
					cp := fmt.Sprintf("%s.cases[%d]", path, ci)
					count += walkSteps(c.Do, cp+".do", ids, depth, r)
				}
				count += walkSteps(s.Default, path+".default", ids, depth, r)
			case KindLoop:
				count += walkSteps(s.Then, path+".then", ids, depth, r)
			case KindParallel:
				var spec SpecParallel
				if len(s.Spec) > 0 {
					_ = json.Unmarshal(s.Spec, &spec)
				}
				for bi, br := range spec.Branches {
					count += walkSteps(br, fmt.Sprintf("%s.branches[%d]", path, bi), ids, depth, r)
				}
			}
		}
		if len(s.OnError) > 0 {
			count += walkSteps(s.OnError, path+".on_error", ids, depth, r)
		}
		// Typed error cases. Each arm's `When` is a list of segment-glob
		// patterns over kind strings (e.g. discord.*, *.timeout, …).
		for ci, ec := range s.OnErrorCases {
			cp := fmt.Sprintf("%s.on_error_cases[%d]", path, ci)
			if len(ec.When) == 0 {
				r.fail(cp+".when", "error_case_when_empty",
					"error case needs at least one kind pattern")
			}
			for wi, w := range ec.When {
				if !validKindPattern(w) {
					r.fail(fmt.Sprintf("%s.when[%d]", cp, wi),
						"error_case_pattern_invalid",
						"kind pattern must be `*`, `group.*`, or a dotted code")
				}
			}
			count += walkSteps(ec.Do, cp+".do", ids, depth, r)
		}
		// wait_for on_timeout is part of its spec, walked here.
		if s.Kind == KindWaitFor && len(s.Spec) > 0 {
			var spec SpecWaitFor
			_ = json.Unmarshal(s.Spec, &spec)
			if len(spec.OnTimeout) > 0 {
				count += walkSteps(spec.OnTimeout, path+".spec.on_timeout", ids, depth, r)
			}
		}
	}
	return count
}

// exprEmpty reports whether an Expr carries neither a template nor a literal.
func exprEmpty(e Expr) bool {
	return strings.TrimSpace(e.Src) == "" && len(e.Value) == 0
}

// requireExpr fails validation when a required Expr field is empty — these
// were previously runtime-only failures ("x required") on published commands.
func requireExpr(e Expr, path, field, kind string, r *ValidationResult) {
	if exprEmpty(e) {
		r.fail(path+".spec."+field, field+"_required", kind+" needs a "+field)
	}
}

func requireStr(v, path, field, kind string, r *ValidationResult) {
	if strings.TrimSpace(v) == "" {
		r.fail(path+".spec."+field, field+"_required", kind+" needs a "+field)
	}
}

// validateSpec runs per-kind shape checks on the Spec JSON.
func validateSpec(s Step, path string, r *ValidationResult) {
	switch s.Kind {
	case KindReply, KindEditReply:
		var spec SpecReply
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		if spec.Content == "" && len(spec.Embeds) == 0 && len(spec.Components) == 0 && len(spec.Attachments) == 0 {
			r.warn(path+".spec", "reply_empty", "this reply sends nothing — add content, an embed or a component")
		}
	case KindSendMessage:
		var spec SpecSendMessage
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
	case KindSendDM:
		var spec SpecSendDM
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.User, path, "user", s.Kind, r)
	case KindEmbedSend:
		var spec SpecEmbedSend
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
	case KindMessageEdit:
		var spec SpecMessageEdit
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		if spec.Target != "" && spec.Target != "reply" {
			r.fail(path+".spec.target", "target_invalid", "message_edit target must be empty or \"reply\"")
		}
		if spec.Target != "reply" {
			requireExpr(spec.Channel, path, "channel", s.Kind, r)
			requireExpr(spec.Message, path, "message", s.Kind, r)
		}
	case KindMessageFetch:
		var spec SpecMessageFetch
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		requireExpr(spec.Message, path, "message", s.Kind, r)
		requireStr(spec.Into, path, "into", s.Kind, r)
	case KindMessagePurge:
		var spec SpecMessagePurge
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		if spec.Limit < 0 || spec.Limit > 100 {
			r.fail(path+".spec.limit", "limit_range", "purge limit must be 1..100")
		}
	case KindMessageCrosspost:
		var spec SpecMessageCrosspost
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		requireExpr(spec.Message, path, "message", s.Kind, r)
	case KindMessageDelete, KindPinAdd, KindPinRemove:
		var spec SpecMessageOp
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		requireExpr(spec.Message, path, "message", s.Kind, r)
	case KindReactAdd, KindReactRemove:
		var spec SpecReact
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		requireExpr(spec.Message, path, "message", s.Kind, r)
		requireStr(spec.Emoji, path, "emoji", s.Kind, r)
	case KindReactClear:
		var spec SpecReactClear
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		requireExpr(spec.Message, path, "message", s.Kind, r)
	case KindRoleAdd, KindRoleRemove:
		var spec SpecRole
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.User, path, "user", s.Kind, r)
		requireExpr(spec.Role, path, "role", s.Kind, r)
	case KindMemberNickname, KindMemberKick, KindMemberBan, KindMemberUnban:
		var spec SpecMember
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.User, path, "user", s.Kind, r)
	case KindMemberFetch:
		var spec SpecMemberFetch
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.User, path, "user", s.Kind, r)
		requireStr(spec.Into, path, "into", s.Kind, r)
	case KindChannelCreate:
		var spec SpecChannelCreate
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireStr(spec.Name, path, "name", s.Kind, r)
	case KindChannelEdit:
		var spec SpecChannelEdit
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
	case KindChannelDelete:
		var spec SpecChannelDelete
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
	case KindThreadCreate:
		var spec SpecThreadCreate
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		requireStr(spec.Name, path, "name", s.Kind, r)
	case KindThreadArchive:
		var spec SpecThreadArchive
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Thread, path, "thread", s.Kind, r)
	case KindThreadMember:
		var spec SpecThreadMember
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Thread, path, "thread", s.Kind, r)
		requireExpr(spec.User, path, "user", s.Kind, r)
		if spec.Action != "" && spec.Action != "add" && spec.Action != "remove" {
			r.fail(path+".spec.action", "action_invalid", "thread_member action must be add or remove")
		}
	case KindInviteCreate:
		var spec SpecInviteCreate
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Channel, path, "channel", s.Kind, r)
		if spec.MaxAge != "" {
			if _, err := time.ParseDuration(spec.MaxAge); err != nil {
				r.fail(path+".spec.max_age", "duration_invalid", err.Error())
			}
		}
	case KindVoiceMove:
		var spec SpecVoiceMove
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.User, path, "user", s.Kind, r)
	case KindVoiceSet:
		var spec SpecVoiceSet
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.User, path, "user", s.Kind, r)
		if spec.Mute == nil && spec.Deafen == nil {
			r.fail(path+".spec", "voice_set_noop", "voice_set needs mute and/or deafen")
		}
	case KindModalOpen:
		var spec SpecModalOpen
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireStr(spec.Title, path, "title", s.Kind, r)
		requireStr(spec.CustomIDSuffix, path, "custom_id_suffix", s.Kind, r)
		if len(spec.Fields) == 0 {
			r.fail(path+".spec.fields", "fields_required", "modal_open needs at least one field")
		}
		for fi, f := range spec.Fields {
			if strings.TrimSpace(f.Label) == "" || strings.TrimSpace(f.CustomID) == "" {
				r.fail(fmt.Sprintf("%s.spec.fields[%d]", path, fi), "field_invalid",
					"modal fields need a label and a custom_id")
			}
		}
	case KindImageAttach:
		var spec SpecImageAttach
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireStr(spec.FromVar, path, "from_var", s.Kind, r)
	case KindImageLoad:
		var spec SpecImageLoad
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Source, path, "source", s.Kind, r)
		requireStr(spec.Into, path, "into", s.Kind, r)
	case KindPickRandom:
		var spec SpecPickRandom
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.From, path, "from", s.Kind, r)
		requireStr(spec.Into, path, "into", s.Kind, r)
	case KindJSONParse:
		var spec SpecJSONParse
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireExpr(spec.Value, path, "value", s.Kind, r)
		requireStr(spec.Into, path, "into", s.Kind, r)
	case KindRunCommand:
		var spec SpecRunCommand
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireStr(spec.Command, path, "command", s.Kind, r)
	case KindAuditNote:
		var spec SpecAuditNote
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		requireStr(spec.Action, path, "action", s.Kind, r)
	case KindWait:
		var spec SpecWait
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", "wait spec: "+err.Error())
			return
		}
		if d, err := time.ParseDuration(spec.Duration); err != nil {
			r.fail(path+".spec.duration", "duration_invalid", "duration must be a Go duration (e.g. 10s, 30s): "+err.Error())
		} else if d > time.Minute {
			r.fail(path+".spec.duration", "wait_too_long", "wait is capped at 1 minute")
		}
	case KindWaitFor:
		var spec SpecWaitFor
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", "wait_for spec: "+err.Error())
			return
		}
		if spec.Timeout != "" {
			if d, err := time.ParseDuration(spec.Timeout); err != nil {
				r.fail(path+".spec.timeout", "duration_invalid", err.Error())
			} else if d > 24*time.Hour {
				r.fail(path+".spec.timeout", "timeout_too_long", "wait_for timeout is capped at 24h")
			}
		}
		if spec.Trigger == "" {
			r.fail(path+".spec.trigger", "trigger_required", "wait_for needs a trigger kind")
		}
	case KindIf:
		var spec SpecIf
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", "if spec: "+err.Error())
		}
	case KindSwitch:
		var spec SpecSwitch
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", "switch spec: "+err.Error())
		}
	case KindLoop:
		var spec SpecLoop
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", "loop spec: "+err.Error())
			return
		}
		if spec.As == "" {
			r.fail(path+".spec.as", "loop_as_required", "loop needs an 'as' variable name")
		}
		if spec.MaxIter > 1000 {
			r.fail(path+".spec.max_iter", "loop_max_too_big", "max_iter cannot exceed 1000")
		}
	case KindSetVar:
		var spec SpecSetVar
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", "set_var spec: "+err.Error())
		} else if spec.Name == "" {
			r.fail(path+".spec.name", "var_name_required", "set_var needs a name")
		}
	case KindIncrVar:
		var spec SpecIncrVar
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", "incr_var spec: "+err.Error())
		} else if spec.Name == "" {
			r.fail(path+".spec.name", "var_name_required", "incr_var needs a name")
		}
	case KindMemberTimeout:
		var spec SpecMember
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		if spec.Duration != "" {
			if _, err := time.ParseDuration(spec.Duration); err != nil {
				r.fail(path+".spec.duration", "duration_invalid", err.Error())
			}
		}
	case KindHTTPReq:
		var spec SpecHTTP
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		if !strings.HasPrefix(spec.URL, "http://") && !strings.HasPrefix(spec.URL, "https://") && !strings.Contains(spec.URL, "{{") {
			r.warn(path+".spec.url", "url_suspicious", "URL doesn't look like an http(s) URL and isn't templated")
		}
	case KindImageRender:
		var spec SpecImageRender
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		if spec.TemplateID == 0 {
			r.fail(path+".spec.template_id", "template_required", "image_render needs a template_id")
		}
		if spec.Into == "" {
			r.fail(path+".spec.into", "into_required", "image_render needs an 'into' variable name")
		}
	case KindKVGet, KindKVSet, KindKVDelete:
		var spec SpecKV
		if err := decodeSpec(s.Spec, &spec); err != nil {
			r.fail(path+".spec", "spec_invalid", err.Error())
			return
		}
		if spec.Scope != "guild" && spec.Scope != "member" && spec.Scope != "" {
			r.fail(path+".spec.scope", "scope_invalid", "kv scope must be guild or member")
		}
		if spec.TTL != "" {
			if _, err := time.ParseDuration(spec.TTL); err != nil {
				r.fail(path+".spec.ttl", "ttl_invalid", err.Error())
			}
		}
	}
}

// decodeSpec unmarshals a step spec into v, returning a friendly error.
func decodeSpec(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// requiresDefer returns true when the worst-case path from root to the first
// user-visible reply has any slow/defer step on it. We treat the program as
// "must defer" when the first reply-shaped step is preceded by anything that
// isn't instant — including across `then`/`else`/`cases` branches.
func requiresDefer(steps []Step) bool {
	hadSlow := false
	return walkLatency(steps, &hadSlow)
}

// walkLatency walks the tree depth-first; returns true once the analysis can
// conclude that a defer is required (a slow step seen before a reply, OR a
// reply step never reached at all but slow steps present at all).
func walkLatency(steps []Step, hadSlow *bool) bool {
	for _, s := range steps {
		// Reply checks come FIRST: a step's own latency must not force a
		// defer before itself (modal_open is both slow and a reply — deferring
		// for it would make the modal impossible to open).
		if s.Kind == KindDeferReply {
			return false // explicit defer handles it
		}
		if IsUserVisibleReply(s.Kind) {
			return *hadSlow
		}
		switch LatencyOf(s.Kind) {
		case LatencySlow, LatencyDefer:
			*hadSlow = true
		}
		// Recurse: if EITHER branch contains a reply preceded by slow ops, mark.
		switch s.Kind {
		case KindIf:
			localHad := *hadSlow
			localThen := localHad
			localElse := localHad
			if walkLatency(s.Then, &localThen) {
				return true
			}
			if walkLatency(s.Else, &localElse) {
				return true
			}
			if localThen || localElse {
				*hadSlow = true
			}
		case KindSwitch:
			for _, c := range s.Cases {
				local := *hadSlow
				if walkLatency(c.Do, &local) {
					return true
				}
				if local {
					*hadSlow = true
				}
			}
			local := *hadSlow
			if walkLatency(s.Default, &local) {
				return true
			}
			if local {
				*hadSlow = true
			}
		case KindLoop:
			local := *hadSlow
			if walkLatency(s.Then, &local) {
				return true
			}
			if local {
				*hadSlow = true
			}
		case KindParallel:
			var spec SpecParallel
			_ = json.Unmarshal(s.Spec, &spec)
			for _, br := range spec.Branches {
				local := *hadSlow
				if walkLatency(br, &local) {
					return true
				}
				if local {
					*hadSlow = true
				}
			}
		}
	}
	return false
}

func validOptionKind(s string) bool {
	switch s {
	case "string", "int", "integer", "bool", "boolean", "user", "role", "channel",
		"mentionable", "attachment", "number":
		return true
	}
	return false
}

// optionKindBucket coarsely groups option kinds so we can validate which
// per-type fields are legal: "string", "numeric", "channel", or "other".
func optionKindBucket(k string) string {
	switch k {
	case "string":
		return "string"
	case "int", "integer", "number":
		return "numeric"
	case "channel":
		return "channel"
	}
	return "other"
}

// validChannelType is the union of Discord's documented guild channel type IDs.
// (Forum/media/stage included; DM-only types excluded since slash commands
// run in a guild context.)
func validChannelType(t int) bool {
	switch t {
	case 0, 2, 4, 5, 10, 11, 12, 13, 15, 16:
		return true
	}
	return false
}

// validateOptionFields enforces per-type rules for slash-arg field combos:
// numeric bounds only on numeric kinds, length bounds only on strings,
// channel_types only on channels, autocomplete + choices mutually exclusive.
// Discord rejects mismatched combos at registration time — we catch them
// pre-publish so the user sees a useful error instead of a silent sync failure.
func validateOptionFields(o CommandOption, path string, r *ValidationResult) {
	bucket := optionKindBucket(o.Kind)

	if len(o.Description) > 100 {
		r.fail(path+".description", "option_description_too_long",
			"option description must be 1-100 chars (Discord limit)")
	}

	switch bucket {
	case "numeric":
		if o.MinLength != nil || o.MaxLength != nil {
			r.fail(path+".min_length", "option_length_on_numeric",
				"min/max length only apply to string options")
		}
		if o.MinValue != nil && o.MaxValue != nil && *o.MinValue > *o.MaxValue {
			r.fail(path+".min_value", "option_min_above_max",
				"min_value cannot exceed max_value")
		}
		if len(o.ChannelTypes) > 0 {
			r.fail(path+".channel_types", "option_channel_types_on_non_channel",
				"channel_types only apply to channel options")
		}
	case "string":
		if o.MinValue != nil || o.MaxValue != nil {
			r.fail(path+".min_value", "option_value_on_string",
				"min/max value only apply to int/number options")
		}
		if o.MinLength != nil && *o.MinLength < 0 {
			r.fail(path+".min_length", "option_min_length_negative",
				"min_length must be >= 0")
		}
		if o.MaxLength != nil && (*o.MaxLength < 1 || *o.MaxLength > 6000) {
			r.fail(path+".max_length", "option_max_length_range",
				"max_length must be 1..6000")
		}
		if o.MinLength != nil && o.MaxLength != nil && *o.MinLength > *o.MaxLength {
			r.fail(path+".min_length", "option_min_above_max",
				"min_length cannot exceed max_length")
		}
		if len(o.ChannelTypes) > 0 {
			r.fail(path+".channel_types", "option_channel_types_on_non_channel",
				"channel_types only apply to channel options")
		}
	case "channel":
		if o.MinValue != nil || o.MaxValue != nil || o.MinLength != nil || o.MaxLength != nil {
			r.fail(path+".min_value", "option_bounds_on_channel",
				"value / length bounds do not apply to channel options")
		}
		if o.Autocomplete {
			r.fail(path+".autocomplete", "option_autocomplete_on_non_text",
				"autocomplete only applies to string/int/number options")
		}
		for ci, ct := range o.ChannelTypes {
			if !validChannelType(ct) {
				r.fail(fmt.Sprintf("%s.channel_types[%d]", path, ci), "option_channel_type_invalid",
					fmt.Sprintf("unknown Discord channel type %d", ct))
			}
		}
	default:
		if o.MinValue != nil || o.MaxValue != nil || o.MinLength != nil || o.MaxLength != nil ||
			len(o.ChannelTypes) > 0 || o.Autocomplete {
			r.fail(path, "option_constraints_unsupported",
				"this option kind does not accept bounds / choices / autocomplete")
		}
	}

	// Autocomplete is text-input-only and mutually exclusive with Choices.
	if o.Autocomplete && bucket != "string" && bucket != "numeric" {
		r.fail(path+".autocomplete", "option_autocomplete_on_non_text",
			"autocomplete only applies to string/int/number options")
	}
	if o.Autocomplete && len(o.Choices) > 0 {
		r.fail(path+".choices", "option_choices_with_autocomplete",
			"choices and autocomplete are mutually exclusive (Discord rejects both)")
	}
	if len(o.Choices) > 25 {
		r.fail(path+".choices", "option_too_many_choices",
			"at most 25 choices per option")
	}
	for ci, c := range o.Choices {
		cp := fmt.Sprintf("%s.choices[%d]", path, ci)
		if c.Name == "" || len(c.Name) > 100 {
			r.fail(cp+".name", "option_choice_name_invalid",
				"choice name must be 1-100 chars")
		}
		if len(c.Value) == 0 {
			r.fail(cp+".value", "option_choice_value_required",
				"choice value is required")
		}
	}
}

// validKindPattern accepts `*`, exact dotted codes (`discord.permission_denied`),
// segment globs (`discord.*`, `*.timeout`, `*.*`). It deliberately rejects
// substring globs like `discord*` to keep matching unambiguous.
func validKindPattern(s string) bool {
	if s == "" {
		return false
	}
	if s == "*" {
		return true
	}
	for _, seg := range strings.Split(s, ".") {
		if seg == "" {
			return false
		}
		if seg == "*" {
			continue
		}
		for _, r := range seg {
			if r >= 'a' && r <= 'z' {
				continue
			}
			if r >= '0' && r <= '9' {
				continue
			}
			if r == '_' || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func validVarType(s string) bool {
	switch s {
	case "string", "int", "float", "bool", "list", "object":
		return true
	}
	return false
}

func validTriggerKind(s string) bool {
	switch s {
	case "slash", "component", "modal", "event", "schedule":
		return true
	}
	return false
}

func validStepKind(s string) bool {
	switch s {
	case KindDeferReply, KindReply, KindEditReply, KindSendMessage, KindSendDM,
		KindEmbedSend, KindModalOpen, KindMessageEdit, KindMessageFetch,
		KindMessageDelete, KindMessagePurge, KindMessageCrosspost,
		KindReactAdd, KindReactRemove, KindReactClear, KindPinAdd, KindPinRemove,
		KindRoleAdd, KindRoleRemove, KindMemberNickname,
		KindMemberKick, KindMemberBan, KindMemberUnban, KindMemberTimeout,
		KindMemberFetch,
		KindChannelCreate, KindChannelEdit, KindChannelDelete,
		KindThreadCreate, KindThreadArchive, KindThreadMember, KindInviteCreate,
		KindVoiceMove, KindVoiceSet,
		KindImageRender, KindImageAttach, KindImageLoad,
		KindSetVar, KindIncrVar, KindPickRandom, KindJSONParse,
		KindKVGet, KindKVSet, KindKVDelete, KindHTTPReq,
		KindIf, KindSwitch, KindLoop, KindParallel, KindWait, KindWaitFor,
		KindExit, KindFail, KindNoop, KindRunCommand, KindAuditNote:
		return true
	}
	return false
}

type stackDepth struct {
	depth int
	max   int
}

func newStackDepth() *stackDepth { return &stackDepth{max: 16} }

// enter increments depth and returns true when the cap is exceeded.
func (s *stackDepth) enter() bool {
	s.depth++
	return s.depth > s.max
}
func (s *stackDepth) leave() { s.depth-- }
