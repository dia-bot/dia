// Package tickets is Dia's support-ticket feature: a fully-customizable ticket
// system built on the plugin SDK. Admins design panels (an embed with buttons or
// a select) on the dashboard; each panel offers one or more categories (ticket
// types) with their own permissions, opening message, optional pre-open form,
// transcript, auto-close, feedback and automation hooks. Opening a ticket
// creates a private channel (or thread) visible to the opener and support staff;
// closing it can post an HTML transcript and ask the opener to rate the help.
//
// Panels + their categories live in the ticket_panels table (categories are a
// JSONB array on the panel config); live tickets are rows in tickets. Only the
// feature toggle + shared settings live in guild_feature_configs under
// FeatureKey. Ticket lifecycle events are published on the bus so the built-in
// automations engine can trigger flows off them.
package tickets

import (
	"encoding/json"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// FeatureKey is the guild_feature_configs key and dashboard route for tickets.
const FeatureKey = "tickets"

// Open modes for a category.
const (
	OpenModeChannel = "channel"
	OpenModeThread  = "thread"
)

// Config is the feature-level settings blob (guild_feature_configs, key
// "tickets"). Per-panel and per-category settings live on the panels themselves.
type Config struct {
	// StaffRoleIDs are granted access to every ticket in addition to a
	// category's own support roles, and may run staff-only ticket commands.
	StaffRoleIDs []string `json:"staff_role_ids,omitempty"`
	// LogChannel receives an embed for every open/claim/close/reopen/delete.
	LogChannel string `json:"log_channel,omitempty"`
	// TranscriptChannel receives generated transcripts (falls back to LogChannel).
	TranscriptChannel string `json:"transcript_channel,omitempty"`
	// Blacklisted roles/users can never open a ticket.
	BlacklistRoleIDs []string `json:"blacklist_role_ids,omitempty"`
	BlacklistUserIDs []string `json:"blacklist_user_ids,omitempty"`
	// MaxOpenPerUser caps how many tickets one member may have open at once
	// across all categories (0 = unlimited). A category can tighten this further.
	MaxOpenPerUser int `json:"max_open_per_user"`
	// DefaultParentID is the Discord category new ticket channels are created
	// under when a ticket category doesn't set its own.
	DefaultParentID string `json:"default_parent_id,omitempty"`
	// NamePrefix is the default ticket channel name prefix (e.g. "ticket").
	NamePrefix string `json:"name_prefix,omitempty"`
}

// Default returns sensible starting settings, reused by the seed and the
// dashboard's first save.
func Default() Config {
	return Config{
		MaxOpenPerUser: 3,
		NamePrefix:     "ticket",
	}
}

// StaffRoles returns the effective support roles for a category: its own support
// roles plus the guild-wide staff roles, de-duplicated.
func (c Config) StaffRoles(cat CategoryConfig) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range append(append([]string{}, cat.SupportRoleIDs...), c.StaffRoleIDs...) {
		if r == "" || seen[r] {
			continue
		}
		seen[r] = true
		out = append(out, r)
	}
	return out
}

// PanelConfig is the JSONB shape stored on a ticket_panels row: the panel message
// (embed + optional content) plus its categories. Kept in lockstep with
// web/src/lib/tickets/types.ts.
type PanelConfig struct {
	Content           string           `json:"content,omitempty"`
	Embed             cc.EmbedSpec     `json:"embed"`
	SelectPlaceholder string           `json:"select_placeholder,omitempty"`
	Categories        []CategoryConfig `json:"categories,omitempty"`
}

