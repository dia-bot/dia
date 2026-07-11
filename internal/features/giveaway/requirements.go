package giveaway

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
)

// discordEpochMS is Discord's snowflake epoch (2015-01-01) in milliseconds.
const discordEpochMS = 1420070400000

// entrant is the resolved state used to evaluate one member's eligibility. Ages
// are durations (memberAge < 0 = unknown join time); level is 0 when leveling is
// off or unqueried.
type entrant struct {
	roles      []string
	accountAge time.Duration
	memberAge  time.Duration
	level      int
}

// evaluateEntry checks a member against the requirements and returns their
// weighted ticket count. ok=false with a user-facing reason blocks entry. Bypass
// roles skip every gate (but still earn bonus entries).
func evaluateEntry(req RequirementConfig, e entrant) (ok bool, reason string, entries int) {
	entries = 1 + bonusEntries(req, e.roles)

	if hasAny(e.roles, req.BypassRoles) {
		return true, "", entries
	}
	if hasAny(e.roles, req.BlockedRoles) {
		return false, "You have a role that's blocked from this giveaway.", 0
	}
	if len(req.RequiredRoles) > 0 && !hasAny(e.roles, req.RequiredRoles) {
		return false, "You need " + roleList(req.RequiredRoles, " or ") + " to enter this giveaway.", 0
	}
	if req.MinAccountAgeDays > 0 && e.accountAge < daysDur(req.MinAccountAgeDays) {
		return false, fmt.Sprintf("Your account must be at least %d day(s) old to enter.", req.MinAccountAgeDays), 0
	}
	if req.MinMemberAgeDays > 0 && (e.memberAge < 0 || e.memberAge < daysDur(req.MinMemberAgeDays)) {
		return false, fmt.Sprintf("You must have been in the server for at least %d day(s) to enter.", req.MinMemberAgeDays), 0
	}
	if req.MinLevel > 0 && e.level < req.MinLevel {
		return false, fmt.Sprintf("You must be at least level %d to enter.", req.MinLevel), 0
	}
	return true, "", entries
}

// bonusEntries sums the extra tickets granted by every bonus role the member
// holds (additive, capped so a misconfiguration can't mint absurd weights).
func bonusEntries(req RequirementConfig, roles []string) int {
	total := 0
	for _, b := range req.BonusEntries {
		if b.Entries > 0 && contains(roles, b.RoleID) {
			total += b.Entries
		}
	}
	if total > 100 {
		total = 100
	}
	return total
}

// requiresLevelLookup reports whether evaluating this spec needs a leveling read.
func (r RequirementConfig) requiresLevelLookup() bool { return r.MinLevel > 0 }

// hasAny reports whether roles intersects any of want.
func hasAny(roles, want []string) bool {
	for _, w := range want {
		if contains(roles, w) {
			return true
		}
	}
	return false
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func daysDur(d int) time.Duration { return time.Duration(d) * 24 * time.Hour }

// accountCreated derives a Discord account's creation time from its snowflake id.
func accountCreated(userID string) (time.Time, bool) {
	id, ok := event.ParseID(userID)
	if !ok || id <= 0 {
		return time.Time{}, false
	}
	ms := (id >> 22) + discordEpochMS
	return time.UnixMilli(ms), true
}

// roleList renders role ids as mentions joined by sep (e.g. "<@&1> or <@&2>").
func roleList(ids []string, sep string) string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, "<@&"+id+">")
	}
	return strings.Join(out, sep)
}

// decodeRequirements parses a stored per-giveaway requirements blob.
func decodeRequirements(raw json.RawMessage) RequirementConfig {
	var r RequirementConfig
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &r)
	}
	return r
}

// decodeSpec parses a stored per-giveaway presentation Spec (the composed
// message + button + announce + behaviour).
func decodeSpec(raw json.RawMessage) Spec {
	var s Spec
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &s)
	}
	return s
}

// requirementSummary renders the requirements as a compact human list for the
// embed (empty string when there are none).
func requirementSummary(req RequirementConfig) string {
	var lines []string
	if len(req.RequiredRoles) > 0 {
		lines = append(lines, "• Must have "+roleList(req.RequiredRoles, " or "))
	}
	if len(req.BlockedRoles) > 0 {
		lines = append(lines, "• Must not have "+roleList(req.BlockedRoles, ", "))
	}
	if req.MinAccountAgeDays > 0 {
		lines = append(lines, "• Account age ≥ "+strconv.Itoa(req.MinAccountAgeDays)+" day(s)")
	}
	if req.MinMemberAgeDays > 0 {
		lines = append(lines, "• In server ≥ "+strconv.Itoa(req.MinMemberAgeDays)+" day(s)")
	}
	if req.MinLevel > 0 {
		lines = append(lines, "• Level ≥ "+strconv.Itoa(req.MinLevel))
	}
	for _, b := range req.BonusEntries {
		if b.Entries > 0 {
			lines = append(lines, "• <@&"+b.RoleID+"> gets +"+strconv.Itoa(b.Entries)+" entries")
		}
	}
	return strings.Join(lines, "\n")
}
