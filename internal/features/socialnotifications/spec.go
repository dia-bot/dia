package socialnotifications

import (
	"encoding/json"

	"github.com/dia-bot/dia/internal/event"
	cc "github.com/dia-bot/dia/internal/features/customcommands"
	"github.com/dia-bot/dia/internal/social"
)

// MessageSpec is one composed announcement message, mirroring the dashboard's
// shared message editor output (web/src/lib/social.ts). Every string renders
// as a Go template against the announcement scope. Mirrors tickets.MessageSpec.
type MessageSpec struct {
	Content    string            `json:"content,omitempty"`
	Embeds     []cc.EmbedSpec    `json:"embeds,omitempty"`
	Components []cc.ComponentRow `json:"components,omitempty"`
	// ButtonActions maps a composed button's custom_id_suffix to the saved
	// automation its click runs (social:act:<subID>:<suffix> routes it here).
	ButtonActions map[string]string `json:"button_actions,omitempty"`
}

// Empty reports whether the spec has nothing to send.
func (m MessageSpec) Empty() bool {
	return m.Content == "" && len(m.Embeds) == 0 && len(m.Components) == 0
}

// KindConfig configures what happens for one update kind (live_start,
// live_end, new_video, new_post) on a subscription.
type KindConfig struct {
	// Disabled turns the announcement off for this kind. Absent kinds default
	// to announcing, except live_end which is opt-in (see Announces).
	Disabled bool `json:"disabled,omitempty"`
	// Message is the composed announcement; empty falls back to the legacy
	// template + brand-embed path.
	Message MessageSpec `json:"message,omitempty"`
}

// SubSpec is the per-subscription JSONB on social_subscriptions.spec.
type SubSpec struct {
	Kinds map[string]KindConfig `json:"kinds,omitempty"`
}

// DecodeSubSpec parses a subscription's spec column; broken or empty JSON
// yields the zero value (pure legacy behavior).
func DecodeSubSpec(raw json.RawMessage) SubSpec {
	var s SubSpec
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

// Kind returns the config for one update kind (zero value when unset).
func (s SubSpec) Kind(kind string) KindConfig { return s.Kinds[kind] }

// Announces reports whether an update kind should post an announcement.
// Unconfigured kinds announce by default, except live_end which only exists
// as a trigger until a server explicitly enables it.
func (s SubSpec) Announces(kind string) bool {
	kc, ok := s.Kinds[kind]
	if !ok {
		return kind != social.KindLiveEnd
	}
	return !kc.Disabled
}

// EventMap builds the .Event scope for a social update, mirroring the
// automations runtime's prepare() case for TypeSocialUpdate (the two must
// stay in lockstep so per-subscription automations and trigger automations
// see identical data).
func EventMap(upd event.SocialUpdate) map[string]any {
	return map[string]any{
		"provider":     upd.Provider,
		"kind":         upd.Kind,
		"account":      upd.AccountName,
		"account_id":   upd.AccountID,
		"account_url":  upd.AccountURL,
		"item_id":      upd.ItemID,
		"title":        upd.Title,
		"url":          upd.URL,
		"description":  upd.Description,
		"category":     upd.Category,
		"started_at":   upd.StartedAt,
		"subscription": upd.SubscriptionID,
	}
}
