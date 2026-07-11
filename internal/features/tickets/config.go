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

// PanelConfig is the JSONB shape stored on a ticket_panels row: the panel
// message (fully composed: content + any number of embeds, the same shape the
// shared MessageEditor produces) plus its categories. Kept in lockstep with
// web/src/lib/tickets/types.ts.
type PanelConfig struct {
	Content           string           `json:"content,omitempty"`
	Embeds            []cc.EmbedSpec   `json:"embeds,omitempty"`
	SelectPlaceholder string           `json:"select_placeholder,omitempty"`
	Categories        []CategoryConfig `json:"categories,omitempty"`

	// Components are user-composed button rows for the panel (the same shape the
	// shared MessageEditor produces). When present they replace the auto-generated
	// per-category buttons ("buttons" style) or are appended after the select
	// ("select" style), so the panel's buttons are edited in the preview exactly
	// like a giveaway's. A button routes by what it's wired to: ButtonBindings
	// maps its custom_id_suffix to the category it opens, ButtonActions maps it
	// to a saved automation, a link button opens its URL, and an unwired button
	// is acknowledged silently. With no composed components the classic generated
	// controls are used, so a panel is always openable.
	Components     []cc.ComponentRow `json:"components,omitempty"`
	ButtonBindings map[string]string `json:"button_bindings,omitempty"` // suffix → category id
	ButtonActions  map[string]string `json:"button_actions,omitempty"`  // suffix → automation id
}

