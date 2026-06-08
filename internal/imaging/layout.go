package imaging

import (
	"context"
	"image"
	"image/color"
	"math"
	"strings"
	"unicode"

	xdraw "github.com/disintegration/imaging"
	"github.com/fogleman/gg"

	"github.com/dia-bot/dia/internal/layout"
	"github.com/dia-bot/dia/internal/templating"
)

// renderText renders a card layer's text/image-source as a pure Go template
// against the nested card data. On a malformed template it returns the raw
// string so a bad template never crashes the render.
func (r *Renderer) renderText(ctx context.Context, s string, data map[string]any) string {
	if s == "" {
		return ""
	}
	if out, err := r.tmpl.RenderCard(ctx, s, data); err == nil {
		return out
	}
	return s
}

// applyVars replaces every occurrence of each key in vars with its value.
func applyVars(s string, vars map[string]string) string {
	if s == "" || len(vars) == 0 {
		return s
	}
	for k, v := range vars {
		if strings.Contains(s, k) {
			s = strings.ReplaceAll(s, k, v)
		}
	}
	return s
}

// withAlpha multiplies the alpha channel of c by mul (clamped to [0,1]).
func withAlpha(c color.Color, mul float64) color.Color {
	if mul >= 1 {
		return c
	}
	if mul < 0 {
		mul = 0
	}
	nr := color.NRGBAModel.Convert(c).(color.NRGBA)
	nr.A = uint8(float64(nr.A)*mul + 0.5)
	return nr
}

