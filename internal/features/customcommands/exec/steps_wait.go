package exec

import (
	"context"
	"errors"
	"time"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// hWait suspends the run for a fixed duration. The walker catches the
// PauseError and persists scope+cursor; the scheduler resumes when due.
func hWait(ctx context.Context, h *Halt) error {
	var spec cc.SpecWait
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	d, err := time.ParseDuration(spec.Duration)
	if err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	// Hard cap regardless of what the stored spec says: public bot, no
	// long pauses.
	if d > time.Minute {
		d = time.Minute
	}
	resume := time.Now().Add(d)
	h.Run.markDurable()
	h.SetOutput(map[string]any{"resume_at": resume})
	return &PauseError{Kind: "wait", ResumeAt: &resume}
}

// hWaitFor parks the run until a Discord event matches. The full custom_id is
// "ccmd:<run_id>:<suffix>" (or just the suffix for messages/reactions); the
// router intercepts matching components before falling through.
func hWaitFor(ctx context.Context, h *Halt) error {
	var spec cc.SpecWaitFor
	if err := decodeSpec(h.Step.Spec, &spec); err != nil {
		return err
	}
	if spec.Trigger == "" {
		return errors.New("wait_for: trigger required")
	}
	timeout := 10 * time.Minute
	if spec.Timeout != "" {
		if d, err := time.ParseDuration(spec.Timeout); err == nil && d > 0 {
			timeout = d
		}
	}
	// Hard cap: the interaction token itself dies after ~15 minutes, so a
	// longer park can never be answered anyway.
	if timeout > 10*time.Minute {
		timeout = 10 * time.Minute
	}
	resume := time.Now().Add(timeout)

	customID := ""
	if spec.Trigger == "component" || spec.Trigger == "modal" {
		// We route via prefix match. Storing the run-id-bearing prefix means
		// the router can match the suffix the admin set. The suffix is a
		// template (e.g. vote_{{ .Vars.idx }}) so per-item buttons inside a
		// loop each get their own wait; the run id already isolates users.
		customID = h.Engine.routePrefix + h.Run.ID
		if spec.CustomIDSuffix != "" {
			customID += ":" + templated(ctx, h, spec.CustomIDSuffix)
		}
	}

	awaitingUserID := ""
	if id, err := cc.EvalSnowflake(ctx, spec.FromUser, h.Scope); err == nil {
		awaitingUserID = id
	}
	uid, _ := event.ParseID(awaitingUserID)

	h.Run.markDurable()
	h.SetOutput(map[string]any{
		"trigger":   spec.Trigger,
		"timeout":   resume,
		"custom_id": customID,
		"from_user": awaitingUserID,
	})
	return &PauseError{
		Kind:             "wait_for",
		ResumeAt:         &resume,
		AwaitingCustomID: customID,
		AwaitingUserID:   formatID(uid),
		AwaitingKind:     spec.Trigger,
	}
}

func formatID(n int64) string {
	if n == 0 {
		return ""
	}
	return event.FormatID(n)
}