// UnmarshalJSON folds the legacy single-embed shape ({"embed": {...}}) into
// Embeds so panels saved before the composed-message rework keep rendering.
func (pc *PanelConfig) UnmarshalJSON(b []byte) error {
	type alias PanelConfig
	var a struct {
		alias
		Embed *cc.EmbedSpec `json:"embed"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*pc = PanelConfig(a.alias)
	if len(pc.Embeds) == 0 && a.Embed != nil && !embedSpecEmpty(*a.Embed) {
		pc.Embeds = []cc.EmbedSpec{*a.Embed}
	}
	return nil
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

	// Closed is the message posted in a channel-mode ticket when it closes
	// (empty = the built-in closed card). The system Reopen / Delete / Transcript
	// buttons are appended after any composed rows.
	Closed MessageSpec `json:"closed"`

	// CloseRequest is the message /ticket closerequest posts to ask the opener to
	// confirm a close (empty = the built-in card). The system Accept / Keep-open
	// buttons are appended after any composed rows.
	CloseRequest MessageSpec `json:"close_request"`

	// Buttons restyles the system control buttons (label / emoji / style, and for
	// the optional ones a hide toggle). Zero values keep the built-in look.
	Buttons ControlButtons `json:"buttons"`

	Transcript TranscriptConfig `json:"transcript"`
	Feedback   FeedbackConfig   `json:"feedback"`
	AutoClose  AutoCloseConfig  `json:"auto_close"`

	ClaimEnabled bool `json:"claim_enabled,omitempty"`

	// OnOpenAutomation / OnCloseAutomation optionally launch a saved automation
	// (by id) as a durable run when a ticket in this category opens/closes.
	OnOpenAutomation  string `json:"on_open_automation,omitempty"`
	OnCloseAutomation string `json:"on_close_automation,omitempty"`
}

// MessageSpec is one fully-composed ticket message: content + embeds + extra
// button rows, the same shape the shared MessageEditor produces (and welcome /
// giveaway render). Composed buttons route by style: a link button opens its
// URL, any other button runs the saved automation ButtonActions points its
// custom_id_suffix at (or acknowledges silently when unwired). System control
// buttons (Claim/Close/…) are appended by the renderer, never composed here.
type MessageSpec struct {
	Content    string            `json:"content,omitempty"`
	Embeds     []cc.EmbedSpec    `json:"embeds,omitempty"`
	Components []cc.ComponentRow `json:"components,omitempty"`
	// ButtonActions maps a composed button's custom_id_suffix to the saved
	// automation it launches on click (mirrors giveaway Spec.ButtonActions).
	ButtonActions map[string]string `json:"button_actions,omitempty"`
	// ButtonBindings maps a composed button's custom_id_suffix to the SYSTEM
	// action it performs on this surface (welcome: claim/close; closed:
	// reopen/delete/transcript; close request: accept/deny), so the built-in
	// controls are composed and edited in the preview like any other button. A
	// binding wins over ButtonActions for the same suffix. When a surface
	// composes no buttons at all, the classic system row is appended instead.
	ButtonBindings map[string]string `json:"button_bindings,omitempty"`
}

// UnmarshalJSON also accepts the legacy single-embed shape
// ({content, use_embed, embed}) so panels saved before the composed-message
// rework keep decoding: the legacy embed becomes Embeds[0] when it was in use.
func (m *MessageSpec) UnmarshalJSON(b []byte) error {
	type alias MessageSpec
	var a struct {
		alias
		UseEmbed bool          `json:"use_embed"`
		Embed    *cc.EmbedSpec `json:"embed"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*m = MessageSpec(a.alias)
	if len(m.Embeds) == 0 && a.UseEmbed && a.Embed != nil && !embedSpecEmpty(*a.Embed) {
		m.Embeds = []cc.EmbedSpec{*a.Embed}
	}
	return nil
}

// Empty reports whether the spec composes nothing (so the renderer should fall
// back to the built-in message).
func (m MessageSpec) Empty() bool {
	return m.Content == "" && len(m.Embeds) == 0 && len(m.Components) == 0
}

// embedSpecEmpty reports whether a composed embed carries nothing displayable.
func embedSpecEmpty(e cc.EmbedSpec) bool {
	return e.Title == "" && e.Description == "" && e.AuthorName == "" &&
		e.ImageURL == "" && e.Thumbnail == "" && e.FooterText == "" && len(e.Fields) == 0
}

// ControlButtons restyles the system buttons the bot places on ticket messages.
type ControlButtons struct {
	Claim      SystemButton `json:"claim"`
	Close      SystemButton `json:"close"`
	Reopen     SystemButton `json:"reopen"`
	Delete     SystemButton `json:"delete"`
	Transcript SystemButton `json:"transcript"`
}

// SystemButton customizes one system control button. Zero values keep the
// built-in label / emoji / style. Hide removes the button entirely; it is
// honoured for Reopen / Delete / Transcript (the actions stay reachable via
// /ticket) but ignored for Close so a ticket can always be closed in place.
type SystemButton struct {
	Label string `json:"label,omitempty"`
	Emoji string `json:"emoji,omitempty"`
	Style string `json:"style,omitempty"` // primary | secondary | success | danger
	Hide  bool   `json:"hide,omitempty"`
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
// Message fully composes the DM above the rating select; when empty the
// built-in card (with Prompt as its description) is used. ThanksMessage is the
// templated reply shown after the opener picks a rating ({{ .Ticket.Rating }}
// is set; empty = the built-in thanks).
type FeedbackConfig struct {
	Enabled       bool        `json:"enabled,omitempty"`
	Prompt        string      `json:"prompt,omitempty"`
	Message       MessageSpec `json:"message"`
	ThanksMessage string      `json:"thanks_message,omitempty"`
}

// AutoCloseConfig closes a ticket after a period of inactivity. WarnMessage
// fully composes the inactivity warning (empty = the built-in line); it renders
// with the ticket scope and should tell the opener how to keep the ticket open.
type AutoCloseConfig struct {
	Enabled           bool        `json:"enabled,omitempty"`
	InactivityMinutes int         `json:"inactivity_minutes,omitempty"`
	WarnMinutes       int         `json:"warn_minutes,omitempty"` // grace after the warning (0 = close at once)
	WarnMessage       MessageSpec `json:"warn_message"`
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
		Embeds: []cc.EmbedSpec{{
			Title:       "Need help?",
			Description: "Open a ticket and our team will be with you shortly. Pick the option that best fits your request below.",
			Color:       "#ff6363",
		}},
		SelectPlaceholder: "Choose a ticket type",
		Categories:        []CategoryConfig{DefaultCategory("support", "General support")},
		// A real, visible open button in the composition (edited in the preview
		// like any other button); ButtonBindings routes its click to the category.
		Components: []cc.ComponentRow{{Components: []cc.Component{
			{Type: "button", Style: "primary", Label: "General support", Emoji: "🎫", CustomIDSuffix: "support"},
		}}},
		ButtonBindings: map[string]string{"support": "support"},
	}
}

// DefaultCategory returns a ready-to-use general-support category, shared by
// the default panel and the dashboard's "add category" (via the TS mirror).
func DefaultCategory(id, label string) CategoryConfig {
	return CategoryConfig{
		ID:          id,
		Label:       label,
		Emoji:       "🎫",
		Description: "Questions and general help",
		ButtonStyle: "primary",
		OpenMode:    OpenModeChannel,
		NameScheme:  "ticket-{{ printf \"%04d\" .Ticket.Number }}",
		Welcome: MessageSpec{
			Content: "{{ .User.Mention }}",
			Embeds: []cc.EmbedSpec{{
				Title:       "Ticket #{{ .Ticket.Number }}",
				Description: "Thanks for reaching out, {{ .User.Mention }}. Describe your issue and a staff member will help you soon.\n\nUse the buttons below to claim or close this ticket.",
				Color:       "#ff6363",
			}},
			// Real, visible Claim/Close buttons in the composition (edited in the
			// preview like any other button); ButtonBindings routes their clicks to
			// the native handlers. Nothing is appended invisibly.
			Components: []cc.ComponentRow{{Components: []cc.Component{
				{Type: "button", Style: "success", Label: "Claim", Emoji: "🙋", CustomIDSuffix: "claim"},
				{Type: "button", Style: "danger", Label: "Close", Emoji: "🔒", CustomIDSuffix: "close"},
			}}},
			ButtonBindings: map[string]string{"claim": "claim", "close": "close"},
		},
		Transcript:   TranscriptConfig{Enabled: true},
		Feedback:     FeedbackConfig{Enabled: false, Prompt: "How was your support experience?"},
		AutoClose:    AutoCloseConfig{Enabled: false, InactivityMinutes: 1440, WarnMinutes: 60},
		ClaimEnabled: true,
	}
}
