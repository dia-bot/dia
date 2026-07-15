// Package social holds the third-party clients behind the social notification
// feature: Twitch (EventSub webhooks), Kick (event webhooks), YouTube (WebSub
// push + Data API enrichment), Bluesky (public AppView polling) and generic
// RSS/Atom feeds. Each provider is unlocked by its own environment credentials
// (config.SocialConfig); the catalogue below is what the dashboard renders,
// locked tiles included.
package social

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/config"
)

// Provider keys (stored in social_subscriptions.provider).
const (
	ProviderTwitch  = "twitch"
	ProviderYouTube = "youtube"
	ProviderKick    = "kick"
	ProviderBluesky = "bluesky"
	ProviderRSS     = "rss"
)

// Update kinds (event.SocialUpdate.Kind).
const (
	KindLiveStart = "live_start"
	KindLiveEnd   = "live_end"
	KindNewVideo  = "new_video"
	KindNewPost   = "new_post"
)

// Capability statuses.
const (
	StatusAvailable  = "available"
	StatusComingSoon = "coming_soon"
)

// Capability describes one provider tile for the dashboard: whether this
// deployment has the credentials to offer it, and what to ask the user for.
type Capability struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Status   string `json:"status"` // available | coming_soon
	// Input labels the account field ("Twitch username", "Feed URL", …).
	Input string `json:"input,omitempty"`
	// Kinds are the update kinds the provider emits.
	Kinds []string `json:"kinds,omitempty"`
}

// Capabilities returns the full provider catalogue for a deployment. Providers
// without credentials — and platforms with no viable API — surface as
// coming_soon so the dashboard can render them as locked tiles.
func Capabilities(cfg *config.Config) []Capability {
	avail := func(ok bool) string {
		if ok {
			return StatusAvailable
		}
		return StatusComingSoon
	}
	return []Capability{
		{Provider: ProviderTwitch, Name: "Twitch", Status: avail(cfg.Social.TwitchEnabled()),
			Input: "Twitch username", Kinds: []string{KindLiveStart, KindLiveEnd}},
		{Provider: ProviderYouTube, Name: "YouTube", Status: avail(cfg.Social.YouTubeEnabled()),
			Input: "Channel ID or @handle", Kinds: []string{KindNewVideo, KindLiveStart}},
		{Provider: ProviderKick, Name: "Kick", Status: avail(cfg.Social.KickEnabled()),
			Input: "Kick channel", Kinds: []string{KindLiveStart, KindLiveEnd}},
		{Provider: ProviderBluesky, Name: "Bluesky", Status: StatusAvailable,
			Input: "Handle (e.g. name.bsky.social)", Kinds: []string{KindNewPost}},
		{Provider: ProviderRSS, Name: "RSS / Atom", Status: StatusAvailable,
			Input: "Feed URL", Kinds: []string{KindNewPost}},
		// No affordable / public API today; permanent roadmap tiles.
		{Provider: "x", Name: "X"},
		{Provider: "instagram", Name: "Instagram"},
		{Provider: "tiktok", Name: "TikTok"},
	}
}

// Available reports whether a provider key can be subscribed on this deployment.
func Available(cfg *config.Config, provider string) bool {
	for _, c := range Capabilities(cfg) {
		if c.Provider == provider {
			return c.Status == StatusAvailable
		}
	}
	return false
}

// Clients bundles the per-provider API clients. A nil field means the provider
// isn't configured on this deployment.
type Clients struct {
	Twitch  *Twitch
	Kick    *Kick
	YouTube *YouTube
	Bluesky *Bluesky
	RSS     *RSS
}

// NewClients wires the configured providers. Bluesky and RSS are keyless and
// always present.
func NewClients(cfg *config.Config) *Clients {
	c := &Clients{Bluesky: &Bluesky{}, RSS: &RSS{}}
	base := cfg.Social.WebhookBaseURL
	if cfg.Social.TwitchEnabled() {
		c.Twitch = NewTwitch(cfg.Social.TwitchClientID, cfg.Social.TwitchClientSecret,
			cfg.Social.TwitchSecret(), base+"/webhooks/twitch")
	}
	if cfg.Social.KickEnabled() {
		c.Kick = NewKick(cfg.Social.KickClientID, cfg.Social.KickClientSecret)
	}
	if cfg.Social.YouTubeEnabled() {
		c.YouTube = NewYouTube(cfg.Social.YouTubeAPIKey, base+"/webhooks/youtube", cfg.API.SessionSecret)
	}
	return c
}

// httpc is the shared HTTP client for every provider call: generous enough for
// a slow feed, short enough that a hung host can't stall a poll cycle.
var httpc = &http.Client{Timeout: 12 * time.Second}

// maxBody caps how much of any provider response is read (a hostile feed URL
// must not balloon memory).
const maxBody = 2 << 20 // 2 MiB

// getJSON GETs a URL (with optional headers) and decodes the JSON response.
func getJSON(ctx context.Context, url string, headers map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return json.Unmarshal(body, out)
}
