package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"

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
	guildID := c.Param("id")

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

	key := fmt.Sprintf("uploads/%s/%s%s", numericID(guildID), randHex(16), ext)
	url, err := s.storage.Put(c.Request.Context(), key, ct, data)
	if err != nil {
		s.log.Error("upload failed", "guild", guildID, "err", err)
		fail(c, http.StatusBadGateway, "upload failed")
		return
	}
	c.JSON(http.StatusOK, gin.H{"url": url})
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
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
