package imaging

// Figma-style fill paints: a layer's fill is a stack of paints composited
// bottom→top — solid colours, four gradient types (linear / radial / angular /
// diamond) and image fills. Gradients are evaluated per-pixel through a custom
// gg.Pattern so all four types render exactly (the web preview approximates
// angular/diamond on vector paths; this renderer is authoritative).
// Mirrored conceptually in web/src/lib/layout/schema.ts (paintsOf/paintPrimary)
// and Canvas.svelte (cssBackgrounds / pathFillMarkup).

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"math"

	xdraw "github.com/disintegration/imaging"
	"github.com/fogleman/gg"

	"github.com/dia-bot/dia/internal/layout"
)

// paintsOf normalizes a layer's fill to its paint stack: Fills when present,
// else the legacy single hex as one solid paint. nil = no fill.
func paintsOf(l layout.Layer) []layout.Paint {
	if len(l.Fills) > 0 {
		return l.Fills
	}
	if l.Fill != "" {
		return []layout.Paint{{Type: "solid", Color: l.Fill}}
	}
	return nil
}

// stackPrimary returns a representative solid hex for a paint stack: the
// topmost visible paint's colour (a gradient contributes its first stop).
func stackPrimary(ps []layout.Paint) string {
	for i := len(ps) - 1; i >= 0; i-- {
		p := ps[i]
		if p.Hidden {
			continue
		}
		if p.Type == "solid" && p.Color != "" {
			return p.Color
		}
		if len(p.Stops) > 0 {
			return p.Stops[0].Color
		}
	}
	return ""
}

// paintPrimary returns a representative solid hex for consumers that need ONE
// colour (boolean composites, luminance masks, wobbled outlines).
func paintPrimary(l layout.Layer) string {
	return stackPrimary(paintsOf(l))
}

// strokePaintsOf normalizes a layer's stroke to its paint stack: Strokes when
// present, else the legacy single StrokeColor as one solid paint. An unset
// colour keeps the renderer's historic white default. Only meaningful when the
// layer actually has a stroke (StrokeWidth > 0).
func strokePaintsOf(l layout.Layer) []layout.Paint {
	if len(l.Strokes) > 0 {
		return l.Strokes
	}
	c := l.StrokeColor
	if c == "" {
		c = "#FFFFFF"
	}
	return []layout.Paint{{Type: "solid", Color: c}}
}

// strokePrimary returns ONE representative stroke colour for consumers that
// can't paint a stack (brush stamps, the text outline ring, arrowheads).
func strokePrimary(l layout.Layer) string {
	return stackPrimary(strokePaintsOf(l))
}

// textPaintsOf normalizes a text layer's fill to a paint stack: Fills when
// present, else the legacy Color as one solid paint.
func textPaintsOf(l layout.Layer) []layout.Paint {
	if len(l.Fills) > 0 {
		return l.Fills
	}
	if l.Color != "" {
		return []layout.Paint{{Type: "solid", Color: l.Color}}
	}
	return nil
}

func paintOpacity(p layout.Paint) float64 {
	if p.Opacity == nil {
		return 1
	}
	return math.Min(1, math.Max(0, *p.Opacity))
}

func stopAlpha(s layout.GradientStop) float64 {
	if s.Alpha == nil {
		return 1
	}
	return math.Min(1, math.Max(0, *s.Alpha))
}

// gradPattern evaluates one gradient paint over the layer's bounding box.
type gradPattern struct {
	p          layout.Paint
	x, y, w, h float64
	alpha      float64 // layer opacity × paint opacity
}

// stopColorAt interpolates the gradient's stops at t (clamped), with alpha baked in.
func (g gradPattern) stopColorAt(t float64) color.Color {
	stops := g.p.Stops
	if len(stops) == 0 {
		return color.Transparent
	}
	if t <= stops[0].Pos {
		return scaleAlpha(parseHex(stops[0].Color, color.Black), stopAlpha(stops[0])*g.alpha)
	}
	last := stops[len(stops)-1]
	if t >= last.Pos {
		return scaleAlpha(parseHex(last.Color, color.Black), stopAlpha(last)*g.alpha)
	}
	for i := 1; i < len(stops); i++ {
		a, b := stops[i-1], stops[i]
		if t > b.Pos {
			continue
		}
		f := 0.0
		if b.Pos > a.Pos {
			f = (t - a.Pos) / (b.Pos - a.Pos)
		}
		ca := color.NRGBAModel.Convert(parseHex(a.Color, color.Black)).(color.NRGBA)
		cb := color.NRGBAModel.Convert(parseHex(b.Color, color.Black)).(color.NRGBA)
		aa, ab := stopAlpha(a), stopAlpha(b)
		mix := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*f + 0.5) }
		al := (aa + (ab-aa)*f) * g.alpha
		return color.NRGBA{
			R: mix(ca.R, cb.R), G: mix(ca.G, cb.G), B: mix(ca.B, cb.B),
			A: uint8(al*255 + 0.5),
		}
	}
	return color.Transparent
}