// RenderLayout renders a declarative layout document to a PNG. Text and image
// sources are substituted with vars before drawing; the renderer is otherwise a
// pure projection of the layout schema onto a gg canvas.
// fonts is an optional family → URL map of the guild's uploaded custom fonts;
// a text layer naming one of those families renders with it. Pass nil when there
// are none.
func (r *Renderer) RenderLayout(ctx context.Context, in layout.Layout, vars map[string]string, fonts map[string]string) ([]byte, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	defer r.release()

	// Clamp to the canvas limits so a malformed/oversized layout can't blow up
	// memory (gg allocates w*h*4 bytes up front).
	w, h := layout.ClampSize(in.Width, in.Height)

	dc := gg.NewContext(w, h)

	fallbackBG := parseHex(BrandInk, color.Black)
	imgFallback := parseHex("#0b0b0e", color.Black) // matches the DOM's image-bg base
	fillRect := func(c color.Color) {
		dc.SetColor(c)
		dc.DrawRectangle(0, 0, float64(w), float64(h))
		dc.Fill()
	}
	// drawImageBG paints a cover-fitted, optionally-blurred background image. The
	// blur radius is the numeric canvas-px value the editor's slider sets, matching
	// the DOM preview. Returns false if there's no usable image.
	drawImageBG := func(url string) bool {
		if url == "" {
			return false
		}
		img := r.fetchImage(ctx, url)
		if img == nil {
			return false
		}
		fitted := xdraw.Fill(img, w, h, xdraw.Center, xdraw.Lanczos)
		if b := in.Background.Blur; b > 0 {
			fitted = xdraw.Blur(fitted, b)
		}
		dc.DrawImage(fitted, 0, 0)
		return true
	}
	// Branch on the declared background Type (matching the DOM's type-first logic);
	// fall back to field-presence only for legacy documents with no Type set.
	switch in.Background.Type {
	case "image":
		if !drawImageBG(in.Background.ImageURL) {
			fillRect(imgFallback)
		}
	case "solid":
		r.drawBackground(ctx, dc, w, h, Background{Color: in.Background.Color}, fallbackBG)
	case "gradient":
		r.drawBackground(ctx, dc, w, h, Background{
			From:  in.Background.From,
			To:    in.Background.To,
			Angle: in.Background.Angle,
		}, fallbackBG)
	default:
		if in.Background.ImageURL != "" {
			if !drawImageBG(in.Background.ImageURL) {
				fillRect(fallbackBG)
			}
		} else {
			r.drawBackground(ctx, dc, w, h, Background{
				Color: in.Background.Color,
				From:  in.Background.From,
				To:    in.Background.To,
				Angle: in.Background.Angle,
			}, fallbackBG)
		}
	}

	// Card data root (pure Go template): {{.User.Username}}, {{.User.Avatar}}, …
	data := templating.DataFromVars(vars)

	// drawRaw paints a single layer's own content (no effects) onto the given
	// context. Factored out so a mask group can render its content + stencil onto
	// separate sub-contexts, and so the effect-aware draw can render to a buffer.
	drawRaw := func(dc *gg.Context, l layout.Layer) {
		if l.Hidden {
			return
		}

		// Opacity mirrors the DOM exactly: unset -> 1, explicit 0 -> not drawn.
		opacity := 1.0
		if l.Opacity != nil {
			opacity = *l.Opacity
		}
		if opacity <= 0 {
			return
		}
		if opacity > 1 {
			opacity = 1
		}

		text := r.renderText(ctx, l.Text, data)
		src := r.renderText(ctx, l.Src, data)

		rotate := l.Rotation != 0
		if rotate {
			dc.Push()
			dc.RotateAbout(l.Rotation*math.Pi/180, l.X+l.W/2, l.Y+l.H/2)
		}
		switch l.Type {
		case "rect":
			tl, tr, br, bl := cornerRadii(l)
			dc.SetColor(withAlpha(parseHex(l.Fill, color.Black), opacity))
			drawRoundRect(dc, l.X, l.Y, l.W, l.H, tl, tr, br, bl)
			dc.Fill()
			if l.StrokeWidth > 0 {
				dc.SetColor(withAlpha(parseHex(l.StrokeColor, color.White), opacity))
				dc.SetLineWidth(l.StrokeWidth)
				drawRoundRect(dc, l.X, l.Y, l.W, l.H, tl, tr, br, bl)
				dc.Stroke()
			}

		case "ellipse":
			dc.SetColor(withAlpha(parseHex(l.Fill, color.Black), opacity))
			dc.DrawEllipse(l.X+l.W/2, l.Y+l.H/2, l.W/2, l.H/2)
			dc.Fill()
			if l.StrokeWidth > 0 {
				dc.SetColor(withAlpha(parseHex(l.StrokeColor, color.White), opacity))
				dc.SetLineWidth(l.StrokeWidth)
				dc.DrawEllipse(l.X+l.W/2, l.Y+l.H/2, l.W/2, l.H/2)
				dc.Stroke()
			}

		case "path":
			if len(l.Nodes) >= 2 {
				dc.SetLineCapRound()
				dc.SetLineJoinRound()
				dc.MoveTo(l.Nodes[0].X, l.Nodes[0].Y)
				for k := 1; k < len(l.Nodes); k++ {
					a, c := l.Nodes[k-1], l.Nodes[k]
					dc.CubicTo(a.H2X, a.H2Y, c.H1X, c.H1Y, c.X, c.Y)
				}
				if l.Closed && len(l.Nodes) >= 3 {
					a, c := l.Nodes[len(l.Nodes)-1], l.Nodes[0]
					dc.CubicTo(a.H2X, a.H2Y, c.H1X, c.H1Y, c.X, c.Y)
					dc.ClosePath()
				}
				hasStroke := l.StrokeWidth > 0
				if l.Closed && len(l.Nodes) >= 3 && l.Fill != "" {
					dc.SetColor(withAlpha(parseHex(l.Fill, color.White), opacity))
					if hasStroke {
						dc.FillPreserve()
					} else {
						dc.Fill()
					}
				}
				if hasStroke {
					dc.SetColor(withAlpha(parseHex(l.StrokeColor, color.White), opacity))
					dc.SetLineWidth(l.StrokeWidth)
					dc.Stroke()
				}
				dc.ClearPath()
			}

		case "text":
			size := l.FontSize
			if size <= 0 {
				size = 32
			}
			if f := r.faceFor(ctx, l.FontFamily, l.FontWeight >= 700, size, fonts); f != nil {
				dc.SetFontFace(f)
			}
			dc.SetColor(withAlpha(parseHex(l.Color, color.White), opacity))

			width := l.W
			if width <= 0 {
				width = float64(w)
			}
			s := applyTextCase(text, l.TextCase)
			tracking := l.LetterSpacing
			lineMul := l.LineHeight
			if lineMul <= 0 {
				lineMul = 1.3 // matches the DOM preview's default line-height
			}
			// Wrap: gg's WordWrap for the plain case (identical to the legacy path),
			// a tracking-aware greedy wrap when letter-spacing is set.
			var lines []string
			if tracking == 0 {
				lines = dc.WordWrap(s, width)
			} else {
				lines = wrapText(dc, s, width, tracking)
			}
			if len(lines) == 0 {
				lines = []string{""}
			}
			_, lh := dc.MeasureString("Ag") // font line height (string-independent)
			if lh <= 0 {
				lh = size
			}
			adv := lh * lineMul
			block := lh
			if len(lines) > 1 {
				block += adv * float64(len(lines)-1)
			}
			// Vertical alignment within the box. 'top'/unset keeps the legacy origin
			// (first line top at l.Y) so existing cards don't shift.
			top := l.Y
			switch l.VAlign {
			case "middle":
				top = l.Y + (l.H-block)/2
			case "bottom":
				top = l.Y + l.H - block
			}
			underline := l.TextDecoration == "underline"
			strike := l.TextDecoration == "strike"
			thick := math.Max(1, size*0.06)
			for i, line := range lines {
				lineTop := top + float64(i)*adv
				lw := lineWidth(dc, line, tracking)
				x0 := l.X
				switch l.Align {
				case "center":
					x0 = l.X + (l.W-lw)/2
				case "right":
					x0 = l.X + l.W - lw
				}
				// ay=1 anchors each line by its top (baseline = lineTop+lh), exactly as
				// gg's DrawStringWrapped does, so the plain case matches the old output.
				if tracking == 0 {
					dc.DrawStringAnchored(line, x0, lineTop, 0, 1)
				} else {
					cx := x0
					for _, ru := range line {
						g := string(ru)
						dc.DrawStringAnchored(g, cx, lineTop, 0, 1)
						aw, _ := dc.MeasureString(g)
						cx += aw + tracking
					}
				}
				if (underline || strike) && lw > 0 {
					baseline := lineTop + lh
					dy := baseline + thick // underline just below the baseline
					if strike {
						dy = baseline - lh*0.28 // strike through the x-height
					}
					dc.DrawRectangle(x0, dy, lw, thick)
					dc.Fill()
				}
			}

		case "image":
			if src == "" {
				break
			}
			img := r.fetchImage(ctx, src)
			if img == nil {
				break
			}
			// A circle mask fills a centered SQUARE (side=min(W,H)) to match the DOM
			// exactly; every other mask fills the box.
			drawW, drawH := l.W, l.H
			if l.Mask == "circle" {
				side := l.W
				if l.H < side {
					side = l.H
				}
				drawW, drawH = side, side
			}
			iw, ih := int(drawW), int(drawH)
			if iw <= 0 || ih <= 0 {
				break
			}
			var fitted = img
			switch l.Fit {
			case "contain":
				fitted = xdraw.Fit(img, iw, ih, xdraw.Lanczos)
			default: // cover
				fitted = xdraw.Fill(img, iw, ih, xdraw.Center, xdraw.Lanczos)
			}
			if opacity < 1 {
				fitted = xdraw.AdjustFunc(fitted, func(c color.NRGBA) color.NRGBA {
					c.A = uint8(float64(c.A) * opacity)
					return c
				})
			}
			cx, cy := l.X+l.W/2, l.Y+l.H/2
			// Ring: a coloured border behind the image (matches the DOM box-shadow),
			// so a circular image is a full member avatar.
			if l.RingWidth > 0 {
				dc.SetColor(withAlpha(parseHex(l.RingColor, color.White), opacity))
				switch l.Mask {
				case "circle":
					dc.DrawCircle(cx, cy, drawW/2+l.RingWidth)
				case "ellipse":
					dc.DrawEllipse(cx, cy, l.W/2+l.RingWidth, l.H/2+l.RingWidth)
				default:
					rtl, rtr, rbr, rbl := cornerRadii(l)
					rc := func(c float64) float64 {
						if c > 0 {
							return c + l.RingWidth
						}
						return 0
					}
					drawRoundRect(dc, l.X-l.RingWidth, l.Y-l.RingWidth, l.W+2*l.RingWidth, l.H+2*l.RingWidth, rc(rtl), rc(rtr), rc(rbr), rc(rbl))
				}
				dc.Fill()
			}
			dc.Push()
			switch l.Mask {
			case "circle":
				dc.DrawCircle(cx, cy, drawW/2)
			case "ellipse":
				dc.DrawEllipse(cx, cy, l.W/2, l.H/2)
			default:
				ctl, ctr, cbr, cbl := cornerRadii(l)
				drawRoundRect(dc, l.X, l.Y, l.W, l.H, ctl, ctr, cbr, cbl)
			}
			dc.Clip()
			// Center the fitted image at the box centre.
			ox := int(cx) - fitted.Bounds().Dx()/2
			oy := int(cy) - fitted.Bounds().Dy()/2
			dc.DrawImage(fitted, ox, oy)
			dc.ResetClip()
			dc.Pop()

		case "avatar":
			cx := l.X + l.W/2
			cy := l.Y + l.H/2
			radius := l.W
			if l.H < l.W {
				radius = l.H
			}
			radius /= 2
			if radius <= 0 {
				break
			}
			ring := parseHex(l.RingColor, color.White)
			img := r.fetchAvatar(ctx, src, int(radius*2), ring)
			if l.Shape == "rounded" {
				tl, tr, br, bl := cornerRadii(l)
				// No explicit radius at all → a gently rounded default.
				if len(l.Corners) != 4 && l.Radius <= 0 {
					d := radius * 0.3
					tl, tr, br, bl = d, d, d, d
				}
				if l.RingWidth > 0 {
					dc.SetColor(ring)
					rc := func(c float64) float64 {
						if c > 0 {
							return c + l.RingWidth
						}
						return 0
					}
					drawRoundRect(dc, l.X-l.RingWidth, l.Y-l.RingWidth, l.W+2*l.RingWidth, l.H+2*l.RingWidth, rc(tl), rc(tr), rc(br), rc(bl))
					dc.Fill()
				}
				dc.Push()
				drawRoundRect(dc, l.X, l.Y, l.W, l.H, tl, tr, br, bl)
				dc.Clip()
				dc.DrawImageAnchored(img, int(cx), int(cy), 0.5, 0.5)
				dc.ResetClip()
				dc.Pop()
			} else {
				r.drawAvatar(dc, img, cx, cy, radius, ring, l.RingWidth)
			}
		}
		if rotate {
			dc.Pop()
		}
	}

	// drawSilhouette paints a vector layer's filled outline in opaque white onto its
	// own context — used to read a member's coverage for boolean ops (only the alpha
	// channel matters).
	drawSilhouette := func(c *gg.Context, l layout.Layer) {
		rotate := l.Rotation != 0
		if rotate {
			c.Push()
			c.RotateAbout(l.Rotation*math.Pi/180, l.X+l.W/2, l.Y+l.H/2)
		}
		c.SetColor(color.White)
		switch l.Type {
		case "ellipse":
			c.DrawEllipse(l.X+l.W/2, l.Y+l.H/2, l.W/2, l.H/2)
			c.Fill()
		case "path":
			if len(l.Nodes) >= 2 {
				c.MoveTo(l.Nodes[0].X, l.Nodes[0].Y)
				for k := 1; k < len(l.Nodes); k++ {
					a, nn := l.Nodes[k-1], l.Nodes[k]
					c.CubicTo(a.H2X, a.H2Y, nn.H1X, nn.H1Y, nn.X, nn.Y)
				}
				a, nn := l.Nodes[len(l.Nodes)-1], l.Nodes[0]
				c.CubicTo(a.H2X, a.H2Y, nn.H1X, nn.H1Y, nn.X, nn.Y) // close for a filled region
				c.ClosePath()
				c.Fill()
			}
		default: // rect (any non-vector falls back to its box)
			stl, str, sbr, sbl := cornerRadii(l)
			drawRoundRect(c, l.X, l.Y, l.W, l.H, stl, str, sbr, sbl)
			c.Fill()
		}
		if rotate {
			c.Pop()
		}
	}

	// draw paints a layer with its effects (shadows / blur). With no visible
	// effects it's just drawRaw. Effects apply in a FIXED order (not list order),
	// mirroring the web preview: background blur (frost what's behind) → render the
	// layer to a buffer → layer blur → drop shadows under → the layer → inner
	// shadows over.
	draw := func(dc *gg.Context, l layout.Layer) {
		if l.Hidden {
			return
		}
		hasFX := false
		for _, e := range l.Effects {
			if !e.Hidden {
				hasFX = true
				break
			}
		}
		if !hasFX {
			drawRaw(dc, l)
			return
		}

		// Background blur: blur what's already painted, clipped to this layer's
		// shape, so a translucent layer reads as frosted glass over it.
		for _, e := range l.Effects {
			if e.Hidden || e.Type != "background_blur" || e.Radius <= 0 {
				continue
			}
			blurred := xdraw.Blur(dc.Image(), e.Radius)
			stencil := gg.NewContext(w, h)
			drawSilhouette(stencil, l)
			dc.DrawImage(applyMask(blurred, stencil.Image(), "alpha", false), 0, 0)
		}

		// The layer's own pixels, on a transparent buffer (so shadows read its alpha).
		sub := gg.NewContext(w, h)
		drawRaw(sub, l)
		content := image.Image(sub.Image())
		for _, e := range l.Effects {
			if e.Hidden || e.Type != "layer_blur" || e.Radius <= 0 {
				continue
			}
			content = xdraw.Blur(content, e.Radius)
		}

		for _, e := range l.Effects {
			if e.Hidden || e.Type != "drop_shadow" {
				continue
			}
			dc.DrawImage(shadowImage(content, e, w, h, false), 0, 0)
		}
		dc.DrawImage(content, 0, 0)
		for _, e := range l.Effects {
			if e.Hidden || e.Type != "inner_shadow" {
				continue
			}
			dc.DrawImage(shadowImage(content, e, w, h, true), 0, 0)
		}
	}

	// renderBoolean composites a boolean group's member silhouettes with op and
	// paints the result with one fill (Figma: the bottom member's fill for subtract,
	// the top member's otherwise). Members are bottom→top; hidden ones are dropped.
	renderBoolean := func(members []layout.Layer, op string) {
		vis := make([]layout.Layer, 0, len(members))
		for _, m := range members {
			if !m.Hidden {
				vis = append(vis, m)
			}
		}
		if len(vis) < 2 {
			for _, m := range vis {
				draw(dc, m)
			}
			return
		}
		covers := make([]image.Image, len(vis))
		for k, m := range vis {
			sub := gg.NewContext(w, h)
			drawSilhouette(sub, m)
			covers[k] = sub.Image()
		}
		// The result takes the top member's content (the bottom member's for subtract),
		// matching Figma's "result inherits the front-most style".
		source := vis[len(vis)-1]
		if op == "subtract" {
			source = vis[0]
		}
		// An image/avatar source keeps its pixels: paint its real content and clip it to
		// the boolean coverage (so image ∩ shape crops the photo, image − shape cuts a
		// hole, etc.). Any other source fills the coverage with its solid colour.
		if source.Type == "image" || source.Type == "avatar" {
			content := gg.NewContext(w, h)
			drawRaw(content, source) // opacity / fit / mask / ring — but no effects (members composite effect-free, like the fill branch)
			coverage := combineBoolean(covers, op, color.White, 1)
			dc.DrawImage(applyMask(content.Image(), coverage, "alpha", false), 0, 0)
			return
		}
		fill := parseHex(source.Fill, color.White)
		opacity := 1.0
		if source.Opacity != nil {
			opacity = *source.Opacity
		}
		if opacity <= 0 {
			return
		}
		if opacity > 1 {
			opacity = 1
		}
		dc.DrawImage(combineBoolean(covers, op, fill, opacity), 0, 0)
	}

	// Render the layers, honouring "use as mask": a clip layer is a stencil that
	// masks the contiguous run of same-group layers above it (a "mask group"). The
	// stencil itself is not painted — only its alpha/vector/luminance coverage
	// shapes the masked content. Masks are group-scoped (mirrors web maskFor).
	// A group whose metadata carries a boolean op is composited via renderBoolean.
	n := len(in.Layers)
	if n > 50 { // safety backstop; the editor caps layers well below this
		n = 50
	}
	for i := 0; i < n; i++ {
		l := in.Layers[i]
		// Boolean group: the bottom member of a group whose metadata carries a bool
		// op. Composite the whole same-group run, then advance past it.
		if l.Group != "" {
			if g, ok := in.Groups[l.Group]; ok && g.BoolOp != "" {
				j := i + 1
				for j < n && in.Layers[j].Group == l.Group {
					j++
				}
				renderBoolean(in.Layers[i:j], g.BoolOp)
				i = j - 1
				continue
			}
		}
		// A mask must belong to a group; a clip layer with no group draws as a
		// normal layer (the editor never produces one, but stay defensive).
		if !l.Clip || l.Group == "" {
			draw(dc, l)
			continue
		}
		// Gather the masked run above this stencil within its own group.
		j := i + 1
		for j < n && !in.Layers[j].Clip && in.Layers[j].Group == l.Group {
			j++
		}
		if j > i+1 && !l.Hidden {
			content := gg.NewContext(w, h)
			for k := i + 1; k < j; k++ {
				draw(content, in.Layers[k])
			}
			stencil := gg.NewContext(w, h)
			drawRaw(stencil, l) // a stencil is a mask shape — its own effects must not distort it
			dc.DrawImage(applyMask(content.Image(), stencil.Image(), l.ClipMode, l.ClipInvert), 0, 0)
		} else if j > i+1 {
			// A hidden stencil clips nothing → its masked layers draw normally.
			for k := i + 1; k < j; k++ {
				draw(dc, in.Layers[k])
			}
		}
		i = j - 1
	}

	return encodePNG(dc)
}

