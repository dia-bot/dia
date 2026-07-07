// Package giveaway runs fully-customizable prize draws. Each giveaway is
// composed on the dashboard (or from a reusable preset, or by a custom-command
// "Start giveaway" step) with the same message editor as every other tab: its
// content + embeds are the shared MessageEditor shape (cc.EmbedSpec), rendered
// server-side against the giveaway scope. A hosted giveaway posts a live
// message with an Enter button and a countdown, accumulates weighted entries,
// and at its deadline draws random winners (biased by role bonus entries),
// announces them, optionally DMs them, and publishes a GIVEAWAY_ENDED event so
// Automations can react. Timers are durable: a background sweeper posts
// scheduled giveaways and ends due ones, so a restart never drops a draw.
//
// Every user-facing string is a Go text/template rendered against the giveaway
// scope (see render.go), matching the repo-wide templating contract.
package giveaway

import (
	"encoding/json"

	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// FeatureKey is the stable identifier used in guild_feature_configs and as the
// dashboard route segment.
const FeatureKey = "giveaway"

// Config is the giveaway feature's per-guild configuration (JSONB in
// guild_feature_configs). It no longer holds a single shared style: it's a
// library of reusable presets plus the roles allowed to run giveaways. Every
// giveaway stores its OWN composed Spec on the giveaway row, so presets only
// seed new giveaways and later config edits never change a live giveaway.
type Config struct {
	// ManagerRoles may create and manage giveaways in addition to admins
	// (members with Manage Server always can).
	ManagerRoles []string `json:"manager_roles,omitempty"`

	// Presets is the library of reusable giveaway templates the dashboard and
	// the custom-command step start new giveaways from.
	Presets []Preset `json:"presets,omitempty"`

	// DefaultPresetID selects the preset used when none is named (the "New
	// giveaway" starting point and the step's default).
	DefaultPresetID string `json:"default_preset_id,omitempty"`

	// Tail is the canvas-owned follow-up flow for the built-in "Draw giveaway
	// winners" automation: the steps an admin wires after a giveaway is drawn and
	// announced (e.g. reward each winner a role, post a recap elsewhere). It runs
	// as a durable automation run on the giveaway_ended event. It is owned by the
	// Automations canvas, NOT the Giveaways settings page, so settings saves pass
	// through MergeStoredTail and can't clobber a flow wired on the canvas.
	Tail []cc.Step `json:"tail,omitempty"`

	// EntryTail is the canvas-owned follow-up flow for the built-in "On giveaway
	// entry" automation: the steps an admin wires to run every time a member clicks
	// Enter (reward a role on entry, log a denial, DM a confirmation), branching on
	// .Event.outcome. It runs as a durable automation run on the giveaway_entered
	// event. Like Tail it is owned by the Automations canvas, so MergeStoredTail
	// preserves it across Giveaways-settings saves.
	EntryTail []cc.Step `json:"entry_tail,omitempty"`
}

// Preset is a reusable giveaway template. It bundles the composed message Spec,
// the default entry requirements, and the pre-filled channel/duration/winners a
// new giveaway starts from.
type Preset struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	DefaultChannelID   string            `json:"default_channel_id,omitempty"`
	DefaultDuration    string            `json:"default_duration,omitempty"` // e.g. "24h", "3d", "1w"
	DefaultWinnerCount int               `json:"default_winner_count,omitempty"`
	Spec               Spec              `json:"spec"`
	Requirements       RequirementConfig `json:"requirements"`
}

