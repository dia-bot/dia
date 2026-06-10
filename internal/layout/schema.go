// Package layout defines the declarative card-layout schema shared by the
// dashboard editor and the Go renderer (internal/imaging renders it to PNG).
// Keep this in sync with web/src/lib/layout/schema.ts (same JSON shape).
package layout

import "math"

// Layer is a single element on the canvas. It's a "fat" struct: only the fields
// relevant to Type are used. Geometry is in canvas pixels.
type Layer struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"` // text | image | avatar | rect | ellipse | path
	Name     string   `json:"name"`
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	W        float64  `json:"w"`
	H        float64  `json:"h"`
	Opacity  *float64 `json:"opacity"`            // pointer so an explicit 0 (fully transparent) is distinct from unset (=1)
	Rotation float64  `json:"rotation,omitempty"` // degrees, about the layer centre
	Hidden   bool     `json:"hidden"`
	Group    string   `json:"group,omitempty"`  // soft-group id; scopes a mask group (read by the mask loop). Members must be contiguous.
	Locked   bool     `json:"locked,omitempty"` // editor-only; ignored when rendering

	// text
	Text       string  `json:"text,omitempty"`
	FontSize   float64 `json:"font_size,omitempty"`
	FontWeight int     `json:"font_weight,omitempty"`
	FontFamily string  `json:"font_family,omitempty"` // picker family name; "" = default
	Color      string  `json:"color,omitempty"`
	Align      string  `json:"align,omitempty"` // horizontal alignment
	// typography (Figma's Type panel). Kept in sync with web/src/lib/layout/schema.ts.
	LineHeight     float64 `json:"line_height,omitempty"`     // multiplier (default 1.3)
	LetterSpacing  float64 `json:"letter_spacing,omitempty"`  // tracking in canvas px
	VAlign         string  `json:"valign,omitempty"`          // top|middle|bottom (default top)
	TextCase       string  `json:"text_case,omitempty"`       // none|upper|lower|title
	TextDecoration string  `json:"text_decoration,omitempty"` // none|underline|strike

	// image
	Src string `json:"src,omitempty"`
	Fit string `json:"fit,omitempty"`

	// rect / ellipse / common
	Fill        string    `json:"fill,omitempty"`
	Radius      float64   `json:"radius,omitempty"`
	Corners     []float64 `json:"corners,omitempty"` // independent corner radii [tl,tr,br,bl]; overrides Radius when len==4
	StrokeColor string    `json:"stroke_color,omitempty"`
	StrokeWidth float64   `json:"stroke_width,omitempty"`
	StrokeAlign string    `json:"stroke_align,omitempty"` // inside|center|outside (Figma stroke Position); default center
	StrokeStyle string    `json:"stroke_style,omitempty"` // solid|dashed (default solid)
	Dash        float64   `json:"dash,omitempty"`         // dash length, px (dashed)
	Gap         float64   `json:"gap,omitempty"`          // gap length, px (dashed)
	StrokeCap   string    `json:"stroke_cap,omitempty"`   // butt|round|square (default round)
	StrokeJoin  string    `json:"stroke_join,omitempty"`  // miter|bevel|round (default round)
	StrokeSides []string  `json:"stroke_sides,omitempty"` // rect per-side strokes; empty OR all 4 = full outline

	// advanced stroke (Figma's Stroke-settings popover; mostly path-only). Kept in sync
	// with web/src/lib/layout/schema.ts.
	WidthProfile     string  `json:"width_profile,omitempty"`     // uniform|taper_start|taper_end|taper|lens (default uniform)
	StartCap         string  `json:"start_cap,omitempty"`         // none|line|arrow|triangle|circle|diamond — arrowhead at first node
	EndCap           string  `json:"end_cap,omitempty"`           // none|line|arrow|triangle|circle|diamond — arrowhead at last node
	MiterAngle       float64 `json:"miter_angle,omitempty"`       // miter join cutoff, degrees (default ~28.96)
	BrushName        string  `json:"brush_name,omitempty"`        // brush id from the catalog (brushes.go); center-only
	BrushDirection   string  `json:"brush_direction,omitempty"`   // forward|backward — stretch nib direction
	ScatterGap       float64 `json:"scatter_gap,omitempty"`       // scatter: stamp spacing × stroke weight (unset = brush preset)
	ScatterWiggle    float64 `json:"scatter_wiggle,omitempty"`    // scatter: perpendicular position jitter % (0..100)
	ScatterSize      float64 `json:"scatter_size,omitempty"`      // scatter: mark size jitter % (0..100)
	DynamicFrequency float64 `json:"dynamic_frequency,omitempty"` // hand-drawn wobble density 0..100 (0 = off)
	DynamicWiggle    float64 `json:"dynamic_wiggle,omitempty"`    // wobble amplitude % 0..200
	DynamicSmoothen  float64 `json:"dynamic_smoothen,omitempty"`  // wobble smoothing 0..100

	// path (pen / pencil)
	Nodes  []PathNode `json:"nodes,omitempty"`
	Closed bool       `json:"closed,omitempty"`

	// masking ("use as mask"): Clip marks this layer as a stencil that clips the
	// layers ABOVE it (until the next mask). ClipMode = "alpha" | "luminance".
	Clip       bool   `json:"clip,omitempty"`
	ClipMode   string `json:"clip_mode,omitempty"`
	ClipInvert bool   `json:"clip_invert,omitempty"` // hide inside the shape / show outside

	// effects (shadows / blur), applied per layer in a fixed order. Kept in sync
	// with web/src/lib/layout/schema.ts.
	Effects []Effect `json:"effects,omitempty"`
}

