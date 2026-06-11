package exec

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// KindedError tags a step failure with a stable, well-known code so that
// command authors can dispatch on it from on_error_cases (e.g. retry only
// on `discord.rate_limited`, surface a custom message on
// `discord.permission_denied`).
//
// The code space is intentionally hierarchical with `.` as the separator
// (`discord.permission_denied`, `http.timeout`, `template.parse`) so case
// patterns can match `discord.*` to handle anything Discord-side.
type KindedError struct {
	Kind      string
	Msg       string
	Retryable bool
	Wrapped   error
}

// Error returns a human-readable form: "<msg> (<kind>)".
func (e *KindedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Msg != "" {
		return e.Msg + " (" + e.Kind + ")"
	}
	if e.Wrapped != nil {
		return e.Wrapped.Error() + " (" + e.Kind + ")"
	}
	return e.Kind
}

// Unwrap exposes the wrapped cause so errors.Is / errors.As keep working
// across the kinded layer.
func (e *KindedError) Unwrap() error { return e.Wrapped }

// Kinded constructs a typed error with no wrapped cause.
func Kinded(kind, msg string) *KindedError {
	return &KindedError{Kind: kind, Msg: msg}
}

// Kindedf is the printf variant of Kinded.
func Kindedf(kind, format string, args ...any) *KindedError {
	return &KindedError{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}

// KindedWrap wraps a cause with a typed kind (and inherits its message
// when none is given).
func KindedWrap(kind string, cause error) *KindedError {
	if cause == nil {
		return nil
	}
	return &KindedError{Kind: kind, Msg: cause.Error(), Wrapped: cause}
}

// KindOf walks an error chain and returns a stable kind string.
// Resolution order:
//  1. If any error in the chain is a *KindedError, return its Kind.
//  2. Recognise external error types (discordgo REST/rate-limit, context
//     deadline, plain net errors) and map them to the canonical code.
//  3. Fall back to "runtime.unknown" — the catch-all the `*` pattern
//     matches.
func KindOf(err error) string {
	if err == nil {
		return ""
	}
	var k *KindedError
	if errors.As(err, &k) && k != nil && k.Kind != "" {
		return k.Kind
	}

	// Discord rate-limit error: typed in discordgo.
	var rl *discordgo.RateLimitError
	if errors.As(err, &rl) {
		return "discord.rate_limited"
	}
	// Discord REST error: classify by HTTP status.
	var rest *discordgo.RESTError
	if errors.As(err, &rest) && rest != nil && rest.Response != nil {
		return classifyDiscordStatus(rest.Response.StatusCode, rest.ResponseBody)
	}

	// Deadline / cancellation — treat as a timeout (the action budget &
	// run wall-clock both end up here).
	if errors.Is(err, context.DeadlineExceeded) {
		return "runtime.timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "runtime.canceled"
	}

	// Plain HTTP / network errors from the http_request step.
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "http.timeout"
		}
		return "http.connection"
	}

	// Heuristic fallbacks for unwrapped string-only errors. Cheap, but
	// catches the common cases where a handler returned fmt.Errorf("…")
	// without going through Kinded.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "template:") || strings.Contains(msg, "parse error"):
		return "template.parse"
	case strings.Contains(msg, "action limit reached"):
		return "runtime.budget_exceeded"
	case strings.Contains(msg, "step budget exceeded"):
		return "runtime.budget_exceeded"
	case strings.Contains(msg, "timeout"):
		return "runtime.timeout"
	}
	return "runtime.unknown"
}

// classifyDiscordStatus maps an HTTP status from a Discord REST call into
// our canonical kind. The body is inspected to distinguish "unknown user"
// from "unknown role" on 404s — Discord uses an embedded code field.
func classifyDiscordStatus(status int, body []byte) string {
	switch {
	case status == 401:
		return "discord.unauthorized"
	case status == 403:
		return "discord.permission_denied"
	case status == 404:
		// Discord 404s include `"code": 10003` style errors that say what
		// is missing. Cheap substring sniff is enough.
		b := strings.ToLower(string(body))
		switch {
		case strings.Contains(b, "unknown channel"):
			return "discord.unknown_channel"
		case strings.Contains(b, "unknown role"):
			return "discord.unknown_role"
		case strings.Contains(b, "unknown user"):
			return "discord.unknown_user"
		case strings.Contains(b, "unknown member"):
			return "discord.unknown_user"
		case strings.Contains(b, "unknown message"):
			return "discord.unknown_message"
		}
		return "discord.unknown_resource"
	case status == 429:
		return "discord.rate_limited"
	case status >= 500:
		return "discord.unavailable"
	case status >= 400:
		return "discord.bad_request"
	}
	return "discord.unknown"
}

// MatchKind reports whether `kind` matches `pattern`. Patterns:
//
//   - — match anything
//     discord.*                — match any kind in the discord group
//     *.timeout                — match any kind ending in .timeout
//     discord.permission_denied — exact match
//
// The matcher is intentionally tiny — segment-level glob, no regexp.
func MatchKind(kind, pattern string) bool {
	if pattern == "" || pattern == "*" {
		return true
	}
	if pattern == kind {
		return true
	}
	// Split on dots and walk segment-by-segment so we don't accidentally
	// match across the hierarchy boundary.
	ks := strings.Split(kind, ".")
	ps := strings.Split(pattern, ".")
	// Allow trailing `.*` to match any further segments.
	if len(ps) > 0 && ps[len(ps)-1] == "*" {
		if len(ks) < len(ps)-1 {
			return false
		}
		for i := 0; i < len(ps)-1; i++ {
			if ps[i] != "*" && ps[i] != ks[i] {
				return false
			}
		}
		return true
	}
	if len(ks) != len(ps) {
		return false
	}
	for i, p := range ps {
		if p != "*" && p != ks[i] {
			return false
		}
	}
	return true
}

// ErrorInfoFrom builds a cc.ErrorInfo from a step + underlying error.
// The cc-package type is what Scope carries so on_error templates can
// read `.Error.Kind` etc. without an exec-package import.
func ErrorInfoFrom(stepKind, stepID string, err error) cc.ErrorInfo {
	if err == nil {
		return cc.ErrorInfo{}
	}
	info := cc.ErrorInfo{
		Kind:    KindOf(err),
		Message: err.Error(),
		Step:    stepKind,
		StepID:  stepID,
	}
	var k *KindedError
	if errors.As(err, &k) && k != nil {
		info.Retryable = k.Retryable
		if k.Msg != "" {
			info.Message = k.Msg
		}
	}
	// Conservative retry-hint defaults — anything we know is transient
	// flips Retryable on even if the handler didn't say so explicitly.
	switch info.Kind {
	case "discord.rate_limited", "discord.unavailable",
		"http.timeout", "http.status_5xx", "http.connection",
		"runtime.timeout", "kv.conflict":
		info.Retryable = true
	}
	return info
}
