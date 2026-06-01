package discord

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/dia-bot/dia/pkg/discordgo"
)

// AvatarURL returns a user's avatar CDN URL at the requested size (px), falling
// back to the correct default avatar when the user has none. For rendering we
// always request PNG even for animated (a_) avatars.
func AvatarURL(userID, avatarHash string, size int) string {
	if avatarHash == "" {
		return defaultAvatarURL(userID)
	}
	ext := "png"
	if strings.HasPrefix(avatarHash, "a_") {
		// Request the static PNG frame of an animated avatar.
		ext = "png"
	}
	return fmt.Sprintf("%savatars/%s/%s.%s?size=%d", discordgo.EndpointCDN, userID, avatarHash, ext, clampSize(size))
}

// defaultAvatarURL computes the embed default avatar. For migrated (pomelo)
// accounts the index is (snowflake >> 22) % 6.
func defaultAvatarURL(userID string) string {
	idx := 0
	if id, err := strconv.ParseUint(userID, 10, 64); err == nil {
		idx = int((id >> 22) % 6)
	}
	return fmt.Sprintf("%sembed/avatars/%d.png", discordgo.EndpointCDN, idx)
}

// GuildIconURL returns a guild icon URL, or "" if the guild has no icon.
func GuildIconURL(guildID, iconHash string, size int) string {
	if iconHash == "" {
		return ""
	}
	ext := "png"
	if strings.HasPrefix(iconHash, "a_") {
		ext = "gif"
	}
	return fmt.Sprintf("%sicons/%s/%s.%s?size=%d", discordgo.EndpointCDN, guildID, iconHash, ext, clampSize(size))
}

func clampSize(size int) int {
	if size <= 0 {
		return 256
	}
	// Discord allows power-of-two sizes 16..4096; round down to a sane value.
	switch {
	case size > 4096:
		return 4096
	case size < 16:
		return 16
	default:
		return size
	}
}
