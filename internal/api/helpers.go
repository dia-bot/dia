package api

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
)

// audit records a dashboard mutation in the audit log (best-effort).
func (s *Server) audit(c *gin.Context, guildID int64, action string, detail gin.H) {
	sess := currentSession(c)
	var uid int64
	if sess != nil {
		uid, _ = event.ParseID(sess.UserID)
	}
	raw, _ := json.Marshal(detail)
	_ = s.store.Audit.Add(c.Request.Context(), store.AuditEntry{
		GuildID: guildID, UserID: uid, Action: action, Detail: raw,
	})
}

// newSampleVars builds a placeholder replacer for image previews.
func newSampleVars(username, server string, count int) *strings.Replacer {
	return strings.NewReplacer(
		"{user.mention}", "@"+username,
		"{username}", username,
		"{user}", username,
		"{server}", server,
		"{count}", strconv.Itoa(count),
	)
}