// applyMask multiplies content's alpha by the stencil's coverage (Figma's three
// mask types):
//
//	"alpha"     — the stencil's opacity (soft edges)
//	"vector"    — hard clip: any covered pixel is fully revealed, the rest hidden
//	"luminance" — the stencil's brightness × opacity
//
// invert flips the coverage (a local extra Figma doesn't have). Both images are
// the same size (the canvas).
func applyMask(content, stencil image.Image, mode string, invert bool) image.Image {
	b := content.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(content.At(x, y)).(color.NRGBA)
			if c.A == 0 {
				continue
			}
			m := color.NRGBAModel.Convert(stencil.At(x, y)).(color.NRGBA)
			var f float64
			switch mode {
			case "luminance":
				lum := (0.2126*float64(m.R) + 0.7152*float64(m.G) + 0.0722*float64(m.B)) / 255
				f = lum * float64(m.A) / 255
			case "vector":
				if m.A > 0 {
					f = 1
				}
			default: // alpha
				f = float64(m.A) / 255
			}
			if invert {
				f = 1 - f
			}
			c.A = uint8(float64(c.A)*f + 0.5)
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

// tintAlpha returns a canvas-sized image whose RGB is col and whose alpha is
// src's alpha × mul — i.e. src's silhouette painted in one flat colour. Used to
// seed a drop shadow from a layer's rendered pixels.
func tintAlpha(src image.Image, col color.Color, mul float64) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(b)
	cc := color.NRGBAModel.Convert(col).(color.NRGBA)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			a := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA).A
			if a == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{R: cc.R, G: cc.G, B: cc.B, A: uint8(float64(a)*mul + 0.5)})
		}
	}
	return out
}

