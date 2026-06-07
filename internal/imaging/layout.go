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

	// Render the layers, honouring "use as mask": a clip layer is a stencil that
	// masks the run of layers above it (until the next clip layer). The stencil
	// itself is not painted — only its alpha/luminance shapes the masked content.
	n := len(in.Layers)
	if n > 50 { // safety backstop; the editor caps layers well below this
		n = 50
	}
	for i := 0; i < n; i++ {
		l := in.Layers[i]
		if !l.Clip {
			draw(dc, l)
			continue
		}
		// Gather the masked run above this stencil. A grouped mask clips only its
		// own group (a "mask group"); a groupless mask clips until the next mask.
		j := i + 1
		if l.Group != "" {
			for j < n && !in.Layers[j].Clip && in.Layers[j].Group == l.Group {
				j++
			}
		} else {
			for j < n && !in.Layers[j].Clip {
				j++
			}
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

// applyMask multiplies content's alpha by the stencil's coverage. "alpha" uses
// the stencil's opacity; "luminance" uses its brightness × opacity (Figma's two
// core mask types). Both images are the same size (the canvas).
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
			if mode == "luminance" {
				lum := (0.2126*float64(m.R) + 0.7152*float64(m.G) + 0.0722*float64(m.B)) / 255
				f = lum * float64(m.A) / 255
			} else {
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
