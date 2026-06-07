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
	Align      string  `json:"align,omitempty"`

	// image / avatar
	Src       string  `json:"src,omitempty"`
	Fit       string  `json:"fit,omitempty"`
	Shape     string  `json:"shape,omitempty"`
	Mask      string  `json:"mask,omitempty"` // image clip: "circle" | "ellipse" (else rounded-rect via radius)
	RingColor string  `json:"ring_color,omitempty"`
	RingWidth float64 `json:"ring_width,omitempty"`

	// rect / ellipse / common
	Fill        string  `json:"fill,omitempty"`
	Radius      float64 `json:"radius,omitempty"`
	StrokeColor string  `json:"stroke_color,omitempty"`
	StrokeWidth float64 `json:"stroke_width,omitempty"`

	// path (pen / pencil)
	Nodes  []PathNode `json:"nodes,omitempty"`
	Closed bool       `json:"closed,omitempty"`

	// masking ("use as mask"): Clip marks this layer as a stencil that clips the
	// layers ABOVE it (until the next mask). ClipMode = "alpha" | "luminance".
	Clip       bool   `json:"clip,omitempty"`
	ClipMode   string `json:"clip_mode,omitempty"`
	ClipInvert bool   `json:"clip_invert,omitempty"` // hide inside the shape / show outside
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
