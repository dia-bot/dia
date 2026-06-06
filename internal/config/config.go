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
}

// DiscordConfig holds Discord application credentials.
type DiscordConfig struct {
	Token        string
	ClientID     string
	ClientSecret string
	PublicKey    string
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

// OAuthRedirectURL returns the absolute Discord OAuth2 callback URL.
func (a APIConfig) OAuthRedirectURL() string {
	return strings.TrimRight(a.BaseURL, "/") + a.OAuthRedirectPath
}

// Load resolves configuration from the environment, loading a .env file first
// if one is found in the working directory or any parent directory.
func Load() (*Config, error) {
	loadDotEnv()

	c := &Config{
		Env:      env("ENV", "development"),
		LogLevel: env("LOG_LEVEL", "info"),
		Discord: DiscordConfig{
			Token:        env("DISCORD_TOKEN", ""),
			ClientID:     env("DISCORD_CLIENT_ID", ""),
			ClientSecret: env("DISCORD_CLIENT_SECRET", ""),
			PublicKey:    env("DISCORD_PUBLIC_KEY", ""),
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

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
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
