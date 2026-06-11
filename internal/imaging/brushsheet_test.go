package imaging

// Dev-only visual harness for the brush engine (all tests skip unless BRUSH_SHEET=1):
//
//	BRUSH_SHEET=1 go test ./internal/imaging/ -run TestBrush -v
//
//   - TestBrushSheet          → /tmp/brush-sheet.png: every brush as a labeled sample stroke.
//   - TestBrushClosedAndSmall → /tmp/brush-closed.png: closed rects/ellipses + small widths.
//   - TestBrushCompare        → /tmp/cmp-*.png: [official Figma preview | ours] per brush.
//     Needs the official previews saved as /tmp/figma-brush-research/img/figma-<id>.png; their
//     URLs are on developers.figma.com/docs/plugins/api/ComplexStrokeProperties (one
//     static.figma.com upload per brushName, in document order).
//   - TestBrushParity         → /tmp/parity.png + per-brush mean pixel diff between this Go
//     engine and the TS twin in web/src/lib/layout/brushes.ts. Generate the TS side first:
//     esbuild brushes.ts to /tmp/brushes.mjs, then for each BRUSHES entry build the same
//     64-pt arc as below, call brushStrokeMarkup with {width: 32, color: '#000000'}, wrap in
//     <svg viewBox='0 0 480 200'> over a white rect and rasterise with
//     `rsvg-convert -w 480 -h 200 -b white` to /tmp/ts-<id>.png.
//     The engines are mirrored bit-for-bit; mean diffs above ~0.01 mean they diverged.

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"testing"

	"github.com/fogleman/gg"

	"github.com/dia-bot/dia/internal/layout"
)

func TestBrushSheet(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip("set BRUSH_SHEET=1 to render /tmp/brush-sheet.png")
	}
	ids := make([]string, 0, len(brushCatalog))
	for id := range brushCatalog {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := brushCatalog[ids[i]], brushCatalog[ids[j]]
		if a.kind != b.kind {
			return a.kind < b.kind
		}
		return ids[i] < ids[j]
	})
	const (
		rowH    = 84
		w       = 920
		sw      = 10.0 // sample stroke weight
		padX    = 200
		samples = 160
	)
	dc := gg.NewContext(w, rowH*len(ids))
	dc.SetColor(color.White)
	dc.Clear()
	for row, id := range ids {
		y0 := float64(row*rowH) + rowH/2
		pts := make([]pathPt, samples)
		for i := range pts {
			t := float64(i) / float64(samples-1)
			pts[i] = pathPt{
				x: padX + t*(w-padX-40),
				y: y0 + math.Sin(t*math.Pi*2)*rowH*0.22,
			}
		}
		l := layout.Layer{BrushName: id, StrokeWidth: sw, StrokeColor: "#111111"}
		strokeBrush(dc, pts, l, 1, false)
		dc.SetColor(color.Black)
		dc.DrawString(brushCatalog[id].kind+" / "+id, 16, y0+4)
	}
	if err := dc.SavePNG("/tmp/brush-sheet.png"); err != nil {
		t.Fatalf("save: %v", err)
	}
	t.Log("wrote /tmp/brush-sheet.png")
}