// invFill is tintAlpha's complement: alpha is (255 − src.alpha) × mul, so it's
// opaque OUTSIDE src's silhouette. Used to seed an inner shadow (the dark comes
// from outside the shape, blurs inward, then is clipped back to the shape).
func invFill(src image.Image, col color.Color, mul float64) *image.NRGBA {
	b := src.Bounds()
	out := image.NewNRGBA(b)
	cc := color.NRGBAModel.Convert(col).(color.NRGBA)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			a := color.NRGBAModel.Convert(src.At(x, y)).(color.NRGBA).A
			out.SetNRGBA(x, y, color.NRGBA{R: cc.R, G: cc.G, B: cc.B, A: uint8(float64(255-a)*mul + 0.5)})
		}
	}
	return out
}

// growAlpha approximates a shadow spread: dilating (px>0) or eroding (px<0) an
// image's alpha by ~px. A small blur turns each edge into a ramp; remapping that
// ramp around a lower (dilate) or higher (erode) threshold shifts the edge out/in
// while keeping anti-aliasing. RGB is preserved (the input is already flat-tinted).
func growAlpha(src image.Image, px float64) image.Image {
	amt := math.Abs(px)
	if amt < 0.5 {
		return src
	}
	blurred := xdraw.Blur(src, amt*0.6)
	thr := 0.2 // dilate: a low crossing point pushes the edge outward
	if px < 0 {
		thr = 0.8 // erode: a high crossing point pulls it inward
	}
	const aa = 0.18 // soft ramp half-width (keeps edges anti-aliased)
	b := blurred.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(blurred.At(x, y)).(color.NRGBA)
			na := (float64(c.A)/255 - (thr - aa)) / (2 * aa)
			switch {
			case na <= 0:
				c.A = 0
			case na >= 1:
				c.A = 255
			default:
				c.A = uint8(na*255 + 0.5)
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}

// shadowImage builds a drop (inner=false) or inner (inner=true) shadow for a layer
// whose rendered pixels are content. Returns a canvas-sized image to composite —
// under the content for a drop shadow, over it for an inner shadow. Mirrors the
// web preview's CSS drop-shadow / inset shadow.
func shadowImage(content image.Image, e layout.Effect, w, h int, inner bool) image.Image {
	op := 0.25
	if e.Opacity != nil {
		op = *e.Opacity
	}
	if op <= 0 {
		return image.NewNRGBA(image.Rect(0, 0, w, h))
	}
	if op > 1 {
		op = 1
	}
	col := parseHex(e.Color, color.Black)
	if !inner {
		shadow := image.Image(tintAlpha(content, col, op))
		if e.Spread != 0 {
			shadow = growAlpha(shadow, e.Spread)
		}
		if e.Radius > 0 {
			shadow = xdraw.Blur(shadow, e.Radius)
		}
		out := gg.NewContext(w, h)
		out.DrawImage(shadow, int(math.Round(e.X)), int(math.Round(e.Y)))
		return out.Image()
	}
	// Inner: seed with the inverse silhouette, grow(+ contracts the lit core),
	// offset + blur, then clip the result back to the shape so only its inner edge
	// darkens.
	inv := image.Image(invFill(content, col, op))
	if e.Spread != 0 {
		inv = growAlpha(inv, e.Spread)
	}
	shifted := gg.NewContext(w, h)
	shifted.DrawImage(inv, int(math.Round(e.X)), int(math.Round(e.Y)))
	blurred := image.Image(shifted.Image())
	if e.Radius > 0 {
		blurred = xdraw.Blur(blurred, e.Radius)
	}
	return applyMask(blurred, content, "alpha", false)
}

// combineBoolean composites member coverage (each image's alpha) with a boolean
// op and paints the result with a single fill colour. covers are bottom→top
// (covers[0] is the base for subtract):
//
//	union     = max(coverage)
//	intersect = min(coverage)
//	subtract  = base · ∏(1 − others)
//	exclude   = odd-parity (matches SVG even-odd in the preview)
//
// All images are canvas-sized.
func combineBoolean(covers []image.Image, op string, fill color.Color, opacity float64) image.Image {
	b := covers[0].Bounds()
	out := image.NewNRGBA(b)
	fc := color.NRGBAModel.Convert(fill).(color.NRGBA)
	alphas := make([]float64, len(covers))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			for k, im := range covers {
				alphas[k] = float64(color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA).A) / 255
			}
			var cov float64
			switch op {
			case "intersect":
				cov = alphas[0]
				for _, a := range alphas[1:] {
					if a < cov {
						cov = a
					}
				}
			case "subtract":
				cov = alphas[0]
				for _, a := range alphas[1:] {
					cov *= 1 - a
				}
			case "exclude":
				cnt := 0
				for _, a := range alphas {
					if a >= 0.5 {
						cnt++
					}
				}
				if cnt%2 == 1 {
					cov = 1
				}
			default: // union
				for _, a := range alphas {
					if a > cov {
						cov = a
					}
				}
			}
			cov *= opacity
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			out.SetNRGBA(x, y, color.NRGBA{R: fc.R, G: fc.G, B: fc.B, A: uint8(cov*255 + 0.5)})
		}
	}
	return out
}