// Spec is one giveaway's composed presentation + behaviour. It is stored on the
// giveaway row (store.Giveaway.Spec) and also embedded in a Preset. The message
// (Content + Embeds) is the same shape the shared MessageEditor produces and
// that Welcome/Leveling render server-side, so a giveaway is edited exactly like
// a message in any other tab. The Enter button is system-managed (its custom_id
// routes clicks back to this feature); the user only styles it via Button.
type Spec struct {
	Content    string         `json:"content,omitempty"`
	Embeds     []cc.EmbedSpec `json:"embeds,omitempty"`
	Button     ButtonConfig   `json:"button"`
	Announce   AnnounceConfig `json:"announce"`
	Entry      EntryConfig    `json:"entry"`
	PingRoleID string         `json:"ping_role_id,omitempty"` // pinged above the live message on start

	// Components are extra user-composed button rows shown under the giveaway
	// message (the same shape the shared MessageEditor produces). A button is
	// either the entry button (its custom_id_suffix matches EnterButtonSuffix) or
	// a link (its URL is set). When Components carries no entry button, the system
	// Enter button (styled by Button) is appended so a giveaway is always
	// enterable.
	Components []cc.ComponentRow `json:"components,omitempty"`

	// EnterButtonSuffix is the custom_id_suffix of the component that enters the
	// giveaway. Empty (or no matching component) → the auto-appended system Enter
	// button is used instead.
	EnterButtonSuffix string `json:"enter_button_suffix,omitempty"`

	// ButtonActions maps a composed button's custom_id_suffix to the saved
	// automation it runs on click ("point a button at an automation"). The entry
	// button and link buttons are excluded (they route natively / open a URL).
	ButtonActions map[string]string `json:"button_actions,omitempty"`

	// ShowRequirements appends a rendered requirement summary field to the
	// primary embed (the rules are dynamic, so this saves hand-authoring them).
	ShowRequirements bool `json:"show_requirements"`

	// Draw behaviour.
	ExcludeHost    bool `json:"exclude_host"` // the host can't win their own giveaway
	AllowBotsToWin bool `json:"allow_bots_to_win"`
}

// EntryConfig customizes the ephemeral reply a member gets when they click the
// Enter button, per outcome. Every field is a Go template over the giveaway scope
// plus {{ .Entries }} (their weighted ticket count) and {{ .Reason }} (why entry
// was denied). An empty field falls back to the built-in copy, so a giveaway is
// never left with a blank reply. These are what the member sees; side effects
// ("also give them a role", "log the denial") live in the built-in on-entry
// automation (Config.EntryTail), reachable from the editor's Advanced button.
type EntryConfig struct {
	Entered     string `json:"entered,omitempty"`      // success (has {{ .Entries }})
	Left        string `json:"left,omitempty"`         // toggled off (member leaves)
	NotEligible string `json:"not_eligible,omitempty"` // failed a requirement (has {{ .Reason }})
	BotsBlocked string `json:"bots_blocked,omitempty"` // a bot clicked and bots can't win
	Ended       string `json:"ended,omitempty"`        // clicked after the giveaway ended
}

// ButtonConfig customizes the Enter button.
type ButtonConfig struct {
	Label string `json:"label"`
	Emoji string `json:"emoji"` // unicode glyph or "name:id"
	Style string `json:"style"` // primary | secondary | success | danger
}

// AnnounceConfig controls how winners are announced and DMed. The ended embed is
// a compact system card (title + winners + footer) built from the giveaway's
// colour/image, keeping the ended state tidy without hand-authoring it.
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

// RequirementConfig is the entry-eligibility spec. It is both the preset default
// and the per-giveaway resolved requirements (stored on each giveaway row), so a
// giveaway keeps its rules even if the preset later changes.
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

// defaultPresetID is the stable id of the built-in preset.
const defaultPresetID = "default"

// Default entry-reply copy, shared by defaultSpec() (what the editor seeds and
// shows) and the runtime (the fallback when a giveaway leaves a field blank), so
// the two can never drift. Each is a Go template over the giveaway scope plus
// {{ .Entries }} / {{ .Reason }}.
const (
	defaultEntered     = "🎉 You're entered into the giveaway for **{{ .Prize }}**!{{ if gt .Entries 1 }} You have **{{ .Entries }}** entries.{{ end }}"
	defaultLeft        = "You've left the giveaway for **{{ .Prize }}**."
	defaultNotEligible = "❌ {{ .Reason }}"
	defaultBotsBlocked = "Bots can't enter this giveaway."
	defaultEnded       = "This giveaway has already ended."
)

