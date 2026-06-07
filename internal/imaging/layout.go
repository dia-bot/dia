package imaging

import (
	"context"
	"image"
	"image/color"
	"math"
	"strings"

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
	nr.A = uint8(float64(nr.A) * mul)
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

	// draw paints a single layer onto the given context. Factored out so a mask
	// group can render its content + stencil onto separate sub-contexts.
	draw := func(dc *gg.Context, l layout.Layer) {
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
			fill := withAlpha(parseHex(l.Fill, color.Black), opacity)
			dc.SetColor(fill)
			if l.Radius > 0 {
				dc.DrawRoundedRectangle(l.X, l.Y, l.W, l.H, l.Radius)
			} else {
				dc.DrawRectangle(l.X, l.Y, l.W, l.H)
			}
			dc.Fill()
			if l.StrokeWidth > 0 {
				dc.SetColor(withAlpha(parseHex(l.StrokeColor, color.White), opacity))
				dc.SetLineWidth(l.StrokeWidth)
				if l.Radius > 0 {
					dc.DrawRoundedRectangle(l.X, l.Y, l.W, l.H, l.Radius)
				} else {
					dc.DrawRectangle(l.X, l.Y, l.W, l.H)
				}
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
			switch l.Align {
			case "center":
				dc.DrawStringWrapped(text, l.X+l.W/2, l.Y, 0.5, 0, width, 1.3, gg.AlignCenter)
			case "right":
				dc.DrawStringWrapped(text, l.X+l.W, l.Y, 1, 0, width, 1.3, gg.AlignRight)
			default: // left
				dc.DrawStringWrapped(text, l.X, l.Y, 0, 0, width, 1.3, gg.AlignLeft)
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
					if l.Radius > 0 {
						dc.DrawRoundedRectangle(l.X-l.RingWidth, l.Y-l.RingWidth, l.W+2*l.RingWidth, l.H+2*l.RingWidth, l.Radius+l.RingWidth)
					} else {
						dc.DrawRectangle(l.X-l.RingWidth, l.Y-l.RingWidth, l.W+2*l.RingWidth, l.H+2*l.RingWidth)
					}
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
				if l.Radius > 0 {
					dc.DrawRoundedRectangle(l.X, l.Y, l.W, l.H, l.Radius)
				} else {
					dc.DrawRectangle(l.X, l.Y, l.W, l.H)
				}
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
				rad := l.Radius
				if rad <= 0 {
					rad = radius * 0.3
				}
				if l.RingWidth > 0 {
					dc.SetColor(ring)
					dc.DrawRoundedRectangle(l.X-l.RingWidth, l.Y-l.RingWidth, l.W+2*l.RingWidth, l.H+2*l.RingWidth, rad+l.RingWidth)
					dc.Fill()
				}
				dc.Push()
				dc.DrawRoundedRectangle(l.X, l.Y, l.W, l.H, rad)
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
			if l.Radius > 0 {
				c.DrawRoundedRectangle(l.X, l.Y, l.W, l.H, l.Radius)
			} else {
				c.DrawRectangle(l.X, l.Y, l.W, l.H)
			}
			c.Fill()
		}
		if rotate {
			c.Pop()
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
		fillLayer := vis[len(vis)-1]
		if op == "subtract" {
			fillLayer = vis[0]
		}
		fill := parseHex(fillLayer.Fill, color.White)
		opacity := 1.0
		if fillLayer.Opacity != nil {
			opacity = *fillLayer.Opacity
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
			draw(stencil, l)
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
