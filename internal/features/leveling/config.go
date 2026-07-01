package leveling

import (
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/imaging"
	"github.com/dia-bot/dia/internal/layout"
)

// FeatureKey is the stable identifier (matches guild_feature_configs.feature_key
// and the dashboard route).
const FeatureKey = "leveling"

// Config is the leveling feature's per-guild configuration (stored as JSONB and
// edited from the dashboard).
type Config struct {
	// XP earning
	XPMin           int     `json:"xp_min"`
	XPMax           int     `json:"xp_max"`
	CooldownSeconds int     `json:"cooldown_seconds"`
	Multiplier      float64 `json:"multiplier"`

	// Level-up announcements
	AnnounceLevelUp bool   `json:"announce_level_up"`
	AnnounceChannel string `json:"announce_channel"` // "" = same channel, a channel id, or "dm"

	// LevelUp is the rich announcement edited in the dashboard composer (content
	// + embeds). When it is empty the legacy single-string LevelUpMessage is used
	// instead, so announcements configured before the composer keep working.
	LevelUp LevelUpMsg `json:"level_up_msg"`

	// LevelUpMessage is the legacy single-line announcement. Kept for back-compat:
	// it renders only when LevelUp is empty.
	LevelUpMessage string `json:"level_up_message"`

	// Actions are the per-component click programs for the level-up message's
	// buttons / selects, keyed by the component's custom_id suffix. They are
	// authored by dragging a component's dot on the leveling automation flow and
	// run as durable flows when the component is clicked. A click program is a
	// full durable flow (it may open a modal, wait for a reply, branch, send
	// follow-ups). Mirrors welcome.MessageConfig.Actions.
	Actions []ButtonAction `json:"level_up_actions,omitempty"`

	// Tail is the post-announce flow: the steps wired AFTER the level-up message
	// on the leveling automation canvas. It runs as a durable automation run once
	// the announcement has been posted, so it can do anything an automation can
	// (add roles, post elsewhere, wait, branch, DM). Mirrors
	// welcome.MessageConfig.Tail.
	Tail []cc.Step `json:"level_up_tail,omitempty"`

	// Exclusions
	NoXPChannels []string `json:"no_xp_channels"`
	NoXPRoles    []string `json:"no_xp_roles"`

	// Role rewards
	StackRewards bool `json:"stack_rewards"`

	// Rank card appearance
	RankCard RankCardConfig `json:"rank_card"`
}

// ButtonAction is the click-action program for one interactive component
// (button or select) on the level-up message, keyed by its custom_id suffix.
// The Steps reuse the automation/custom-command step model verbatim, so the
// same canvas editor and runtime engine drive them. This is a leveling-local
// copy of welcome.ButtonAction (identical shape); it is NOT imported from
// welcome, to keep the import graph acyclic (automations imports welcome, and
// welcome must not import leveling).
type ButtonAction struct {
	Suffix string    `json:"suffix"`
	Steps  []cc.Step `json:"steps,omitempty"`
}

// LevelUpMsg is the rich level-up announcement authored in the dashboard
// composer. Content and every embed string are rendered as templates (Go {{ }}
// logic plus the {token} shorthands the rank-card picker documents) against the
// leveling scope at announce time. The embeds reuse the custom-command EmbedSpec
// so the shared MessageEditor edits them in place.
type LevelUpMsg struct {
	Content string         `json:"content"`
	Embeds  []cc.EmbedSpec `json:"embeds,omitempty"`

	// Components are the announcement's button / select rows (up to 5), mirroring
	// the custom-command message shape so the dashboard composer can edit them in
	// place. A non-link component routes its click to this feature's handler
	// (custom_id "leveling:<suffix>"); if no action is wired for it the click is
	// acknowledged silently.
	Components []cc.ComponentRow `json:"components,omitempty"`
}

// RankCardConfig describes the generated /rank image.
type RankCardConfig struct {
	// Layout is a Card Studio design; when set it takes precedence over the
	// legacy preset colours below and is rendered via imaging.RenderLayout.
	Layout *layout.Layout `json:"layout,omitempty"`

	Background   imaging.Background `json:"background"`
	AccentColor  string             `json:"accent_color"`
	TextColor    string             `json:"text_color"`
	SubTextColor string             `json:"sub_text_color"`
	BarColor     string             `json:"bar_color"`
	BarBgColor   string             `json:"bar_bg_color"`
}

// Default returns sensible defaults. The rank card uses the flat palette (a
// solid near-black surface, off-white text, muted sub-text and the rose accent
// bar) with NO gradient — the same values the web rankStarterLayout() seeds, so
// the dashboard preview and the bot's /rank render agree. Card Studio seeds the
// full-space avatar+username layout on the web side.
func Default() Config {
	return Config{
		XPMin:           15,
		XPMax:           25,
		CooldownSeconds: 60,
		Multiplier:      1.0,
		AnnounceLevelUp: true,
		AnnounceChannel: "",
		LevelUp: LevelUpMsg{
			Content: "GG {user.mention}, you reached **level {level}**!",
		},
		LevelUpMessage: "GG {user.mention}, you reached **level {level}**!",
		StackRewards:   true,
		RankCard: RankCardConfig{
			Background:   imaging.Background{Color: "#141417"},
			AccentColor:  "#FF6363",
			TextColor:    "#FAFAFA",
			SubTextColor: "#A4A4AE",
			BarColor:     "#FF6363",
			BarBgColor:   "#212126",
		},
	}
}
