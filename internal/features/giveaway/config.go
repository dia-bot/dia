// Package giveaway runs fully-customizable prize draws: a hosted giveaway posts
// a live embed with an Enter button and a countdown, accumulates weighted
// entries, and at its deadline draws random winners (biased by role bonus
// entries), announces them, optionally DMs them, and publishes a GIVEAWAY_ENDED
// event so Automations can react. Timers are durable — a background sweeper ends
// giveaways whose deadline passed, so a restart never drops a draw.
//
// Every user-facing string (embed title/description/footer, the winner
// announcement, the winner DM) is a Go text/template rendered against the
// giveaway scope (see embeds.go), matching the repo-wide templating contract.
package giveaway

// FeatureKey is the stable identifier used in guild_feature_configs and as the
// dashboard route segment.
const FeatureKey = "giveaway"

// Config is the giveaway feature's per-guild configuration (stored as JSONB,
// edited from the dashboard). Individual giveaways live in their own table; this
// holds the defaults, styling and behaviour every giveaway inherits.
type Config struct {
	// ManagerRoles may create and manage giveaways in addition to admins
	// (members with Manage Server always can).
	ManagerRoles []string `json:"manager_roles,omitempty"`

	// Defaults pre-fill new giveaways when the command doesn't override them.
	DefaultChannelID   string `json:"default_channel_id,omitempty"`
	DefaultWinnerCount int    `json:"default_winner_count"`
	DefaultDuration    string `json:"default_duration"` // e.g. "24h", "3d", "1w"

	// PingRoleID, when set, is pinged above the giveaway embed on start.
	PingRoleID string `json:"ping_role_id,omitempty"`

	Embed        EmbedConfig       `json:"embed"`
	Button       ButtonConfig      `json:"button"`
	Announce     AnnounceConfig    `json:"announce"`
	Requirements RequirementConfig `json:"requirements"`

	// Display toggles for the live embed.
	ShowEntryCount   bool `json:"show_entry_count"`
	ShowRequirements bool `json:"show_requirements"`

	// Draw behaviour.
	AllowBotsToWin bool `json:"allow_bots_to_win"`
	ExcludeHost    bool `json:"exclude_host"` // the host can't win their own giveaway
}

// EmbedConfig styles the live giveaway embed. Title/Description/FooterText are
// templated; the *Label fields rename the inline fields.
type EmbedConfig struct {
	Color         string `json:"color"` // hex, "" = brand accent
	Title         string `json:"title"`
	Description   string `json:"description"`
	FooterText    string `json:"footer_text"`
	Thumbnail     string `json:"thumbnail"` // url, "" = none
	HostedByLabel string `json:"hosted_by_label"`
	EndsLabel     string `json:"ends_label"`
	WinnersLabel  string `json:"winners_label"`
	EntriesLabel  string `json:"entries_label"`
	ShowTimestamp bool   `json:"show_timestamp"`
}

// ButtonConfig customizes the Enter button.
type ButtonConfig struct {
	Label string `json:"label"`
	Emoji string `json:"emoji"` // unicode glyph or "name:id"
	Style string `json:"style"` // primary | secondary | success | danger
}

// AnnounceConfig controls how winners are announced and DMed.
type AnnounceConfig struct {
	Message          string `json:"message"`            // in-channel congrats, templated
	PingWinners      bool   `json:"ping_winners"`       // ping the winners in the announcement
	JumpButton       bool   `json:"jump_button"`        // add a "Jump to giveaway" link button
	EndedTitle       string `json:"ended_title"`        // templated title of the ended embed
	EndedFooter      string `json:"ended_footer"`       // templated footer of the ended embed
	NoWinnersMessage string `json:"no_winners_message"` // templated, shown when nobody eligible entered
	DMWinners        bool   `json:"dm_winners"`
	DMMessage        string `json:"dm_message"` // templated winner DM
}