func (g gradPattern) ColorAt(x, y int) color.Color {
	if g.w <= 0 || g.h <= 0 {
		return color.Transparent
	}
	// position relative to the bbox centre, in px
	dx := float64(x) + 0.5 - (g.x + g.w/2)
	dy := float64(y) + 0.5 - (g.y + g.h/2)
	var t float64
	switch g.p.Type {
	case "radial":
		// 0 at the centre, 1 at the nearest edge midpoint (Figma's default circle)
		t = math.Hypot(dx/(g.w/2), dy/(g.h/2))
	case "angular":
		rot := g.p.Angle * math.Pi / 180
		t = math.Mod((math.Atan2(dy, dx)+math.Pi/2-rot)/(2*math.Pi)+2, 1)
	case "diamond":
		t = math.Abs(dx)/(g.w/2) + math.Abs(dy)/(g.h/2)
	default: // linear — CSS convention: 0deg points up, increasing clockwise
		a := g.p.Angle * math.Pi / 180
		ux, uy := math.Sin(a), -math.Cos(a)
		// CSS gradient-line length: the projection of the box onto the axis
		length := math.Abs(g.w*ux) + math.Abs(g.h*uy)
		if length <= 0 {
			length = 1
		}
		t = (dx*ux+dy*uy)/length + 0.5
	}
	return g.stopColorAt(t)
}

// scaleAlpha multiplies a colour's alpha by mul.
func scaleAlpha(c color.Color, mul float64) color.Color {
	nr := color.NRGBAModel.Convert(c).(color.NRGBA)
	nr.A = uint8(float64(nr.A)*math.Min(1, math.Max(0, mul)) + 0.5)
	return nr
}

// drawImageAlpha draws img at (x,y) composited with a uniform alpha.
func drawImageAlpha(dc *gg.Context, img image.Image, x, y int, alpha float64) {
	if alpha >= 1 {
		dc.DrawImage(img, x, y)
		return
	}
	mask := image.NewUniform(color.Alpha{A: uint8(alpha*255 + 0.5)})
	b := img.Bounds()
	target := image.Rect(x, y, x+b.Dx(), y+b.Dy())
	draw.DrawMask(dc.Image().(draw.Image), target, img, b.Min, mask, image.Point{}, draw.Over)
}

// imagePattern is the gg.Pattern for an image STROKE paint: a pre-fitted image
// anchored at (ox,oy) in canvas px, optionally tiling, with a uniform alpha.
// (Fills clip + draw the image directly; a stroke needs a pattern because gg
// can't clip to a stroked region.)
type imagePattern struct {
	img    image.Image
	ox, oy int
	tile   bool
	alpha  float64
}

func (ip imagePattern) ColorAt(x, y int) color.Color {
	b := ip.img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return color.Transparent
	}
	px, py := x-ip.ox, y-ip.oy
	if ip.tile {
		px = ((px % b.Dx()) + b.Dx()) % b.Dx()
		py = ((py % b.Dy()) + b.Dy()) % b.Dy()
	} else if px < 0 || py < 0 || px >= b.Dx() || py >= b.Dy() {
		return color.Transparent
	}
	return scaleAlpha(ip.img.At(b.Min.X+px, b.Min.Y+py), ip.alpha)
}

