package moderation

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/dia-bot/dia/internal/cache"
	"github.com/dia-bot/dia/internal/event"
)

// scanInput is the pure, Discord-free snapshot a detector needs. The engine
// builds one per evaluated message / member so detection stays unit-testable
// (no REST calls, no plugin.Deps reach-through except the optional rate cache).
type scanInput struct {
	GuildID   string
	UserID    string
	Username  string
	Nick      string
	Content   string
	Mentions  []event.User
	Everyone  bool     // MentionEveryone
	RolePings []string // MentionRoles
	Attach    int      // AttachmentCount

	// Cache is the rate-limit backend for spam/duplicates. nil => those
	// triggers are skipped gracefully.
	Cache *cache.Store
	Ctx   context.Context
}

// detect runs a single trigger against the input and returns (reason, hit). It
// never touches Discord and never mutates state apart from the rate counters in
// Redis (spam/duplicates), which are inherently side-effecting fixed windows.
func detect(in scanInput, t RuleTrigger) (string, bool) {
	switch t.Type {
	case TriggerWords:
		return detectWords(t, in.Content)
	case TriggerRegex:
		return detectRegex(t, in.Content)
	case TriggerInvites:
		return detectInvites(t, in.Content)
	case TriggerLinks:
		return detectLinks(t, in.Content)
	case TriggerScamLinks:
		return detectScamLinks(in, t)
	case TriggerMentions:
		return detectMentions(t, in)
	case TriggerMassMention:
		return detectMassMention(t, in)
	case TriggerCaps:
		return detectCaps(t, in.Content)
	case TriggerEmojis:
		return detectEmojis(t, in.Content)
	case TriggerNewlines:
		return detectNewlines(t, in.Content)
	case TriggerZalgo:
		return detectZalgo(t, in.Content)
	case TriggerSpoilers:
		return detectSpoilers(t, in.Content)
	case TriggerAttachments:
		return detectAttachments(t, in.Attach)
	case TriggerSpam:
		return detectSpam(in, t)
	case TriggerDuplicates:
		return detectDuplicates(in, t)
	case TriggerAccountAge:
		return detectAccountAge(t, in.UserID)
	case TriggerName:
		return detectName(t, in)
	default:
		return "", false
	}
}

// ── Word / regex matching ────────────────────────────────────

var wildcardSpecial = regexp.MustCompile(`[.+^${}()|\[\]\\]`)

// matchWords reports the first word that matches text under the given mode,
// honouring the allow-list. modes: substring | word | wildcard.
func matchWords(words, allow []string, mode, text string) (string, bool) {
	lc := strings.ToLower(text)
	allowSet := lowerSet(allow)
	for _, raw := range words {
		w := strings.TrimSpace(raw)
		if w == "" {
			continue
		}
		lw := strings.ToLower(w)
		if allowSet[lw] {
			continue
		}
		switch mode {
		case "word":
			if matchWholeWord(lc, lw) {
				if allowedAround(lc, lw, allowSet) {
					continue
				}
				return w, true
			}
		case "wildcard":
			if matchWildcard(lc, lw) {
				return w, true
			}
		default: // substring
			if strings.Contains(lc, lw) {
				return w, true
			}
		}
	}
	return "", false
}

// matchWholeWord reports whether needle appears in text bounded by non-letter
// (and non-digit) runes, so "ass" does not trip on "class".
func matchWholeWord(text, needle string) bool {
	if needle == "" {
		return false
	}
	from := 0
	for {
		i := strings.Index(text[from:], needle)
		if i < 0 {
			return false
		}
		start := from + i
		end := start + len(needle)
		beforeOK := start == 0 || !isWordRune(rune(text[start-1]))
		afterOK := end >= len(text) || !isWordRune(rune(text[end]))
		if beforeOK && afterOK {
			return true
		}
		from = start + 1
	}
}

// allowedAround returns false; whole-word allow-list exemptions are handled by
// the simple set test above. Kept as a hook for clarity/future tuning.
func allowedAround(string, string, map[string]bool) bool { return false }

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

// matchWildcard supports '*' (any run) and '?' (one char) glob patterns,
// anchored to the whole message? No: it scans for the pattern anywhere, so
// "*bad*" and "bad" both work. We compile to an unanchored RE2.
func matchWildcard(text, pattern string) bool {
	var b strings.Builder
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(wildcardSpecial.ReplaceAllString(string(r), `\$0`))
		}
	}
	re, err := regexp.Compile(b.String())
	if err != nil {
		return strings.Contains(text, strings.ReplaceAll(strings.ReplaceAll(pattern, "*", ""), "?", ""))
	}
	return re.MatchString(text)
}