// RequirementConfig is the entry-eligibility spec. It is both the feature-level
// default and the per-giveaway resolved requirements (stored on each giveaway
// row), so a giveaway keeps its rules even if the defaults later change.
type RequirementConfig struct {
	// RequiredRoles: the member must hold at least one (empty = no requirement).
	RequiredRoles []string `json:"required_roles,omitempty"`
	// BlockedRoles: holding any of these blocks entry.
	BlockedRoles []string `json:"blocked_roles,omitempty"`
	// BypassRoles: holding any skips ALL requirements (still gets bonus entries).
	BypassRoles []string `json:"bypass_roles,omitempty"`
	// BonusEntries: extra weighted tickets per role held (stacks additively).
	BonusEntries []BonusEntry `json:"bonus_entries,omitempty"`
	// MinAccountAgeDays: minimum Discord account age, in days (0 = off).
	MinAccountAgeDays int `json:"min_account_age_days,omitempty"`
	// MinMemberAgeDays: minimum time in this server, in days (0 = off).
	MinMemberAgeDays int `json:"min_member_age_days,omitempty"`
	// MinLevel: minimum leveling level, when the leveling feature is on (0 = off).
	MinLevel int `json:"min_level,omitempty"`
}

// BonusEntry grants extra tickets to members holding a role.
type BonusEntry struct {
	RoleID  string `json:"role_id"`
	Entries int    `json:"entries"`
}

// Default returns sensible, ready-to-use defaults. The templates use the
// giveaway scope documented in embeds.go.
func Default() Config {
	return Config{
		DefaultWinnerCount: 1,
		DefaultDuration:    "24h",
		Embed: EmbedConfig{
			Color:         "#FF6363",
			Title:         "🎉 {{ .Prize }}",
			Description:   "Click the button below to enter!",
			FooterText:    "{{ .WinnerCount }} winner(s) · ends",
			HostedByLabel: "Hosted by",
			EndsLabel:     "Ends",
			WinnersLabel:  "Winners",
			EntriesLabel:  "Entries",
			ShowTimestamp: true,
		},
		Button: ButtonConfig{
			Label: "Enter Giveaway",
			Emoji: "🎉",
			Style: "primary",
		},
		Announce: AnnounceConfig{
			Message:          "Congratulations {{ .Winners }}! You won **{{ .Prize }}** 🎉",
			PingWinners:      true,
			JumpButton:       true,
			EndedTitle:       "🎉 {{ .Prize }}",
			EndedFooter:      "Ended",
			NoWinnersMessage: "Not enough valid entries to draw a winner for **{{ .Prize }}**.",
			DMWinners:        false,
			DMMessage:        "🎉 You won **{{ .Prize }}** in {{ .Server }}! Contact the host {{ .Host }} to claim your prize.",
		},
		Requirements:     RequirementConfig{},
		ShowEntryCount:   true,
		ShowRequirements: true,
		AllowBotsToWin:   false,
		ExcludeHost:      false,
	}
}

// buttonStyle maps a config style string to Discord's button style.
func buttonComponentStyle(s string) string {
	switch s {
	case "primary", "secondary", "success", "danger":
		return s
	default:
		return "primary"
	}
}

// resolveRequirements merges the feature defaults with per-command overrides.
// A nil override slice inherits the default; a non-nil (possibly empty) one
// replaces it, so a giveaway can explicitly clear a requirement.
func (c Config) resolveRequirements(override *RequirementConfig) RequirementConfig {
	r := c.Requirements
	if override == nil {
		return r
	}
	if override.RequiredRoles != nil {
		r.RequiredRoles = override.RequiredRoles
	}
	if override.BlockedRoles != nil {
		r.BlockedRoles = override.BlockedRoles
	}
	if override.BypassRoles != nil {
		r.BypassRoles = override.BypassRoles
	}
	if override.BonusEntries != nil {
		r.BonusEntries = override.BonusEntries
	}
	if override.MinAccountAgeDays > 0 {
		r.MinAccountAgeDays = override.MinAccountAgeDays
	}
	if override.MinMemberAgeDays > 0 {
		r.MinMemberAgeDays = override.MinMemberAgeDays
	}
	if override.MinLevel > 0 {
		r.MinLevel = override.MinLevel
	}
	return r
}