// cornerRadii returns the four corner radii (tl, tr, br, bl) for a layer: its
// independent Corners when set (len 4), else its uniform Radius on all four.
func cornerRadii(l layout.Layer) (tl, tr, br, bl float64) {
	if len(l.Corners) == 4 {
		return l.Corners[0], l.Corners[1], l.Corners[2], l.Corners[3]
	}
	return l.Radius, l.Radius, l.Radius, l.Radius
}

// drawRoundRect adds a rounded rectangle with per-corner radii to dc's path (so
// callers Fill/Stroke it). Radii are clamped so adjacent corners can't overlap;
// equal corners fall through to gg's built-ins. Order: tl, tr, br, bl.
func drawRoundRect(dc *gg.Context, x, y, w, h, tl, tr, br, bl float64) {
	mx := math.Min(w, h) / 2
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > mx {
			return mx
		}
		return v
	}
	tl, tr, br, bl = clamp(tl), clamp(tr), clamp(br), clamp(bl)
	if tl == tr && tr == br && br == bl {
		if tl <= 0 {
			dc.DrawRectangle(x, y, w, h)
		} else {
			dc.DrawRoundedRectangle(x, y, w, h, tl)
		}
		return
	}
	const k = 0.5522847498 // circle→cubic-bezier constant
	dc.MoveTo(x+tl, y)
	dc.LineTo(x+w-tr, y)
	dc.CubicTo(x+w-tr+tr*k, y, x+w, y+tr-tr*k, x+w, y+tr)
	dc.LineTo(x+w, y+h-br)
	dc.CubicTo(x+w, y+h-br+br*k, x+w-br+br*k, y+h, x+w-br, y+h)
	dc.LineTo(x+bl, y+h)
	dc.CubicTo(x+bl-bl*k, y+h, x, y+h-bl+bl*k, x, y+h-bl)
	dc.LineTo(x, y+tl)
	dc.CubicTo(x, y+tl-tl*k, x+tl-tl*k, y, x+tl, y)
	dc.ClosePath()
}

