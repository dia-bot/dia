package api

import (
	"context"
	"net/http"
	"time"

	"github.com/dia-bot/dia/internal/templating"
	"github.com/gin-gonic/gin"
)

type resolveReq struct {
	Strings   []string          `json:"strings"`
	ExtraVars map[string]string `json:"extra_vars"`
}

// handleResolveCard renders a batch of card template strings against the sample
// data — so the Card Studio's live canvas can show resolved {{.User.Username}} /
// {{.User.Avatar}} text exactly as the server would, including conditionals and
// functions (the browser can't run Go templates).
func (s *Server) handleResolveCard(c *gin.Context) {
	var req resolveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Strings) > 40 {
		req.Strings = req.Strings[:40] // bound work; cards are small
	}
	data := templating.DataFromVars(s.cardSampleVars(c, "", "", req.ExtraVars))
	eng := templating.New()
	// One shared deadline for the whole batch so the request can't be stretched to
	// (count × per-string timeout); each RenderCard is also internally capped.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	out := make([]string, len(req.Strings))
	for i, str := range req.Strings {
		if r, err := eng.RenderCard(ctx, str, data); err == nil {
			out[i] = r
		} else {
			out[i] = str // surface the raw template on error rather than blank
		}
	}
	c.JSON(http.StatusOK, gin.H{"resolved": out})
}