// TestBrushCompare renders [official figma preview | ours] side by side per brush.
func TestBrushCompare(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip("set BRUSH_SHEET=1")
	}
	groups := map[string][]string{
		"/tmp/cmp-stretch1.png": {"heist", "blockbuster", "grindhouse", "biopic", "spaghetti_western", "slasher", "hardboiled", "verite"},
		"/tmp/cmp-stretch2.png": {"epic", "screwball", "rom_com", "noir", "propaganda", "melodrama", "new_wave"},
		"/tmp/cmp-scatter1.png": {"bubblegum", "witch_house", "shoegaze", "honky_tonk", "screamo"},
		"/tmp/cmp-scatter2.png": {"drone", "doo_wop", "spoken_word", "vaporwave", "oi"},
	}
	const tw, th, sc = 240, 100, 2 // official preview tile size, drawn at 2x
	for out, names := range groups {
		dc := gg.NewContext(tw*2*sc+40, th*sc*len(names))
		dc.SetColor(color.White)
		dc.Clear()
		for row, n := range names {
			img, err := gg.LoadImage("/tmp/figma-brush-research/img/figma-" + n + ".png")
			if err != nil {
				t.Fatalf("%s: %v", n, err)
			}
			y0 := float64(row * th * sc)
			dc.Push()
			dc.Translate(0, y0)
			dc.Scale(sc, sc)
			dc.DrawImage(img, 0, 0)
			dc.Pop()
			// our stroke on the right tile, mimicking the official sample curve:
			// a gentle arc from (15,55) over (~120,30) to (225,50), stroke ≈ 16px.
			pts := make([]pathPt, 64)
			for i := range pts {
				tt := float64(i) / 63
				x := 15 + 210*tt
				y := 52 - 26*math.Sin(tt*math.Pi)*(1-0.35*tt) + 6*tt
				pts[i] = pathPt{(x + tw) * sc, y0/1 + y*sc}
			}
			// scale stroke up by the tile zoom so texture density matches
			l := layout.Layer{BrushName: n, StrokeWidth: 16 * sc, StrokeColor: "#000000"}
			strokeBrush(dc, pts, l, 1, false)
			dc.SetColor(color.Black)
			dc.DrawString(n, float64(tw*sc)-60, y0+14)
		}
		if err := dc.SavePNG(out); err != nil {
			t.Fatal(err)
		}
	}
}

// TestBrushClosedAndSmall sanity-checks brushes on closed outlines and small widths.
func TestBrushClosedAndSmall(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip()
	}
	ids := []string{"heist", "biopic", "spaghetti_western", "slasher", "verite", "noir", "new_wave", "propaganda", "honky_tonk", "drone", "bubblegum", "oi"}
	dc := gg.NewContext(4*170, len(ids)*170)
	dc.SetColor(color.White)
	dc.Clear()
	for row, id := range ids {
		y := float64(row*170) + 30
		// closed rounded rect
		rect := roundRectPoints(20, y, 110, 110, 18, 18, 18, 18)
		strokeBrush(dc, rect, layout.Layer{BrushName: id, StrokeWidth: 10, StrokeColor: "#000000"}, 1, true)
		// closed ellipse
		ell := ellipsePoints(170+75, y+55, 55, 55)
		strokeBrush(dc, ell, layout.Layer{BrushName: id, StrokeWidth: 10, StrokeColor: "#000000"}, 1, true)
		// small-width open stroke
		pts := make([]pathPt, 48)
		for i := range pts {
			tt := float64(i) / 47
			pts[i] = pathPt{2*170 + 20 + tt*130, y + 55 + math.Sin(tt*math.Pi*2)*28}
		}
		strokeBrush(dc, pts, layout.Layer{BrushName: id, StrokeWidth: 4, StrokeColor: "#000000"}, 1, false)
		// medium open stroke backward direction
		pts2 := make([]pathPt, 48)
		for i := range pts2 {
			tt := float64(i) / 47
			pts2[i] = pathPt{3*170 + 20 + tt*130, y + 55 + math.Sin(tt*math.Pi*2)*28}
		}
		strokeBrush(dc, pts2, layout.Layer{BrushName: id, StrokeWidth: 10, StrokeColor: "#000000", BrushDirection: "backward"}, 1, false)
		dc.SetColor(color.Black)
		dc.DrawString(id, 8, y-10)
	}
	if err := dc.SavePNG("/tmp/brush-closed.png"); err != nil {
		t.Fatal(err)
	}
}

