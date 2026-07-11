package templating

import (
	"context"
	"strings"
)

// shared is the process-wide engine for message rendering (stateless + safe).
var shared = New()

// RenderMessage renders admin-authored message text: first the pure template
// (logic + functions + read-only guild lookups — templates never perform
// actions), then the simple {token} shorthands. lookup may be nil to disable
// getRole/getChannel (previews). A template parse/exec error falls back to the
// shorthands on the raw source, so a typo never blanks a message. ctx may be
// context.Background(); the engine applies its own timeout on top.
func RenderMessage(ctx context.Context, src string, data *Context, lookup Lookup, tokens map[string]string) string {
	out := src
	if data != nil {
		if rendered, err := shared.Render(ctx, src, data, lookup); err == nil {
			out = rendered
		}
	}
	return applyTokens(out, tokens)
}

// sampleLookup resolves getRole/getChannel to plausible demo values so a "test
// render" of a snippet like {{(getRole "Member").mention}} shows output instead
// of erroring. It performs no I/O — previews stay pure and safe.
type sampleLookup struct{}

func (sampleLookup) Role(nameOrID string) (*RoleInfo, bool) {
	return &RoleInfo{ID: "0", Name: nameOrID, Color: 0xB244FC}, true
}
func (sampleLookup) Channel(nameOrID string) (*ChannelInfo, bool) {
	return &ChannelInfo{ID: "0", Name: strings.TrimPrefix(nameOrID, "#")}, true
}

// Preview renders a template for a dashboard "test render". Unlike
// RenderMessage it returns any template error as a human-readable string (so
// authors see mistakes instead of a silent fallback). Guild lookups resolve to
// sample values — it's a safe, pure preview.
func Preview(ctx context.Context, src string, data *Context, tokens map[string]string) (rendered, errMsg string) {
	out, err := shared.Render(ctx, src, data, sampleLookup{})
	if err != nil {
		return "", err.Error()
	}
	return applyTokens(out, tokens), ""
}

// PreviewCard renders a "test render" against a caller-supplied sample data
// map via the CARD engine — the same engine that renders giveaway (and card)
// strings at runtime, where the scope is a data map addressed with fields like
// {{ .Prize }} rather than the slash/message *Context struct. Rendering those
// strings through Preview (the *Context engine) fails with "can't evaluate
// field Prize in type *templating.Context"; this path resolves them correctly.
// Any template error is returned as a human-readable string so authors see
// mistakes. Pure + safe: no store access (getKV is nil for this ctx).
func PreviewCard(ctx context.Context, src string, data map[string]any) (rendered, errMsg string) {
	out, err := shared.RenderCardStrict(ctx, src, data)
	if err != nil {
		return "", err.Error()
	}
	return out, ""
}

// applyTokens replaces the brace-delimited {token} shorthands. Tokens are
// distinct (the closing brace disambiguates {user} from {user.name}), so order
// is irrelevant and a single Replacer pass is safe.
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
