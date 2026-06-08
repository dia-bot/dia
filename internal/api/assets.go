package api

import (
	"net/http"
	"strconv"

	"github.com/dia-bot/dia/internal/event"
	"github.com/gin-gonic/gin"
)

// handleListAssets returns a guild's uploaded assets (images + fonts) with total
// usage and the plan quota — the storage overview.
func (s *Server) handleListAssets(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	list, err := s.store.Uploads.List(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load assets")
		return
	}
	var used int64
	items := make([]gin.H, 0, len(list))
	for _, u := range list {
		used += u.Bytes
		items = append(items, gin.H{
			"id": u.ID, "kind": u.Kind, "family": u.Family,
			"url": u.URL, "bytes": u.Bytes, "created_at": u.CreatedAt.Unix(),
		})
	}
	premium := s.isPremium(c.Request.Context(), guildID(c))
	c.JSON(http.StatusOK, gin.H{
		"assets":  items,
		"used":    used,
		"quota":   storageQuota(premium),
		"premium": premium,
	})
}

// handleDeleteAsset removes one uploaded asset (row + stored file).
func (s *Server) handleDeleteAsset(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	id, err := strconv.ParseInt(c.Param("aid"), 10, 64)
	if err != nil {
		fail(c, http.StatusBadRequest, "invalid asset id")
		return
	}
	u, found, err := s.store.Uploads.Get(c.Request.Context(), gidInt, id)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load asset")
		return
	}
	if !found {
		c.Status(http.StatusNoContent)
		return
	}
	if err := s.store.Uploads.Delete(c.Request.Context(), gidInt, id); err != nil {
		fail(c, http.StatusInternalServerError, "could not delete asset")
		return
	}
	if u.ObjectKey != "" && s.storage != nil {
		_ = s.storage.Delete(c.Request.Context(), u.ObjectKey)
	}
	c.Status(http.StatusNoContent)
}