// defaultEntry returns the built-in per-outcome entry replies.
func defaultEntry() EntryConfig {
	return EntryConfig{
		Entered:     defaultEntered,
		Left:        defaultLeft,
		NotEligible: defaultNotEligible,
		BotsBlocked: defaultBotsBlocked,
		Ended:       defaultEnded,
	}
}

// Default returns a ready-to-use config with one built-in preset that reproduces
// the classic giveaway look (a rose-accent embed with Host/Ends/Winners/Entries
// fields). The templates use the giveaway scope documented in render.go.
func Default() Config {
	return Config{
		DefaultPresetID: defaultPresetID,
		Presets:         []Preset{defaultPreset()},
	}
}

// defaultPreset is the built-in starting template.
func defaultPreset() Preset {
	return Preset{
		ID:                 defaultPresetID,
		Name:               "Classic",
		DefaultDuration:    "24h",
		DefaultWinnerCount: 1,
		Spec:               defaultSpec(),
		Requirements:       RequirementConfig{},
	}
}

// defaultSpec is the composed message + button + announce a fresh giveaway
// starts from.
func defaultSpec() Spec {
	return Spec{
		Embeds: []cc.EmbedSpec{{
			Color:       "#FF6363",
			Title:       "🎉 {{ .Prize }}",
			Description: "Click the button below to enter!",
			Fields: []cc.EmbedField{
				{Name: "Hosted by", Value: "{{ .Host }}", Inline: true},
				{Name: "Ends", Value: "{{ .Ends }}", Inline: true},
				{Name: "Winners", Value: "{{ .WinnerCount }}", Inline: true},
				{Name: "Entries", Value: "{{ .EntryCount }}", Inline: true},
			},
		}},
		// A real, visible Enter button in the message (edited in the preview like any
		// other button), so nothing is added invisibly. EnterButtonSuffix points at
		// it, routing its clicks to the native entry handler.
		Components: []cc.ComponentRow{{Components: []cc.Component{{
			Type:           "button",
			Style:          "primary",
			Label:          "Enter Giveaway",
			Emoji:          "🎉",
			CustomIDSuffix: "enter",
		}}}},
		EnterButtonSuffix: "enter",
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
		Entry:            defaultEntry(),
		ShowRequirements: true,
		ExcludeHost:      false,
		AllowBotsToWin:   false,
	}
}

// preset returns the preset with the given id, falling back to the configured
// default (then the built-in) so a start always has a usable template.
func (c Config) preset(id string) Preset {
	if id != "" {
		for _, p := range c.Presets {
			if p.ID == id {
				return p
			}
		}
	}
	if c.DefaultPresetID != "" && c.DefaultPresetID != id {
		for _, p := range c.Presets {
			if p.ID == c.DefaultPresetID {
				return p
			}
		}
	}
	if len(c.Presets) > 0 {
		return c.Presets[0]
	}
	return defaultPreset()
}

// MergeStoredTail returns the incoming giveaway config JSON with its
// canvas-owned tails (Tail, EntryTail) replaced by the stored ones, so a save
// from the Giveaways settings page (which doesn't know about the built-in
// automations' follow-up flows) can't wipe a flow wired on the Automations
// canvas. Mirrors roles.MergeStoredActions / moderation.MergeStoredRuleTails. On
// any decode/encode error the incoming bytes are returned unchanged.
func MergeStoredTail(incoming, stored []byte) []byte {
	var in, st Config
	if err := json.Unmarshal(incoming, &in); err != nil {
		return incoming
	}
	if err := json.Unmarshal(stored, &st); err != nil {
		return incoming
	}
	in.Tail = st.Tail
	in.EntryTail = st.EntryTail
	out, err := json.Marshal(in)
	if err != nil {
		return incoming
	}
	return out
}

// buttonComponentStyle normalizes a config style string.
func buttonComponentStyle(s string) string {
	switch s {
	case "primary", "secondary", "success", "danger":
		return s
	default:
		return "primary"
	}
}
