package imaging

import (
	"image"
	"image/color"
	"testing"
)

// px1 returns a 1×1 NRGBA image of the given colour, for exercising applyMask.
func px1(r, g, b, a uint8) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: r, G: g, B: b, A: a})
	return img
}

func alphaAt(img image.Image) int {
	return int(color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA).A)
}

// TestApplyMaskModes covers the three Figma mask types (alpha / vector /
// luminance) plus invert and the default (empty mode == alpha).
func TestApplyMaskModes(t *testing.T) {
	content := px1(0, 0, 255, 200) // blue, alpha 200

	cases := []struct {
		name    string
		stencil image.Image
		mode    string
		invert  bool
		want    int
	}{
		{"alpha full", px1(255, 255, 255, 255), "alpha", false, 200},
		{"alpha half", px1(255, 255, 255, 128), "alpha", false, 100}, // 200×128/255 ≈ 100
		{"alpha zero", px1(255, 255, 255, 0), "alpha", false, 0},
		{"vector any coverage", px1(255, 255, 255, 10), "vector", false, 200}, // >0% → fully revealed
		{"vector empty", px1(0, 0, 0, 0), "vector", false, 0},
		{"luminance white", px1(255, 255, 255, 255), "luminance", false, 200},
		{"luminance black", px1(0, 0, 0, 255), "luminance", false, 0},
		{"alpha invert full", px1(255, 255, 255, 255), "alpha", true, 0},
		{"vector invert covered", px1(255, 255, 255, 255), "vector", true, 0},
		{"empty mode defaults to alpha", px1(255, 255, 255, 255), "", false, 200},
	}
	for _, tc := range cases {
		got := alphaAt(applyMask(content, tc.stencil, tc.mode, tc.invert))
		if d := got - tc.want; d < -1 || d > 1 { // allow ±1 for rounding
			t.Errorf("%s: result alpha = %d, want ≈ %d", tc.name, got, tc.want)
		}
	}
}

// TestCombineBoolean covers the four boolean ops at a single pixel: A covers it,
// B covers it (so both = overlap), and the fill colour is applied.
func TestCombineBoolean(t *testing.T) {
	both := []image.Image{px1(255, 255, 255, 255), px1(255, 255, 255, 255)} // overlap
	onlyA := []image.Image{px1(255, 255, 255, 255), px1(0, 0, 0, 0)}        // base only
	fill := color.NRGBA{R: 10, G: 20, B: 30, A: 255}

	cases := []struct {
		name string
		cov  []image.Image
		op   string
		want int
	}{
		{"union overlap", both, "union", 255},
		{"union base-only", onlyA, "union", 255},
		{"intersect overlap", both, "intersect", 255},
		{"intersect base-only", onlyA, "intersect", 0},
		{"subtract overlap", both, "subtract", 0},      // base minus the overlapping other
		{"subtract base-only", onlyA, "subtract", 255}, // nothing to subtract
		{"exclude overlap", both, "exclude", 0},        // even parity → hole
		{"exclude base-only", onlyA, "exclude", 255},   // odd parity → visible
	}
	for _, tc := range cases {
		out := combineBoolean(tc.cov, tc.op, fill, 1)
		got := alphaAt(out)
		if d := got - tc.want; d < -1 || d > 1 {
			t.Errorf("%s: alpha = %d, want ≈ %d", tc.name, got, tc.want)
		}
		if tc.want > 0 {
			c := color.NRGBAModel.Convert(out.At(0, 0)).(color.NRGBA)
			if c.R != fill.R || c.G != fill.G || c.B != fill.B {
				t.Errorf("%s: fill = %v, want %v", tc.name, c, fill)
			}
		}
	}
}
