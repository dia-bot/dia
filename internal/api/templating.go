package api

import (
	"net/http"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/templating"
	"github.com/gin-gonic/gin"
)

type templatingPreviewReq struct {
	Template string `json:"template"`
	// ExtraVars overlays feature-specific {tokens} (e.g. rank {level}, {xp}).
	ExtraVars map[string]string `json:"extra_vars"`
}

// handleTemplatingPreview runs one template string through the engine with the
// admin's identity as sample data and returns the rendered text (or a template
// error). Actions and guild lookups are disabled — previews are pure & safe.
func (s *Server) handleTemplatingPreview(c *gin.Context) {
	var req templatingPreviewReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}

	sess := currentSession(c)
	uid, avatar, username, global := "", "", "ada", "Ada"
	if sess != nil {
		uid, avatar = sess.UserID, sess.Avatar
		username = firstNonEmpty(sess.Username, "ada")
		global = firstNonEmpty(sess.GlobalName, sess.Username, "Ada")
	}

	data := &templating.Context{
		User:  templating.User{ID: uid, Username: username, GlobalName: global, Avatar: discord.AvatarURL(uid, avatar, 256)},
		Guild: templating.Guild{Name: s.guildName(c), MemberCount: 1024},
	}
	tokens := map[string]string{
		"{user}":          global,
		"{user.mention}":  "@" + username,
		"{user.name}":     username,
		"{username}":      username,
		"{user.id}":       uid,
		"{user.avatar}":   discord.AvatarURL(uid, avatar, 256),
		"{server}":        s.guildName(c),
		"{count}":         "1024",
		"{count.ordinal}": "1,024th",
	}
	for k, v := range req.ExtraVars {
		tokens[k] = v
	}

	rendered, errMsg := templating.Preview(c.Request.Context(), req.Template, data, tokens)
	c.JSON(http.StatusOK, gin.H{"rendered": rendered, "error": errMsg})
}
