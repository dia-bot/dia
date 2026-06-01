package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/discord"
	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
)

var oauthHTTP = &http.Client{Timeout: 10 * time.Second}

// handleLogin starts the Discord OAuth2 (Authorization Code + PKCE) flow.
func (s *Server) handleLogin(c *gin.Context) {
	state := randomToken()
	verifier := oauth2.GenerateVerifier()
	// Stash the PKCE verifier keyed by state for 10 minutes.
	if err := s.cache.SetString(c.Request.Context(), "oauth:"+state, verifier, 10*time.Minute); err != nil {
		fail(c, http.StatusInternalServerError, "could not start login")
		return
	}
	url := s.oauth.AuthCodeURL(state, oauth2.AccessTypeOnline, oauth2.S256ChallengeOption(verifier))
	c.Redirect(http.StatusTemporaryRedirect, url)
}

// handleCallback completes the OAuth2 flow, creates a session and redirects to
// the dashboard.
func (s *Server) handleCallback(c *gin.Context) {
	ctx := c.Request.Context()
	state := c.Query("state")
	code := c.Query("code")
	if state == "" || code == "" {
		fail(c, http.StatusBadRequest, "missing code/state")
		return
	}

	verifier, err := s.cache.TakeString(ctx, "oauth:"+state)
	if errors.Is(err, cache.ErrMiss) || verifier == "" {
		fail(c, http.StatusBadRequest, "invalid or expired login state")
		return
	}
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not read login state")
		return
	}

	tok, err := s.oauth.Exchange(ctx, code, oauth2.VerifierOption(verifier))
	if err != nil {
		s.log.Warn("oauth exchange failed", "err", err)
		fail(c, http.StatusBadGateway, "Discord login failed")
		return
	}

	var user discordUser
	if err := discordGet(ctx, tok.AccessToken, "/users/@me", &user); err != nil {
		fail(c, http.StatusBadGateway, "could not fetch your Discord profile")
		return
	}
	var guilds []UserGuild
	if err := discordGet(ctx, tok.AccessToken, "/users/@me/guilds", &guilds); err != nil {
		s.log.Warn("fetch guilds failed", "err", err)
	}

	sess := &Session{
		UserID:      user.ID,
		Username:    user.Username,
		GlobalName:  user.GlobalName,
		Avatar:      user.Avatar,
		CSRF:        randomToken(),
		AccessToken: tok.AccessToken,
		Guilds:      guilds,
	}
	token, err := s.sessions.create(ctx, sess)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not create session")
		return
	}
	s.setSessionCookie(c, token)
	c.Redirect(http.StatusTemporaryRedirect, s.cfg.API.WebBaseURL+"/servers")
}

// handleLogout destroys the current session.
func (s *Server) handleLogout(c *gin.Context) {
	if _, token, ok := s.sessionFromCookie(c); ok {
		_ = s.sessions.delete(c.Request.Context(), token)
	}
	s.clearSessionCookie(c)
	c.Status(http.StatusNoContent)
}

// handleMe returns the current user + CSRF token, or 401.
func (s *Server) handleMe(c *gin.Context) {
	sess, _, ok := s.sessionFromCookie(c)
	if !ok {
		fail(c, http.StatusUnauthorized, "not authenticated")
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user": gin.H{
			"id":          sess.UserID,
			"username":    sess.Username,
			"global_name": sess.GlobalName,
			"avatar":      sess.Avatar,
			"avatar_url":  discord.AvatarURL(sess.UserID, sess.Avatar, 128),
		},
		"csrf_token": sess.CSRF,
	})
}

type discordUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	GlobalName string `json:"global_name"`
	Avatar     string `json:"avatar"`
	Email      string `json:"email"`
}

// discordGet performs an authenticated GET against the Discord API.
func discordGet(ctx context.Context, accessToken, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://discord.com/api/v10"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := oauthHTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discord GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