// TestBrushParity renders each brush with the Go engine at the exact coordinates used
// by /tmp/gen-ts-brushes.mjs, then pixel-compares against the rsvg-rasterised output of
// the TS engine (/tmp/ts-<id>.png) and writes /tmp/parity-*.png side-by-side sheets.
// A geometry/RNG divergence between the two engines shows up as a large mean diff.
func TestBrushParity(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip()
	}
	ids := make([]string, 0, len(brushCatalog))
	for id := range brushCatalog {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	sheet := gg.NewContext(960, 200*len(ids))
	sheet.SetColor(color.White)
	sheet.Clear()
	for row, id := range ids {
		dc := gg.NewContext(480, 200)
		dc.SetColor(color.White)
		dc.Clear()
		pts := make([]pathPt, 64)
		for i := range pts {
			tt := float64(i) / 63
			pts[i] = pathPt{
				(15 + 210*tt) * 2,
				(52 - 26*math.Sin(tt*math.Pi)*(1-0.35*tt) + 6*tt) * 2,
			}
		}
		strokeBrush(dc, pts, layout.Layer{BrushName: id, StrokeWidth: 32, StrokeColor: "#000000"}, 1, false)
		goImg := dc.Image()
		tsImg, err := gg.LoadImage("/tmp/ts-" + id + ".png")
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		var sum, cnt float64
		for y := 0; y < 200; y++ {
			for x := 0; x < 480; x++ {
				gr, _, _, _ := goImg.At(x, y).RGBA()
				tr, _, _, _ := tsImg.At(x, y).RGBA()
				d := float64(gr) - float64(tr)
				if d < 0 {
					d = -d
				}
				sum += d / 65535
				cnt++
			}
		}
		t.Logf("%-18s mean|diff| = %.4f", id, sum/cnt)
		sheet.DrawImage(goImg, 0, row*200)
		sheet.DrawImage(tsImg, 480, row*200)
		sheet.SetColor(color.RGBA{200, 0, 0, 255})
		sheet.DrawString(id, 8, float64(row*200)+14)
	}
	if err := sheet.SavePNG("/tmp/parity.png"); err != nil {
		t.Fatal(err)
	}
}

// TestDynamicWobble renders the dynamic stroke at the settings shown in Figma's help
// screenshot (457px circle, weight 60, frequency 53 / wiggle 119 / smoothen 78) plus
// sweeps of each parameter → /tmp/dynamic-sheet.png.
func TestDynamicWobble(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip()
	}
	type cfg struct {
		label             string
		freq, wig, smooth float64
	}
	cases := []cfg{
		{"figma 53/119/78", 53, 119, 78},
		{"freq 10", 10, 80, 78},
		{"freq 90", 90, 80, 78},
		{"wiggle 30", 53, 30, 78},
		{"wiggle 200", 53, 200, 78},
		{"smooth 0", 53, 80, 0},
		{"smooth 100", 53, 80, 100},
	}
	const tile = 600
	dc := gg.NewContext(tile*len(cases), tile+300)
	dc.SetColor(color.White)
	dc.Clear()
	for col, c := range cases {
		// circle exactly like the help screenshot: 457px diameter, weight 60
		l := layout.Layer{StrokeWidth: 60, StrokeColor: "#000000",
			DynamicFrequency: c.freq, DynamicWiggle: c.wig, DynamicSmoothen: c.smooth}
		ell := ellipsePoints(float64(col*tile)+tile/2, tile/2, 228, 228)
		pts := wobblePath(ell, l, true)
		dc.SetColor(color.Black)
		dc.SetLineWidth(l.StrokeWidth)
		strokeSmoothPath(dc, pts, true)
		dc.Stroke()
		dc.ClearPath()
		// open line
		line := make([]pathPt, 80)
		for i := range line {
			tt := float64(i) / 79
			line[i] = pathPt{float64(col*tile) + 20 + tt*(tile-40), tile + 150}
		}
		lp := wobblePath(line, l, false)
		dc.SetLineWidth(8)
		strokeSmoothPath(dc, lp, false)
		dc.Stroke()
		dc.ClearPath()
		dc.DrawString(c.label, float64(col*tile)+12, 16)
	}
	if err := dc.SavePNG("/tmp/dynamic-sheet.png"); err != nil {
		t.Fatal(err)
	}
}

