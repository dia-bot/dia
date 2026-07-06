package imaging

import (
	"context"
	"image"
	"image/color"
	"math"
	"strconv"
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

// progressFraction parses a rank-card progress value into a 0..1 fraction used to
// fill a progress-bar rect's width. It accepts the token the rank card exposes as
// {{ .Progress }} ("64%"), a bare percent ("64"), or a 0..1 fraction ("0.64"). ok
// is false when the value is absent or unparseable, so the caller leaves the rect
// at full width, welcome cards, which carry no progress var, stay unaffected.
func progressFraction(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	pctForm := strings.HasSuffix(s, "%")
	v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
	if err != nil {
		return 0, false
	}
	// A "%"-suffixed value or any number above 1 is a 0..100 percent; a bare value
	// in [0,1] is already a fraction ("0.45").
	if pctForm || v > 1 {
		v /= 100
	}
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	return v, true
}

// resolveLayerBindings evaluates each layer's `bind` formulas (Go templates over
// the card data root) and overrides the matching static fields with the results,
// so the rest of the renderer draws data-driven values without knowing about
// bindings. Layers with no bindings pass through untouched (and the whole slice
// is shared unchanged when nothing is bound). A formula that errors or yields an
// unparseable value leaves the static field in place, so a bad formula never
// breaks a render. See internal/layout/schema.go Layer.Bind for the key list.
func (r *Renderer) resolveLayerBindings(ctx context.Context, layers []layout.Layer, data map[string]any) []layout.Layer {
	bound := false
	for i := range layers {
		if len(layers[i].Bind) > 0 {
			bound = true
			break
		}
	}
	if !bound {
		return layers
	}
	out := make([]layout.Layer, len(layers))
	copy(out, layers)
	for i := range out {
		l := &out[i]
		if len(l.Bind) == 0 {
			continue
		}
		// eval renders a bound expression; ok is false when the key is absent/empty,
		// the template errors (RenderCardStrict fails on a typo'd/out-of-scope field
		// under missingkey=error), or it yields the "<no value>" sentinel — so a bad
		// formula always keeps the static value instead of clobbering it.
		eval := func(key string) (string, bool) {
			expr, has := l.Bind[key]
			if !has || strings.TrimSpace(expr) == "" {
				return "", false
			}
			s, err := r.tmpl.RenderCardStrict(ctx, expr, data)
			if err != nil || strings.TrimSpace(s) == "<no value>" {
				return "", false
			}
			return s, true
		}
		num := func(key string) (float64, bool) {
			s, ok := eval(key)
			if !ok {
				return 0, false
			}
			s = strings.TrimSuffix(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), "%")
			v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err != nil {
				return 0, false
			}
			return v, true
		}
		if v, ok := num("x"); ok {
			l.X = v
		}
		if v, ok := num("y"); ok {
			l.Y = v
		}
		if v, ok := num("w"); ok {
			l.W = v
		}
		if v, ok := num("h"); ok {
			l.H = v
		}
		if v, ok := num("rotation"); ok {
			l.Rotation = v
		}
		if v, ok := num("font_size"); ok {
			l.FontSize = v
		}
		if v, ok := num("radius"); ok {
			l.Radius = v
			l.Corners = nil // a bound uniform radius overrides per-corner radii
		}
		if v, ok := num("stroke_width"); ok {
			l.StrokeWidth = v
		}
		if v, ok := num("letter_spacing"); ok {
			l.LetterSpacing = v
		}
		if v, ok := num("line_height"); ok {
			l.LineHeight = v
		}
		if v, ok := num("font_weight"); ok {
			l.FontWeight = int(v)
		}
		if v, ok := num("dash"); ok {
			l.Dash = v
		}
		if v, ok := num("gap"); ok {
			l.Gap = v
		}
		if v, ok := num("miter_angle"); ok {
			l.MiterAngle = v
		}
		if v, ok := num("scatter_gap"); ok {
			l.ScatterGap = v
		}
		if v, ok := num("scatter_wiggle"); ok {
			l.ScatterWiggle = v
		}
		if v, ok := num("scatter_size"); ok {
			l.ScatterSize = v
		}
		if v, ok := num("scatter_rotation"); ok {
			l.ScatterRotation = v
		}
		if v, ok := num("scatter_angular"); ok {
			l.ScatterAngular = v
		}
		if v, ok := num("dynamic_frequency"); ok {
			l.DynamicFrequency = v
		}
		if v, ok := num("dynamic_wiggle"); ok {
			l.DynamicWiggle = v
		}
		if v, ok := num("dynamic_smoothen"); ok {
			l.DynamicSmoothen = v
		}
		// Per-corner radii (rect/image): corner_tl/tr/br/bl override Corners[0..3].
		_, c1 := l.Bind["corner_tl"]
		_, c2 := l.Bind["corner_tr"]
		_, c3 := l.Bind["corner_br"]
		_, c4 := l.Bind["corner_bl"]
		if c1 || c2 || c3 || c4 {
			c := make([]float64, 4)
			if len(l.Corners) == 4 {
				copy(c, l.Corners)
			} else {
				for i := range c {
					c[i] = l.Radius
				}
			}
			for i, k := range [4]string{"corner_tl", "corner_tr", "corner_br", "corner_bl"} {
				if v, ok := num(k); ok {
					c[i] = v
				}
			}
			l.Corners = c
		}
		if v, ok := num("opacity"); ok {
			if v < 0 {
				v = 0
			} else if v > 1 {
				v = 1
			}
			l.Opacity = &v
		}
		// Colours: the rendered string is used as a hex value. A bound flat colour
		// clears the paint stack so it actually takes effect (Fills/Strokes else win).
		if s, ok := eval("color"); ok {
			if s = strings.TrimSpace(s); s != "" {
				l.Color = s
				// Text glyphs paint from the Fills stack when present, using Color only
				// as fallback, so a bound flat text colour must clear the stack to win.
				if l.Type == "text" {
					l.Fills = nil
				}
			}
		}
		if s, ok := eval("fill"); ok {
			if s = strings.TrimSpace(s); s != "" {
				l.Fill = s
				l.Fills = nil
			}
		}
		if s, ok := eval("stroke_color"); ok {
			if s = strings.TrimSpace(s); s != "" {
				l.StrokeColor = s
				l.Strokes = nil
			}
		}
		if s, ok := eval("hidden"); ok {
			l.Hidden = truthyBind(s)
		}
		if s, ok := eval("progress"); ok {
			l.Progress = truthyBind(s)
		}
		if s, ok := eval("closed"); ok {
			l.Closed = truthyBind(s)
		}
		if s, ok := eval("clip"); ok {
			l.Clip = truthyBind(s)
		}
		if s, ok := eval("clip_invert"); ok {
			l.ClipInvert = truthyBind(s)
		}
		// Enum / string fields: the formula must output a valid value; the renderer
		// falls back to its own default for anything unknown, so a bad value is safe.
		setStr := func(key string, dst *string) {
			if s, ok := eval(key); ok {
				if s = strings.TrimSpace(s); s != "" {
					*dst = s
				}
			}
		}
		setStr("align", &l.Align)
		setStr("valign", &l.VAlign)
		setStr("text_case", &l.TextCase)
		setStr("text_decoration", &l.TextDecoration)
		setStr("font_family", &l.FontFamily)
		setStr("fit", &l.Fit)
		setStr("stroke_align", &l.StrokeAlign)
		setStr("stroke_style", &l.StrokeStyle)
		setStr("stroke_cap", &l.StrokeCap)
		setStr("stroke_join", &l.StrokeJoin)
		setStr("width_profile", &l.WidthProfile)
		setStr("start_cap", &l.StartCap)
		setStr("end_cap", &l.EndCap)
		setStr("brush_name", &l.BrushName)
		setStr("brush_direction", &l.BrushDirection)
		setStr("clip_mode", &l.ClipMode)
	}
	return out
}

