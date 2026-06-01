package leveling

// XP curve helpers (MEE6-style).
//
// The XP required to advance from level L to L+1 is:
//
//	xpForLevel(L) = 5*L*L + 50*L + 100
//
// TotalXPForLevel(L) is the cumulative XP needed to *reach* level L (i.e. the
// sum of xpForLevel(0..L-1)). LevelFromXP returns the highest level whose
// cumulative requirement is <= the given total XP.

// xpForLevel returns the XP needed to go from level L to L+1.
func xpForLevel(level int) int64 {
	l := int64(level)
	return 5*l*l + 50*l + 100
}

// TotalXPForLevel returns the cumulative XP required to reach the given level.
// Level 0 requires 0 XP.
func TotalXPForLevel(level int) int64 {
	if level <= 0 {
		return 0
	}
	var total int64
	for l := 0; l < level; l++ {
		total += xpForLevel(l)
	}
	return total
}

// LevelFromXP returns the highest level whose cumulative requirement is <= xp.
func LevelFromXP(totalXP int64) int {
	if totalXP <= 0 {
		return 0
	}
	level := 0
	remaining := totalXP
	for {
		need := xpForLevel(level)
		if remaining < need {
			return level
		}
		remaining -= need
		level++
	}
}

// Progress returns how much XP has been earned into the current level
// (xpIntoLevel) and the total XP span of the current level (xpSpanOfLevel),
// suitable for driving a rank-card progress bar.
func Progress(totalXP int64) (xpIntoLevel, xpSpanOfLevel int64) {
	level := LevelFromXP(totalXP)
	base := TotalXPForLevel(level)
	span := xpForLevel(level)
	into := totalXP - base
	if into < 0 {
		into = 0
	}
	return into, span
}