func detectWords(t RuleTrigger, content string) (string, bool) {
	if w, ok := matchWords(t.Words, t.AllowList, t.MatchMode, content); ok {
		return "Blocked word: " + w, true
	}
	return "", false
}

func detectRegex(t RuleTrigger, content string) (string, bool) {
	allow := compileAll(t.AllowList)
	for _, pat := range t.Patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		loc := re.FindStringIndex(content)
		if loc == nil {
			continue
		}
		hit := content[loc[0]:loc[1]]
		if matchedByAny(allow, hit) {
			continue
		}
		return "Matched filter: " + truncate(pat, 60), true
	}
	return "", false
}

func compileAll(pats []string) []*regexp.Regexp {
	var out []*regexp.Regexp
	for _, p := range pats {
		if p = strings.TrimSpace(p); p == "" {
			continue
		}
		if re, err := regexp.Compile(p); err == nil {
			out = append(out, re)
		}
	}
	return out
}

func matchedByAny(res []*regexp.Regexp, s string) bool {
	for _, re := range res {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

// ── Invites & links ──────────────────────────────────────────

var (
	inviteRe = regexp.MustCompile(`(?i)(discord\.gg/|discord(?:app)?\.com/invite/)(\S+)`)
	urlRe    = regexp.MustCompile(`(?i)\bhttps?://([^\s/]+)`)
	bareHost = regexp.MustCompile(`(?i)\b([a-z0-9-]+(?:\.[a-z0-9-]+)+\.[a-z]{2,})\b`)
)

func detectInvites(t RuleTrigger, content string) (string, bool) {
	allow := lowerSet(t.AllowList)
	for _, m := range inviteRe.FindAllStringSubmatch(content, -1) {
		code := strings.Trim(strings.ToLower(m[2]), "/")
		if i := strings.IndexAny(code, "?#"); i >= 0 {
			code = code[:i]
		}
		if allow[code] {
			continue
		}
		return "Invite link: " + code, true
	}
	return "", false
}

func detectLinks(t RuleTrigger, content string) (string, bool) {
	hosts := extractHosts(content)
	if len(hosts) == 0 {
		return "", false
	}
	switch t.LinkMode {
	case "allowlist":
		allow := domainSet(t.AllowList)
		for _, h := range hosts {
			if !hostInSet(h, allow) {
				return "Link not allowed: " + h, true
			}
		}
		return "", false
	case "blocklist":
		block := domainSet(t.Domains)
		for _, h := range hosts {
			if hostInSet(h, block) {
				return "Blocked link: " + h, true
			}
		}
		return "", false
	default: // all
		return "Link not allowed: " + hosts[0], true
	}
}

// detectScamLinks flags a message when any host it links to is on the
// package-level phishing/scam blocklist (threatfeed.go). The trigger's AllowList
// exempts trusted hosts (useful for false positives in the feed).
func detectScamLinks(in scanInput, t RuleTrigger) (string, bool) {
	allow := domainSet(t.AllowList)
	for _, h := range extractHosts(in.Content) {
		if hostInSet(h, allow) {
			continue
		}
		if blocklist.has(h) {
			return "Scam/phishing link: " + h, true
		}
	}
	return "", false
}

// extractHosts pulls hostnames from explicit http(s) URLs and, failing those,
// bare domains. Deduplicated and lower-cased.
func extractHosts(content string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(h string) {
		h = strings.ToLower(strings.TrimSuffix(h, "."))
		if h == "" || seen[h] {
			return
		}
		seen[h] = true
		out = append(out, h)
	}
	for _, m := range urlRe.FindAllStringSubmatch(content, -1) {
		add(m[1])
	}
	if len(out) == 0 {
		for _, m := range bareHost.FindAllStringSubmatch(content, -1) {
			add(m[1])
		}
	}
	return out
}

func domainSet(domains []string) map[string]bool {
	out := map[string]bool{}
	for _, d := range domains {
		d = strings.ToLower(strings.TrimSpace(d))
		d = strings.TrimPrefix(d, "*.")
		if d != "" {
			out[d] = true
		}
	}
	return out
}

// hostInSet matches a host against a domain set, treating each set entry as a
// suffix (so "discord.com" matches "cdn.discord.com").
func hostInSet(host string, set map[string]bool) bool {
	if set[host] {
		return true
	}
	for d := range set {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// ── Mentions ─────────────────────────────────────────────────

func detectMentions(t RuleTrigger, in scanInput) (string, bool) {
	if t.Limit <= 0 {
		return "", false
	}
	n := len(in.Mentions)
	if n > t.Limit {
		return fmt.Sprintf("Too many mentions (%d > %d)", n, t.Limit), true
	}
	return "", false
}

var hereRe = regexp.MustCompile(`(?i)@here`)

func detectMassMention(t RuleTrigger, in scanInput) (string, bool) {
	// Limit is the number ALLOWED before tripping (0 => trip on the first one),
	// matching the dashboard catalogue; trip when the count exceeds it.
	limit := t.Limit
	if limit < 0 {
		limit = 0
	}
	if t.Everyone {
		count := 0
		if in.Everyone {
			count++
		}
		count += len(hereRe.FindAllString(in.Content, -1))
		if count > limit {
			return fmt.Sprintf("Mass @everyone/@here (%d)", count), true
		}
	}
	if t.Roles {
		if len(in.RolePings) > limit {
			return fmt.Sprintf("Mass role mention (%d)", len(in.RolePings)), true
		}
	}
	return "", false
}

// ── Caps / emojis / newlines / zalgo / spoilers / attachments ─

func detectCaps(t RuleTrigger, content string) (string, bool) {
	if t.Limit <= 0 {
		return "", false
	}
	var letters, upper int
	for _, r := range content {
		if unicode.IsLetter(r) {
			letters++
			if unicode.IsUpper(r) {
				upper++
			}
		}
	}
	if letters == 0 {
		return "", false
	}
	if t.MinLength > 0 && letters < t.MinLength {
		return "", false
	}
	pct := upper * 100 / letters
	if pct > t.Limit {
		return fmt.Sprintf("Excessive caps (%d%%)", pct), true
	}
	return "", false
}

var customEmojiRe = regexp.MustCompile(`<a?:[A-Za-z0-9_]+:\d+>`)

func detectEmojis(t RuleTrigger, content string) (string, bool) {
	if t.Limit <= 0 {
		return "", false
	}
	count := len(customEmojiRe.FindAllString(content, -1))
	stripped := customEmojiRe.ReplaceAllString(content, "")
	for _, r := range stripped {
		if isEmojiRune(r) {
			count++
		}
	}
	if count > t.Limit {
		return fmt.Sprintf("Excessive emoji (%d > %d)", count, t.Limit), true
	}
	return "", false
}

// isEmojiRune is a pragmatic emoji test: the common pictographic and symbol
// blocks plus regional indicators. It does not aim for full grapheme-cluster
// correctness (a combined emoji may count as a few), which is fine for a "too
// many emoji" heuristic.
func isEmojiRune(r rune) bool {
	switch {
	case r >= 0x1F300 && r <= 0x1FAFF: // pictographs, symbols, supplemental
		return true
	case r >= 0x1F1E6 && r <= 0x1F1FF: // regional indicators
		return true
	case r >= 0x2600 && r <= 0x27BF: // misc symbols + dingbats
		return true
	case r >= 0x2190 && r <= 0x21FF: // arrows
		return true
	case r == 0x2B50 || r == 0x2B55: // star, circle
		return true
	}
	return false
}

func detectNewlines(t RuleTrigger, content string) (string, bool) {
	if t.Limit <= 0 {
		return "", false
	}
	n := strings.Count(content, "\n")
	if n > t.Limit {
		return fmt.Sprintf("Excessive newlines (%d > %d)", n, t.Limit), true
	}
	return "", false
}

func detectZalgo(t RuleTrigger, content string) (string, bool) {
	limit := t.Limit
	if limit <= 0 {
		limit = 50
	}
	var total, marks int
	for _, r := range content {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.Is(unicode.Mn, r) {
			marks++
		}
	}
	if total == 0 {
		return "", false
	}
	pct := marks * 100 / total
	if pct > limit {
		return fmt.Sprintf("Disruptive text (%d%% combining marks)", pct), true
	}
	return "", false
}

var spoilerRe = regexp.MustCompile(`\|\|[^|]*\|\|`)

func detectSpoilers(t RuleTrigger, content string) (string, bool) {
	if t.Limit <= 0 {
		return "", false
	}
	n := len(spoilerRe.FindAllString(content, -1))
	if n > t.Limit {
		return fmt.Sprintf("Excessive spoilers (%d > %d)", n, t.Limit), true
	}
	return "", false
}

func detectAttachments(t RuleTrigger, count int) (string, bool) {
	if t.Limit <= 0 {
		return "", false
	}
	if count > t.Limit {
		return fmt.Sprintf("Too many attachments (%d > %d)", count, t.Limit), true
	}
	return "", false
}

// ── Rate-based: spam & duplicates ────────────────────────────

func detectSpam(in scanInput, t RuleTrigger) (string, bool) {
	if in.Cache == nil || in.Ctx == nil || t.Count <= 0 || t.Window <= 0 {
		return "", false
	}
	key := fmt.Sprintf("automod:spam:%s:%s:%d", in.GuildID, in.UserID, t.Window)
	n, err := in.Cache.Incr(in.Ctx, key, time.Duration(t.Window)*time.Second)
	if err != nil {
		return "", false
	}
	if int(n) > t.Count {
		return fmt.Sprintf("Spam (%d msgs / %ds)", n, t.Window), true
	}
	return "", false
}

func detectDuplicates(in scanInput, t RuleTrigger) (string, bool) {
	if in.Cache == nil || in.Ctx == nil || t.Count <= 0 || t.Window <= 0 {
		return "", false
	}
	body := strings.TrimSpace(strings.ToLower(in.Content))
	if body == "" {
		return "", false
	}
	sum := sha1.Sum([]byte(body))
	hash := hex.EncodeToString(sum[:8])
	key := fmt.Sprintf("automod:dup:%s:%s:%d:%s", in.GuildID, in.UserID, t.Window, hash)
	n, err := in.Cache.Incr(in.Ctx, key, time.Duration(t.Window)*time.Second)
	if err != nil {
		return "", false
	}
	if int(n) >= t.Count {
		return fmt.Sprintf("Repeated message (%dx / %ds)", n, t.Window), true
	}
	return "", false
}

// ── Member triggers: account age & name ──────────────────────

// discordEpoch is the Discord snowflake epoch in unix milliseconds.
const discordEpoch = 1420070400000

// accountCreated derives an account's creation time from its snowflake ID.
func accountCreated(userID string) (time.Time, bool) {
	id, ok := event.ParseID(userID)
	if !ok || id <= 0 {
		return time.Time{}, false
	}
	ms := (id >> 22) + discordEpoch
	return time.UnixMilli(ms), true
}

func detectAccountAge(t RuleTrigger, userID string) (string, bool) {
	if t.Limit <= 0 {
		return "", false
	}
	created, ok := accountCreated(userID)
	if !ok {
		return "", false
	}
	age := time.Since(created)
	if age < time.Duration(t.Limit)*time.Hour {
		hrs := int(age.Hours())
		return fmt.Sprintf("New account (%dh old, min %dh)", hrs, t.Limit), true
	}
	return "", false
}

func detectName(t RuleTrigger, in scanInput) (string, bool) {
	var targets []string
	switch t.Scan {
	case "username":
		targets = []string{in.Username}
	case "nick":
		targets = []string{in.Nick}
	default: // both
		targets = []string{in.Username, in.Nick}
	}
	allow := compileAll(t.AllowList)
	for _, name := range targets {
		if strings.TrimSpace(name) == "" {
			continue
		}
		if w, ok := matchWords(t.Words, t.AllowList, t.MatchMode, name); ok {
			return "Blocked name: " + w, true
		}
		for _, pat := range t.Patterns {
			re, err := regexp.Compile(pat)
			if err != nil {
				continue
			}
			if loc := re.FindStringIndex(name); loc != nil {
				if matchedByAny(allow, name[loc[0]:loc[1]]) {
					continue
				}
				return "Blocked name: " + truncate(name, 40), true
			}
		}
	}
	return "", false
}

// ── small helpers ────────────────────────────────────────────

func lowerSet(items []string) map[string]bool {
	out := make(map[string]bool, len(items))
	for _, s := range items {
		s = strings.ToLower(strings.TrimSpace(s))
		if s != "" {
			out[s] = true
		}
	}
	return out
}
