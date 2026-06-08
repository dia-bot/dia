package api

import (
	"net/http"

	"github.com/dia-bot/dia/internal/features/leveling"
	"github.com/gin-gonic/gin"
)

// handleLevelingVariables returns the rank-card placeholder tokens for the
// dashboard variable picker (single source of truth: leveling.RankVariables).
func (s *Server) handleLevelingVariables(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"variables": leveling.RankVariables})
}
