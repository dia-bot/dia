// Package config loads Dia's configuration from the environment.
//
// Every service (the Go worker, the gin API) reads the same Config so that a
// single .env file drives the whole stack. Values are resolved from process
// environment variables; for local development a .env file in the working
// directory (or any parent) is loaded first if present.
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config is the fully-resolved configuration for a Dia Go service.
type Config struct {
	Env      string // "development" | "production"
	LogLevel string // debug | info | warn | error

	Discord  DiscordConfig
	NATS     NATSConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	API      APIConfig
	Imaging  ImagingConfig
	Storage  StorageConfig
	Premium  PremiumConfig
	Billing  BillingConfig
	Social   SocialConfig
}

// SocialConfig configures the social notification providers. Each provider is
// unlocked by its own credentials, so a deployment enables exactly the set it
// has keys for; the dashboard shows the rest as "coming soon". Push providers
// (Twitch, Kick, YouTube WebSub) additionally need WebhookBaseURL — the public
// origin of the API service — to receive callbacks.
type SocialConfig struct {
	// WebhookBaseURL is the public https origin the API is reachable on
	// (PUBLIC_WEBHOOK_BASE_URL, e.g. https://api.example.com). Push providers
	// deliver to <base>/webhooks/<provider>.
	WebhookBaseURL string

	TwitchClientID     string // TWITCH_CLIENT_ID
	TwitchClientSecret string // TWITCH_CLIENT_SECRET
	// TwitchEventSubSecret signs EventSub webhook deliveries
	// (TWITCH_EVENTSUB_SECRET). Falls back to the client secret when unset.
	TwitchEventSubSecret string

	KickClientID     string // KICK_CLIENT_ID
	KickClientSecret string // KICK_CLIENT_SECRET

	// YouTubeAPIKey (YOUTUBE_API_KEY) enriches WebSub pushes (live vs upload)
	// and resolves @handles; the push subscription itself is keyless.
	YouTubeAPIKey string
}

// TwitchEnabled reports whether Twitch stream alerts can be offered.
func (s SocialConfig) TwitchEnabled() bool {
	return s.TwitchClientID != "" && s.TwitchClientSecret != "" && s.WebhookBaseURL != ""
}

// KickEnabled reports whether Kick stream alerts can be offered.
func (s SocialConfig) KickEnabled() bool {
	return s.KickClientID != "" && s.KickClientSecret != "" && s.WebhookBaseURL != ""
}

// YouTubeEnabled reports whether YouTube upload/live alerts can be offered.
// WebSub pushes are keyless; only the public callback origin is required.
func (s SocialConfig) YouTubeEnabled() bool { return s.WebhookBaseURL != "" }

// TwitchSecret returns the EventSub HMAC secret (dedicated, or the client
// secret as fallback).
func (s SocialConfig) TwitchSecret() string {
	if s.TwitchEventSubSecret != "" {
		return s.TwitchEventSubSecret
	}
	return s.TwitchClientSecret
}

// BillingConfig configures Stripe billing for the $3.99/mo premium plan. Empty
// (no secret/price) disables billing; the dashboard then hides upgrade UI and
// premium falls back to the PREMIUM_GUILD_IDS allowlist.
type BillingConfig struct {
	SecretKey     string // sk_live_… / sk_test_…
	WebhookSecret string // whsec_… (verifies inbound webhooks)
	PriceID       string // the $3.99/mo recurring Price id
}

// Enabled reports whether checkout can be offered.
func (b BillingConfig) Enabled() bool {
	return b.SecretKey != "" && b.PriceID != ""
}

// PremiumConfig is a placeholder entitlement source until a real billing system
// exists: an allowlist of premium guild IDs (env PREMIUM_GUILD_IDS, comma-sep).
type PremiumConfig struct {
	GuildIDs []string
}

// StorageConfig configures an S3-compatible object store for user uploads
// (images, custom fonts). Works with AWS S3, Cloudflare R2, MinIO and
// DigitalOcean Spaces. Uploads are disabled unless Bucket+keys+endpoint are set.
type StorageConfig struct {
	Endpoint       string
	Region         string
	Bucket         string
	AccessKey      string
	SecretKey      string
	PublicBaseURL  string
	ForcePathStyle bool
	ACL            string
}

// Enabled reports whether uploads can be served.
func (s StorageConfig) Enabled() bool {
	return s.Endpoint != "" && s.Bucket != "" && s.AccessKey != "" && s.SecretKey != ""
}

