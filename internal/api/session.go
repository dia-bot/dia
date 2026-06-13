package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/dia-bot/dia/internal/cache"
)

// errNoSession indicates a missing/expired session.
var errNoSession = errors.New("no session")

// Session is the authenticated dashboard session stored in Redis and referenced
// by an opaque HttpOnly cookie. The Discord access token never reaches the
// browser.
type Session struct {
	UserID      string      `json:"user_id"`
	Username    string      `json:"username"`
	GlobalName  string      `json:"global_name"`
	Avatar      string      `json:"avatar"`
	CSRF        string      `json:"csrf"`
	AccessToken string      `json:"access_token"`
	Guilds      []UserGuild `json:"guilds"`
}

// UserGuild is a guild from the user's Discord account (used for authorization).
type UserGuild struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Icon        string `json:"icon"`
	Owner       bool   `json:"owner"`
	Permissions string `json:"permissions"`
}

type sessionStore struct {
	cache *cache.Store
	ttl   time.Duration
}

func newSessionStore(cache *cache.Store, ttl time.Duration) *sessionStore {
	return &sessionStore{cache: cache, ttl: ttl}
}

func (s *sessionStore) key(token string) string { return "sess:" + token }

func (s *sessionStore) create(ctx context.Context, sess *Session) (string, error) {
	token := randomToken()
	if err := s.cache.SetJSON(ctx, s.key(token), sess, s.ttl); err != nil {
		return "", err
	}
	return token, nil
}

func (s *sessionStore) get(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, errNoSession
	}
	var sess Session
	err := s.cache.GetJSON(ctx, s.key(token), &sess)
	if errors.Is(err, cache.ErrMiss) {
		return nil, errNoSession
	}
	if err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *sessionStore) delete(ctx context.Context, token string) error {
	return s.cache.Delete(ctx, s.key(token))
}

// save overwrites the session at the given token, resetting its TTL. Used to
// refresh ephemeral session state (e.g. the Discord guild list) without
// forcing a full re-login.
func (s *sessionStore) save(ctx context.Context, token string, sess *Session) error {
	if token == "" {
		return errNoSession
	}
	return s.cache.SetJSON(ctx, s.key(token), sess, s.ttl)
}

// randomToken returns a 256-bit opaque token.
func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
