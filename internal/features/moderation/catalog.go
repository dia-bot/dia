package moderation

import (
	"fmt"
	"regexp"
	"strings"
)

// Trigger* are the detection types a rule can use. They split into message
// triggers (screened on MESSAGE_CREATE / MESSAGE_UPDATE) and member triggers
// (screened on GUILD_MEMBER_ADD / GUILD_MEMBER_UPDATE).
const (
	TriggerWords       = "words"        // blocked words / phrases
	TriggerRegex       = "regex"        // custom regular expressions
	TriggerInvites     = "invites"      // Discord invite links
	TriggerLinks       = "links"        // URLs (all / allowlist / blocklist)
	TriggerSpam        = "spam"         // message flood (rate)
	TriggerDuplicates  = "duplicates"   // repeated identical messages (rate)
	TriggerMentions    = "mentions"     // too many user mentions
	TriggerMassMention = "mass_mention" // @everyone/@here and/or role pings
	TriggerCaps        = "caps"         // excessive capital letters
	TriggerEmojis      = "emojis"       // excessive emoji
	TriggerNewlines    = "newlines"     // excessive newlines / wall of text
	TriggerZalgo       = "zalgo"        // disruptive combining-mark text
	TriggerSpoilers    = "spoilers"     // excessive spoiler tags
	TriggerAttachments = "attachments"  // too many attachments
	TriggerAccountAge  = "account_age"  // new-account gate (on join)
	TriggerName        = "name"         // username / nickname filter
)

// Action* are the effects a rule can apply when it fires.
const (
	ActionDelete      = "delete"
	ActionWarn        = "warn"
	ActionTimeout     = "timeout"
	ActionKick        = "kick"
	ActionBan         = "ban"
	ActionAddRole     = "add_role"
	ActionRemoveRole  = "remove_role"
	ActionSendMessage = "send_message"
	ActionDM          = "dm"
	ActionAddPoints   = "add_points"
)

// messageTriggers are screened against message content/metadata.
var messageTriggers = map[string]bool{
	TriggerWords: true, TriggerRegex: true, TriggerInvites: true, TriggerLinks: true,
	TriggerSpam: true, TriggerDuplicates: true, TriggerMentions: true, TriggerMassMention: true,
	TriggerCaps: true, TriggerEmojis: true, TriggerNewlines: true, TriggerZalgo: true,
	TriggerSpoilers: true, TriggerAttachments: true,
}

// memberTriggers are screened against member identity / join.
var memberTriggers = map[string]bool{
	TriggerAccountAge: true, TriggerName: true,
}

// knownActions is the set of valid action types.
var knownActions = map[string]bool{
	ActionDelete: true, ActionWarn: true, ActionTimeout: true, ActionKick: true,
	ActionBan: true, ActionAddRole: true, ActionRemoveRole: true, ActionSendMessage: true,
	ActionDM: true, ActionAddPoints: true,
}

// IsMessageTrigger reports whether a trigger type screens message content.
func IsMessageTrigger(t string) bool { return messageTriggers[t] }

// IsMemberTrigger reports whether a trigger type screens member identity/join.
func IsMemberTrigger(t string) bool { return memberTriggers[t] }

// KnownTrigger reports whether t is a recognised trigger type.
func KnownTrigger(t string) bool { return messageTriggers[t] || memberTriggers[t] }

// triggerLabel is the human description used in audit reasons and logs.
func triggerLabel(t string) string {
	switch t {
	case TriggerWords:
		return "Blocked word"
	case TriggerRegex:
		return "Matched filter"
	case TriggerInvites:
		return "Invite link"
	case TriggerLinks:
		return "Link not allowed"
	case TriggerSpam:
		return "Spam"
	case TriggerDuplicates:
		return "Repeated messages"
	case TriggerMentions:
		return "Too many mentions"
	case TriggerMassMention:
		return "Mass mention"
	case TriggerCaps:
		return "Excessive caps"
	case TriggerEmojis:
		return "Excessive emoji"
	case TriggerNewlines:
		return "Excessive newlines"
	case TriggerZalgo:
		return "Disruptive text"
	case TriggerSpoilers:
		return "Excessive spoilers"
	case TriggerAttachments:
		return "Too many attachments"
	case TriggerAccountAge:
		return "New account"
	case TriggerName:
		return "Blocked name"
	default:
		return "Automod"
	}
}

