package welcome

import "github.com/dia-bot/dia/internal/imaging"

// Preset is a named, ready-made welcome-card look the dashboard can apply.
type Preset struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Background imaging.Background `json:"background"`
	Accent     string             `json:"accent_color"`
	Text       string             `json:"text_color"`
	SubText    string             `json:"sub_text_color"`
}

// Presets are the built-in welcome-card themes (Dia-branded by default).
var Presets = []Preset{
	{
		ID: "aurora", Name: "Aurora (Dia)",
		Background: imaging.Background{From: imaging.BrandPink, To: imaging.BrandPurple, Angle: 45},
		Accent:     "#FFFFFF", Text: "#FFFFFF", SubText: "#F7E9F2",
	},
	{
		ID: "midnight", Name: "Midnight",
		Background: imaging.Background{From: "#1F1B2E", To: "#3A2E5C", Angle: 30},
		Accent:     imaging.BrandPurple, Text: "#FFFFFF", SubText: "#C9C3DA",
	},
	{
		ID: "blush", Name: "Blush",
		Background: imaging.Background{Color: imaging.BrandBlush},
		Accent:     imaging.BrandPink, Text: imaging.BrandInk, SubText: "#7A6B73",
	},
	{
		ID: "sunset", Name: "Sunset",
		Background: imaging.Background{From: "#FF6363", To: "#FFB347", Angle: 60},
		Accent:     "#FFFFFF", Text: "#FFFFFF", SubText: "#FFF1E6",
	},
}

// PresetByID returns a preset by id (ok=false if unknown).
func PresetByID(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == id {
			return p, true
		}
	}
	return Preset{}, false
}
