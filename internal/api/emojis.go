package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// handleListEmojis returns the guild's custom emojis for the dashboard's
// emoji picker. Emojis aren't part of the cached guild snapshot, so this is
// a live REST read against Discord.
func (s *Server) handleListEmojis(c *gin.Context) {
	gid := guildID(c)
	emojis, err := s.discord.GuildEmojis(gid)
	if err != nil {
		fail(c, http.StatusBadGateway, "could not load server emojis")
		return
	}
	type emojiOut struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Animated bool   `json:"animated"`
	}
	out := make([]emojiOut, 0, len(emojis))
	for _, e := range emojis {
		if e == nil || e.ID == "" || !e.Available {
			continue
		}
		out = append(out, emojiOut{ID: e.ID, Name: e.Name, Animated: e.Animated})
	}
	c.JSON(http.StatusOK, gin.H{"emojis": out})
}