// ValidateAutomod returns a list of human-readable problems with an automod
// configuration. An empty slice means the config is structurally sound. The API
// layer may surface these; the engine itself is defensive and simply skips rules
// it can't run.
func ValidateAutomod(cfg AutomodConfig) []string {
	var errs []string
	seen := map[string]bool{}
	for i, r := range cfg.Rules {
		where := fmt.Sprintf("rule %d (%q)", i+1, r.Name)
		if strings.TrimSpace(r.Name) == "" {
			errs = append(errs, where+": name is required")
		}
		if r.ID != "" {
			if seen[r.ID] {
				errs = append(errs, where+": duplicate rule id")
			}
			seen[r.ID] = true
		}
		if !KnownTrigger(r.Trigger.Type) {
			errs = append(errs, where+": unknown trigger type "+quote(r.Trigger.Type))
		}
		errs = append(errs, validateTrigger(where, r.Trigger)...)
		if len(r.Actions) == 0 {
			errs = append(errs, where+": needs at least one action")
		}
		for _, a := range r.Actions {
			if !knownActions[a.Type] {
				errs = append(errs, where+": unknown action type "+quote(a.Type))
				continue
			}
			if (a.Type == ActionAddRole || a.Type == ActionRemoveRole) && a.RoleID == "" {
				errs = append(errs, where+": "+a.Type+" needs a role")
			}
			if a.Type == ActionAddPoints && a.Points <= 0 {
				errs = append(errs, where+": add_points needs a positive point value")
			}
			if a.Type == ActionBan && (a.DeleteDays < 0 || a.DeleteDays > 7) {
				errs = append(errs, where+": ban delete_days must be 0-7")
			}
		}
	}
	for i, tier := range cfg.Escalation.Tiers {
		if tier.Points <= 0 {
			errs = append(errs, fmt.Sprintf("escalation tier %d: points must be positive", i+1))
		}
		switch tier.Action {
		case "timeout", "kick", "ban":
		default:
			errs = append(errs, fmt.Sprintf("escalation tier %d: action must be timeout, kick or ban", i+1))
		}
	}
	return errs
}

func validateTrigger(where string, t RuleTrigger) []string {
	var errs []string
	switch t.Type {
	case TriggerRegex, TriggerName:
		for _, p := range t.Patterns {
			if _, err := regexp.Compile(p); err != nil {
				errs = append(errs, where+": invalid regex "+quote(p)+": "+err.Error())
			}
		}
		if t.Type == TriggerName && len(t.Words) == 0 && len(t.Patterns) == 0 {
			errs = append(errs, where+": name filter needs words or patterns")
		}
	case TriggerWords:
		if len(t.Words) == 0 {
			errs = append(errs, where+": word filter needs at least one word")
		}
	case TriggerSpam, TriggerDuplicates:
		if t.Count <= 1 {
			errs = append(errs, where+": rate trigger needs a count of 2 or more")
		}
		if t.Window <= 0 {
			errs = append(errs, where+": rate trigger needs a positive window")
		}
	case TriggerMentions, TriggerEmojis, TriggerNewlines, TriggerSpoilers, TriggerAttachments, TriggerCaps, TriggerAccountAge:
		if t.Limit <= 0 {
			errs = append(errs, where+": "+t.Type+" needs a positive limit")
		}
	}
	return errs
}

// quote wraps a value in double quotes for an error message (avoids importing
// strconv just for Quote).
func quote(s string) string { return "\"" + s + "\"" }
