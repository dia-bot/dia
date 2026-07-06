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

// TestResolveBindingsSupersede covers the "superseded legacy field" fixes: a
// bound flat colour must clear the paint stack that would otherwise win, a bound
// uniform radius clears per-corner radii, and a typo'd/out-of-scope field keeps
// the static value (missingkey=error) instead of zeroing the layer.
func TestResolveBindingsSupersede(t *testing.T) {
	r := &Renderer{tmpl: templating.New()}
	data := templating.DataFromVars(map[string]string{"{level}": "60"})

	in := []layout.Layer{
		// Text with a fill stack: a bound color must clear Fills so it takes effect.
		{
			ID: "t", Type: "text", Color: "#FFFFFF",
			Fills: []layout.Paint{{Type: "solid", Color: "#111111"}},
			Bind:  map[string]string{"color": "#00FF00"},
		},
		// Rect with per-corner radii: a bound radius must clear Corners.
		{
			ID: "r", Type: "rect", Radius: 4, Corners: []float64{1, 2, 3, 4},
			Bind: map[string]string{"radius": "20"},
		},
		// Typo / out-of-scope field: static value kept, not zeroed.
		{ID: "z", Type: "rect", W: 200, Bind: map[string]string{"w": "{{ fmul .Widht 2 }}"}},
	}
	out := r.resolveLayerBindings(context.Background(), in, data)

	if out[0].Color != "#00FF00" || out[0].Fills != nil {
		t.Errorf("text color bind: color=%q fills=%v, want #00FF00 / nil", out[0].Color, out[0].Fills)
	}
	if out[1].Radius != 20 || out[1].Corners != nil {
		t.Errorf("radius bind: radius=%v corners=%v, want 20 / nil", out[1].Radius, out[1].Corners)
	}
	if out[2].W != 200 {
		t.Errorf("typo formula width = %v, want 200 (static kept via missingkey=error)", out[2].W)
	}
}