// CategoryConfig is one ticket type on a panel: the button/select entry plus all
// of that type's behaviour.
type CategoryConfig struct {
	ID          string `json:"id"` // stable key referenced by tickets.category_id
	Label       string `json:"label"`
	Emoji       string `json:"emoji,omitempty"`
	Description string `json:"description,omitempty"`
	ButtonStyle string `json:"button_style,omitempty"` // primary | secondary | success | danger

	OpenMode   string `json:"open_mode,omitempty"`   // channel (default) | thread
	ParentID   string `json:"parent_id,omitempty"`   // Discord category for channel mode
	NameScheme string `json:"name_scheme,omitempty"` // Go template for the channel name

	SupportRoleIDs  []string `json:"support_role_ids,omitempty"`
	PingRoleIDs     []string `json:"ping_role_ids,omitempty"`
	PingOpener      bool     `json:"ping_opener,omitempty"`
	RequiredRoleIDs []string `json:"required_role_ids,omitempty"` // must hold one to open
	MaxOpenPerUser  int      `json:"max_open_per_user,omitempty"`
	CooldownSeconds int      `json:"cooldown_seconds,omitempty"`

	Form    []FormField `json:"form,omitempty"` // pre-open modal (max 5 fields)
	Welcome MessageSpec `json:"welcome"`        // the ticket's opening message

	Transcript TranscriptConfig `json:"transcript"`
	Feedback   FeedbackConfig   `json:"feedback"`
	AutoClose  AutoCloseConfig  `json:"auto_close"`

	ClaimEnabled bool `json:"claim_enabled,omitempty"`

	// OnOpenAutomation / OnCloseAutomation optionally launch a saved automation
	// (by id) as a durable run when a ticket in this category opens/closes.
	OnOpenAutomation  string `json:"on_open_automation,omitempty"`
	OnCloseAutomation string `json:"on_close_automation,omitempty"`
}

// MessageSpec is a customizable content + embed message (ticket opening message).
type MessageSpec struct {
	Content  string       `json:"content,omitempty"`
	UseEmbed bool         `json:"use_embed,omitempty"`
	Embed    cc.EmbedSpec `json:"embed"`
}

// FormField is one input in a category's pre-open modal form.
type FormField struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Placeholder string `json:"placeholder,omitempty"`
	Style       string `json:"style,omitempty"` // short | paragraph
	Required    bool   `json:"required,omitempty"`
	MinLength   int    `json:"min_length,omitempty"`
	MaxLength   int    `json:"max_length,omitempty"`
}

// TranscriptConfig controls transcript generation on close.
type TranscriptConfig struct {
	Enabled  bool `json:"enabled,omitempty"`
	DMOpener bool `json:"dm_opener,omitempty"`
}

// FeedbackConfig controls the post-close rating prompt DMed to the opener.
type FeedbackConfig struct {
	Enabled bool   `json:"enabled,omitempty"`
	Prompt  string `json:"prompt,omitempty"`
}

// AutoCloseConfig closes a ticket after a period of inactivity.
type AutoCloseConfig struct {
	Enabled           bool `json:"enabled,omitempty"`
	InactivityMinutes int  `json:"inactivity_minutes,omitempty"`
	WarnMinutes       int  `json:"warn_minutes,omitempty"` // grace after the warning (0 = close at once)
}

// Category finds a category by id (ok=false if not present).
func (pc PanelConfig) Category(id string) (CategoryConfig, bool) {
	for _, c := range pc.Categories {
		if c.ID == id {
			return c, true
		}
	}
	return CategoryConfig{}, false
}

// DecodePanel decodes a panel's Config JSONB, tolerating an empty blob.
func DecodePanel(raw json.RawMessage) PanelConfig {
	var pc PanelConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &pc)
	}
	return pc
}

// DefaultPanelConfig returns a starter panel with one general-support category,
// used by the dashboard when creating a panel and by the seed.
func DefaultPanelConfig() PanelConfig {
	return PanelConfig{
		Content: "",
		Embed: cc.EmbedSpec{
			Title:       "Need help?",
			Description: "Open a ticket and our team will be with you shortly. Pick the option that best fits your request below.",
			Color:       "#ff6363",
		},
		SelectPlaceholder: "Choose a ticket type",
		Categories: []CategoryConfig{
			{
				ID:          "support",
				Label:       "General support",
				Emoji:       "🎫",
				Description: "Questions and general help",
				ButtonStyle: "primary",
				OpenMode:    OpenModeChannel,
				NameScheme:  "ticket-{{ printf \"%04d\" .Ticket.Number }}",
				Welcome: MessageSpec{
					Content:  "{{ .User.Mention }}",
					UseEmbed: true,
					Embed: cc.EmbedSpec{
						Title:       "Ticket #{{ .Ticket.Number }}",
						Description: "Thanks for reaching out, {{ .User.Mention }}. Describe your issue and a staff member will help you soon.\n\nUse the buttons below to claim or close this ticket.",
						Color:       "#ff6363",
					},
				},
				Transcript:   TranscriptConfig{Enabled: true},
				Feedback:     FeedbackConfig{Enabled: false, Prompt: "How was your support experience?"},
				AutoClose:    AutoCloseConfig{Enabled: false, InactivityMinutes: 1440, WarnMinutes: 60},
				ClaimEnabled: true,
			},
		},
	}
}