// DiscordConfig holds Discord application credentials.
type DiscordConfig struct {
	Token        string
	ClientID     string
	ClientSecret string
	PublicKey    string
	// CustomBotEncKey is the AES-256 key (base64 or hex, 32 bytes) used to
	// encrypt customer bot tokens at rest for the custom-bot feature. Empty
	// disables custom bots (the dashboard reports it as unavailable).
	CustomBotEncKey string
}

// NATSConfig configures the JetStream event bus connection.
type NATSConfig struct {
	URL    string
	Stream string
}

// PostgresConfig configures the Postgres connection pool.
type PostgresConfig struct {
	URL      string
	MaxConns int32
}

// RedisConfig configures the Redis client.
type RedisConfig struct {
	URL string
}

// APIConfig configures the gin HTTP API + OAuth2 + sessions.
type APIConfig struct {
	Addr              string
	BaseURL           string
	WebBaseURL        string
	OAuthRedirectPath string
	SessionSecret     string
	SessionCookieName string
	// CORSAllowOrigins are the browser origins permitted by CORS. Empty means
	// "just WebBaseURL"; set CORS_ALLOW_ORIGINS (comma-separated) to allow more
	// than one — e.g. localhost plus a Tailscale/LAN address for off-box access.
	CORSAllowOrigins []string
}

// ImagingConfig configures the image renderer.
type ImagingConfig struct {
	FontsDir string
}

// OAuthRedirectURL returns the absolute Discord OAuth2 callback URL. The callback
// lands on the dashboard (web) origin, which completes the exchange against the
// API — so the session cookie is set first-party on the web origin.
func (a APIConfig) OAuthRedirectURL() string {
	return strings.TrimRight(a.WebBaseURL, "/") + a.OAuthRedirectPath
}

// Load resolves configuration from the environment, loading a .env file first
// if one is found in the working directory or any parent directory.
func Load() (*Config, error) {
	loadDotEnv()

	c := &Config{
		Env:      env("ENV", "development"),
		LogLevel: env("LOG_LEVEL", "info"),
		Discord: DiscordConfig{
			Token:           env("DISCORD_TOKEN", ""),
			ClientID:        env("DISCORD_CLIENT_ID", ""),
			ClientSecret:    env("DISCORD_CLIENT_SECRET", ""),
			PublicKey:       env("DISCORD_PUBLIC_KEY", ""),
			CustomBotEncKey: env("CUSTOM_BOT_ENC_KEY", ""),
		},
		NATS: NATSConfig{
			URL:    env("NATS_URL", "nats://localhost:4222"),
			Stream: env("NATS_STREAM", "DIA_EVENTS"),
		},
		Postgres: PostgresConfig{
			URL:      env("DATABASE_URL", "postgres://dia:dia@localhost:5432/dia?sslmode=disable"),
			MaxConns: int32(envInt("PG_MAX_CONNS", 10)),
		},
		Redis: RedisConfig{
			URL: env("REDIS_URL", "redis://localhost:6379/0"),
		},
		API: APIConfig{
			Addr:              env("API_ADDR", ":8080"),
			BaseURL:           env("API_BASE_URL", "http://localhost:8080"),
			WebBaseURL:        env("WEB_BASE_URL", "http://localhost:5173"),
			OAuthRedirectPath: env("OAUTH_REDIRECT_PATH", "/auth/callback"),
			SessionSecret:     env("SESSION_SECRET", ""),
			SessionCookieName: env("SESSION_COOKIE_NAME", "dia_session"),
			CORSAllowOrigins:  splitList(env("CORS_ALLOW_ORIGINS", "")),
		},
		Imaging: ImagingConfig{
			FontsDir: env("FONTS_DIR", "./assets/fonts"),
		},
		Premium: PremiumConfig{
			GuildIDs: splitList(env("PREMIUM_GUILD_IDS", "")),
		},
		Billing: BillingConfig{
			SecretKey:     env("STRIPE_SECRET_KEY", ""),
			WebhookSecret: env("STRIPE_WEBHOOK_SECRET", ""),
			PriceID:       env("STRIPE_PRICE_ID", ""),
		},
		Social: SocialConfig{
			WebhookBaseURL:       strings.TrimRight(env("PUBLIC_WEBHOOK_BASE_URL", ""), "/"),
			TwitchClientID:       env("TWITCH_CLIENT_ID", ""),
			TwitchClientSecret:   env("TWITCH_CLIENT_SECRET", ""),
			TwitchEventSubSecret: env("TWITCH_EVENTSUB_SECRET", ""),
			KickClientID:         env("KICK_CLIENT_ID", ""),
			KickClientSecret:     env("KICK_CLIENT_SECRET", ""),
			YouTubeAPIKey:        env("YOUTUBE_API_KEY", ""),
		},
		Storage: StorageConfig{
			Endpoint:       env("S3_ENDPOINT", ""),
			Region:         env("S3_REGION", "us-east-1"),
			Bucket:         env("S3_BUCKET", ""),
			AccessKey:      env("S3_ACCESS_KEY_ID", ""),
			SecretKey:      env("S3_SECRET_ACCESS_KEY", ""),
			PublicBaseURL:  env("S3_PUBLIC_BASE_URL", ""),
			ForcePathStyle: envBool("S3_FORCE_PATH_STYLE", false),
			ACL:            env("S3_OBJECT_ACL", ""),
		},
	}
	return c, nil
}