// setPaintStyle points dc's stroke AND fill styles at one paint — a solid
// colour, a gradient pattern over the layer's bbox, or an image pattern — so
// the caller can Stroke()/Fill() with it. alpha is the combined layer × paint
// opacity. Returns false when the paint has nothing to draw.
func (r *Renderer) setPaintStyle(ctx context.Context, dc *gg.Context, l layout.Layer, p layout.Paint, alpha float64) bool {
	switch p.Type {
	case "linear", "radial", "angular", "diamond":
		if len(p.Stops) == 0 {
			return false
		}
		pat := gradPattern{p: p, x: l.X, y: l.Y, w: l.W, h: l.H, alpha: alpha}
		dc.SetFillStyle(pat)
		dc.SetStrokeStyle(pat)
	case "image":
		img := r.fetchImage(ctx, p.Src)
		if img == nil {
			return false
		}
		w := int(math.Max(1, l.W))
		h := int(math.Max(1, l.H))
		var pat imagePattern
		switch p.Fit {
		case "contain":
			fit := xdraw.Fit(img, w, h, xdraw.Lanczos)
			fb := fit.Bounds()
			pat = imagePattern{img: fit, ox: int(l.X) + (w-fb.Dx())/2, oy: int(l.Y) + (h-fb.Dy())/2, alpha: alpha}
		case "tile":
			tile := img
			if tb := tile.Bounds(); tb.Dx() > 256 {
				tile = xdraw.Resize(tile, 256, 0, xdraw.Lanczos)
			}
			pat = imagePattern{img: tile, ox: int(l.X), oy: int(l.Y), tile: true, alpha: alpha}
		default: // cover (Figma's "Fill")
			pat = imagePattern{img: xdraw.Fill(img, w, h, xdraw.Center, xdraw.Lanczos), ox: int(l.X), oy: int(l.Y), alpha: alpha}
		}
		dc.SetFillStyle(pat)
		dc.SetStrokeStyle(pat)
	default: // solid
		if p.Color == "" {
			return false
		}
		dc.SetColor(withAlpha(parseHex(p.Color, color.White), alpha))
	}
	return true
}

// strokeShape strokes the path built by buildPath once per visible stroke
// paint, bottom→top — Figma's stroke stack (multiple colours, gradients or
// images on one stroke). The caller configures width/cap/join/dash first
// (applyStroke); gg's Stroke() consumes the path, so it's rebuilt per paint.
func (r *Renderer) strokeShape(ctx context.Context, dc *gg.Context, l layout.Layer, opacity float64, buildPath func(*gg.Context)) {
	for _, p := range strokePaintsOf(l) {
		if p.Hidden {
			continue
		}
		a := opacity * paintOpacity(p)
		if a <= 0 {
			continue
		}
		if !r.setPaintStyle(ctx, dc, l, p, a) {
			continue
		}
		buildPath(dc)
		dc.Stroke()
	}
}

// fillShape fills the shape built by buildPath with the layer's paint stack,
// bottom→top. The gradient/image space is the layer's bounding box.
func (r *Renderer) fillShape(ctx context.Context, dc *gg.Context, l layout.Layer, opacity float64, buildPath func(*gg.Context)) {
	ps := paintsOf(l)
	if len(ps) == 0 {
		return
	}
	for _, p := range ps {
		if p.Hidden {
			continue
		}
		a := opacity * paintOpacity(p)
		if a <= 0 {
			continue
		}
		switch p.Type {
		case "linear", "radial", "angular", "diamond":
			if len(p.Stops) == 0 {
				continue
			}
			dc.SetFillStyle(gradPattern{p: p, x: l.X, y: l.Y, w: l.W, h: l.H, alpha: a})
			buildPath(dc)
			dc.Fill()
		case "image":
			img := r.fetchImage(ctx, p.Src)
			if img == nil {
				continue
			}
			w := int(math.Max(1, l.W))
			h := int(math.Max(1, l.H))
			dc.Push()
			buildPath(dc)
			dc.Clip()
			switch p.Fit {
			case "contain":
				fit := xdraw.Fit(img, w, h, xdraw.Lanczos)
				fb := fit.Bounds()
				drawImageAlpha(dc, fit, int(l.X)+(w-fb.Dx())/2, int(l.Y)+(h-fb.Dy())/2, a)
			case "tile":
				tile := img
				if tb := tile.Bounds(); tb.Dx() > 256 {
					tile = xdraw.Resize(tile, 256, 0, xdraw.Lanczos)
				}
				tb := tile.Bounds()
				for ty := int(l.Y); ty < int(l.Y)+h; ty += tb.Dy() {
					for tx := int(l.X); tx < int(l.X)+w; tx += tb.Dx() {
						drawImageAlpha(dc, tile, tx, ty, a)
					}
				}
			default: // cover (Figma's "Fill")
				drawImageAlpha(dc, xdraw.Fill(img, w, h, xdraw.Center, xdraw.Lanczos), int(l.X), int(l.Y), a)
			}
			dc.ResetClip()
			dc.Pop()
		default: // solid
			if p.Color == "" {
				continue
			}
			dc.SetColor(withAlpha(parseHex(p.Color, color.Black), a))
			buildPath(dc)
			dc.Fill()
		}
	}
}
