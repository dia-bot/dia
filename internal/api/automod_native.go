package api

import (
	"errors"
	"net/http"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/pkg/discordgo"
	"github.com/gin-gonic/gin"
)

// Native Discord AutoMod management. These wrap Discord's built-in AutoMod REST
// API so the dashboard panel can list, upsert and delete native rules. They all
// require the caller to have already passed the guild auth middleware, and the
// bot needs MANAGE_GUILD; a 403 from Discord surfaces as a clear error.

// supportedTriggerTypes are the trigger types the dashboard can manage. Discord
// also defines 5 (mention spam); the vendored library lacks a const but the
// wire value is an int, so we accept it explicitly here.
const triggerMentionSpam = 5

func supportedTrigger(t int) bool {
	switch discordgo.AutoModerationRuleTriggerType(t) {
	case discordgo.AutoModerationEventTriggerKeyword,
		discordgo.AutoModerationEventTriggerHarmfulLink,
		discordgo.AutoModerationEventTriggerSpam,
		discordgo.AutoModerationEventTriggerKeywordPreset:
		return true
	}
	return t == triggerMentionSpam
}

// nativeRuleJSON is the JSON-friendly shape the dashboard consumes. It is a thin
// pass-through of discordgo.AutoModerationRule with stable, explicit fields.
type nativeRuleJSON struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Enabled         bool               `json:"enabled"`
	EventType       int                `json:"event_type"`
	TriggerType     int                `json:"trigger_type"`
	Actions         []nativeActionJSON `json:"actions"`
	ExemptRoles     []string           `json:"exempt_roles"`
	ExemptChannels  []string           `json:"exempt_channels"`
	TriggerMetadata nativeTriggerMeta  `json:"trigger_metadata"`
}

type nativeActionJSON struct {
	Type          int    `json:"type"`
	ChannelID     string `json:"channel_id,omitempty"`
	Duration      int    `json:"duration_seconds,omitempty"`
	CustomMessage string `json:"custom_message,omitempty"`
}

type nativeTriggerMeta struct {
	KeywordFilter     []string `json:"keyword_filter"`
	RegexPatterns     []string `json:"regex_patterns"`
	Presets           []int    `json:"presets"`
	AllowList         []string `json:"allow_list"`
	MentionTotalLimit int      `json:"mention_total_limit"`
}

func toNativeRule(r *discordgo.AutoModerationRule) nativeRuleJSON {
	out := nativeRuleJSON{
		ID:             r.ID,
		Name:           r.Name,
		Enabled:        r.Enabled != nil && *r.Enabled,
		EventType:      int(r.EventType),
		TriggerType:    int(r.TriggerType),
		ExemptRoles:    []string{},
		ExemptChannels: []string{},
		Actions:        []nativeActionJSON{},
		TriggerMetadata: nativeTriggerMeta{
			KeywordFilter: []string{},
			RegexPatterns: []string{},
			Presets:       []int{},
			AllowList:     []string{},
		},
	}
	if r.ExemptRoles != nil {
		out.ExemptRoles = *r.ExemptRoles
	}
	if r.ExemptChannels != nil {
		out.ExemptChannels = *r.ExemptChannels
	}
	for _, a := range r.Actions {
		aj := nativeActionJSON{Type: int(a.Type)}
		if a.Metadata != nil {
			aj.ChannelID = a.Metadata.ChannelID
			aj.Duration = a.Metadata.Duration
			aj.CustomMessage = a.Metadata.CustomMessage
		}
		out.Actions = append(out.Actions, aj)
	}
	if m := r.TriggerMetadata; m != nil {
		if m.KeywordFilter != nil {
			out.TriggerMetadata.KeywordFilter = m.KeywordFilter
		}
		if m.RegexPatterns != nil {
			out.TriggerMetadata.RegexPatterns = m.RegexPatterns
		}
		for _, p := range m.Presets {
			out.TriggerMetadata.Presets = append(out.TriggerMetadata.Presets, int(p))
		}
		if m.AllowList != nil {
			out.TriggerMetadata.AllowList = *m.AllowList
		}
		out.TriggerMetadata.MentionTotalLimit = m.MentionTotalLimit
	}
	return out
}

// handleListAutoModRules returns the guild's native AutoMod rules.
func (s *Server) handleListAutoModRules(c *gin.Context) {
	rules, err := s.discord.AutoModRules(guildID(c))
	if err != nil {
		failDiscord(c, err, "could not list automod rules")
		return
	}
	out := make([]nativeRuleJSON, 0, len(rules))
	for _, r := range rules {
		out = append(out, toNativeRule(r))
	}
	c.JSON(http.StatusOK, gin.H{"rules": out})
}

// upsertRuleReq is the dashboard's rule body. ID is optional: present means edit.
type upsertRuleReq struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	Enabled         bool               `json:"enabled"`
	EventType       int                `json:"event_type"`
	TriggerType     int                `json:"trigger_type"`
	Actions         []nativeActionJSON `json:"actions"`
	ExemptRoles     []string           `json:"exempt_roles"`
	ExemptChannels  []string           `json:"exempt_channels"`
	TriggerMetadata nativeTriggerMeta  `json:"trigger_metadata"`
}

