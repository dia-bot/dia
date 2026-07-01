// Package verification gates new members behind a verification step (a button
// click or an image captcha) before they get access to the server. New joiners
// receive an "unverified" role limiting them to the verification channel; once
// they pass, the unverified role is removed and an optional verified role added.
package verification

import (
	cc "github.com/dia-bot/dia/internal/features/customcommands"
)

// FeatureKey is the stable identifier for the verification feature (matches
// guild_feature_configs.feature_key and the dashboard route).
const FeatureKey = "verification"

// Mode is how a member proves they are human.
const (
	// ModeButton: a single "Verify" button. Lowest friction, stops the laziest
	// bots and gives a clear opt-in.
	ModeButton = "button"
	// ModeCaptcha: a generated image-captcha the member must read and type back
	// via a modal. Stronger, stops basic OCR-less bots.
	ModeCaptcha = "captcha"
)

// Config is the verification feature's per-guild configuration (JSONB).
type Config struct {
	// Mode is "button" or "captcha".
	Mode string `json:"mode"`
	// UnverifiedRole is granted on join and removed on success. Required for the
	// gate to actually restrict access (the role's channel permissions are set up
	// by the server owner; the dashboard explains this).
	UnverifiedRole string `json:"unverified_role"`
	// VerifiedRole is optionally granted on success (some servers gate on a
	// positive "member" role instead of, or in addition to, the unverified role).
	VerifiedRole string `json:"verified_role"`
	// Channel is where the verification prompt is posted / the gate lives.
	Channel string `json:"channel"`
	// WelcomeText is the legacy gate-message text (templated; supports
	// {{ .User.Mention }}, {{ .Guild.Name }}). Superseded by Content for the rich
	// composer; kept as the fallback so old saved configs keep rendering.
	WelcomeText string `json:"welcome_text"`

	// Content is the rich gate-message text edited by the dashboard composer
	// (Go template; same vars as WelcomeText). When empty the backend falls back
	// to WelcomeText.
	Content string `json:"content"`
	// Embeds are the gate message's rich embeds, the SAME shape the
	// customcommands send_message step uses, so the dashboard's MessageEditor
	// output decodes 1:1 here.
	Embeds []cc.EmbedSpec `json:"embeds,omitempty"`
	// Components are the gate message's custom button / select rows
	// (cc.ComponentRow). The persistent "Verify" button is NOT stored here; the
	// backend injects it as the first row. A custom button gets the custom_id
	// "vbtn:<custom_id_suffix>" and, when mapped via ButtonActions, runs the named
	// automation on click.
	Components []cc.ComponentRow `json:"components,omitempty"`
	// ButtonActions maps each custom button (by its custom_id_suffix) to the saved
	// automation launched when that button is clicked.
	ButtonActions []VButtonAction `json:"button_actions,omitempty"`
	// KickAfterMinutes kicks members who never verify after this many minutes
	// (0 = never auto-kick).
	KickAfterMinutes int `json:"kick_after_minutes"`
	// OnlySuspicious gates only risky joiners (account younger than
	// MinAccountAgeHours or with no avatar); everyone else passes instantly.
	OnlySuspicious     bool `json:"only_suspicious"`
	MinAccountAgeHours int  `json:"min_account_age_hours,omitempty"`
	// RequireAvatar (with OnlySuspicious) treats a joiner who has no profile
	// picture as suspicious, so they get gated. On by default. No omitempty: the
	// "off" choice must serialize so it survives a round-trip (default is true).
	RequireAvatar bool `json:"require_avatar"`
	// RunAutomation optionally launches a saved automation flow when a member
	// verifies (in addition to the "verification_passed" trigger any flow can use).
	RunAutomation string `json:"run_automation,omitempty"`
}

// VButtonAction binds one custom gate button (matched by its custom_id_suffix)
// to the saved automation that runs when the button is clicked.
type VButtonAction struct {
	Suffix       string `json:"suffix"`
	AutomationID string `json:"automation_id"`
}

// Default returns sensible verification defaults (button mode, no auto-kick).
func Default() Config {
	return Config{
		Mode:             ModeButton,
		RequireAvatar:    true,
		WelcomeText:      "Welcome to {{ .Guild.Name }}! Click the button below to verify you are human and unlock the server.",
		KickAfterMinutes: 0,
	}
}
