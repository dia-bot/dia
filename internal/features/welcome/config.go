package welcome

import "github.com/dia-bot/dia/internal/imaging"

// FeatureKey is the stable identifier (matches guild_feature_configs.feature_key
// and the dashboard route).
const FeatureKey = "welcome"

// Config is the welcome feature's per-guild configuration (stored as JSONB and
// edited from the dashboard).
type Config struct {
	// Join
	ChannelID  string `json:"channel_id"`
	Message    string `json:"message"`     // template; placeholders below
	UseEmbed   bool   `json:"use_embed"`
	EmbedColor string `json:"embed_color"` // hex
	DMMessage  string `json:"dm_message"`  // optional DM template

	// Welcome card image
	Card CardConfig `json:"card"`

	// Leave
	LeaveEnabled   bool   `json:"leave_enabled"`
	LeaveChannelID string `json:"leave_channel_id"`
	LeaveMessage   string `json:"leave_message"`
}

// CardConfig describes the generated welcome image.
//
// Supported placeholders in Title/Subtitle: {user} {username} {server} {count}
type CardConfig struct {
	Enabled      bool               `json:"enabled"`
	Preset       string             `json:"preset"`
	Title        string             `json:"title"`
	Subtitle     string             `json:"subtitle"`
	Footer       string             `json:"footer"`
	Background    imaging.Background `json:"background"`
	AccentColor  string             `json:"accent_color"`
	TextColor    string             `json:"text_color"`
	SubTextColor string             `json:"sub_text_color"`
}

// Default returns sensible defaults using the Dia brand palette.
func Default() Config {
	return Config{
		Message:  "Hey {user.mention}, welcome to **{server}**! 🎉",
		UseEmbed: false,
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
		LeaveMessage: "**{username}** just left. We're now {count} members.",
	}
}
