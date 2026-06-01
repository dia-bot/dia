package moderation

import (
	"strconv"
	"strings"
	"time"
)

// parseDuration parses a human duration string such as "30s", "10m", "2h" or
// "7d". time.ParseDuration understands ns..h but not days, so a trailing 'd' is
// handled here as a multiple of 24h.
func parseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if strings.HasSuffix(s, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err != nil {
			return 0, err
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	return time.ParseDuration(s)
}