// resolveBackgroundBindings evaluates the canvas background's `bind` formulas
// (color / from / to / angle / blur) against the card data, so the whole card
// backdrop can be data-driven (e.g. tint by level). Like the layer resolver, a
// bad formula keeps the static value. A bound colour/gradient forces the legacy
// solid/gradient path (clears the paint stack) so it actually takes effect.
func (r *Renderer) resolveBackgroundBindings(ctx context.Context, bg layout.Background, data map[string]any) layout.Background {
	if len(bg.Bind) == 0 {
		return bg
	}
	eval := func(key string) (string, bool) {
		expr, has := bg.Bind[key]
		if !has || strings.TrimSpace(expr) == "" {
			return "", false
		}
		s, err := r.tmpl.RenderCardStrict(ctx, expr, data)
		if err != nil || strings.TrimSpace(s) == "<no value>" {
			return "", false
		}
		return s, true
	}
	num := func(key string) (float64, bool) {
		s, ok := eval(key)
		if !ok {
			return 0, false
		}
		s = strings.TrimSuffix(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), "%")
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return 0, false
		}
		return v, true
	}
	if s, ok := eval("color"); ok {
		if s = strings.TrimSpace(s); s != "" {
			bg.Color, bg.Type, bg.Fills = s, "solid", nil
		}
	}
	if s, ok := eval("from"); ok {
		if s = strings.TrimSpace(s); s != "" {
			bg.From, bg.Type, bg.Fills = s, "gradient", nil
		}
	}
	if s, ok := eval("to"); ok {
		if s = strings.TrimSpace(s); s != "" {
			bg.To, bg.Type, bg.Fills = s, "gradient", nil
		}
	}
	if v, ok := num("angle"); ok {
		bg.Angle = v
	}
	if v, ok := num("blur"); ok {
		bg.Blur = v
	}
	return bg
}

