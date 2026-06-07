// Package billing is a minimal Stripe client for Dia's $3.99/mo premium plan. It
// speaks the Stripe REST API directly over HTTPS (Checkout + Billing Portal) and
// verifies inbound webhooks with HMAC-SHA256 — no SDK dependency, matching the
// repo's lean, dependency-light approach (cf. internal/storage's SigV4).
package billing

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://api.stripe.com"

// Client calls the Stripe API with a secret key.
type Client struct {
	secretKey string
	http      *http.Client
}

// New returns a Stripe client.
func New(secretKey string) *Client {
	return &Client{secretKey: secretKey, http: &http.Client{Timeout: 20 * time.Second}}
}

// CheckoutParams configures a subscription Checkout Session.
type CheckoutParams struct {
	PriceID       string
	GuildID       string // carried as client_reference_id + metadata so the webhook can map back
	SuccessURL    string
	CancelURL     string
	CustomerEmail string // optional; prefills checkout
}

// CreateCheckoutSession starts a subscription checkout and returns its hosted URL.
func (c *Client) CreateCheckoutSession(ctx context.Context, p CheckoutParams) (string, error) {
	form := url.Values{}
	form.Set("mode", "subscription")
	form.Set("line_items[0][price]", p.PriceID)
	form.Set("line_items[0][quantity]", "1")
	form.Set("success_url", p.SuccessURL)
	form.Set("cancel_url", p.CancelURL)
	form.Set("client_reference_id", p.GuildID)
	form.Set("metadata[guild_id]", p.GuildID)
	form.Set("subscription_data[metadata][guild_id]", p.GuildID)
	form.Set("allow_promotion_codes", "true")
	if p.CustomerEmail != "" {
		form.Set("customer_email", p.CustomerEmail)
	}
	var out struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/checkout/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

// CreatePortalSession opens the Stripe Billing Portal (manage/cancel) and returns
// its URL.
func (c *Client) CreatePortalSession(ctx context.Context, customerID, returnURL string) (string, error) {
	form := url.Values{}
	form.Set("customer", customerID)
	form.Set("return_url", returnURL)
	var out struct {
		URL string `json:"url"`
	}
	if err := c.post(ctx, "/v1/billing_portal/sessions", form, &out); err != nil {
		return "", err
	}
	return out.URL, nil
}

func (c *Client) post(ctx context.Context, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiBase+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.secretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("stripe: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("stripe %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// ── Webhooks ────────────────────────────────────────────────────────────────

// Event is the slice of a Stripe webhook event we care about.
type Event struct {
	Type string `json:"type"`
	Data struct {
		Object json.RawMessage `json:"object"`
	} `json:"data"`
}

// Subscription is the subset of a Stripe subscription / checkout-session object we
// read. CheckoutSession populates SubscriptionID + Customer + the guild metadata;
// subscription.* events populate Status + CurrentPeriodEnd.
type Subscription struct {
	ID               string
	Customer         string
	Status           string
	CurrentPeriodEnd int64
	GuildID          string
}

type rawSubscription struct {
	ID                string            `json:"id"`
	Customer          string            `json:"customer"`
	Status            string            `json:"status"`
	CurrentPeriodEnd  int64             `json:"current_period_end"`
	Subscription      string            `json:"subscription"`        // present on checkout.session
	ClientReferenceID string            `json:"client_reference_id"` // present on checkout.session
	Metadata          map[string]string `json:"metadata"`
	// Recent Stripe API versions moved current_period_end onto subscription items.
	Items struct {
		Data []struct {
			CurrentPeriodEnd int64 `json:"current_period_end"`
		} `json:"data"`
	} `json:"items"`
}

// ParseSubscription extracts the fields we persist from an event object (works
// for both checkout.session and subscription objects).
func ParseSubscription(obj json.RawMessage) (Subscription, error) {
	var r rawSubscription
	if err := json.Unmarshal(obj, &r); err != nil {
		return Subscription{}, err
	}
	s := Subscription{
		ID:               r.ID,
		Customer:         r.Customer,
		Status:           r.Status,
		CurrentPeriodEnd: r.CurrentPeriodEnd,
		GuildID:          r.Metadata["guild_id"],
	}
	// On a checkout.session the subscription id is in `subscription`, and the
	// guild is in client_reference_id.
	if r.Subscription != "" {
		s.ID = r.Subscription
	}
	if s.GuildID == "" {
		s.GuildID = r.ClientReferenceID
	}
	// Fall back to the item-level period end (newer API versions).
	if s.CurrentPeriodEnd == 0 && len(r.Items.Data) > 0 {
		s.CurrentPeriodEnd = r.Items.Data[0].CurrentPeriodEnd
	}
	return s, nil
}

// VerifyWebhook checks a Stripe-Signature header against the payload using the
// webhook signing secret (HMAC-SHA256 over "t.payload"), with a 5-minute
// timestamp tolerance to bound replay.
func VerifyWebhook(payload []byte, sigHeader, secret string, now time.Time) error {
	var ts string
	var sigs []string
	for _, part := range strings.Split(sigHeader, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			ts = kv[1]
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if ts == "" || len(sigs) == 0 {
		return fmt.Errorf("stripe: malformed signature header")
	}
	tsi, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return fmt.Errorf("stripe: bad signature timestamp")
	}
	if d := now.Sub(time.Unix(tsi, 0)); d > 5*time.Minute || d < -5*time.Minute {
		return fmt.Errorf("stripe: signature timestamp outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(payload)
	expected := mac.Sum(nil)
	for _, s := range sigs {
		if raw, err := hex.DecodeString(s); err == nil && hmac.Equal(raw, expected) {
			return nil
		}
	}
	return fmt.Errorf("stripe: signature mismatch")
}
