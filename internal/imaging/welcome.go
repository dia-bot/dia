package imaging

import (
	"context"
	"image/color"

	"github.com/fogleman/gg"
)

// WelcomeInput is a fully-resolved welcome card (text already substituted by
// the caller — the renderer is purely visual).
type WelcomeInput struct {
	Width, Height int

	Background   Background
	AccentColor  string // ring + flourishes
	TextColor    string // title
	SubTextColor string // subtitle/footer

	AvatarURL string
	Title     string // e.g. "Welcome, Ada!"
	Subtitle  string // e.g. "You're our 1,024th member"
	Footer    string // optional small line
}

// RenderWelcome produces a PNG welcome card.
func (r *Renderer) RenderWelcome(ctx context.Context, in WelcomeInput) ([]byte, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	defer r.release()

	w, h := in.Width, in.Height
	if w <= 0 {
		w = 1024
	}
	if h <= 0 {
		h = 450
	}

	accent := parseHex(in.AccentColor, parseHex(BrandPurple, color.White))
	textCol := parseHex(in.TextColor, color.White)
	subCol := parseHex(in.SubTextColor, color.NRGBA{R: 235, G: 225, B: 240, A: 255})

	dc := gg.NewContext(w, h)
	fallbackBG := parseHex(BrandInk, color.Black)
	r.drawBackground(ctx, dc, w, h, in.Background, fallbackBG)

	// Legibility scrim when a background image is used.
	if in.Background.ImageURL != "" {
		dc.SetColor(color.NRGBA{A: 110})
		dc.DrawRectangle(0, 0, float64(w), float64(h))
		dc.Fill()
	}

	// Avatar, centered near the top.
	radius := float64(h) * 0.22
	cx := float64(w) / 2
	cy := radius + float64(h)*0.06
	avatar := r.fetchAvatar(ctx, in.AvatarURL, int(radius*2), accent)
	r.drawAvatar(dc, avatar, cx, cy, radius, accent, 6)

	// Title.
	titleY := cy + radius + float64(h)*0.10
	r.setFont(dc, true, 52)
	dc.SetColor(textCol)
	dc.DrawStringWrapped(in.Title, cx, titleY, 0.5, 0.5, float64(w)-100, 1.2, gg.AlignCenter)

	// Subtitle.
	if in.Subtitle != "" {
		r.setFont(dc, false, 28)
		dc.SetColor(subCol)
		dc.DrawStringWrapped(in.Subtitle, cx, titleY+58, 0.5, 0.5, float64(w)-120, 1.3, gg.AlignCenter)
	}

	// Footer.
	if in.Footer != "" {
		r.setFont(dc, false, 20)
		dc.SetColor(subCol)
		dc.DrawStringAnchored(in.Footer, cx, float64(h)-26, 0.5, 0.5)
	}

	return encodePNG(dc)
}
