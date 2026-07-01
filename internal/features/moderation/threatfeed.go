package moderation

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dia-bot/dia/internal/plugin"
)

// ── Scam / phishing threat feed ──────────────────────────────
//
// A package-level, in-memory blocklist of known phishing/scam domains, kept
// fresh from the community feed at phish.sinking.yachts. The scam_links trigger
// (detect.go) consults this set. A worker cold-loads the full list on start and
// then polls for recent add/delete deltas every few minutes. Network failures
// are non-fatal: the feature simply runs with a stale or empty list.

// threatList is a concurrency-safe set of blocked hosts.
type threatList struct {
	mu    sync.RWMutex
	hosts map[string]struct{}
}

// blocklist is the single package-level threat feed instance.
var blocklist = &threatList{hosts: map[string]struct{}{}}

// has reports whether host (or any of its parent domains) is on the blocklist.
// It strips leading labels so "login.paypal.scam.tld" matches a "scam.tld"
// entry.
func (l *threatList) has(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	for {
		if _, ok := l.hosts[host]; ok {
			return true
		}
		i := strings.IndexByte(host, '.')
		if i < 0 {
			return false
		}
		host = host[i+1:]
	}
}

// replaceAll swaps the entire set for a fresh cold-load.
func (l *threatList) replaceAll(domains []string) {
	next := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		if d = normalizeHost(d); d != "" {
			next[d] = struct{}{}
		}
	}
	l.mu.Lock()
	l.hosts = next
	l.mu.Unlock()
}

// apply merges an incremental update (adds + removes).
func (l *threatList) apply(adds, removes []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, d := range adds {
		if d = normalizeHost(d); d != "" {
			l.hosts[d] = struct{}{}
		}
	}
	for _, d := range removes {
		if d = normalizeHost(d); d != "" {
			delete(l.hosts, d)
		}
	}
}

// count returns the number of domains currently on the blocklist.
func (l *threatList) count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.hosts)
}

func normalizeHost(d string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(d), "."))
}

const (
	threatFeedAll    = "https://phish.sinking.yachts/v2/all"
	threatFeedRecent = "https://phish.sinking.yachts/v2/recent/300"
	threatIdentity   = "Dia (github.com/dia-bot/dia)"
)

// runThreatFeed is the "automod-threatfeed" worker: cold-load on start, then
// poll the recent-changes feed every 5 minutes. It never crashes on a network
// error; it logs and retries on the next tick.
func runThreatFeed(ctx context.Context, d plugin.Deps) {
	client := &http.Client{Timeout: 10 * time.Second}

	if err := coldLoadThreats(ctx, client); err != nil {
		d.Log.Warn("automod-threatfeed: cold load failed", "err", err)
	} else {
		d.Log.Info("automod-threatfeed: loaded", "domains", blocklist.count())
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := pollThreatChanges(ctx, client); err != nil {
				d.Log.Warn("automod-threatfeed: poll failed", "err", err)
			}
		}
	}
}

func coldLoadThreats(ctx context.Context, client *http.Client) error {
	var domains []string
	if err := getThreatJSON(ctx, client, threatFeedAll, &domains); err != nil {
		return err
	}
	blocklist.replaceAll(domains)
	return nil
}

// threatChange is one entry of the /v2/recent feed.
type threatChange struct {
	Type    string   `json:"type"` // "add" | "delete"
	Domains []string `json:"domains"`
}

func pollThreatChanges(ctx context.Context, client *http.Client) error {
	var changes []threatChange
	if err := getThreatJSON(ctx, client, threatFeedRecent, &changes); err != nil {
		return err
	}
	for _, c := range changes {
		switch c.Type {
		case "add":
			blocklist.apply(c.Domains, nil)
		case "delete":
			blocklist.apply(nil, c.Domains)
		}
	}
	return nil
}

func getThreatJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Identity", threatIdentity)
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return &threatFeedError{status: resp.StatusCode}
	}
	return json.Unmarshal(body, out)
}

type threatFeedError struct{ status int }

func (e *threatFeedError) Error() string {
	return "threat feed returned status " + http.StatusText(e.status)
}
