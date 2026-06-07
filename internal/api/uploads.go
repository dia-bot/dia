package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/gin-gonic/gin"
)

// maxUploadBytes caps a single image upload. Cards are small; 8 MiB is generous.
const maxUploadBytes = 8 << 20

// allowedImageType maps a sniffed content type to the extension we store it as.
// The type is detected from the bytes, never trusted from the client.
var allowedImageType = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// handleUpload accepts a multipart "file" field, validates it's a real image
// within the size cap, stores it in the object store under a guild-scoped key,
// and returns its public URL. Uploads are rejected if storage isn't configured.
func (s *Server) handleUpload(c *gin.Context) {
	if s.storage == nil {
		fail(c, http.StatusServiceUnavailable, "uploads are not configured on this server")
		return
	}
	gid := guildID(c)
	gidInt, _ := event.ParseID(gid)

	// Bound the request body before touching it so a huge upload can't exhaust
	// memory; +1 lets us distinguish "exactly at limit" from "over".
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes+512)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		fail(c, http.StatusBadRequest, "could not read upload")
		return
	}
	if len(data) == 0 {
		fail(c, http.StatusBadRequest, "empty file")
		return
	}
	if len(data) > maxUploadBytes {
		fail(c, http.StatusRequestEntityTooLarge, "file too large (max 8 MB)")
		return
	}

	ct := http.DetectContentType(data)
	ext, ok := allowedImageType[ct]
	if !ok {
		fail(c, http.StatusUnsupportedMediaType, "only PNG, JPEG, WebP or GIF images are allowed")
		return
	}

	if !s.withinQuota(c, gidInt, int64(len(data))) {
		return // withinQuota writes the error
	}

	key := fmt.Sprintf("uploads/%s/%s%s", numericID(gid), randHex(16), ext)
	url, err := s.storage.Put(c.Request.Context(), key, ct, data)
	if err != nil {
		s.log.Error("upload failed", "guild", gid, "err", err)
		fail(c, http.StatusBadGateway, "upload failed")
		return
	}
	if _, err := s.store.Uploads.InsertImage(c.Request.Context(), gidInt, key, url, int64(len(data))); err != nil {
		// Don't leave an un-accounted (quota-invisible, undeletable) orphan: remove
		// the object we just stored and report failure rather than a false success.
		s.log.Error("record upload failed; rolling back object", "guild", gid, "err", err)
		_ = s.storage.Delete(c.Request.Context(), key)
		fail(c, http.StatusInternalServerError, "could not record upload")
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

// ── quota ─────────────────────────────────────────────────────────────────
const (
	freeQuotaBytes    = 500 << 20 // 500 MB
	premiumQuotaBytes = 5 << 30   // 5 GB
)

// storageQuota is the byte budget for a plan.
func storageQuota(premium bool) int64 {
	if premium {
		return premiumQuotaBytes
	}
	return freeQuotaBytes
}

// withinQuota reports whether the guild has room for addBytes more, writing the
// error and returning false otherwise. It fails closed on a usage-read error so
// a transient DB issue can't silently disable quota enforcement.
func (s *Server) withinQuota(c *gin.Context, gidInt int64, addBytes int64) bool {
	premium := s.isPremium(c.Request.Context(), guildID(c))
	used, err := s.store.Uploads.Usage(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusServiceUnavailable, "could not check storage usage; please retry")
		return false
	}
	quota := storageQuota(premium)
	if used+addBytes > quota {
		extra := " or upgrade to Premium"
		if premium {
			extra = ""
		}
		fail(c, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("storage full (%s of %s used) — delete assets%s", humanBytes(used), humanBytes(quota), extra))
		return false
	}
	return true
}

// humanBytes formats a byte count compactly (e.g. "1.5 GB").
func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		// crypto/rand failing is catastrophic; never emit a zero/predictable key.
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// numericID keeps only digits (guild ids are snowflakes) so a key can't be used
// to traverse or inject into the object path.
func numericID(id string) string {
	out := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, id)
	if out == "" {
		return "misc"
	}
	return out
}
