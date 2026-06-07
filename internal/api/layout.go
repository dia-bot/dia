package api

import (
	"net/http"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/layout"
	"github.com/gin-gonic/gin"
)

type layoutPreviewReq struct {
	Layout layout.Layout `json:"layout"`
	// Optional sample identity so {user.avatar} resolves to a real image in the
	// preview; defaults to the logged-in admin.
	UserID string `json:"user_id"`
	Avatar string `json:"avatar"`
	// ExtraVars overlays feature-specific tokens (e.g. rank card {level}, {xp},
	// {rank}, {progress}) onto the base sample vars.
	ExtraVars map[string]string `json:"extra_vars"`
}

// handleLayoutPreview renders a layout document to a PNG so the editor shows the
// exact image the bot would post. The renderer lives in internal/imaging.
func (s *Server) handleLayoutPreview(c *gin.Context) {
	var req layoutPreviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}

	sess := currentSession(c)
	uid := req.UserID
	avatar := req.Avatar
	if uid == "" && sess != nil {
		uid, avatar = sess.UserID, sess.Avatar
	}

	vars := map[string]string{
		"{user}":          firstNonEmpty(sess.GlobalName, sess.Username, "Ada"),
		"{user.mention}":  "@" + firstNonEmpty(sess.Username, "ada"),
		"{user.name}":     firstNonEmpty(sess.Username, "ada"),
		"{username}":      firstNonEmpty(sess.Username, "ada"),
		"{user.id}":       uid,
		"{user.avatar}":   discord.AvatarURL(uid, avatar, 256),
		"{server}":        s.guildName(c),
		"{count}":         "1024",
		"{count.ordinal}": "1,024th",
	}
	// Feature-specific tokens (rank card level/xp/rank/progress, etc.).
	for k, val := range req.ExtraVars {
		vars[k] = val
	}

	png, err := s.imaging.RenderLayout(c.Request.Context(), req.Layout, vars)
	if err != nil {
		fail(c, http.StatusInternalServerError, "render failed")
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
