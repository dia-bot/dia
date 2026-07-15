package social

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Kick calls the Kick public API with an app access token and manages
// livestream.status.updated event subscriptions delivered to the webhook URL
// configured on the Kick app.
type Kick struct {
	clientID     string
	clientSecret string

	mu       sync.Mutex
	token    string
	tokenExp time.Time
	pubKey   *rsa.PublicKey
}

// NewKick builds a Kick client.
func NewKick(clientID, clientSecret string) *Kick {
	return &Kick{clientID: clientID, clientSecret: clientSecret}
}

// appToken returns a cached client-credentials token, refreshing when expired.
func (k *Kick) appToken(ctx context.Context) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.token != "" && time.Now().Before(k.tokenExp) {
		return k.token, nil
	}
	form := url.Values{
		"client_id":     {k.clientID},
		"client_secret": {k.clientSecret},
		"grant_type":    {"client_credentials"},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://id.kick.com/oauth2/token", strings.NewReader(form.Encode()))
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
		return "", fmt.Errorf("kick token: status %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	k.token = tok.AccessToken
	k.tokenExp = time.Now().Add(time.Duration(tok.ExpiresIn)*time.Second - time.Minute)
	return k.token, nil
}

// api performs an authenticated request against api.kick.com/public/v1.
func (k *Kick) api(ctx context.Context, method, path string, body, out any) error {
	tok, err := k.appToken(ctx)
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
	req, err := http.NewRequestWithContext(ctx, method, "https://api.kick.com/public/v1"+path, rd)
	if err != nil {
		return err
	}
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
		return fmt.Errorf("kick %s %s: status %d: %s", method, path, resp.StatusCode, truncate(string(data), 200))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// KickChannel is the resolved broadcaster a subscription follows.
type KickChannel struct {
	BroadcasterUserID int64
	Slug              string
	StreamTitle       string
	Live              bool
}

// ResolveChannel resolves a Kick channel slug to its broadcaster user id and
// current stream snapshot.
func (k *Kick) ResolveChannel(ctx context.Context, slug string) (KickChannel, error) {
	slug = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(slug), "@"))
	var out struct {
		Data []struct {
			BroadcasterUserID int64  `json:"broadcaster_user_id"`
			Slug              string `json:"slug"`
			StreamTitle       string `json:"stream_title"`
			Stream            struct {
				IsLive bool `json:"is_live"`
			} `json:"stream"`
		} `json:"data"`
	}
	if err := k.api(ctx, http.MethodGet, "/channels?slug="+url.QueryEscape(slug), nil, &out); err != nil {
		return KickChannel{}, err
	}
	if len(out.Data) == 0 {
		return KickChannel{}, fmt.Errorf("kick channel %q not found", slug)
	}
	c := out.Data[0]
	return KickChannel{
		BroadcasterUserID: c.BroadcasterUserID,
		Slug:              c.Slug,
		StreamTitle:       c.StreamTitle,
		Live:              c.Stream.IsLive,
	}, nil
}

// KickSubscription is one active event subscription on the app.
type KickSubscription struct {
	ID                string `json:"id"`
	Event             string `json:"event"`
	BroadcasterUserID int64  `json:"broadcaster_user_id"`
}

// ListSubscriptions returns the app's event subscriptions.
func (k *Kick) ListSubscriptions(ctx context.Context) ([]KickSubscription, error) {
	var out struct {
		Data []KickSubscription `json:"data"`
	}
	if err := k.api(ctx, http.MethodGet, "/events/subscriptions", nil, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// Subscribe registers the livestream.status.updated webhook for a broadcaster.
func (k *Kick) Subscribe(ctx context.Context, broadcasterUserID int64) error {
	body := map[string]any{
		"broadcaster_user_id": broadcasterUserID,
		"method":              "webhook",
		"events": []map[string]any{
			{"name": "livestream.status.updated", "version": 1},
		},
	}
	return k.api(ctx, http.MethodPost, "/events/subscriptions", body, nil)
}

// Unsubscribe deletes event subscriptions by id.
func (k *Kick) Unsubscribe(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	q := url.Values{}
	for _, id := range ids {
		q.Add("id", id)
	}
	return k.api(ctx, http.MethodDelete, "/events/subscriptions?"+q.Encode(), nil, nil)
}

// publicKey fetches (and caches) the RSA key Kick signs webhooks with.
func (k *Kick) publicKey(ctx context.Context) (*rsa.PublicKey, error) {
	k.mu.Lock()
	if k.pubKey != nil {
		defer k.mu.Unlock()
		return k.pubKey, nil
	}
	k.mu.Unlock()

	var out struct {
		Data struct {
			PublicKey string `json:"public_key"`
		} `json:"data"`
	}
	if err := getJSON(ctx, "https://api.kick.com/public/v1/public-key", nil, &out); err != nil {
		return nil, err
	}
	block, _ := pem.Decode([]byte(out.Data.PublicKey))
	if block == nil {
		return nil, fmt.Errorf("kick public key: not PEM")
	}
	parsed, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("kick public key: %w", err)
	}
	pub, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("kick public key: not RSA")
	}
	k.mu.Lock()
	k.pubKey = pub
	k.mu.Unlock()
	return pub, nil
}

// VerifySignature checks a Kick webhook delivery: base64 RSA-SHA256 over
// "<message id>.<timestamp>.<body>" from the Kick-Event-* headers.
func (k *Kick) VerifySignature(ctx context.Context, h http.Header, body []byte) bool {
	sig, err := base64.StdEncoding.DecodeString(h.Get("Kick-Event-Signature"))
	if err != nil {
		return false
	}
	pub, err := k.publicKey(ctx)
	if err != nil {
		return false
	}
	msg := h.Get("Kick-Event-Message-Id") + "." + h.Get("Kick-Event-Message-Timestamp") + "."
	digest := sha256.Sum256(append([]byte(msg), body...))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig) == nil
}

// FormatKickID renders a Kick broadcaster user id as the account_id string.
func FormatKickID(id int64) string { return strconv.FormatInt(id, 10) }
