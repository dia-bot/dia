package imaging

import (
	"context"
	"testing"

	"github.com/dia-bot/dia/internal/layout"
	"github.com/dia-bot/dia/internal/templating"
)

// TestResolveLayerBindings covers the property-formula system: numeric geometry
// scaled by member data, a conditional colour, a boolean visibility toggle, the
// unbound fast path, and graceful fallback on a bad formula.
func TestResolveLayerBindings(t *testing.T) {
	r := &Renderer{tmpl: templating.New()}
	// {progress} 50% → .ProgressFrac 0.5, .LevelNum 60.
	data := templating.DataFromVars(map[string]string{
		"{progress}": "50%",
		"{level}":    "60",
	})

	in := []layout.Layer{
		{
			ID: "bar", Type: "rect", W: 618, H: 22, Fill: "#FF6363",
			Bind: map[string]string{
				"w":      "{{ round (fmul .ProgressFrac 618) }}", // 0.5 * 618 = 309
				"fill":   "{{ if gt .LevelNum 50 }}#FFD700{{ else }}#FF6363{{ end }}",
				"hidden": "{{ if lt .LevelNum 10 }}true{{ else }}false{{ end }}",
			},
		},
		{ID: "static", Type: "rect", W: 100}, // no bindings: must pass through
	}

	out := r.resolveLayerBindings(context.Background(), in, data)

	if got := out[0].W; got != 309 {
		t.Errorf("bound width = %v, want 309", got)
	}
	if got := out[0].Fill; got != "#FFD700" {
		t.Errorf("conditional fill = %q, want #FFD700", got)
	}
	if out[0].Fills != nil {
		t.Errorf("bound flat fill must clear the paint stack")
	}
	if out[0].Hidden {
		t.Errorf("hidden formula (level 60 >= 10) should be false")
	}
	if got := out[1].W; got != 100 {
		t.Errorf("unbound layer width = %v, want 100 (untouched)", got)
	}

	// A bad formula leaves the static value in place (never breaks the render).
	bad := []layout.Layer{{ID: "x", Type: "rect", W: 200, Bind: map[string]string{"w": "{{ .Nope | bogusFunc }}"}}}
	if got := r.resolveLayerBindings(context.Background(), bad, data)[0].W; got != 200 {
		t.Errorf("bad formula width = %v, want 200 (fallback)", got)
	}

	// The whole slice is returned unchanged when nothing is bound.
	none := []layout.Layer{{ID: "a", W: 1}, {ID: "b", W: 2}}
	if got := r.resolveLayerBindings(context.Background(), none, data); &got[0] != &none[0] {
		t.Errorf("unbound slice should be returned as-is, not copied")
	}
}
