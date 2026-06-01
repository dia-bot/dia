package imaging

import (
	"context"
	"fmt"
	"image/color"
	"strconv"

	"github.com/fogleman/gg"
)

// RankInput is a fully-resolved rank card.
type RankInput struct {
	Width, Height int

	Background   Background
	AccentColor  string
	TextColor    string
	SubTextColor string
	BarColor     string // progress fill (defaults to accent)
	BarBgColor   string // progress track

	AvatarURL string
	Username  string
	Rank      int
	Level     int
	LevelXP   int64 // XP accumulated within the current level
	NeededXP  int64 // XP span of the current level
	TotalXP   int64
}

// RenderRank produces a PNG rank card.
func (r *Renderer) RenderRank(ctx context.Context, in RankInput) ([]byte, error) {
	if err := r.acquire(ctx); err != nil {
		return nil, err
	}
	defer r.release()

	w, h := in.Width, in.Height
	if w <= 0 {
		w = 934
	}
	if h <= 0 {
		h = 282
	}

	accent := parseHex(in.AccentColor, parseHex(BrandPurple, color.White))
	textCol := parseHex(in.TextColor, color.White)
	subCol := parseHex(in.SubTextColor, color.NRGBA{R: 200, G: 195, B: 210, A: 255})
	barFg := parseHex(in.BarColor, accent)
	barBg := parseHex(in.BarBgColor, color.NRGBA{R: 255, G: 255, B: 255, A: 40})

	dc := gg.NewContext(w, h)
	r.drawBackground(ctx, dc, w, h, in.Background, parseHex(BrandInk, color.Black))

	// Inset translucent panel for content legibility.
	dc.SetColor(color.NRGBA{A: 90})
	dc.DrawRoundedRectangle(18, 18, float64(w-36), float64(h-36), 24)
	dc.Fill()

	// Avatar (left).
	radius := float64(h)*0.5 - 46
	cx := 40 + radius
	cy := float64(h) / 2
	avatar := r.fetchAvatar(ctx, in.AvatarURL, int(radius*2), accent)
	r.drawAvatar(dc, avatar, cx, cy, radius, accent, 5)

	leftX := cx + radius + 36

	// Username.
	r.setFont(dc, true, 40)
	dc.SetColor(textCol)
	dc.DrawStringAnchored(truncate(in.Username, 18), leftX, float64(h)*0.34, 0, 0.5)

	// Rank / Level (top-right).
	r.setFont(dc, true, 34)
	dc.SetColor(accent)
	right := float64(w) - 40
	rankStr := fmt.Sprintf("RANK #%d", in.Rank)
	lvlStr := fmt.Sprintf("LEVEL %d", in.Level)
	rankW, _ := dc.MeasureString(rankStr)
	dc.DrawStringAnchored(lvlStr, right, float64(h)*0.30, 1, 0.5)
	dc.SetColor(textCol)
	dc.DrawStringAnchored(rankStr, right-rankW-28, float64(h)*0.30, 1, 0.5)
	_ = rankW

	// XP text.
	r.setFont(dc, false, 24)
	dc.SetColor(subCol)
	xpStr := fmt.Sprintf("%s / %s XP", formatInt(in.LevelXP), formatInt(in.NeededXP))
	dc.DrawStringAnchored(xpStr, right, float64(h)*0.58, 1, 0.5)

	// Progress bar.
	barX := leftX
	barW := right - barX
	barY := float64(h)*0.66
	barH := 30.0
	pct := 0.0
	if in.NeededXP > 0 {
		pct = float64(in.LevelXP) / float64(in.NeededXP)
	}
	drawProgressBar(dc, barX, barY, barW, barH, pct, barBg, barFg)

	return encodePNG(dc)
}

func truncate(s string, max int) string {
	rs := []rune(s)
	if len(rs) <= max {
		return s
	}
	return string(rs[:max-1]) + "…"
}

// formatInt adds thousands separators.
func formatInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := false
	if len(s) > 0 && s[0] == '-' {
		neg = true
		s = s[1:]
	}
	var out []byte
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}
