package templating

import (
	"context"
	"fmt"
	"strings"
	"text/template"
)

// RenderCard renders a card layer's text/image-source as a pure Go template with
// a nested data root, so authors write {{.User.Username}}, {{.User.Avatar}},
// {{.Server.Name}}, {{.Count}}, {{.Level}}, … plus the full template language
// (conditionals, ranges, the stateless baseFuncs). No single-{curly} tokens.
//
// It is pure: no actions, no guild lookups, output + time capped like Render.
func (e *Engine) RenderCard(ctx context.Context, src string, data map[string]any) (string, error) {
	if src == "" {
		return "", nil
	}
	fm := make(template.FuncMap, len(baseFuncs))
	for k, fn := range baseFuncs {
		fm[k] = fn
	}
	// Bound total elements produced by the loop-builders so a nested {{range}} /
	// repeat can't burn CPU even though text/template execution can't be cancelled
	// mid-run. Once the budget is spent these return empty, so loops stop.
	const maxElems = 100_000
	elems := 0
	fm["seq"] = func(a ...int) []int {
		r := seq(a...)
		if elems+len(r) > maxElems {
			return nil
		}
		elems += len(r)
		return r
	}
	fm["list"] = func(xs ...any) []any {
		if elems+len(xs) > maxElems {
			return nil
		}
		elems += len(xs)
		return xs
	}
	fm["repeat"] = func(n int, s string) string {
		if n < 0 {
			n = 0
		}
		if elems+n > maxElems {
			return ""
		}
		elems += n
		return strings.Repeat(s, n)
	}
	tmpl, err := template.New("card").Funcs(fm).Option("missingkey=zero").Parse(src)
	if err != nil {
		return "", fmt.Errorf("card template parse error: %w", err)
	}

	cctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	buf := &limitedBuffer{max: e.maxOutput}
	done := make(chan error, 1)
	go func() { done <- tmpl.Execute(buf, data) }()
	select {
	case err := <-done:
		if err != nil {
			return buf.String(), fmt.Errorf("card template error: %w", err)
		}
		return buf.String(), nil
	case <-cctx.Done():
		return "", fmt.Errorf("card template timed out after %s", e.timeout)
	}
}

// DataFromVars adapts the flat {token} sample/runtime map (built by the welcome,
// leveling and preview code paths) into the nested data root cards address with
// dotted Go-template syntax. Keeping this adapter means the render call sites
// don't have to change shape — only the template syntax did.
func DataFromVars(vars map[string]string) map[string]any {
	pick := func(keys ...string) string {
		for _, k := range keys {
			if v := vars[k]; v != "" {
				return v
			}
		}
		return ""
	}
	guild := map[string]any{
		"Name":        vars["{server}"],
		"ID":          vars["{server.id}"],
		"MemberCount": vars["{count}"],
		"Count":       vars["{count}"],
		"Icon":        vars["{server.icon}"],
		"Banner":      vars["{server.banner}"],
	}
	return map[string]any{
		"User": map[string]any{
			"Username":   pick("{user.name}", "{username}"),
			"Name":       vars["{user}"],
			"GlobalName": vars["{user}"],
			"Mention":    vars["{user.mention}"],
			"ID":         vars["{user.id}"],
			"Avatar":     vars["{user.avatar}"],
		},
		"Server":       guild,
		"Guild":        guild,
		"Count":        vars["{count}"],
		"CountOrdinal": vars["{count.ordinal}"],
		// rank-card fields (empty for welcome cards)
		"Level":       vars["{level}"],
		"Rank":        vars["{rank}"],
		"XP":          vars["{xp}"],
		"LevelXP":     vars["{level.xp}"],
		"LevelNeeded": vars["{level.needed}"],
		"Progress":    vars["{progress}"],
	}
}
