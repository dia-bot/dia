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

// TestResolveBindingsExpanded covers the expanded key set: an int (font_weight),
// a float (dash), and an enum/string (stroke_align) driven by a conditional.
func TestResolveBindingsExpanded(t *testing.T) {
	r := &Renderer{tmpl: templating.New()}
	data := templating.DataFromVars(map[string]string{"{level}": "80"})

	in := []layout.Layer{{
		ID: "t", Type: "text", FontWeight: 400, Dash: 1, StrokeAlign: "center",
		Bind: map[string]string{
			"font_weight":  "{{ if gt .LevelNum 50 }}700{{ else }}400{{ end }}",
			"dash":         "{{ fmul .LevelNum 0.5 }}", // 80 * 0.5 = 40
			"stroke_align": "{{ if gt .LevelNum 50 }}outside{{ else }}center{{ end }}",
		},
	}}
	out := r.resolveLayerBindings(context.Background(), in, data)

	if out[0].FontWeight != 700 {
		t.Errorf("font_weight = %d, want 700", out[0].FontWeight)
	}
	if out[0].Dash != 40 {
		t.Errorf("dash = %v, want 40", out[0].Dash)
	}
	if out[0].StrokeAlign != "outside" {
		t.Errorf("stroke_align = %q, want outside", out[0].StrokeAlign)
	}
}

// TestResolveBindingsEverything covers the remaining "everything bindable" keys:
// a bool (clip), an enum (end_cap), per-corner radii, and a scatter number.
func TestResolveBindingsEverything(t *testing.T) {
	r := &Renderer{tmpl: templating.New()}
	data := templating.DataFromVars(map[string]string{"{level}": "80"})

	in := []layout.Layer{{
		ID: "p", Type: "path", Radius: 8,
		Bind: map[string]string{
			"clip":         "{{ if gt .LevelNum 50 }}true{{ else }}false{{ end }}",
			"end_cap":      "arrow",
			"scatter_size": "{{ .LevelNum }}",
			"corner_tl":    "20",
			"corner_br":    "4",
		},
	}}
	out := r.resolveLayerBindings(context.Background(), in, data)

	if !out[0].Clip {
		t.Errorf("clip bind (level 80 > 50) should be true")
	}
	if out[0].EndCap != "arrow" {
		t.Errorf("end_cap = %q, want arrow", out[0].EndCap)
	}
	if out[0].ScatterSize != 80 {
		t.Errorf("scatter_size = %v, want 80", out[0].ScatterSize)
	}
	// corner_tl=20, corner_br=4 set; the untouched tr/bl fall back to Radius (8).
	want := []float64{20, 8, 4, 8}
	if len(out[0].Corners) != 4 {
		t.Fatalf("corners len = %d, want 4", len(out[0].Corners))
	}
	for i, w := range want {
		if out[0].Corners[i] != w {
			t.Errorf("corners[%d] = %v, want %v (full=%v)", i, out[0].Corners[i], w, out[0].Corners)
		}
	}
}

// TestResolveBackgroundBindings covers the canvas backdrop: a level-gated solid
// colour (clearing the paint stack), a numeric blur, and the unbound fast path.
func TestResolveBackgroundBindings(t *testing.T) {
	r := &Renderer{tmpl: templating.New()}
	data := templating.DataFromVars(map[string]string{"{level}": "80"})

	bg := layout.Background{
		Type: "gradient", From: "#000", To: "#111",
		Fills: []layout.Paint{{Type: "solid", Color: "#222"}},
		Bind: map[string]string{
			"color": "{{ if gt .LevelNum 50 }}#101018{{ else }}#050507{{ end }}",
			"blur":  "{{ .LevelNum }}",
		},
	}
	got := r.resolveBackgroundBindings(context.Background(), bg, data)
	if got.Color != "#101018" || got.Type != "solid" || got.Fills != nil {
		t.Errorf("bound bg color: color=%q type=%q fills=%v, want #101018/solid/nil", got.Color, got.Type, got.Fills)
	}
	if got.Blur != 80 {
		t.Errorf("bound bg blur = %v, want 80", got.Blur)
	}

	// No bindings → returned unchanged.
	plain := layout.Background{Type: "solid", Color: "#abcabc"}
	if out := r.resolveBackgroundBindings(context.Background(), plain, data); out.Color != "#abcabc" {
		t.Errorf("unbound background should be untouched")
	}
}