// Effect is a single layer effect (shadow or blur). Only the fields relevant to
// Type are used. Geometry is in canvas px. Mirrors web/src/lib/layout/schema.ts.
//
//	"drop_shadow"     — blurred, offset, tinted copy of the layer behind it
//	"inner_shadow"    — the same painted inside the silhouette (edge shading)
//	"layer_blur"      — gaussian-blur the layer's own pixels
//	"background_blur" — gaussian-blur whatever sits behind a translucent layer
type Effect struct {
	Type    string   `json:"type"`
	X       float64  `json:"x,omitempty"`       // shadow offset
	Y       float64  `json:"y,omitempty"`       //
	Radius  float64  `json:"radius,omitempty"`  // blur radius (shadow softness or blur strength)
	Spread  float64  `json:"spread,omitempty"`  // shadow grow(+)/shrink(−)
	Color   string   `json:"color,omitempty"`   // shadow colour
	Opacity *float64 `json:"opacity,omitempty"` // shadow alpha 0..1 (pointer: 0 ≠ unset; unset ⇒ 0.25)
	Hidden  bool     `json:"hidden,omitempty"`  // skip without removing
}

// PathNode is a bezier anchor with its two cubic control handles (absolute
// canvas px). A corner node's handles equal the anchor.
type PathNode struct {
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	H1X float64 `json:"h1x"`
	H1Y float64 `json:"h1y"`
	H2X float64 `json:"h2x"`
	H2Y float64 `json:"h2y"`
	// M is the editor's handle-relationship hint ("corner"|"mirror"|"asym").
	// The renderer ignores it — it only reads the handle coords — but it round-
	// trips so the editor can restore a saved point's type. Kept in sync with
	// web/src/lib/layout/schema.ts.
	M string `json:"m,omitempty"`
}

// Background describes the canvas backdrop.
type Background struct {
	Type     string  `json:"type"` // solid | gradient | image
	Color    string  `json:"color,omitempty"`
	From     string  `json:"from,omitempty"`
	To       string  `json:"to,omitempty"`
	Angle    float64 `json:"angle,omitempty"`
	ImageURL string  `json:"image_url,omitempty"`
	Blur     float64 `json:"blur,omitempty"`
}

// LayoutGroup is metadata for a soft group, keyed by Layer.Group id. Name
// round-trips for display. BoolOp, when set ("union"|"subtract"|"intersect"|
// "exclude"), makes the group a boolean group: the renderer composites the run's
// member shapes with that operation. Kept in sync with web/src/lib/layout/schema.ts.
type LayoutGroup struct {
	Name   string `json:"name,omitempty"`
	BoolOp string `json:"bool_op,omitempty"` // union|subtract|intersect|exclude
}

// Layout is a canvas plus an ordered list of layers (first = back-most).
type Layout struct {
	Width      int                    `json:"width"`
	Height     int                    `json:"height"`
	Background Background             `json:"background"`
	Layers     []Layer                `json:"layers"`
	Groups     map[string]LayoutGroup `json:"groups,omitempty"` // editor-only; keyed by Layer.Group id
}

// Canvas size limits — keep server-side allocation bounded. Mirrors the web
// schema.ts constants so the editor and renderer agree on what's allowed.
const (
	MinCanvas      = 64
	MaxCanvasDim   = 4096
	MaxCanvasPixel = 4_000_000
)

// ClampSize constrains a width/height to the canvas limits, scaling both down
// proportionally if the pixel budget is exceeded (keeping the aspect ratio).
// A non-positive dimension falls back to the welcome-card default.
func ClampSize(w, h int) (int, int) {
	if w <= 0 {
		w = 1024
	}
	if h <= 0 {
		h = 450
	}
	clamp := func(v int) int {
		if v < MinCanvas {
			return MinCanvas
		}
		if v > MaxCanvasDim {
			return MaxCanvasDim
		}
		return v
	}
	w, h = clamp(w), clamp(h)
	if px := w * h; px > MaxCanvasPixel {
		s := math.Sqrt(float64(MaxCanvasPixel) / float64(px))
		w = clamp(int(float64(w) * s))
		h = clamp(int(float64(h) * s))
	}
	return w, h
}
