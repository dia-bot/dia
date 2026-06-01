package customcommands

// FeatureKey is the stable identifier (matches guild_feature_configs.feature_key
// and the dashboard route).
const FeatureKey = "customcommands"

// Config is the custom-commands feature's per-guild configuration. The actual
// command definitions live in the custom_commands table (created/edited on the
// dashboard); this struct only carries the feature-level toggle/metadata that the
// dashboard edits via guild_feature_configs.
type Config struct {
	// Enabled mirrors the feature toggle; the per-command Enabled flag lives on
	// each custom_commands row.
	Enabled bool `json:"enabled"`
}

// Default returns sensible defaults.
func Default() Config {
	return Config{Enabled: true}
}

// Response matches the custom_commands.response JSONB shape. It is the rendered
// payload for a single invocation of an admin-defined command.
type Response struct {
	Content   string         `json:"content"`
	Ephemeral bool           `json:"ephemeral"`
	Embed     *ResponseEmbed `json:"embed,omitempty"`
}

// ResponseEmbed is the optional embed portion of a custom-command response.
type ResponseEmbed struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Color       string `json:"color"`     // hex (#RRGGBB)
	ImageURL    string `json:"image_url"` // direct image URL
}
