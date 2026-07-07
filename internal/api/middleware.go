package api

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

const (
	ctxSession = "dia_session"
	ctxGuildID = "dia_guild_id"

	// Guild permissions that grant full dashboard access: Manage Server OR
	// Administrator (the guild owner implicitly qualifies — see canManage).
	// Manage Server is the conventional "runs this server" permission, so server
	// staff who aren't full Administrators are still treated as admins here — an
	// Administrator, holding every permission, always qualifies too.
	manageMask = discordgo.PermissionManageGuild | discordgo.PermissionAdministrator
)

func (s *Server) sessionFromCookie(c *gin.Context) (*Session, string, bool) {
	token, err := c.Cookie(s.cfg.API.SessionCookieName)
	if err != nil || token == "" {
		return nil, "", false
	}
	sess, err := s.sessions.get(c.Request.Context(), token)
	if err != nil {
		return nil, token, false
	}
	return sess, token, true
}

func (s *Server) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		sess, _, ok := s.sessionFromCookie(c)
		if !ok {
			fail(c, http.StatusUnauthorized, "not authenticated")
			return
		}
		c.Set(ctxSession, sess)
		c.Next()
	}
}

// csrf enforces a double-submit token on unsafe methods.
func (s *Server) csrf() gin.HandlerFunc {
	return func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			sess := currentSession(c)
			if sess == nil || sess.CSRF == "" || c.GetHeader("X-CSRF-Token") != sess.CSRF {
				fail(c, http.StatusForbidden, "invalid or missing CSRF token")
				return
			}
		}
		c.Next()
	}
}

// guildGate is the shared bootstrap for every guild route: it validates the
// guild id, confirms the bot is present, ensures a guilds row exists (for
// downstream FK-constrained writes like custom_commands / guild_feature_configs
// — botInGuild may have passed via the live bot-list signal alone, before the
// worker processed GUILD_CREATE, so without this the next INSERT would hit a
// foreign-key violation; cheap no-op upsert when the row already exists),
// resolves the caller's access, and stashes the guild id + access on the
// context. It returns false (after writing the response) when the request
// should stop.
func (s *Server) guildGate(c *gin.Context) (guildAccess, bool) {
	sess := currentSession(c)
	gid := c.Param("id")
	gidInt, ok := event.ParseID(gid)
	if !ok {
		fail(c, http.StatusBadRequest, "invalid guild id")
		return guildAccess{}, false
	}
	if !s.botInGuild(c.Request.Context(), gid) {
		fail(c, http.StatusNotFound, "Dia is not in this server")
		return guildAccess{}, false
	}
	acc := s.accessFor(c.Request.Context(), sess, gid)
	s.ensureGuildRow(c.Request.Context(), gidInt, gid, sess)
	c.Set(ctxGuildID, gid)
	c.Set(ctxAccess, acc)
	return acc, true
}

// requireGuild authorizes that the session user is a server admin/owner AND the
// bot is present. This is the strictest gate, used for every route that isn't
// explicitly delegated to feature managers.
func (s *Server) requireGuild() gin.HandlerFunc {
	return func(c *gin.Context) {
		acc, ok := s.guildGate(c)
		if !ok {
			return
		}
		if !acc.Admin {
			fail(c, http.StatusForbidden, "you don't manage this server")
			return
		}
		c.Next()
	}
}

// requireGuildAccess authorizes that the caller can reach the guild dashboard at
// all: an admin, or a manager of at least one delegated feature. Used for the
// shared surfaces a feature manager needs to load their tab (guild detail, their
// feature's config).
func (s *Server) requireGuildAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		acc, ok := s.guildGate(c)
		if !ok {
			return
		}
		if !acc.any() {
			fail(c, http.StatusForbidden, "you don't have access to this server")
			return
		}
		c.Next()
	}
}

// requireFeature authorizes that the caller may manage a specific feature: an
// admin, or a holder of one of that feature's configured manager roles.
func (s *Server) requireFeature(feature string) gin.HandlerFunc {
	return func(c *gin.Context) {
		acc, ok := s.guildGate(c)
		if !ok {
			return
		}
		if !acc.can(feature) {
			fail(c, http.StatusForbidden, "you don't have access to this feature")
			return
		}
		c.Next()
	}
}

// ensureGuildRow synthesises a minimal guilds row from session data when the
// gateway hasn't populated one yet. Name/icon are taken from the user's OAuth
// guild list; member_count stays 0 until the worker processes a real event.
func (s *Server) ensureGuildRow(ctx context.Context, gidInt int64, gid string, sess *Session) {
	if _, err := s.store.Guilds.Get(ctx, gidInt); err == nil {
		return
	}
	g := store.Guild{ID: gidInt}
	if sess != nil {
		for _, ug := range sess.Guilds {
			if ug.ID != gid {
				continue
			}
			g.Name = ug.Name
			g.Icon = ug.Icon
			break
		}
	}
	if err := s.store.Guilds.Upsert(ctx, g); err != nil {
		s.log.Warn("ensure guild row", "guild", gid, "err", err)
	}
}

func currentSession(c *gin.Context) *Session {
	if v, ok := c.Get(ctxSession); ok {
		if sess, ok := v.(*Session); ok {
			return sess
		}
	}
	return nil
}

func guildID(c *gin.Context) string {
	if v, ok := c.Get(ctxGuildID); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// canManage reports whether the session user can manage the given guild.
func canManage(sess *Session, gid string) bool {
	if sess == nil {
		return false
	}
	for _, g := range sess.Guilds {
		if g.ID != gid {
			continue
		}
		if g.Owner {
			return true
		}
		perms, _ := strconv.ParseInt(g.Permissions, 10, 64)
		return perms&manageMask != 0
	}
	return false
}

func (s *Server) setSessionCookie(c *gin.Context, token string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(s.cfg.API.SessionCookieName, token, int(sessionTTL.Seconds()), "/", "", s.cfg.IsProd(), true)
}

func (s *Server) clearSessionCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(s.cfg.API.SessionCookieName, "", -1, "/", "", s.cfg.IsProd(), true)
}

// originAllowed permits same-origin and the configured web origin (used for the
// WebSocket upgrade check).
func originAllowed(r *http.Request, webOrigin string) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	if o == webOrigin {
		return true
	}
	// Also allow the API's own origin (same-host dashboards behind a proxy).
	if u, err := url.Parse(o); err == nil {
		if strings.EqualFold(u.Host, r.Host) {
			return true
		}
	}
	return false
}