// handleUpsertAutoModRule creates (no id) or edits (id present) a native rule.
func (s *Server) handleUpsertAutoModRule(c *gin.Context) {
	var req upsertRuleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Name == "" {
		fail(c, http.StatusBadRequest, "name is required")
		return
	}
	if !supportedTrigger(req.TriggerType) {
		fail(c, http.StatusBadRequest, "unsupported trigger_type")
		return
	}

	meta := req.TriggerMetadata
	switch req.TriggerType {
	case int(discordgo.AutoModerationEventTriggerKeyword):
		if len(meta.KeywordFilter) == 0 && len(meta.RegexPatterns) == 0 {
			fail(c, http.StatusBadRequest, "keyword trigger needs keyword_filter or regex_patterns")
			return
		}
	case int(discordgo.AutoModerationEventTriggerKeywordPreset):
		if len(meta.Presets) == 0 {
			fail(c, http.StatusBadRequest, "keyword preset trigger needs at least one preset")
			return
		}
	case triggerMentionSpam:
		if meta.MentionTotalLimit <= 0 {
			fail(c, http.StatusBadRequest, "mention spam trigger needs mention_total_limit")
			return
		}
	}
	if len(req.Actions) == 0 {
		fail(c, http.StatusBadRequest, "at least one action is required")
		return
	}

	rule := buildRule(req)

	var (
		saved *discordgo.AutoModerationRule
		err   error
	)
	if req.ID != "" {
		saved, err = s.discord.AutoModRuleEdit(guildID(c), req.ID, rule)
	} else {
		saved, err = s.discord.AutoModRuleCreate(guildID(c), rule)
	}
	if err != nil {
		failDiscord(c, err, "could not save automod rule")
		return
	}

	gidInt, _ := event.ParseID(guildID(c))
	action := "automod_native.create"
	if req.ID != "" {
		action = "automod_native.update"
	}
	s.audit(c, gidInt, action, gin.H{"id": saved.ID, "name": saved.Name})
	c.JSON(http.StatusOK, gin.H{"rule": toNativeRule(saved)})
}

func buildRule(req upsertRuleReq) *discordgo.AutoModerationRule {
	enabled := req.Enabled
	eventType := discordgo.AutoModerationRuleEventType(req.EventType)
	if eventType == 0 {
		eventType = discordgo.AutoModerationEventMessageSend
	}
	rule := &discordgo.AutoModerationRule{
		Name:        req.Name,
		Enabled:     &enabled,
		EventType:   eventType,
		TriggerType: discordgo.AutoModerationRuleTriggerType(req.TriggerType),
	}

	exemptRoles := req.ExemptRoles
	if exemptRoles == nil {
		exemptRoles = []string{}
	}
	exemptChannels := req.ExemptChannels
	if exemptChannels == nil {
		exemptChannels = []string{}
	}
	rule.ExemptRoles = &exemptRoles
	rule.ExemptChannels = &exemptChannels

	// Trigger metadata is only meaningful for the keyword/preset/mention triggers;
	// for spam/harmful-link Discord ignores it, so we only attach what applies.
	m := &discordgo.AutoModerationTriggerMetadata{
		KeywordFilter:     req.TriggerMetadata.KeywordFilter,
		RegexPatterns:     req.TriggerMetadata.RegexPatterns,
		MentionTotalLimit: req.TriggerMetadata.MentionTotalLimit,
	}
	for _, p := range req.TriggerMetadata.Presets {
		m.Presets = append(m.Presets, discordgo.AutoModerationKeywordPreset(p))
	}
	if req.TriggerMetadata.AllowList != nil {
		al := req.TriggerMetadata.AllowList
		m.AllowList = &al
	}
	rule.TriggerMetadata = m

	for _, a := range req.Actions {
		act := discordgo.AutoModerationAction{Type: discordgo.AutoModerationActionType(a.Type)}
		if a.ChannelID != "" || a.Duration != 0 || a.CustomMessage != "" {
			act.Metadata = &discordgo.AutoModerationActionMetadata{
				ChannelID:     a.ChannelID,
				Duration:      a.Duration,
				CustomMessage: a.CustomMessage,
			}
		}
		rule.Actions = append(rule.Actions, act)
	}
	return rule
}

// handleDeleteAutoModRule removes a native rule by id.
func (s *Server) handleDeleteAutoModRule(c *gin.Context) {
	ruleID := c.Param("ruleId")
	if ruleID == "" {
		fail(c, http.StatusBadRequest, "invalid rule id")
		return
	}
	if err := s.discord.AutoModRuleDelete(guildID(c), ruleID, "deleted from dashboard"); err != nil {
		failDiscord(c, err, "could not delete automod rule")
		return
	}
	gidInt, _ := event.ParseID(guildID(c))
	s.audit(c, gidInt, "automod_native.delete", gin.H{"id": ruleID})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// failDiscord maps a Discord REST error to a dashboard-friendly response. A 403
// (missing MANAGE_GUILD) is called out clearly; other 4xx pass through as 400,
// and transport/5xx failures become a 502.
func failDiscord(c *gin.Context, err error, fallback string) {
	var rest *discordgo.RESTError
	if errors.As(err, &rest) && rest.Response != nil {
		code := rest.Response.StatusCode
		msg := fallback
		if rest.Message != nil && rest.Message.Message != "" {
			msg = rest.Message.Message
		}
		switch {
		case code == http.StatusForbidden:
			fail(c, http.StatusBadRequest, "Discord rejected the request: the bot needs the Manage Server permission")
		case code >= 400 && code < 500:
			fail(c, http.StatusBadRequest, msg)
		default:
			fail(c, http.StatusBadGateway, msg)
		}
		return
	}
	fail(c, http.StatusBadGateway, fallback)
}
