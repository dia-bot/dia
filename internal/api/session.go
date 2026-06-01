package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
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
	rdb *redis.Client
	ttl time.Duration
}

func newSessionStore(rdb *redis.Client, ttl time.Duration) *sessionStore {
	return &sessionStore{rdb: rdb, ttl: ttl}
}

func (s *sessionStore) key(token string) string { return "sess:" + token }

func (s *sessionStore) create(ctx context.Context, sess *Session) (string, error) {
	token := randomToken()
	raw, err := json.Marshal(sess)
	if err != nil {
		return "", err
	}
	if err := s.rdb.Set(ctx, s.key(token), raw, s.ttl).Err(); err != nil {
		return "", err
	}
	return token, nil
}

func (s *sessionStore) get(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, errNoSession
	}
	raw, err := s.rdb.Get(ctx, s.key(token)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, errNoSession
	}
	if err != nil {
		return nil, err
	}
	var sess Session
	if err := json.Unmarshal(raw, &sess); err != nil {
		return nil, err
	}
	return &sess, nil
}

func (s *sessionStore) delete(ctx context.Context, token string) error {
	return s.rdb.Del(ctx, s.key(token)).Err()
}

// randomToken returns a 256-bit opaque token.
func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
