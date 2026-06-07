package api

import (
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/gin-gonic/gin"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// maxFontBytes caps a custom font upload. Real TTF/OTF files are well under this.
const maxFontBytes = 2 << 20

// handleListFonts returns a guild's custom fonts plus whether it's premium (so
// the dashboard can show/hide the upload control).
func (s *Server) handleListFonts(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	list, err := s.store.Uploads.List(c.Request.Context(), gidInt)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not load fonts")
		return
	}
	out := make([]gin.H, 0)
	for _, u := range list {
		if u.Kind == "font" {
			out = append(out, gin.H{"family": u.Family, "url": u.URL})
		}
	}
	c.JSON(http.StatusOK, gin.H{"fonts": out, "premium": s.isPremium(c.Request.Context(), guildID(c))})
}

// handleUploadFont accepts a premium guild's font file, validates it is a real
// TTF/OTF (the core security gate — arbitrary bytes are rejected), enforces the
// storage quota, stores it, and records family → URL.
func (s *Server) handleUploadFont(c *gin.Context) {
	gid := guildID(c)
	if !s.isPremium(c.Request.Context(), gid) {
		fail(c, http.StatusForbidden, "custom fonts are a Premium feature")
		return
	}
	if s.storage == nil {
		fail(c, http.StatusServiceUnavailable, "uploads are not configured on this server")
		return
	}
	gidInt, ok := event.ParseID(gid)
	if !ok {
		fail(c, http.StatusBadRequest, "invalid guild")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxFontBytes+512)
	file, _, err := c.Request.FormFile("file")
	if err != nil {
		fail(c, http.StatusBadRequest, "missing file field")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFontBytes+1))
	if err != nil {
		fail(c, http.StatusBadRequest, "could not read upload")
		return
	}
	if len(data) == 0 {
		fail(c, http.StatusBadRequest, "empty file")
		return
	}
	if len(data) > maxFontBytes {
		fail(c, http.StatusRequestEntityTooLarge, "font too large (max 2 MB)")
		return
	}

	// Security gate: only accept files that parse as a valid sfnt font with the
	// exact parser the renderer uses. This rejects images, scripts, archives, and
	// malformed/hostile input; we never execute the file, only re-serve the bytes.
	fontObj, err := opentype.Parse(data)
	if err != nil {
		fail(c, http.StatusUnsupportedMediaType, "not a valid TTF/OTF font file")
		return
	}
	family := fontFamilyName(fontObj)
	if family == "" {
		family = "Custom Font"
	}

	if !s.withinQuota(c, gidInt, int64(len(data))) {
		return // withinQuota writes the error
	}

	ext, ct := ".ttf", "font/ttf"
	if len(data) >= 4 && string(data[:4]) == "OTTO" {
		ext, ct = ".otf", "font/otf"
	}
	key := fmt.Sprintf("fonts/%s/%s%s", numericID(gid), randHex(16), ext)
	url, err := s.storage.Put(c.Request.Context(), key, ct, data)
	if err != nil {
		s.log.Error("font upload failed", "guild", gid, "err", err)
		fail(c, http.StatusBadGateway, "upload failed")
		return
	}
	oldKey, err := s.store.Uploads.UpsertFont(c.Request.Context(), store.GuildUpload{
		GuildID: gidInt, Family: family, ObjectKey: key, URL: url, Bytes: int64(len(data)),
	})
	if err != nil {
		s.log.Error("font save failed", "guild", gid, "err", err)
		fail(c, http.StatusInternalServerError, "could not save font")
		return
	}
	if oldKey != "" && oldKey != key {
		_ = s.storage.Delete(c.Request.Context(), oldKey) // free the replaced file
	}
	c.JSON(http.StatusOK, gin.H{"family": family, "url": url})
}

// handleDeleteFont removes a guild's custom font by family (and its stored file).
func (s *Server) handleDeleteFont(c *gin.Context) {
	gidInt, _ := event.ParseID(guildID(c))
	family := strings.TrimSpace(c.Param("family"))
	if family == "" {
		fail(c, http.StatusBadRequest, "missing font family")
		return
	}
	key, err := s.store.Uploads.DeleteFont(c.Request.Context(), gidInt, family)
	if err != nil {
		fail(c, http.StatusInternalServerError, "could not delete font")
		return
	}
	if key != "" && s.storage != nil {
		_ = s.storage.Delete(c.Request.Context(), key)
	}
	c.Status(http.StatusNoContent)
}

// fontFamilyName reads the font's declared family name (length-bounded).
func fontFamilyName(f *sfnt.Font) string {
	var buf sfnt.Buffer
	name, err := f.Name(&buf, sfnt.NameIDFamily)
	if err != nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if len(name) > 64 {
		name = strings.TrimSpace(name[:64])
	}
	return name
}
