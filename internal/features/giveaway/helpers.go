package giveaway

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
)

// parseGiveawayDuration parses a compact duration like "30m", "2h", "3d", "1w"
// or a combination ("1d12h"). Result is clamped to [10s, 28d]. Shared by the
// dashboard, the scheduler and the custom-command step.
func parseGiveawayDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, errors.New("empty duration")
	}
	var total time.Duration
	num := ""
	sawUnit := false
	for _, r := range s {
		if r >= '0' && r <= '9' {
			num += string(r)
			continue
		}
		if num == "" {
			return 0, errors.New("malformed duration")
		}
		n, _ := strconv.Atoi(num)
		var unit time.Duration
		switch r {
		case 's':
			unit = time.Second
		case 'm':
			unit = time.Minute
		case 'h':
			unit = time.Hour
		case 'd':
			unit = 24 * time.Hour
		case 'w':
			unit = 7 * 24 * time.Hour
		default:
			return 0, errors.New("unknown unit")
		}
		total += time.Duration(n) * unit
		num, sawUnit = "", true
	}
	if num != "" {
		return 0, errors.New("number without a unit")
	}
	if !sawUnit || total <= 0 {
		return 0, errors.New("malformed duration")
	}
	if total < 10*time.Second {
		total = 10 * time.Second
	}
	if total > 28*24*time.Hour {
		total = 28 * 24 * time.Hour
	}
	return total, nil
}

// firstNonEmpty returns the first value that isn't blank after trimming.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// parseSnowflakeID extracts a Discord id from a bare snowflake or a channel/role
// mention wrapper ("<#123>", "<@&123>"), returning 0 when it isn't one.
func parseSnowflakeID(s string) int64 {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "<#")
	s = strings.TrimPrefix(s, "<@&")
	s = strings.TrimPrefix(s, "<@!")
	s = strings.TrimPrefix(s, "<@")
	s = strings.TrimSuffix(s, ">")
	id, _ := event.ParseID(strings.TrimSpace(s))
	return id
}
