package welcome

import (
	cc "github.com/dia-bot/dia/internal/features/customcommands"
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

	// Components are the message's button / select rows (up to 5), mirroring the
	// custom-command message shape so the dashboard composer can edit them in
	// place. A non-link component routes its click to this feature's handler
	// (custom_id "welcome:<tab>:<suffix>"); if no action is wired for it the
	// click is acknowledged silently.
	Components []cc.ComponentRow `json:"components,omitempty"`

	// Actions are the per-component click programs, keyed by the component's
	// custom_id_suffix. Authored by dragging the component's dot on the
	// Welcome automation flow and run when the component is clicked. A click
	// program is a full durable flow (it may open a modal, wait for a reply,
	// branch, send follow-ups).
	Actions []ButtonAction `json:"actions,omitempty"`

	// Tail is the post-message flow: the steps wired AFTER the channel message
	// on the Welcome automation canvas ("connect a new action after sending a
	// message"). It runs as a durable automation run once the join/leave message
	// has been posted, so it can do anything an automation can — add roles, post
	// elsewhere, wait, branch, DM, even wait for the member's reply.
	Tail []cc.Step `json:"tail,omitempty"`

	Card CardConfig `json:"card"`
	DM   DMConfig   `json:"dm"`
}

// ButtonAction is the click-action program for one interactive component
// (button or select), keyed by its custom_id_suffix. The Steps reuse the
// automation/custom-command step model verbatim, so the same canvas editor and
// runtime engine drive them.
type ButtonAction struct {
	Suffix string    `json:"suffix"`
	Steps  []cc.Step `json:"steps,omitempty"`
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

// DMConfig optionally direct-messages the member. It carries the same rich
// surface as the channel message (embeds, button / select rows and their click
// actions); a DM component routes its click to this feature's handler with the
// custom_id "welcome:dm:<tab>:<guildID>:<suffix>" (the guild id is embedded
// because a DM interaction carries no guild on its own).
type DMConfig struct {
	Enabled    bool              `json:"enabled"`
	Content    string            `json:"content"`
	Embeds     []EmbedConfig     `json:"embeds,omitempty"`
	Components []cc.ComponentRow `json:"components,omitempty"`
	Actions    []ButtonAction    `json:"actions,omitempty"`
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