func TestDynamicSideBySide(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip()
	}
	ref, err := gg.LoadImage("/tmp/figma-dynamic-research/img/dynamic-circle.png")
	if err != nil {
		t.Skip("no reference")
	}
	dc := gg.NewContext(1300, 650)
	dc.SetColor(color.White)
	dc.Clear()
	// crop of the official screenshot: circle region ≈ (20,540)-(670,1190)
	dc.Push()
	dc.Translate(-20, -540)
	dc.DrawImage(ref, 0, 0)
	dc.Pop()
	dc.SetColor(color.White)
	dc.DrawRectangle(650, 0, 650, 650)
	dc.Fill()
	l := layout.Layer{StrokeWidth: 60, StrokeColor: "#000000",
		DynamicFrequency: 53, DynamicWiggle: 119, DynamicSmoothen: 78}
	pts := wobblePath(ellipsePoints(650+325, 325, 228, 228), l, true)
	dc.SetColor(color.Black)
	dc.SetLineWidth(60)
	strokeSmoothPath(dc, pts, true)
	dc.Stroke()
	if err := dc.SavePNG("/tmp/dynamic-vs.png"); err != nil {
		t.Fatal(err)
	}
}

// TestInnerShadowRender renders inner/drop shadows on a rect + circle via the full
// RenderLayout path → /tmp/inner-shadow.png (BRUSH_SHEET=1 gated, eyeball check).
func TestInnerShadowRender(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip()
	}
	r := New(t.TempDir(), nil)
	fp := func(v float64) *float64 { return &v }
	doc := layout.Layout{
		Width: 640, Height: 300,
		Background: layout.Background{Type: "solid", Color: "#e8e0f0"},
		Layers: []layout.Layer{
			{ID: "r", Type: "rect", X: 60, Y: 70, W: 200, H: 150, Fill: "#ff6363", Radius: 24,
				Effects: []layout.Effect{{Type: "inner_shadow", X: 10, Y: 10, Radius: 18, Color: "#000000", Opacity: fp(0.8)}}},
			{ID: "c", Type: "ellipse", X: 360, Y: 60, W: 170, H: 170, Fill: "#63a8ff",
				Effects: []layout.Effect{
					{Type: "inner_shadow", X: -8, Y: -8, Radius: 14, Color: "#000000", Opacity: fp(0.9)},
					{Type: "drop_shadow", X: 10, Y: 12, Radius: 16, Color: "#000000", Opacity: fp(0.5)},
				}},
		},
	}
	png, err := r.RenderLayout(context.Background(), doc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/tmp/inner-shadow.png", png, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFillSemantics renders the Figma fill rules: ” = no fill on rect/ellipse, and
// open paths fill with an implicit chord → /tmp/fill-semantics.png.
func TestFillSemantics(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip()
	}
	r := New(t.TempDir(), nil)
	doc := layout.Layout{
		Width: 640, Height: 260,
		Background: layout.Background{Type: "solid", Color: "#202028"},
		Layers: []layout.Layer{
			{ID: "a", Type: "rect", X: 40, Y: 60, W: 140, H: 120, Fill: "", StrokeColor: "#ffffff", StrokeWidth: 1},
			{ID: "b", Type: "ellipse", X: 220, Y: 60, W: 130, H: 130, Fill: ""},
			{ID: "c", Type: "path", X: 400, Y: 50, W: 200, H: 160, Fill: "#63a8ff", StrokeColor: "#ffffff", StrokeWidth: 1,
				Closed: false,
				Nodes: []layout.PathNode{
					{X: 410, Y: 200, H1X: 410, H1Y: 200, H2X: 410, H2Y: 200},
					{X: 470, Y: 60, H1X: 470, H1Y: 60, H2X: 470, H2Y: 60},
					{X: 530, Y: 140, H1X: 530, H1Y: 140, H2X: 530, H2Y: 140},
					{X: 590, Y: 70, H1X: 590, H1Y: 70, H2X: 590, H2Y: 70},
				}},
		},
	}
	png, err := r.RenderLayout(context.Background(), doc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/tmp/fill-semantics.png", png, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPaintStack renders the Figma paint system: stacked fills, all four gradient
// types, and an image fill with the three fit modes → /tmp/paints.png.
func TestPaintStack(t *testing.T) {
	if os.Getenv("BRUSH_SHEET") == "" {
		t.Skip()
	}
	// serve a tiny checker image for the image-fill paints
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			c := color.NRGBA{40, 90, 200, 255}
			if (x/16+y/16)%2 == 0 {
				c = color.NRGBA{240, 200, 60, 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = png.Encode(w, img)
	}))
	defer srv.Close()

	fp := func(v float64) *float64 { return &v }
	stops := []layout.GradientStop{{Pos: 0, Color: "#ff6363"}, {Pos: 1, Color: "#6363ff"}}
	fade := []layout.GradientStop{{Pos: 0, Color: "#ffffff"}, {Pos: 1, Color: "#ffffff", Alpha: fp(0)}}
	doc := layout.Layout{
		Width: 900, Height: 420, Background: layout.Background{Type: "solid", Color: "#1c1c22"},
		Layers: []layout.Layer{
			// stacked: solid base + white fade on top
			{ID: "1", Type: "rect", X: 30, Y: 40, W: 160, H: 120, Radius: 16, Fills: []layout.Paint{
				{Type: "solid", Color: "#ff6363"},
				{Type: "linear", Angle: 180, Stops: fade, Opacity: fp(0.9)},
			}},
			{ID: "2", Type: "rect", X: 230, Y: 40, W: 160, H: 120, Radius: 16, Fills: []layout.Paint{{Type: "linear", Angle: 135, Stops: stops}}},
			{ID: "3", Type: "rect", X: 430, Y: 40, W: 160, H: 120, Radius: 16, Fills: []layout.Paint{{Type: "radial", Stops: stops}}},
			{ID: "4", Type: "rect", X: 630, Y: 40, W: 160, H: 120, Radius: 16, Fills: []layout.Paint{{Type: "angular", Stops: stops}}},
			{ID: "5", Type: "rect", X: 30, Y: 220, W: 160, H: 120, Radius: 16, Fills: []layout.Paint{{Type: "diamond", Stops: stops}}},
			{ID: "6", Type: "ellipse", X: 230, Y: 210, W: 140, H: 140, Fills: []layout.Paint{{Type: "angular", Angle: 45, Stops: stops}}},
			// image fills: cover and tile
			{ID: "7", Type: "rect", X: 430, Y: 220, W: 160, H: 120, Radius: 16, Fills: []layout.Paint{{Type: "image", Src: "https://help.figma.com/hc/article_attachments/31937315309591", Fit: "cover"}}},
			{ID: "8", Type: "rect", X: 630, Y: 220, W: 160, H: 120, Radius: 16, Fills: []layout.Paint{{Type: "image", Src: "https://help.figma.com/hc/article_attachments/31937315309591", Fit: "tile", Opacity: fp(0.8)}}},
			// gradient on an open path
			{ID: "9", Type: "path", X: 810, Y: 40, W: 80, H: 320, StrokeColor: "#ffffff", StrokeWidth: 1,
				Fills: []layout.Paint{{Type: "linear", Angle: 180, Stops: stops}},
				Nodes: []layout.PathNode{
					{X: 820, Y: 350, H1X: 820, H1Y: 350, H2X: 820, H2Y: 350},
					{X: 850, Y: 60, H1X: 850, H1Y: 60, H2X: 850, H2Y: 60},
					{X: 880, Y: 350, H1X: 880, H1Y: 350, H2X: 880, H2Y: 350},
				}},
		},
	}
	r := New(t.TempDir(), nil)
	out, err := r.RenderLayout(context.Background(), doc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("/tmp/paints.png", out, 0o644); err != nil {
		t.Fatal(err)
	}
}
