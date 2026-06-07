package welcome

import (
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/layout"
)

// FeatureKey is the stable identifier (matches guild_feature_configs.feature_key
// and the dashboard route).
const FeatureKey = "welcome"

// Config is the welcome feature's per-guild configuration (stored as JSONB and
// edited from the dashboard). It holds two independently-configurable auto
// messages: one sent when a member joins, one when a member leaves.
type Config struct {
	Welcome MessageConfig `json:"welcome"`
	Goodbye MessageConfig `json:"goodbye"`
}

// MessageConfig fully describes one auto-message: where it posts, the plain
// content, an optional rich embed, an optional generated card image, and an
// optional DM to the member. Every string field supports the template variables
// documented in variables.go.
type MessageConfig struct {
	Enabled   bool          `json:"enabled"`
	ChannelID string        `json:"channel_id"`
	Content   string        `json:"content"`
	PingUser  bool          `json:"ping_user"` // when false, mentions render without pinging
	Embeds    []EmbedConfig `json:"embeds"`    // up to 10, in order
	Card      CardConfig    `json:"card"`
	DM        DMConfig      `json:"dm"`
}

// EmbedConfig is a full Discord embed; all text fields are templated.
type EmbedConfig struct {
	Enabled     bool         `json:"enabled"`
	Color       string       `json:"color"` // hex
	AuthorName  string       `json:"author_name"`
	AuthorIcon  string       `json:"author_icon"` // url; supports {user.avatar}
	Title       string       `json:"title"`
	URL         string       `json:"url"`
	Description string       `json:"description"`
	Fields      []EmbedField `json:"fields"`
	Thumbnail   string       `json:"thumbnail"` // url; supports {user.avatar}
	ImageURL    string       `json:"image_url"` // url; ignored when the card image is shown inside the embed
	FooterText  string       `json:"footer_text"`
	FooterIcon  string       `json:"footer_icon"`
	Timestamp   bool         `json:"timestamp"`
}

// EmbedField is one name/value row in an embed.
type EmbedField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline"`
}

// CardConfig describes the generated card image. (Phase 2 replaces this with a
// full declarative layout; the preset/background/text model stays valid.)
//
// Supported placeholders in Title/Subtitle/Footer: see variables.go.
type CardConfig struct {
	Enabled bool `json:"enabled"`

	// Layout is the Card Studio design — the primary, fully-customizable card.
	// When set it is rendered via the layout engine; the legacy preset fields
	// below are only used for older configs that predate the studio.
	Layout *layout.Layout `json:"layout,omitempty"`

	Preset       string             `json:"preset,omitempty"`
	Title        string             `json:"title,omitempty"`
	Subtitle     string             `json:"subtitle,omitempty"`
	Footer       string             `json:"footer,omitempty"`
	Background   imaging.Background `json:"background,omitempty"`
	AccentColor  string             `json:"accent_color,omitempty"`
	TextColor    string             `json:"text_color,omitempty"`
	SubTextColor string             `json:"sub_text_color,omitempty"`
}

// DMConfig optionally direct-messages the member.
type DMConfig struct {
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

// Default returns sensible defaults using the Dia brand palette: a welcome with
// a card, and a disabled goodbye preconfigured so it's ready to flip on.
func Default() Config {
	return Config{
		Welcome: MessageConfig{
			Enabled:  true,
			Content:  "Hey {user.mention}, welcome to **{server}**! 🎉",
			PingUser: true,
			Card: CardConfig{
				Enabled:  true,
				Preset:   "aurora",
				Title:    "Welcome, {user}!",
				Subtitle: "You're member #{count} of {server}",
				Background: imaging.Background{
					From:  imaging.BrandPink,
					To:    imaging.BrandPurple,
					Angle: 45,
				},
				AccentColor:  "#FFFFFF",
				TextColor:    "#FFFFFF",
				SubTextColor: "#F1DFDF",
			},
		},
		Goodbye: MessageConfig{
			Enabled: false,
			Content: "**{user.name}** just left. We're now {count} members.",
			Card: CardConfig{
				Enabled:      false,
				Preset:       "midnight",
				Title:        "Goodbye, {user}",
				Subtitle:     "{server} · {count} members",
				Background:   imaging.Background{From: "#1F1B2E", To: "#3A2E5C", Angle: 30},
				AccentColor:  imaging.BrandPurple,
				TextColor:    "#FFFFFF",
				SubTextColor: "#C9C3DA",
			},
		},
	}
}
