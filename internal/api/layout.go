package api

import (
	"fmt"
	"net/http"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/layout"
	"github.com/dia-bot/dia/internal/templating"
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
	if len(req.Layout.Layers) > layout.MaxLayers {
		fail(c, http.StatusBadRequest, fmt.Sprintf("layout has too many layers (max %d)", layout.MaxLayers))
		return
	}

	vars := s.cardSampleVars(c, req.UserID, req.Avatar, req.ExtraVars)

	ctx := c.Request.Context()
	var fonts map[string]string
	if gid, ok := event.ParseID(guildID(c)); ok {
		fonts, _ = s.store.Uploads.FontMap(ctx, gid)
		// So getKV / getGuildKV resolve real stored values in the studio preview
		// (the sample user stands in for the member).
		memberID, _ := event.ParseID(vars["{user.id}"])
		ctx = templating.WithCardKV(ctx, s.store.FeatureKV.CardLookup(ctx, gid, memberID))
	}

	png, err := s.imaging.RenderLayout(ctx, req.Layout, vars, fonts)
	if err != nil {
		fail(c, http.StatusInternalServerError, "render failed")
		return
	}
	c.Data(http.StatusOK, "image/png", png)
}

// cardSampleVars builds the flat {token} sample map used to render card previews
// (the logged-in admin stands in for the joining member). extra overlays
// feature-specific tokens (rank-card level/xp/rank/progress). Shared by the
// layout preview and the studio's live resolve endpoint.
func (s *Server) cardSampleVars(c *gin.Context, userID, avatar string, extra map[string]string) map[string]string {
	sess := currentSession(c)
	uid := userID
	av := avatar
	if uid == "" && sess != nil {
		uid, av = sess.UserID, sess.Avatar
	}
	gName, uName := "Ada", "ada"
	if sess != nil {
		gName = firstNonEmpty(sess.GlobalName, sess.Username, "Ada")
		uName = firstNonEmpty(sess.Username, "ada")
	}
	vars := map[string]string{
		"{user}":          gName,
		"{user.mention}":  "@" + uName,
		"{user.name}":     uName,
		"{username}":      uName,
		"{user.id}":       uid,
		"{user.avatar}":   discord.AvatarURL(uid, av, 256),
		"{server}":        s.guildName(c),
		"{server.id}":     guildID(c),
		"{server.icon}":   s.guildIconURL(c),
		"{count}":         "1024",
		"{count.ordinal}": "1,024th",
	}
	for k, val := range extra {
		vars[k] = val
	}
	return vars
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
