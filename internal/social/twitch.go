package social

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Twitch calls the Helix API with an app access token and manages EventSub
// webhook subscriptions (stream.online / stream.offline) pointing at the
// deployment's public callback.
type Twitch struct {
	clientID     string
	clientSecret string
	hmacSecret   string // signs EventSub deliveries
	callback     string // <PUBLIC_WEBHOOK_BASE_URL>/webhooks/twitch

	mu       sync.Mutex
	token    string
	tokenExp time.Time
}

// NewTwitch builds a Twitch client.
func NewTwitch(clientID, clientSecret, hmacSecret, callback string) *Twitch {
	return &Twitch{clientID: clientID, clientSecret: clientSecret, hmacSecret: hmacSecret, callback: callback}
}

// Callback returns the EventSub webhook callback URL this client subscribes with.
func (t *Twitch) Callback() string { return t.callback }

// appToken returns a cached client-credentials token, refreshing when expired.
func (t *Twitch) appToken(ctx context.Context) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.token != "" && time.Now().Before(t.tokenExp) {
		return t.token, nil
	}
	form := url.Values{
		"client_id":     {t.clientID},
		"client_secret": {t.clientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://id.twitch.tv/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitch token: status %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	t.token = tok.AccessToken
	t.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - time.Minute)
	return t.token, nil
}

// helix performs an authenticated Helix request; a nil body sends none.
func (t *Twitch) helix(ctx context.Context, method, path string, body, out any) error {
	tok, err := t.appToken(ctx)
	if err != nil {
		return err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://api.twitch.tv/helix"+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Client-Id", t.clientID)
	req.Header.Set("Authorization", "Bearer "+tok)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("twitch %s %s: status %d: %s", method, path, resp.StatusCode, truncate(string(data), 200))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// TwitchUser is the resolved broadcaster a subscription follows.
type TwitchUser struct {
	ID          string `json:"id"`
	Login       string `json:"login"`
	DisplayName string `json:"display_name"`
}

// ResolveUser resolves a Twitch login (username) to its user id.
func (t *Twitch) ResolveUser(ctx context.Context, login string) (TwitchUser, error) {
	login = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(login), "@"))
	var out struct {
		Data []TwitchUser `json:"data"`
	}
	if err := t.helix(ctx, http.MethodGet, "/users?login="+url.QueryEscape(login), nil, &out); err != nil {
		return TwitchUser{}, err
	}
	if len(out.Data) == 0 {
		return TwitchUser{}, fmt.Errorf("twitch user %q not found", login)
	}
	return out.Data[0], nil
}

// TwitchStream is the live stream snapshot used to enrich announcements.
type TwitchStream struct {
	Title     string
	Game      string
	Thumbnail string
	StartedAt string
	Live      bool
}

// GetStream returns the broadcaster's current stream (Live=false when offline).
func (t *Twitch) GetStream(ctx context.Context, userID string) (TwitchStream, error) {
	var out struct {
		Data []struct {
			Title        string `json:"title"`
			GameName     string `json:"game_name"`
			ThumbnailURL string `json:"thumbnail_url"`
			StartedAt    string `json:"started_at"`
		} `json:"data"`
	}
	if err := t.helix(ctx, http.MethodGet, "/streams?user_id="+url.QueryEscape(userID), nil, &out); err != nil {
		return TwitchStream{}, err
	}
	if len(out.Data) == 0 {
		return TwitchStream{}, nil
	}
	s := out.Data[0]
	thumb := strings.NewReplacer("{width}", "1280", "{height}", "720").Replace(s.ThumbnailURL)
	return TwitchStream{Title: s.Title, Game: s.GameName, Thumbnail: thumb, StartedAt: s.StartedAt, Live: true}, nil
}

// EventSubSubscription is one active EventSub subscription on the app.
type EventSubSubscription struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	Condition struct {
		BroadcasterUserID string `json:"broadcaster_user_id"`
	} `json:"condition"`
	Transport struct {
		Callback string `json:"callback"`
	} `json:"transport"`
}

// ListSubscriptions returns every EventSub subscription on the app (paginated).
func (t *Twitch) ListSubscriptions(ctx context.Context) ([]EventSubSubscription, error) {
	var all []EventSubSubscription
	cursor := ""
	for {
		path := "/eventsub/subscriptions"
		if cursor != "" {
			path += "?after=" + url.QueryEscape(cursor)
		}
		var out struct {
			Data       []EventSubSubscription `json:"data"`
			Pagination struct {
				Cursor string `json:"cursor"`
			} `json:"pagination"`
		}
		if err := t.helix(ctx, http.MethodGet, path, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Data...)
		cursor = out.Pagination.Cursor
		if cursor == "" {
			return all, nil
		}
	}
}

// Subscribe creates one EventSub webhook subscription (e.g. "stream.online")
// for a broadcaster. Twitch answers 409 for an existing identical
// subscription, which callers treat as success via IsConflict.
func (t *Twitch) Subscribe(ctx context.Context, typ, broadcasterID string) error {
	body := map[string]any{
		"type":      typ,
		"version":   "1",
		"condition": map[string]string{"broadcaster_user_id": broadcasterID},
		"transport": map[string]string{"method": "webhook", "callback": t.callback, "secret": t.hmacSecret},
	}
	return t.helix(ctx, http.MethodPost, "/eventsub/subscriptions", body, nil)
}

// Unsubscribe deletes one EventSub subscription by id.
func (t *Twitch) Unsubscribe(ctx context.Context, id string) error {
	return t.helix(ctx, http.MethodDelete, "/eventsub/subscriptions?id="+url.QueryEscape(id), nil, nil)
}

// IsConflict reports whether an error is Twitch's "subscription already
// exists" answer (409), which reconciliation treats as already-subscribed.
func IsConflict(err error) bool {
	return err != nil && strings.Contains(err.Error(), "status 409")
}

// VerifySignature checks an EventSub delivery's HMAC
// (Twitch-Eventsub-Message-Signature over id + timestamp + raw body).
func (t *Twitch) VerifySignature(h http.Header, body []byte) bool {
	mac := hmac.New(sha256.New, []byte(t.hmacSecret))
	mac.Write([]byte(h.Get("Twitch-Eventsub-Message-Id")))
	mac.Write([]byte(h.Get("Twitch-Eventsub-Message-Timestamp")))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(want), []byte(h.Get("Twitch-Eventsub-Message-Signature")))
}

// truncate clips s to n runes for error messages.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