// truthyBind reads a bound `hidden` formula's output as a boolean. Empty, "0",
// "false"/"no"/"off", "null" and Go's "<no value>" are false; anything else true.
func truthyBind(s string) bool {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "", "0", "false", "no", "off", "null", "<no value>":
		return false
	}
	return true
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

	// Card data root (pure Go template): built up-front so BOTH the background and
	// the layers resolve their `bind` formulas against it.
	data := templating.DataFromVars(vars)
	in.Background = r.resolveBackgroundBindings(ctx, in.Background, data)

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
	// New-style background: a Figma paint stack (the same model as a layer's
	// fill), composited over the brand-ink base. A non-nil-but-empty stack means
	// "no background" (just the base). Blur, when set, blurs the whole composite.
	// Legacy documents (Fills == nil) keep the exact old type-switch rendering.
	if in.Background.Fills != nil {
		fillRect(fallbackBG)
		bgl := layout.Layer{W: float64(w), H: float64(h), Fills: in.Background.Fills}
		full := func(c *gg.Context) {
			c.DrawRectangle(0, 0, float64(w), float64(h))
		}
		if in.Background.Blur > 0 {
			sub := gg.NewContext(w, h)
			sub.SetColor(fallbackBG) // opaque base so the blur can't pull in transparency at the edges
			sub.DrawRectangle(0, 0, float64(w), float64(h))
			sub.Fill()
			r.fillShape(ctx, sub, bgl, 1, full)
			dc.DrawImage(xdraw.Blur(sub.Image(), in.Background.Blur), 0, 0)
		} else {
			r.fillShape(ctx, dc, bgl, 1, full)
		}
	} else {
		// Branch on the declared background Type (matching the DOM's type-first
		// logic); fall back to field-presence only for legacy documents with no
		// Type set.
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
	}

	// Resolve per-layer property formulas (l.Bind) into concrete field values, so
	// the rest of the renderer transparently draws data-driven geometry / colours.
	in.Layers = r.resolveLayerBindings(ctx, in.Layers, data)

	// paintText lays out + draws a text layer's glyphs onto c in colour col — shared by
	// the real layer (drawRaw) and its boolean/blur silhouette (drawSilhouette, in
	// white), so a text member of a boolean op follows its GLYPHS, not its bounding box
	// (Figma's behaviour). The caller owns rotation (it draws into an already-rotated
	// context, like drawRaw/drawSilhouette do).
	// part selects what's painted: "all" (outline ring + glyphs + decorations),
	// "fill" (glyphs + decorations only) or "stroke" (just the outline ring) — the
	// split lets the paint-stack text renderer build separate fill/stroke stencils.
	paintText := func(c *gg.Context, l layout.Layer, text string, col color.Color, part string) {
		size := l.FontSize
		if size <= 0 {
			size = 32
		}
		if f := r.faceFor(ctx, l.FontFamily, l.FontWeight >= 700, size, fonts); f != nil {
			c.SetFontFace(f)
		}
		c.SetColor(col)
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
		var lines []string
		if tracking == 0 {
			lines = c.WordWrap(s, width)
		} else {
			lines = wrapText(c, s, width, tracking)
		}
		if len(lines) == 0 {
			lines = []string{""}
		}
		_, lh := c.MeasureString("Ag") // font line height (string-independent)
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
		// drawGlyphs paints every line's glyphs at a pixel offset (ox,oy) in the CURRENT
		// colour. Factored so the text STROKE can paint the same glyphs in a ring of
		// offsets behind the fill (gg can't stroke a font face directly).
		drawGlyphs := func(ox, oy float64) {
			for i, line := range lines {
				lineTop := top + float64(i)*adv + oy
				lw := lineWidth(c, line, tracking)
				x0 := l.X + ox
				switch l.Align {
				case "center":
					x0 = l.X + (l.W-lw)/2 + ox
				case "right":
					x0 = l.X + l.W - lw + ox
				}
				// ay=1 anchors each line by its top (baseline = lineTop+lh), exactly as
				// gg's DrawStringWrapped does, so the plain case matches the old output.
				if tracking == 0 {
					c.DrawStringAnchored(line, x0, lineTop, 0, 1)
				} else {
					cx := x0
					for _, ru := range line {
						g := string(ru)
						c.DrawStringAnchored(g, cx, lineTop, 0, 1)
						aw, _ := c.MeasureString(g)
						cx += aw + tracking
					}
				}
			}
		}
		// Stroke (outline): paint the glyphs in a ring of offsets (radius = weight/2) in
		// the stroke colour BEHIND the fill — an outside outline, mirroring the web's
		// `paint-order:stroke`. 16 steps is smooth for typical weights; the stroke shares
		// the fill's alpha. A "stroke" stencil pass paints the ring in col instead.
		if l.StrokeWidth > 0 && part != "fill" {
			ringCol := col
			if part != "stroke" {
				sc, _ := flatTextColor(strokePaintsOf(l), l.StrokeColor)
				_, _, _, fa := col.RGBA()
				ringCol = withAlpha(sc, float64(fa)/0xffff)
			}
			c.SetColor(ringCol)
			rr := l.StrokeWidth / 2
			const steps = 16
			for k := 0; k < steps; k++ {
				a := 2 * math.Pi * float64(k) / float64(steps)
				drawGlyphs(rr*math.Cos(a), rr*math.Sin(a))
			}
			c.SetColor(col)
		}
		if part == "stroke" {
			return
		}
		drawGlyphs(0, 0)
		// Decorations (underline / strike) paint once, over the fill.
		if underline || strike {
			for i, line := range lines {
				lw := lineWidth(c, line, tracking)
				if lw <= 0 {
					continue
				}
				lineTop := top + float64(i)*adv
				x0 := l.X
				switch l.Align {
				case "center":
					x0 = l.X + (l.W-lw)/2
				case "right":
					x0 = l.X + l.W - lw
				}
				baseline := lineTop + lh
				dy := baseline + thick // underline just below the baseline
				if strike {
					dy = baseline - lh*0.28 // strike through the x-height
				}
				c.DrawRectangle(x0, dy, lw, thick)
				c.Fill()
			}
		}
	}

	// drawTextPaints renders a text layer whose fill or stroke carries a REAL
	// paint stack (gradient / image / multiple paints): gg can't paint a font
	// face with a pattern, so the glyphs (and the outline ring) are drawn white
	// into canvas-sized stencils, the paints are filled over the layer's bbox,
	// and each stencil shapes its paint image. Handles its own rotation (the
	// stencils rotate; gradients stay axis-aligned, like every other layer).
	drawTextPaints := func(dc *gg.Context, l layout.Layer, text string, opacity float64) {
		full := func(c *gg.Context) {
			c.DrawRectangle(0, 0, float64(w), float64(h))
		}
		stencilFor := func(part string) image.Image {
			mc := gg.NewContext(w, h)
			if l.Rotation != 0 {
				mc.RotateAbout(l.Rotation*math.Pi/180, l.X+l.W/2, l.Y+l.H/2)
			}
			paintText(mc, l, text, color.White, part)
			return mc.Image()
		}
		paintImage := func(ps []layout.Paint) image.Image {
			pc := gg.NewContext(w, h)
			pl := l
			pl.Fill = ""
			pl.Fills = ps
			r.fillShape(ctx, pc, pl, opacity, full)
			return pc.Image()
		}
		// Outline ring behind the fill, mirroring the flat-colour pass.
		if l.StrokeWidth > 0 {
			if ps := strokePaintsOf(l); len(ps) > 0 {
				dc.DrawImage(applyMask(paintImage(ps), stencilFor("stroke"), "alpha", false), 0, 0)
			}
		}
		if ps := textPaintsOf(l); len(ps) > 0 {
			dc.DrawImage(applyMask(paintImage(ps), stencilFor("fill"), "alpha", false), 0, 0)
		}
	}

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

		// Paint-stack text takes the stencil route (it composites canvas-sized
		// images, so it must run outside the rotated context below). A fully
		// hidden/transparent fill with a visible stroke goes the same way — the
		// fast path's outline ring shares the fill's alpha, which would wrongly
		// erase the stroke.
		if l.Type == "text" {
			fc, fillFlat := flatTextColor(textPaintsOf(l), l.Color)
			strokeFlat := true
			if l.StrokeWidth > 0 {
				_, strokeFlat = flatTextColor(strokePaintsOf(l), l.StrokeColor)
			}
			hiddenFill := false
			if fillFlat && fc != nil {
				if _, _, _, fa := fc.RGBA(); fa == 0 {
					hiddenFill = true
				}
			}
			if !fillFlat || !strokeFlat || (hiddenFill && l.StrokeWidth > 0) {
				drawTextPaints(dc, l, text, opacity)
				return
			}
		}

		rotate := l.Rotation != 0
		if rotate {
			dc.Push()
			dc.RotateAbout(l.Rotation*math.Pi/180, l.X+l.W/2, l.Y+l.H/2)
		}
		switch l.Type {
		case "rect":
			// Progress-bar rect: fill the WIDTH by the member's XP progress percent,
			// left-anchored (x/y/h and the corner radius are kept). An absent or
			// unparseable progress var (welcome cards carry none) leaves it full width.
			// Skip when the width is already a formula (l.Bind["w"]) so the modern
			// bound-width progress bar isn't scaled by the fraction a second time.
			if l.Progress && l.Bind["w"] == "" {
				if frac, ok := progressFraction(vars["{progress}"]); ok {
					l.W = math.Round(l.W * frac) // frac ∈ [0,1] ⇒ width ∈ [0, l.W]
					if l.W < 0 {
						l.W = 0
					}
				}
			}
			tl, tr, br, bl := cornerRadii(l)
			r.fillShape(ctx, dc, l, opacity, func(c *gg.Context) {
				drawRoundRect(c, l.X, l.Y, l.W, l.H, tl, tr, br, bl)
			})
			if l.StrokeWidth > 0 {
				dc.SetLineWidth(l.StrokeWidth)
				applyStroke(dc, l)
				if l.BrushName != "" {
					dc.SetColor(withAlpha(parseHex(strokePrimary(l), color.White), opacity))
					strokeBrush(dc, roundRectPoints(l.X, l.Y, l.W, l.H, tl, tr, br, bl), l, opacity, true)
				} else if restrictedSides(l.StrokeSides) {
					// Per-side strokes (Figma's individual strokes): draw only the enabled
					// edges as lines, each offset inward/outward by the Position. Corner
					// radius is dropped, matching the web overlay.
					off := strokeInset(l.StrokeAlign, l.StrokeWidth)
					r.strokeShape(ctx, dc, l, opacity, func(c *gg.Context) {
						for _, s := range l.StrokeSides {
							switch s {
							case "top":
								c.DrawLine(l.X, l.Y+off, l.X+l.W, l.Y+off)
							case "bottom":
								c.DrawLine(l.X, l.Y+l.H-off, l.X+l.W, l.Y+l.H-off)
							case "left":
								c.DrawLine(l.X+off, l.Y, l.X+off, l.Y+l.H)
							case "right":
								c.DrawLine(l.X+l.W-off, l.Y, l.X+l.W-off, l.Y+l.H)
							}
						}
					})
				} else {
					d := strokeInset(l.StrokeAlign, l.StrokeWidth)
					if d > 0 { // inside: never inset past the centre (negative box)
						d = math.Min(d, math.Min(l.W, l.H)/2)
					}
					sr := strokeRadius(d)
					if l.DynamicWiggle > 0 {
						r.strokeWobbledOutline(ctx, dc, l, roundRectPoints(l.X+d, l.Y+d, l.W-2*d, l.H-2*d, sr(tl), sr(tr), sr(br), sr(bl)), opacity)
					} else {
						r.strokeShape(ctx, dc, l, opacity, func(c *gg.Context) {
							drawRoundRect(c, l.X+d, l.Y+d, l.W-2*d, l.H-2*d, sr(tl), sr(tr), sr(br), sr(bl))
						})
					}
				}
			}

		case "ellipse":
			r.fillShape(ctx, dc, l, opacity, func(c *gg.Context) {
				c.DrawEllipse(l.X+l.W/2, l.Y+l.H/2, l.W/2, l.H/2)
			})
			if l.StrokeWidth > 0 {
				d := strokeInset(l.StrokeAlign, l.StrokeWidth)
				if l.BrushName != "" {
					dc.SetColor(withAlpha(parseHex(strokePrimary(l), color.White), opacity))
					strokeBrush(dc, ellipsePoints(l.X+l.W/2, l.Y+l.H/2, l.W/2, l.H/2), l, opacity, true)
				} else if l.DynamicWiggle > 0 {
					r.strokeWobbledOutline(ctx, dc, l, ellipsePoints(l.X+l.W/2, l.Y+l.H/2, math.Max(0, l.W/2-d), math.Max(0, l.H/2-d)), opacity)
				} else {
					dc.SetLineWidth(l.StrokeWidth)
					applyStroke(dc, l)
					r.strokeShape(ctx, dc, l, opacity, func(c *gg.Context) {
						c.DrawEllipse(l.X+l.W/2, l.Y+l.H/2, math.Max(0, l.W/2-d), math.Max(0, l.H/2-d))
					})
				}
			}

		case "path":
			if len(l.Nodes) >= 2 {
				// Fill uses the smooth bezier outline. Open paths fill too — the region
				// closes with an implicit straight chord, exactly like Figma (and SVG).
				if len(l.Nodes) >= 3 {
					r.fillShape(ctx, dc, l, opacity, func(c *gg.Context) {
						buildPathBezier(c, l)
					})
					dc.ClearPath()
				}
				// Stroke — Figma's advanced stroke (width profile / dynamic wobble /
				// arrowheads) when any is set, else the plain smooth bezier stroke.
				if l.StrokeWidth > 0 {
					r.strokePathLayer(ctx, dc, l, opacity)
				}
			}
		case "text":
			fillCol, _ := flatTextColor(textPaintsOf(l), l.Color)
			paintText(dc, l, text, withAlpha(fillCol, opacity), "all")

		case "image":
			if src == "" {
				break
			}
			img := r.fetchImage(ctx, src)
			if img == nil {
				break
			}
			iw, ih := int(l.W), int(l.H)
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
			// Clip the image to its rounded-rect shape (a circle = fully-rounded corners).
			dc.Push()
			ctl, ctr, cbr, cbl := cornerRadii(l)
			drawRoundRect(dc, l.X, l.Y, l.W, l.H, ctl, ctr, cbr, cbl)
			dc.Clip()
			ox := int(cx) - fitted.Bounds().Dx()/2
			oy := int(cy) - fitted.Bounds().Dy()/2
			dc.DrawImage(fitted, ox, oy)
			dc.ResetClip()
			dc.Pop()
			// Stroke (border) — outline the image's rounded-rect, honouring stroke Position.
			if l.StrokeWidth > 0 {
				dc.SetLineWidth(l.StrokeWidth)
				applyStroke(dc, l)
				d := strokeInset(l.StrokeAlign, l.StrokeWidth)
				if d > 0 {
					d = math.Min(d, math.Min(l.W, l.H)/2)
				}
				stl, str2, sbr, sbl := cornerRadii(l)
				sr := strokeRadius(d)
				if l.BrushName != "" {
					dc.SetColor(withAlpha(parseHex(strokePrimary(l), color.White), opacity))
					strokeBrush(dc, roundRectPoints(l.X, l.Y, l.W, l.H, stl, str2, sbr, sbl), l, opacity, true)
				} else if l.DynamicWiggle > 0 {
					r.strokeWobbledOutline(ctx, dc, l, roundRectPoints(l.X+d, l.Y+d, l.W-2*d, l.H-2*d, sr(stl), sr(str2), sr(sbr), sr(sbl)), opacity)
				} else {
					r.strokeShape(ctx, dc, l, opacity, func(c *gg.Context) {
						drawRoundRect(c, l.X+d, l.Y+d, l.W-2*d, l.H-2*d, sr(stl), sr(str2), sr(sbr), sr(sbl))
					})
				}
			}
		}
		if rotate {
			dc.Pop()
		}
	}

	// drawSilhouette paints a vector layer's filled outline in opaque white onto its
	// own context — used to read a member's coverage for boolean ops (only the alpha
	// channel matters).
	// textGlyphs=true draws a text layer as its glyph outlines (boolean ops, Figma-style);
	// false keeps its box (the background-blur stencil, matching the web's box-clipped
	// CSS backdrop-filter).
	drawSilhouette := func(c *gg.Context, l layout.Layer, textGlyphs bool) {
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
		case "text":
			// Boolean ops follow the GLYPHS (white) — Figma's behaviour — while the
			// background-blur stencil keeps the BOX so it matches the web preview (CSS
			// backdrop-filter is clipped to the layer box, not the letters).
			if textGlyphs {
				paintText(c, l, r.renderText(ctx, l.Text, data), color.White, "all")
			} else {
				stl, str, sbr, sbl := cornerRadii(l)
				drawRoundRect(c, l.X, l.Y, l.W, l.H, stl, str, sbr, sbl)
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
			drawSilhouette(stencil, l, false) // box (matches the web's box-clipped backdrop-filter)
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
			drawSilhouette(sub, m, true) // boolean coverage follows a text member's glyphs
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
		// A text source has no Fill — use its text Colour so a text-topped boolean keeps
		// the text's colour (Figma takes the front-most member's appearance).
		fillStr := paintPrimary(source)
		if fillStr == "" && source.Type == "text" {
			fillStr = source.Color
		}
		fill := parseHex(fillStr, color.White)
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

// flatTextColor reduces a text paint stack to one flat colour when it is at
// most a single visible solid paint (the fast glyph path): a nil stack keeps
// the legacy fallback hex (historically white), an all-hidden stack paints
// nothing. ok=false means the stack needs the stencil-based paint renderer
// (gradient / image / multiple visible paints).
func flatTextColor(raw []layout.Paint, fallback string) (color.Color, bool) {
	if len(raw) == 0 {
		return parseHex(fallback, color.White), true
	}
	var vis []layout.Paint
	for _, p := range raw {
		if !p.Hidden && paintOpacity(p) > 0 {
			vis = append(vis, p)
		}
	}
	if len(vis) == 0 {
		return color.Transparent, true
	}
	if len(vis) == 1 && vis[0].Type == "solid" {
		return withAlpha(parseHex(vis[0].Color, color.White), paintOpacity(vis[0])), true
	}
	return nil, false
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

// strokeInset returns how far to inset (>0) or outset (<0) a stroke path so gg's
// CENTERED Stroke lands Inside / Center / Outside the shape edge — Figma's stroke
// Position. The shape box/radii shrink by this amount (a negative value grows them).
func strokeInset(align string, sw float64) float64 {
	switch align {
	case "inside":
		return sw / 2
	case "outside":
		return -sw / 2
	default: // center / unset
		return 0
	}
}

// strokeRadius adjusts a corner radius for a stroke inset/outset d: a positive d (inside)
// shrinks it; a negative d (outside) grows a ROUNDED corner but keeps a SHARP (0) corner
// sharp — matching Figma and the web preview (a square corner stays square under an
// outside stroke). Mirrors the ring's grow-only-when-positive pattern.
func strokeRadius(d float64) func(float64) float64 {
	return func(c float64) float64 {
		if d < 0 && c <= 0 {
			return 0
		}
		return math.Max(0, c-d)
	}
}

// restrictedSides reports whether a rect strokes only SOME of its sides (Figma's
// individual strokes): a non-empty list with fewer than all four = a partial outline.
func restrictedSides(sides []string) bool {
	return len(sides) > 0 && len(sides) < 4
}

// applyStroke sets the dash pattern, line cap, and line join on dc before Stroke(),
// from a layer's stroke style. gg has no miter join, so 'miter' approximates as bevel
// (the closest non-round join). Always sets all three so state never leaks between
// layers; SetDash() with no args clears the dashes (solid).
func applyStroke(dc *gg.Context, l layout.Layer) {
	switch l.StrokeCap {
	case "butt":
		dc.SetLineCapButt()
	case "square":
		dc.SetLineCapSquare()
	default: // round
		dc.SetLineCapRound()
	}
	switch l.StrokeJoin {
	case "bevel", "miter": // gg has no miter; bevel is the closest sharp join
		dc.SetLineJoinBevel()
	default: // round
		dc.SetLineJoinRound()
	}
	if l.StrokeStyle == "dashed" {
		d, g := math.Max(0, l.Dash), math.Max(0, l.Gap)
		if d > 0 || g > 0 {
			dc.SetDash(d, g)
			return
		}
	}
	dc.SetDash() // solid
}

// pathPt is a sampled point on a flattened path (canvas px).
type pathPt struct{ x, y float64 }

// buildPathBezier adds a path layer's smooth cubic outline to dc's current path (the
// caller Fills/Strokes/Clears it). This is the exact path the renderer drew before the
// advanced-stroke features, so plain strokes stay byte-identical.
func buildPathBezier(dc *gg.Context, l layout.Layer) {
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
}

// arrowKind normalises an arrowhead value, collapsing "none"/unset to "".
func arrowKind(s string) string {
	if s == "" || s == "none" {
		return ""
	}
	return s
}

// pathHasAdvancedStroke reports whether a path needs the sampled-polyline renderer (a
// non-uniform width profile, a hand-drawn wobble, or an arrowhead). Plain strokes keep
// the smooth bezier path so existing cards render unchanged. Brush + miter angle do not
// trigger it: brushes render as a plain stroke (texture brushes aren't rasterised) and
// gg has no miter join.
func pathHasAdvancedStroke(l layout.Layer) bool {
	if l.DynamicWiggle > 0 {
		return true
	}
	if l.WidthProfile != "" && l.WidthProfile != "uniform" {
		return true
	}
	return arrowKind(l.StartCap) != "" || arrowKind(l.EndCap) != ""
}

// cubicAt evaluates a cubic bezier at t.
func cubicAt(p0, p1, p2, p3 pathPt, t float64) pathPt {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return pathPt{a*p0.x + b*p1.x + c*p2.x + d*p3.x, a*p0.y + b*p1.y + c*p2.y + d*p3.y}
}

// samplePathPoints flattens a path layer into a polyline (steps points per segment).
func samplePathPoints(l layout.Layer, steps int) []pathPt {
	n := l.Nodes
	out := []pathPt{{n[0].X, n[0].Y}}
	seg := func(a, c layout.PathNode) {
		p0, p1, p2, p3 := pathPt{a.X, a.Y}, pathPt{a.H2X, a.H2Y}, pathPt{c.H1X, c.H1Y}, pathPt{c.X, c.Y}
		for i := 1; i <= steps; i++ {
			out = append(out, cubicAt(p0, p1, p2, p3, float64(i)/float64(steps)))
		}
	}
	for k := 1; k < len(n); k++ {
		seg(n[k-1], n[k])
	}
	if l.Closed && len(n) >= 3 {
		seg(n[len(n)-1], n[0])
	}
	return out
}

// wobblePath displaces each sampled point along its normal by smooth deterministic
// value noise — Figma's "Dynamic" hand-drawn look (mirrored in brushes.ts
// dynamicWobble). Like Figma's: Frequency maps to the number of noise cycles along
// the path (their API range is 0.01..20), Wiggle is the bump amplitude (UI % that
// may exceed 100), and Smoothen blends out a high-frequency octave so bumps go
// from jagged to rounded. The bumps are organic (irregular sizes and spacing), not
// a regular sine. Open paths keep their endpoints anchored over a short ramp so
// caps/arrowheads still line up; closed outlines use periodic noise — no seam.
func wobblePath(pts []pathPt, l layout.Layer, closed bool) []pathPt {
	wiggle := l.DynamicWiggle
	n := len(pts)
	if wiggle <= 0 || n < 3 {
		return pts
	}
	A := (wiggle / 100) * 38 // px at 100% (calibrated against Figma's help screenshot)
	cycles := math.Max(0.01, (l.DynamicFrequency/100)*20)
	if closed {
		cycles = math.Max(1, math.Round(cycles)) // integer cycles so the loop closes seamlessly
	}
	smooth := math.Min(1, math.Max(0, l.DynamicSmoothen/100))
	rough := 1 - smooth
	s := make([]float64, n)
	plen := 0.0
	for i := 1; i < n; i++ {
		plen += math.Hypot(pts[i].x-pts[i-1].x, pts[i].y-pts[i-1].y)
		s[i] = plen
	}
	if closed {
		plen += math.Hypot(pts[0].x-pts[n-1].x, pts[0].y-pts[n-1].y)
	}
	if plen <= 0 {
		return pts
	}
	// keep lobes rounded (no cusps/self-intersection) on small shapes
	A = math.Min(A, (plen/cycles)*0.55)
	ramp := math.Min(plen*0.1, plen/cycles*0.5)
	out := make([]pathPt, n)
	for i, p := range pts {
		pa := pts[(i-1+n)%n]
		pb := pts[(i+1)%n]
		tx, ty := pb.x-pa.x, pb.y-pa.y
		tl := math.Hypot(tx, ty)
		if tl == 0 {
			out[i] = p
			continue
		}
		nx, ny := -ty/tl, tx/tl
		u := s[i] / plen * cycles
		// a bump per cycle (Figma: "frequency = the number of bumps") with noise-
		// jittered phase and per-region magnitude, so every bump is pronounced but
		// their sizes and spacing stay organic — not a uniform sine, not flat noise
		ph := 0.65 * (pnoise(57, u, cycles, closed) - 0.5)
		m := 0.3 + 0.75*pnoise(58, u*1.3, cycles*1.3, closed)
		w := math.Sin(2*math.Pi*(u+ph)) * m
		if rough > 0 {
			w += rough * 0.35 * (pnoise(59, u*3.3, cycles*3.3, closed)*2 - 1)
		}
		if w > 1 {
			w = 1
		} else if w < -1 {
			w = -1
		}
		env := 1.0
		if !closed && ramp > 0 {
			env = math.Min(1, math.Min(s[i]/ramp, (plen-s[i])/ramp))
		}
		off := A * w * env
		out[i] = pathPt{p.x + nx*off, p.y + ny*off}
	}
	return out
}

// strokeSmoothPath adds a SMOOTH cubic curve through pts (Catmull-Rom -> bezier) to dc's
// current path, so a wobbled path/outline reads as rounded bumps, not straight segments.
func strokeSmoothPath(dc *gg.Context, pts []pathPt, closed bool) {
	n := len(pts)
	if n < 2 {
		return
	}
	at := func(i int) pathPt {
		if closed {
			return pts[((i%n)+n)%n]
		}
		if i < 0 {
			i = 0
		}
		if i > n-1 {
			i = n - 1
		}
		return pts[i]
	}
	dc.MoveTo(pts[0].x, pts[0].y)
	segs := n - 1
	if closed {
		segs = n
	}
	for i := 0; i < segs; i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		c1x, c1y := p1.x+(p2.x-p0.x)/6, p1.y+(p2.y-p0.y)/6
		c2x, c2y := p2.x-(p3.x-p1.x)/6, p2.y-(p3.y-p1.y)/6
		dc.CubicTo(c1x, c1y, c2x, c2y, p2.x, p2.y)
	}
	if closed {
		dc.ClosePath()
	}
}

// widthFactor is the stroke-weight multiplier at s in [0,1] for a width profile.
func widthFactor(profile string, s float64) float64 {
	switch profile {
	case "taper_start":
		return 0.06 + 0.94*s
	case "taper_end":
		return 0.06 + 0.94*(1-s)
	case "taper":
		return 0.06 + 0.94*math.Sin(math.Pi*s)
	case "lens":
		return 0.10 + 0.90*math.Pow(math.Sin(math.Pi*s), 0.5)
	default:
		return 1
	}
}

// strokeVariableWidth paints a polyline as a variable-width ribbon by stroking each short
// segment at its interpolated weight (round caps/joins blend the segments). gg has no
// native variable-width stroke, so this is the approximation.
func strokeVariableWidth(dc *gg.Context, pts []pathPt, base float64, profile string) {
	dc.SetLineCapRound()
	dc.SetLineJoinRound()
	dc.SetDash()
	last := len(pts) - 1
	for i := 0; i < last; i++ {
		s := (float64(i) + 0.5) / float64(last)
		w := base * widthFactor(profile, s)
		if w < 0.1 {
			w = 0.1
		}
		dc.SetLineWidth(w)
		dc.MoveTo(pts[i].x, pts[i].y)
		dc.LineTo(pts[i+1].x, pts[i+1].y)
		dc.Stroke()
		dc.ClearPath()
	}
}

// unit returns the unit vector of (x,y), or (1,0) for a zero-length input.
func unit(x, y float64) pathPt {
	l := math.Hypot(x, y)
	if l == 0 {
		return pathPt{1, 0}
	}
	return pathPt{x / l, y / l}
}

// drawArrowCap paints an arrowhead/decoration at a path endpoint. `tip` is the endpoint
// and `outDir` the OUTWARD unit direction (pointing away from the path body).
func drawArrowCap(dc *gg.Context, tip, outDir pathPt, kind string, sw float64) {
	if kind == "" {
		return
	}
	s := sw*2.2 + 3                              // marker scale, px
	px, py := -outDir.y, outDir.x                // perpendicular to the path
	ax, ay := tip.x+outDir.x*s, tip.y+outDir.y*s // apex, extended OUTWARD past the endpoint
	switch kind {
	case "line":
		// a bar across the endpoint
		dc.SetLineWidth(sw)
		dc.SetLineCapRound()
		dc.SetDash()
		dc.MoveTo(tip.x+px*s*0.7, tip.y+py*s*0.7)
		dc.LineTo(tip.x-px*s*0.7, tip.y-py*s*0.7)
		dc.Stroke()
		dc.ClearPath()
	case "arrow":
		// open chevron: base corners at the endpoint, point extended outward
		dc.SetLineWidth(sw)
		dc.SetLineCapRound()
		dc.SetLineJoinRound()
		dc.SetDash()
		dc.MoveTo(tip.x+px*s*0.7, tip.y+py*s*0.7)
		dc.LineTo(ax, ay)
		dc.LineTo(tip.x-px*s*0.7, tip.y-py*s*0.7)
		dc.Stroke()
		dc.ClearPath()
	case "triangle":
		// filled head: base at the endpoint, apex extended outward
		dc.MoveTo(ax, ay)
		dc.LineTo(tip.x+px*s*0.6, tip.y+py*s*0.6)
		dc.LineTo(tip.x-px*s*0.6, tip.y-py*s*0.6)
		dc.ClosePath()
		dc.Fill()
		dc.ClearPath()
	case "circle":
		// dot centred on the endpoint
		dc.DrawCircle(tip.x, tip.y, s*0.5)
		dc.Fill()
		dc.ClearPath()
	case "diamond":
		// rhombus centred on the endpoint
		h := s * 0.6
		dc.MoveTo(tip.x+outDir.x*h, tip.y+outDir.y*h)
		dc.LineTo(tip.x+px*h, tip.y+py*h)
		dc.LineTo(tip.x-outDir.x*h, tip.y-outDir.y*h)
		dc.LineTo(tip.x-px*h, tip.y-py*h)
		dc.ClosePath()
		dc.Fill()
		dc.ClearPath()
	}
}

// strokePathLayer renders a path's stroke, once per visible stroke paint (the
// Figma stroke stack — solid colours, gradients or images). Plain strokes use
// the smooth bezier path (identical to the pre-feature output); width profile /
// dynamic wobble / arrowheads use a sampled polyline. Brushes tint their stamps
// with the stack's primary colour; miter angle is unused.
func (r *Renderer) strokePathLayer(ctx context.Context, dc *gg.Context, l layout.Layer, opacity float64) {
	if l.BrushName != "" {
		dc.SetColor(withAlpha(parseHex(strokePrimary(l), color.White), opacity))
		strokeBrush(dc, samplePathPoints(l, 24), l, opacity, l.Closed)
		return
	}
	if !pathHasAdvancedStroke(l) {
		applyStroke(dc, l)
		dc.SetLineWidth(l.StrokeWidth)
		r.strokeShape(ctx, dc, l, opacity, func(c *gg.Context) {
			buildPathBezier(c, l)
		})
		dc.ClearPath()
		return
	}
	pts := samplePathPoints(l, 24)
	if len(pts) < 2 {
		return
	}
	pts = wobblePath(pts, l, l.Closed)
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
		if profile := l.WidthProfile; profile != "" && profile != "uniform" {
			strokeVariableWidth(dc, pts, l.StrokeWidth, profile)
		} else {
			applyStroke(dc, l)
			dc.SetLineWidth(l.StrokeWidth)
			strokeSmoothPath(dc, pts, l.Closed)
			dc.Stroke()
			dc.ClearPath()
		}
		// Arrowheads fill/stroke with this paint's style (setPaintStyle sets both).
		if !l.Closed && len(pts) >= 2 {
			s0, s1 := pts[0], pts[1]
			e0, e1 := pts[len(pts)-1], pts[len(pts)-2]
			drawArrowCap(dc, s0, unit(s0.x-s1.x, s0.y-s1.y), arrowKind(l.StartCap), l.StrokeWidth)
			drawArrowCap(dc, e0, unit(e0.x-e1.x, e0.y-e1.y), arrowKind(l.EndCap), l.StrokeWidth)
		}
	}
}

// roundRectPoints samples a rounded-rect outline into a closed polyline (the input to the
// dynamic wobble on a box shape's border). Radii are clamped; a zero-radius corner is sharp.
func roundRectPoints(x, y, w, h, tl, tr, br, bl float64) []pathPt {
	mx := math.Min(w, h) / 2
	cl := func(v float64) float64 { return math.Max(0, math.Min(v, mx)) }
	tl, tr, br, bl = cl(tl), cl(tr), cl(br), cl(bl)
	const sp = 7.0
	var pts []pathPt
	line := func(x0, y0, x1, y1 float64) {
		n := int(math.Hypot(x1-x0, y1-y0)/sp) + 1
		for i := 0; i < n; i++ {
			t := float64(i) / float64(n)
			pts = append(pts, pathPt{x0 + (x1-x0)*t, y0 + (y1-y0)*t})
		}
	}
	arc := func(cx, cy, r, a0, a1 float64) {
		if r <= 0 {
			return
		}
		n := int(r*math.Abs(a1-a0)/sp) + 2
		for i := 0; i < n; i++ {
			a := a0 + (a1-a0)*float64(i)/float64(n)
			pts = append(pts, pathPt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
	}
	line(x+tl, y, x+w-tr, y)
	arc(x+w-tr, y+tr, tr, -math.Pi/2, 0)
	line(x+w, y+tr, x+w, y+h-br)
	arc(x+w-br, y+h-br, br, 0, math.Pi/2)
	line(x+w-br, y+h, x+bl, y+h)
	arc(x+bl, y+h-bl, bl, math.Pi/2, math.Pi)
	line(x, y+h-bl, x, y+tl)
	arc(x+tl, y+tl, tl, math.Pi, math.Pi*1.5)
	return pts
}

// ellipsePoints samples an ellipse outline into a closed polyline.
func ellipsePoints(cx, cy, rx, ry float64) []pathPt {
	n := int(math.Max(rx, ry)*2*math.Pi/7) + 8
	pts := make([]pathPt, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = pathPt{cx + rx*math.Cos(a), cy + ry*math.Sin(a)}
	}
	return pts
}

// strokeWobbledOutline applies the dynamic wobble to a CLOSED shape outline (a rect/ellipse/
// image border) and strokes it — the hand-drawn look on a box shape, painted once per
// visible stroke paint. The caller passes the outline already inset for the stroke
// Position; cap/join/dash come from the layer.
func (r *Renderer) strokeWobbledOutline(ctx context.Context, dc *gg.Context, l layout.Layer, pts []pathPt, opacity float64) {
	if len(pts) < 3 {
		return
	}
	pts = wobblePath(pts, l, true)
	dc.SetLineWidth(l.StrokeWidth)
	applyStroke(dc, l)
	r.strokeShape(ctx, dc, l, opacity, func(c *gg.Context) {
		strokeSmoothPath(c, pts, true)
	})
	dc.ClearPath()
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
