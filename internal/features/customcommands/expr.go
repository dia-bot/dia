package customcommands

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/templating"
)

// EvalString evaluates an Expr to a string in the current scope. A literal
// value is JSON-marshalled (numbers/bools stringify naturally); a tmpl source
// runs through the dia template engine.
func EvalString(ctx context.Context, ex Expr, scope *Scope) (string, error) {
	switch {
	case len(ex.Value) > 0 || ex.Lang == "literal":
		return literalString(ex.Value)
	case ex.Src != "":
		return renderTmpl(ctx, ex.Src, scope)
	}
	return "", nil
}

// EvalTemplated runs a plain template string against the scope (no Expr
// wrapping). All step strings (Content, Reason, channel-string, etc.) flow
// through here.
func EvalTemplated(ctx context.Context, src string, scope *Scope) (string, error) {
	if src == "" {
		return "", nil
	}
	return renderTmpl(ctx, src, scope)
}

// EvalBool evaluates an Expr to a boolean.
//
// Truthy values (in priority order):
//   - JSON literal true / non-zero number / non-empty string / non-empty list
//   - Template output that case-insensitively equals "true", "yes", "y", "on", "1"
//   - Any non-empty trimmed template output (the YAGPDB convention)
//
// Falsey: anything else, including the empty string. This means
// `{{if eq .Vars.x "go"}}true{{end}}` naturally works for branches.
func EvalBool(ctx context.Context, ex Expr, scope *Scope) (bool, error) {
	if len(ex.Value) > 0 || ex.Lang == "literal" {
		return literalBool(ex.Value)
	}
	if ex.Src == "" {
		return false, nil
	}
	out, err := renderTmpl(ctx, ex.Src, scope)
	if err != nil {
		return false, err
	}
	return parseTruthy(out), nil
}

// EvalSnowflake evaluates an Expr to a Discord snowflake string. Accepts
// "<@id>" / "<@&id>" / "<#id>" mention wrappers as a convenience.
func EvalSnowflake(ctx context.Context, ex Expr, scope *Scope) (string, error) {
	s, err := EvalString(ctx, ex, scope)
	if err != nil {
		return "", err
	}
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<@&")
	s = strings.TrimPrefix(s, "<@!")
	s = strings.TrimPrefix(s, "<@")
	s = strings.TrimPrefix(s, "<#")
	s = strings.TrimSuffix(s, ">")
	if s == "" {
		return "", nil
	}
	if _, err := strconv.ParseUint(s, 10, 64); err != nil {
		return "", fmt.Errorf("expected snowflake id, got %q", s)
	}
	return s, nil
}

// EvalInt evaluates an Expr to int64.
func EvalInt(ctx context.Context, ex Expr, scope *Scope) (int64, error) {
	if len(ex.Value) > 0 || ex.Lang == "literal" {
		var f float64
		if err := json.Unmarshal(ex.Value, &f); err == nil {
			return int64(f), nil
		}
		var s string
		if err := json.Unmarshal(ex.Value, &s); err == nil {
			return strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		}
		return 0, fmt.Errorf("literal is not a number: %s", ex.Value)
	}
	s, err := EvalString(ctx, ex, scope)
	if err != nil {
		return 0, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	return strconv.ParseInt(s, 10, 64)
}

// EvalJSON evaluates an Expr and returns the value as json.RawMessage. A
// literal returns as-is; a template's output is parsed if it looks like JSON
// or stored as a JSON string otherwise.
func EvalJSON(ctx context.Context, ex Expr, scope *Scope) (json.RawMessage, error) {
	if len(ex.Value) > 0 {
		return ex.Value, nil
	}
	s, err := EvalString(ctx, ex, scope)
	if err != nil {
		return nil, err
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return json.RawMessage("null"), nil
	}
	// Heuristic: tries JSON parse, falls back to a string.
	if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) ||
		(strings.HasPrefix(s, "\"") && strings.HasSuffix(s, "\"")) ||
		s == "true" || s == "false" || s == "null" {
		var any interface{}
		if err := json.Unmarshal([]byte(s), &any); err == nil {
			out, _ := json.Marshal(any)
			return out, nil
		}
	}
	out, _ := json.Marshal(s)
	return out, nil
}

// EvalList evaluates an Expr to []any.
func EvalList(ctx context.Context, ex Expr, scope *Scope) ([]any, error) {
	raw, err := EvalJSON(ctx, ex, scope)
	if err != nil {
		return nil, err
	}
	var arr []any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr, nil
	}
	// Comma-split fallback so admins can write `1,2,3` without quoting.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		parts := strings.Split(s, ",")
		out := make([]any, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
		return out, nil
	}
	return nil, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

var sharedEngine = templating.New()

// renderTmpl runs a templated source through the shared engine with the
// current scope as `.` (the template root).
func renderTmpl(ctx context.Context, src string, scope *Scope) (string, error) {
	if scope == nil {
		return src, nil
	}
	// Use shared Engine.Render directly — lookup carried by scope.
	out, err := sharedEngine.Render(ctx, src, scope.TemplateContext(), scope.Lookup())
	if err != nil {
		// Fall back to applying tokens on the raw source so a typo doesn't
		// blank the value (mirrors templating.RenderMessage semantics).
		return applyTokens(src, scope.Tokens()), nil
	}
	return applyTokens(out, scope.Tokens()), nil
}

// applyTokens replaces brace-delimited shorthand tokens after template
// rendering, matching the behavior of templating.RenderMessage.
func applyTokens(s string, tokens map[string]string) string {
	if s == "" || len(tokens) == 0 {
		return s
	}
	pairs := make([]string, 0, len(tokens)*2)
	for k, v := range tokens {
		pairs = append(pairs, k, v)
	}
	return strings.NewReplacer(pairs...).Replace(s)
}

// literalString stringifies a literal Expr.Value.
func literalString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return strconv.FormatFloat(f, 'f', -1, 64), nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return strconv.FormatBool(b), nil
	}
	return string(raw), nil
}

// literalBool reads a JSON literal as a boolean.
func literalBool(raw json.RawMessage) (bool, error) {
	if len(raw) == 0 {
		return false, nil
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err == nil {
		return b, nil
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return f != 0, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return parseTruthy(s), nil
	}
	return false, nil
}

func parseTruthy(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	switch strings.ToLower(t) {
	case "false", "no", "n", "off", "0", "null":
		return false
	}
	return true
}