// RequireBot validates the configuration needed by the worker (bot) service.
func (c *Config) RequireBot() error {
	var missing []string
	if c.Discord.Token == "" {
		missing = append(missing, "DISCORD_TOKEN")
	}
	if c.Discord.ClientID == "" {
		missing = append(missing, "DISCORD_CLIENT_ID")
	}
	return missingErr(missing)
}

// RequireAPI validates the configuration needed by the API service.
func (c *Config) RequireAPI() error {
	var missing []string
	// The API calls Discord's REST API with the bot token (e.g. the live
	// /users/@me/guilds membership check that backs the dashboard's bot_present
	// fallback), so the token is required, not optional.
	if c.Discord.Token == "" {
		missing = append(missing, "DISCORD_TOKEN")
	}
	if c.Discord.ClientID == "" {
		missing = append(missing, "DISCORD_CLIENT_ID")
	}
	if c.Discord.ClientSecret == "" {
		missing = append(missing, "DISCORD_CLIENT_SECRET")
	}
	if len(c.API.SessionSecret) < 16 {
		missing = append(missing, "SESSION_SECRET (>=16 bytes)")
	}
	return missingErr(missing)
}

func missingErr(missing []string) error {
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("missing required configuration: %s", strings.Join(missing, ", "))
}

// IsProd reports whether the service is running in production mode.
func (c *Config) IsProd() bool { return c.Env == "production" }

// IsPremiumGuild reports whether a guild has premium entitlements. This is a
// stub (env allowlist) until a real billing/entitlement system lands.
func (c *Config) IsPremiumGuild(id string) bool {
	for _, g := range c.Premium.GuildIDs {
		if g == id {
			return true
		}
	}
	return false
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

// splitList parses a comma-separated value into a trimmed, non-empty slice.
func splitList(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Duration parses an env var as a Go duration with a default fallback.
func Duration(key string, def time.Duration) time.Duration {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// loadDotEnv looks for a .env file in the cwd and its parents and loads any
// variables not already present in the environment. It is intentionally
// tolerant: a missing file is not an error.
func loadDotEnv() {
	dir, err := os.Getwd()
	if err != nil {
		return
	}
	for {
		path := filepath.Join(dir, ".env")
		if f, err := os.Open(path); err == nil {
			parseDotEnv(f)
			_ = f.Close()
			return
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return // reached filesystem root
		}
		dir = parent
	}
}

func parseDotEnv(f *os.File) {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = stripInlineComment(val)
		val = strings.Trim(val, `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}

// stripInlineComment removes a trailing "# comment" from a .env value so that
// lines like `NATS_STREAM=DIA_EVENTS   # stream name` resolve to `DIA_EVENTS`.
// It is quote-aware: inside a quoted value a '#' is literal, and only a comment
// after the closing quote is dropped. For an unquoted value a '#' starts a
// comment only at the value start or when preceded by whitespace, so values
// that legitimately contain '#' (e.g. a URL fragment like host#frag) survive.
func stripInlineComment(val string) string {
	if val == "" {
		return val
	}
	if q := val[0]; q == '"' || q == '\'' {
		if end := strings.IndexByte(val[1:], q); end >= 0 {
			return val[:end+2] // keep through the closing quote; drop the rest
		}
		return val // unterminated quote: leave untouched
	}
	for i := 0; i < len(val); i++ {
		if val[i] == '#' && (i == 0 || val[i-1] == ' ' || val[i-1] == '\t') {
			return strings.TrimRight(val[:i], " \t")
		}
	}
	return val
}
