package leveling

import (
	"strconv"

	"github.com/dia-bot/dia/internal/discord"
	"github.com/dia-bot/dia/internal/event"
)

// RankVariable documents one rank-card placeholder for the dashboard picker.
type RankVariable struct {
	Token string `json:"token"`
	Desc  string `json:"desc"`
}

// RankVariables is the single source of truth for the rank-card token set
// (used by Card Studio text/image layers and the dashboard variable picker).
var RankVariables = []RankVariable{
	{"{user}", "Member's display name"},
	{"{user.mention}", "Pings the member"},
	{"{user.name}", "Username"},
	{"{user.id}", "Member ID"},
	{"{user.avatar}", "Avatar image URL"},
	{"{level}", "Current level"},
	{"{rank}", "Leaderboard position"},
	{"{xp}", "Total XP"},
	{"{level.xp}", "XP into the current level"},
	{"{level.needed}", "XP needed to reach the next level"},
	{"{progress}", "Progress to next level, like 64%"},
}

// levelEventMap is the .Event.* map a durable level-up flow (the tail and the
// button click actions) is run with. It mirrors the runtime's decode of
// event.LevelUp (runtime.prepare's TypeLevelUp case) so a tail authored on the
// canvas sees the same .Event.level / .Event.xp / .Event.rank / .Event.new_level
// / .Event.channel_id vars as a hand-built level_up automation.
func levelEventMap(level, newLevel, rank int, xp int64, channelID string) map[string]any {
	return map[string]any{
		"level":      level,
		"new_level":  newLevel,
		"xp":         xp,
		"rank":       rank,
		"channel_id": channelID,
	}
}

// rankVars builds the placeholder→value map a rank card is rendered with.
func rankVars(user event.User, level, rank int, into, span, total int64) map[string]string {
	pct := 0
	if span > 0 {
		pct = int(float64(into) / float64(span) * 100)
	}
	return map[string]string{
		"{user}":         displayName(user),
		"{user.mention}": "<@" + user.ID + ">",
		"{user.name}":    user.Username,
		"{user.id}":      user.ID,
		"{user.avatar}":  discord.AvatarURL(user.ID, user.Avatar, 256),
		"{level}":        strconv.Itoa(level),
		"{rank}":         strconv.Itoa(rank),
		"{xp}":           formatInt(total),
		"{level.xp}":     formatInt(into),
		"{level.needed}": formatInt(span),
		"{progress}":     strconv.Itoa(pct) + "%",
	}
}
