package templating

import (
	"context"
	"fmt"
	"math"
	"strconv"
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
	// Float-aware math so property formulas can scale by fractional values (the
	// integer add/sub/mul/div in baseFuncs truncate). These accept any numeric or
	// numeric-string arg (toFloat), so `{{ fmul .ProgressFrac 618 }}` and
	// `{{ round (fmul .ProgressFrac .W) }}` work regardless of source type.
	for k, fn := range cardMathFuncs {
		fm[k] = fn
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
	// Numeric siblings of the formatted rank fields, so FORMULAS (layer bindings,
	// {{ if gt .LevelNum 50 }}, {{ fmul .ProgressFrac 618 }}) can do real math while
	// the string fields above stay for display (e.g. .XP keeps its "1,234" commas).
	frac := progressFrac(vars["{progress}"]) // 0..1
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
		// numeric forms for formulas (0 when absent/unparseable). Whole-number
		// fields are int so natural comparisons work ({{ if gt .LevelNum 50 }});
		// ProgressFrac stays float for fractional math ({{ fmul .ProgressFrac 618 }}).
		"LevelNum":     int(numFromVar(vars["{level}"])),
		"RankNum":      int(numFromVar(vars["{rank}"])),
		"XpNum":        int(numFromVar(vars["{xp}"])),
		"LevelXpNum":   int(numFromVar(vars["{level.xp}"])),
		"NeededNum":    int(numFromVar(vars["{level.needed}"])),
		"MemberCount":  int(numFromVar(vars["{count}"])),
		"ProgressFrac": frac,                // 0..1
		"ProgressPct":  int(frac*100 + 0.5), // 0..100
	}
}

// numFromVar parses a formatted rank var ("1,234", "64%", " 12 ") into a float,
// tolerating thousands separators, a percent suffix and surrounding spaces.
// Returns 0 when empty or unparseable so formulas degrade gracefully.
func numFromVar(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	s = strings.TrimSuffix(strings.ReplaceAll(s, ",", ""), "%")
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

// progressFrac parses the rank {progress} var ("64%", "64", "0.64") into a 0..1
// fraction (mirrors imaging.progressFraction; kept here so the scope is
// self-contained). Absent/unparseable → 0.
func progressFrac(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	pct := strings.HasSuffix(s, "%")
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
	if err != nil {
		return 0
	}
	if pct || v > 1 {
		v /= 100
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// toFloat coerces a template value (number or numeric string) to float64.
func toFloat(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case string:
		return numFromVar(n)
	default:
		return 0
	}
}

// cardMathFuncs are float-aware math helpers added to the card template funcmap,
// so property formulas can scale by fractional values and clamp/round the result.
var cardMathFuncs = template.FuncMap{
	"fadd": func(a, b any) float64 { return toFloat(a) + toFloat(b) },
	"fsub": func(a, b any) float64 { return toFloat(a) - toFloat(b) },
	"fmul": func(a, b any) float64 { return toFloat(a) * toFloat(b) },
	"fdiv": func(a, b any) float64 {
		d := toFloat(b)
		if d == 0 {
			return 0
		}
		return toFloat(a) / d
	},
	"fmin":  func(a, b any) float64 { return math.Min(toFloat(a), toFloat(b)) },
	"fmax":  func(a, b any) float64 { return math.Max(toFloat(a), toFloat(b)) },
	"round": func(a any) float64 { return math.Round(toFloat(a)) },
	"floor": func(a any) float64 { return math.Floor(toFloat(a)) },
	"ceil":  func(a any) float64 { return math.Ceil(toFloat(a)) },
	"float": toFloat,
	"lerp":  func(a, b, t any) float64 { fa, fb := toFloat(a), toFloat(b); return fa + (fb-fa)*toFloat(t) },
	"clamp": func(v, lo, hi any) float64 {
		return math.Max(toFloat(lo), math.Min(toFloat(hi), toFloat(v)))
	},
}
