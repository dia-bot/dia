package exec

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/store"
)

// ── Variable mutation ────────────────────────────────────────────────────────

func hSetVar(ctx context.Context, h *Halt) error {
	var spec cc.SpecSetVar
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.Name == "" {
		return errors.New("set_var: name required")
	}
	raw, err := cc.EvalJSON(ctx, spec.Value, h.Scope)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		v = string(raw)
	}
	h.Scope.Set(spec.Name, v)
	h.SetOutput(map[string]any{spec.Name: v})
	return nil
}

func hIncrVar(ctx context.Context, h *Halt) error {
	var spec cc.SpecIncrVar
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.Name == "" {
		return errors.New("incr_var: name required")
	}
	cur := toFloat(h.Scope.Get(spec.Name))
	cur += spec.By
	h.Scope.Set(spec.Name, cur)
	h.SetOutput(map[string]any{spec.Name: cur})
	return nil
}

func toFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	}
	return 0
}

// ── Terminal control ─────────────────────────────────────────────────────────

func hExit(ctx context.Context, h *Halt) error {
	var spec cc.SpecExit
	_ = decodeSpec(h.Step.Spec, &spec)
	h.SetOutput(map[string]any{"reason": spec.Reason})
	return errExit
}

func hFail(ctx context.Context, h *Halt) error {
	var spec cc.SpecFail
	_ = decodeSpec(h.Step.Spec, &spec)
	msg, _ := cc.EvalTemplated(ctx, spec.Message, h.Scope)
	if msg == "" {
		msg = "command failed"
	}
	return &errFail{msg: msg}
}

func hNoop(ctx context.Context, h *Halt) error { return nil }

// ── Audit note ───────────────────────────────────────────────────────────────

func hAuditNote(ctx context.Context, h *Halt) error {
	var spec cc.SpecAuditNote
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.Action == "" {
		return errors.New("audit_note: action required")
	}
	detail, _ := cc.EvalJSON(ctx, spec.Detail, h.Scope)
	if len(detail) == 0 {
		detail = json.RawMessage("{}")
	}
	gid, _ := event.ParseID(h.Run.GuildID)
	uid, _ := event.ParseID(h.Run.InvokerID)
	return h.Deps.Store.AppendAudit(ctx, store.AuditEntry{
		GuildID: gid, UserID: uid, Action: "ccmd." + spec.Action, Detail: detail,
	})
}

// ── Sub-command ──────────────────────────────────────────────────────────────

func hRunCommand(ctx context.Context, h *Halt) error {
	var spec cc.SpecRunCommand
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.Command == "" {
		return errors.New("run_command: command name required")
	}
	gid, _ := event.ParseID(h.Run.GuildID)
	target, err := h.Deps.Store.GetCommandByName(ctx, gid, spec.Command)
	if err != nil {
		return err
	}
	if !target.Enabled {
		return errors.New("run_command: target is disabled")
	}
	// Decode the sub-program and walk it inline against the current scope.
	var def cc.Definition
	if err := json.Unmarshal(target.Definition, &def); err != nil {
		return err
	}
	// Merge in args as new input keys (overlay).
	if len(spec.Args) > 0 {
		var args map[string]any
		if err := json.Unmarshal(spec.Args, &args); err == nil {
			for k, v := range args {
				h.Scope.Data.Input[k] = v
			}
		}
	}
	// Walk the sub-program inline against the current scope. Sub-commands
	// inherit the parent's logs, cursor and durability semantics.
	return h.Engine.walk(ctx, h.Run, h.Scope, def.Steps, "subcommand")
}
