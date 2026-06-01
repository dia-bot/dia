package api

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

const (
	ctxSession = "dia_session"
	ctxGuildID = "dia_guild_id"

	// Guild permissions that grant dashboard management access.
	manageMask = discordgo.PermissionAdministrator | discordgo.PermissionManageServer
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

// requireGuild authorizes that the session user manages the guild AND the bot is
// present in it.
func (s *Server) requireGuild() gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := currentSession(c)
		gid := c.Param("id")
		if !canManage(sess, gid) {
			fail(c, http.StatusForbidden, "you don't manage this server")
			return
		}
		idInt, ok := event.ParseID(gid)
		if !ok {
			fail(c, http.StatusBadRequest, "invalid guild id")
			return
		}
		if _, err := s.store.Guilds.Get(c.Request.Context(), idInt); err != nil {
			fail(c, http.StatusNotFound, "Dia is not in this server")
			return
		}
		c.Set(ctxGuildID, gid)
		c.Next()
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
