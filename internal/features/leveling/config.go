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

	// Exclusions
	NoXPChannels []string `json:"no_xp_channels"`
	NoXPRoles    []string `json:"no_xp_roles"`

	// Role rewards
	StackRewards bool `json:"stack_rewards"`

	// Rank card appearance
	RankCard RankCardConfig `json:"rank_card"`
}

// LevelUpMsg is the rich level-up announcement authored in the dashboard
// composer. Content and every embed string are rendered as templates (Go {{ }}
// logic plus the {token} shorthands the rank-card picker documents) against the
// leveling scope at announce time. The embeds reuse the custom-command EmbedSpec
// so the shared MessageEditor edits them in place.
type LevelUpMsg struct {
	Content string         `json:"content"`
	Embeds  []cc.EmbedSpec `json:"embeds,omitempty"`
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

// Default returns sensible defaults using the Dia brand palette.
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
			Background: imaging.Background{
				From:  imaging.BrandPink,
				To:    imaging.BrandPurple,
				Angle: 45,
			},
			AccentColor:  "#FFFFFF",
			TextColor:    "#FFFFFF",
			SubTextColor: "#F1DFDF",
			BarColor:     imaging.BrandPink,
			BarBgColor:   "#FFFFFF28",
		},
	}
}
