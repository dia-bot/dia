package templating

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"
)

// funcMap clones the stateless base functions and adds the per-render lookup
// (read-only guild data). Templates stay pure — there are deliberately NO
// side-effecting functions; actions are custom-command steps, never template
// calls.
func (e *Engine) funcMap(lookup Lookup) template.FuncMap {
	fm := make(template.FuncMap, len(baseFuncs)+2)
	for k, v := range baseFuncs {
		fm[k] = v
	}
	addLookupFuncs(fm, lookup)
	return fm
}

// addLookupFuncs wires the read-only guild-data functions. With a nil lookup
// (e.g. previews) they return an error so the gap is visible, not silent.
func addLookupFuncs(fm template.FuncMap, lookup Lookup) {
	fm["getRole"] = func(nameOrID string) (map[string]any, error) {
		if lookup == nil {
			return nil, errors.New("getRole is unavailable here (no guild data)")
		}
		r, ok := lookup.Role(nameOrID)
		if !ok {
			return map[string]any{"id": "", "name": "", "color": 0, "mention": ""}, nil
		}
		return map[string]any{"id": r.ID, "name": r.Name, "color": r.Color, "mention": "<@&" + r.ID + ">"}, nil
	}
	fm["getChannel"] = func(nameOrID string) (map[string]any, error) {
		if lookup == nil {
			return nil, errors.New("getChannel is unavailable here (no guild data)")
		}
		c, ok := lookup.Channel(nameOrID)
		if !ok {
			return map[string]any{"id": "", "name": "", "type": 0, "mention": ""}, nil
		}
		return map[string]any{"id": c.ID, "name": c.Name, "type": c.Type, "mention": "<#" + c.ID + ">"}, nil
	}
}

// baseFuncs are stateless and safe — pure logic/format/lookup helpers.
var baseFuncs = template.FuncMap{
	// strings
	"upper":      strings.ToUpper,
	"lower":      strings.ToLower,
	"title":      titleCase,
	"trim":       strings.TrimSpace,
	"trimPrefix": func(s, p string) string { return strings.TrimPrefix(s, p) },
	"trimSuffix": func(s, p string) string { return strings.TrimSuffix(s, p) },
	"replace":    func(s, old, neu string) string { return strings.ReplaceAll(s, old, neu) },
	"contains":   strings.Contains,
	"hasPrefix":  strings.HasPrefix,
	"hasSuffix":  strings.HasSuffix,
	"split":      strings.Split,
	"join":       func(sep string, parts []string) string { return strings.Join(parts, sep) },
	"repeat": func(n int, s string) string {
		if n < 0 || n > 1000 {
			return ""
		}
		return strings.Repeat(s, n)
	},
	"slice": substr,

	// numbers
	"add": func(xs ...int) int {
		s := 0
		for _, x := range xs {
			s += x
		}
		return s
	},
	"sub": func(a, b int) int { return a - b },
	"mul": func(xs ...int) int {
		p := 1
		for _, x := range xs {
			p *= x
		}
		return p
	},
	"div": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a / b
	},
	"mod": func(a, b int) int {
		if b == 0 {
			return 0
		}
		return a % b
	},
	"max": func(a, b int) int {
		if a > b {
			return a
		}
		return b
	},
	"min": func(a, b int) int {
		if a < b {
			return a
		}
		return b
	},
	"randInt": func(xs ...int) int {
		switch len(xs) {
		case 0:
			return rand.Intn(1 << 31)
		case 1:
			if xs[0] <= 0 {
				return 0
			}
			return rand.Intn(xs[0])
		default:
			lo, hi := xs[0], xs[1]
			if hi <= lo {
				return lo
			}
			return lo + rand.Intn(hi-lo)
		}
	},

	// conversion / defaults
	"toString": func(v any) string { return fmt.Sprint(v) },
	"toInt":    toInt,
	"default": func(def, val any) any {
		if isEmpty(val) {
			return def
		}
		return val
	},

	// collections (bounded)
	"list": func(xs ...any) []any { return xs },
	"dict": dict,
	"seq":  seq,
	"in":   inList,

	// time
	"now":        func() time.Time { return time.Now().UTC() },
	"formatTime": func(layout string, t time.Time) string { return t.Format(layout) },

	// discord
	"mentionUser":    func(id string) string { return "<@" + id + ">" },
	"mentionRole":    func(id string) string { return "<@&" + id + ">" },
	"mentionChannel": func(id string) string { return "<#" + id + ">" },
}

// ── helpers ──────────────────────────────────────────────────────────────────

func titleCase(s string) string {
	var b strings.Builder
	upNext := true
	for _, r := range s {
		if upNext && unicode.IsLetter(r) {
			b.WriteRune(unicode.ToUpper(r))
			upNext = false
		} else {
			b.WriteRune(r)
			if unicode.IsSpace(r) {
				upNext = true
			}
		}
	}
	return b.String()
}

func substr(s string, i, j int) string {
	r := []rune(s)
	if i < 0 {
		i = 0
	}
	if j > len(r) {
		j = len(r)
	}
	if i >= j {
		return ""
	}
	return string(r[i:j])
}

func toInt(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(x))
		return n
	default:
		return 0
	}
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	case reflect.Int, reflect.Int64, reflect.Int32:
		return rv.Int() == 0
	case reflect.Float64, reflect.Float32:
		return rv.Float() == 0
	default:
		return false
	}
}

func dict(args ...any) (map[string]any, error) {
	if len(args)%2 != 0 {
		return nil, errors.New("dict needs an even number of arguments")
	}
	m := make(map[string]any, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		m[fmt.Sprint(args[i])] = args[i+1]
	}
	return m, nil
}

func seq(args ...int) []int {
	lo, hi := 0, 0
	if len(args) == 1 {
		hi = args[0]
	} else if len(args) >= 2 {
		lo, hi = args[0], args[1]
	}
	if hi-lo > maxListLen {
		hi = lo + maxListLen
	}
	out := make([]int, 0, max0(hi-lo))
	for i := lo; i < hi; i++ {
		out = append(out, i)
	}
	return out
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

func inList(item, list any) bool {
	rv := reflect.ValueOf(list)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return false
	}
	needle := fmt.Sprint(item)
	for i := 0; i < rv.Len(); i++ {
		if fmt.Sprint(rv.Index(i).Interface()) == needle {
			return true
		}
	}
	return false
}
