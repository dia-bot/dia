package event

import "strconv"

// ParseID converts a Discord snowflake string to int64. Empty/invalid yields
// (0, false).
func ParseID(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// MustParseID is ParseID returning 0 on failure (convenient when the source is
// already known to be a valid snowflake).
func MustParseID(s string) int64 {
	n, _ := ParseID(s)
	return n
}

// FormatID converts a snowflake int64 back to its decimal-string wire form.
func FormatID(id int64) string { return strconv.FormatInt(id, 10) }
