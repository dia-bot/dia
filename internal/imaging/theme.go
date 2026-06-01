package imaging

import (
	"context"
	"image"
	"image/color"
	"math"
	"net/http"
	"strconv"
	"strings"

	xdraw "github.com/disintegration/imaging"
	"github.com/fogleman/gg"
)

// Dia brand palette (from the logo gradient on a blush base).
const (
	BrandPink   = "#FF6363"
	BrandPurple = "#B244FC"
	BrandBlush  = "#F1DFDF"
	BrandInk    = "#2B2233"
)

// Background describes how to fill a card behind everything else.
type Background struct {
	Color    string  `json:"color,omitempty"`     // solid fill (hex)
	From     string  `json:"from,omitempty"`      // gradient start (hex)
	To       string  `json:"to,omitempty"`        // gradient end (hex)
	Angle    float64 `json:"angle,omitempty"`     // gradient angle (deg)
	ImageURL string  `json:"image_url,omitempty"` // background image (fitted)
	Blur     bool    `json:"blur,omitempty"`      // blur the background image
}

// parseHex parses #RGB, #RRGGBB or #RRGGBBAA into a color, using fallback on error.
func parseHex(s string, fallback color.Color) color.Color {
	s = strings.TrimSpace(strings.TrimPrefix(s, "#"))
	if s == "" {
		return fallback
	}
	switch len(s) {
	case 3:
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]}) + "ff"
	case 6:
		s += "ff"
	case 8:
		// ok
	default:
		return fallback
	}
	v, err := strconv.ParseUint(s, 16, 64)
	if err != nil {
		return fallback
	}
	return color.NRGBA{
		R: uint8(v >> 24),
		G: uint8(v >> 16),
		B: uint8(v >> 8),
		A: uint8(v),
	}
}

// drawBackground paints the card background (image > gradient > solid).
func (r *Renderer) drawBackground(ctx context.Context, dc *gg.Context, w, h int, bg Background, fallback color.Color) {
	if bg.ImageURL != "" {
		if img := r.fetchImage(ctx, bg.ImageURL); img != nil {
			fitted := xdraw.Fill(img, w, h, xdraw.Center, xdraw.Lanczos)
			if bg.Blur {
				fitted = xdraw.Blur(fitted, 8)
			}
			dc.DrawImage(fitted, 0, 0)
			return
		}
	}
	if bg.From != "" && bg.To != "" {
		x0, y0, x1, y1 := gradientLine(float64(w), float64(h), bg.Angle)
		grad := gg.NewLinearGradient(x0, y0, x1, y1)
		grad.AddColorStop(0, parseHex(bg.From, fallback))
		grad.AddColorStop(1, parseHex(bg.To, fallback))
		dc.SetFillStyle(grad)
		dc.DrawRectangle(0, 0, float64(w), float64(h))
		dc.Fill()
		return
	}
	dc.SetColor(parseHex(bg.Color, fallback))
	dc.DrawRectangle(0, 0, float64(w), float64(h))
	dc.Fill()
}

// gradientLine returns the start/end points of a gradient at angle (degrees)
// spanning the canvas.
func gradientLine(w, h, angle float64) (x0, y0, x1, y1 float64) {
	if angle == 0 {
		angle = 45 // pleasant default diagonal
	}
	rad := angle * math.Pi / 180
	cx, cy := w/2, h/2
	dx, dy := math.Cos(rad), math.Sin(rad)
	half := math.Max(w, h)
	return cx - dx*half, cy - dy*half, cx + dx*half, cy + dy*half
}

// drawAvatar draws a circular avatar with an optional ring at center (cx,cy).
func (r *Renderer) drawAvatar(dc *gg.Context, img image.Image, cx, cy, radius float64, ring color.Color, ringWidth float64) {
	if ringWidth > 0 {
		dc.SetColor(ring)
		dc.DrawCircle(cx, cy, radius+ringWidth)
		dc.Fill()
	}
	dc.Push()
	dc.DrawCircle(cx, cy, radius)
	dc.Clip()
	dc.DrawImageAnchored(img, int(cx), int(cy), 0.5, 0.5)
	dc.ResetClip()
	dc.Pop()
}

// drawProgressBar draws a rounded progress bar (used by rank cards).
func drawProgressBar(dc *gg.Context, x, y, w, h, pct float64, bg, fg color.Color) {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	rad := h / 2
	dc.SetColor(bg)
	dc.DrawRoundedRectangle(x, y, w, h, rad)
	dc.Fill()
	if pct > 0 {
		fw := w * pct
		if fw < h { // keep the rounded cap visible at tiny percentages
			fw = h
		}
		dc.SetColor(fg)
		dc.DrawRoundedRectangle(x, y, fw, h, rad)
		dc.Fill()
	}
}

// fetchImage downloads and decodes an image, returning nil on any failure.
func (r *Renderer) fetchImage(ctx context.Context, url string) image.Image {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return nil
	}
	return img
}