// applyTextCase mirrors the editor's text-case transform (Figma's case control).
func applyTextCase(s, mode string) string {
	switch mode {
	case "upper":
		return strings.ToUpper(s)
	case "lower":
		return strings.ToLower(s)
	case "title":
		var b strings.Builder
		prevSpace := true
		for _, r := range s {
			if prevSpace && unicode.IsLetter(r) {
				b.WriteRune(unicode.ToUpper(r))
			} else {
				b.WriteRune(r)
			}
			prevSpace = unicode.IsSpace(r)
		}
		return b.String()
	default:
		return s
	}
}

// lineWidth measures a single line's advance, adding letter-spacing tracking
// between glyphs (so wrapping + alignment account for it).
func lineWidth(dc *gg.Context, s string, tracking float64) float64 {
	w, _ := dc.MeasureString(s)
	if tracking != 0 {
		if n := len([]rune(s)); n > 1 {
			w += tracking * float64(n-1)
		}
	}
	return w
}

// wrapText greedily word-wraps each paragraph (split on '\n') to maxw, measuring
// with tracking. Used only when letter-spacing is set; otherwise the renderer uses
// gg's own WordWrap to keep wrapping identical to the legacy text path.
func wrapText(dc *gg.Context, text string, maxw, tracking float64) []string {
	var lines []string
	for _, para := range strings.Split(text, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := words[0]
		for _, wd := range words[1:] {
			trial := cur + " " + wd
			if lineWidth(dc, trial, tracking) > maxw {
				lines = append(lines, cur)
				cur = wd
			} else {
				cur = trial
			}
		}
		lines = append(lines, cur)
	}
	return lines
}
