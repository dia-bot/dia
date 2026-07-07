package giveaway

import (
	"context"
	"encoding/json"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// finishGiveaway ends a still-running giveaway: it draws winners FIRST, then
// atomically claims the giveaway (running→ended) while recording the winners in
// the same UPDATE, and only then announces. Returns false when the claim lost a
// race (already ended/cancelled), so the caller must not report success. Drawing
// before the claim means a crash after the claim still leaves the correct winners
// stored — only the best-effort announcement is lost. Shared by the sweeper, the
// /giveaway end command, and the dashboard.
func (p *Plugin) finishGiveaway(ctx context.Context, cfg Config, g store.Giveaway) bool {
	winners, entryCount := p.drawFor(ctx, g, g.WinnerCount, p.baseExclude(cfg, g, nil))
	claimed, ok, err := p.deps.Store.Giveaways.ClaimEnd(ctx, g.ID, winners)
	if err != nil {
		p.deps.Log.Warn("giveaway: claim end", "giveaway", g.ID, "err", err)
		return false
	}
	if !ok {
		return false
	}
	p.announce(ctx, cfg, claimed, winners, entryCount, false)
	return true
}

// rerollGiveaway draws `count` replacement winners for an ended giveaway,
// excluding the current winners (and the host when ExcludeHost is set). A draw
// that yields nobody is a NO-OP: the existing winners, the public message and the
// stored record are left untouched (returns nil). Otherwise it records + announces
// the new winners and returns them.
func (p *Plugin) rerollGiveaway(ctx context.Context, cfg Config, g store.Giveaway, count int) []int64 {
	count = clampWinners(count, g.WinnerCount)
	winners, entryCount := p.drawFor(ctx, g, count, p.baseExclude(cfg, g, g.WinnerIDs))
	if len(winners) == 0 {
		return nil // no eligible replacements — keep the original winners + message
	}
	if err := p.deps.Store.Giveaways.SetWinners(ctx, g.ID, winners); err != nil {
		p.deps.Log.Warn("giveaway: set reroll winners", "giveaway", g.ID, "err", err)
	}
	p.announce(ctx, cfg, g, winners, entryCount, true)
	return winners
}

// drawFor lists the entries and draws up to `count` winners with `exclude`
// applied, returning the winners and the distinct entrant count.
func (p *Plugin) drawFor(ctx context.Context, g store.Giveaway, count int, exclude map[int64]bool) ([]int64, int) {
	entries, err := p.deps.Store.Giveaways.ListEntries(ctx, g.ID)
	if err != nil {
		p.deps.Log.Warn("giveaway: list entries for draw", "giveaway", g.ID, "err", err)
	}
	return drawWinners(entries, count, exclude), len(entries)
}

// baseExclude builds the set of ids the draw must skip: previously-drawn winners
// (reroll) and, when configured, the host.
func (p *Plugin) baseExclude(cfg Config, g store.Giveaway, prev []int64) map[int64]bool {
	ex := make(map[int64]bool, len(prev)+1)
	for _, id := range prev {
		ex[id] = true
	}
	if cfg.ExcludeHost && g.HostID != 0 {
		ex[g.HostID] = true
	}
	return ex
}

// announce switches the giveaway message to its ended state, posts the winner
// announcement, DMs the winners, and publishes GIVEAWAY_ENDED. The winners are
// already drawn + persisted by the caller.
func (p *Plugin) announce(ctx context.Context, cfg Config, g store.Giveaway, winners []int64, entryCount int, rerolled bool) {
	d := p.deps
	mentions := userMentions(winners)
	name, memberCount := p.guildInfo(ctx, g.GuildID)
	data := scopeData(g, entryCount, mentions, name, memberCount)

	// Switch the giveaway message to its ended state and drop the Enter button.
	if g.MessageID != 0 {
		em := buildEndedEmbed(ctx, cfg, g, mentions, entryCount, memberCount, name)
		embeds := []*discordgo.MessageEmbed{em}
		noComponents := []discordgo.MessageComponent{}
		if _, err := d.Discord.EditMessage(&discordgo.MessageEdit{
			Channel:    event.FormatID(g.ChannelID),
			ID:         event.FormatID(g.MessageID),
			Embeds:     &embeds,
			Components: &noComponents,
		}); err != nil {
			d.Log.Debug("giveaway: edit ended message", "giveaway", g.ID, "err", err)
		}
	}

	p.postAnnouncement(ctx, cfg, g, winners, data)

	if cfg.Announce.DMWinners && len(winners) > 0 {
		if dm := renderText(ctx, cfg.Announce.DMMessage, data); dm != "" {
			for _, w := range winners {
				if err := d.Discord.SendDM(event.FormatID(w), dm); err != nil {
					d.Log.Debug("giveaway: winner DM failed", "user", w, "err", err)
				}
			}
		}
	}

	p.publishEnded(ctx, g, winners, entryCount, rerolled)
}

// clampWinners bounds a requested winner count to a sane range, defaulting to
// fallback when unset. The upper cap protects drawWinners' allocation and Discord
// from an absurd value passed to /giveaway reroll.
func clampWinners(count, fallback int) int {
	if count <= 0 {
		count = fallback
	}
	if count <= 0 {
		count = 1
	}
	if count > 100 {
		count = 100
	}
	return count
}

// postAnnouncement posts the in-channel winner announcement (or the no-winners
// message), optionally pinging the winners and adding a "Jump to giveaway"
// button.
func (p *Plugin) postAnnouncement(ctx context.Context, cfg Config, g store.Giveaway, winners []int64, data map[string]any) {
	var content string
	if len(winners) == 0 {
		content = renderText(ctx, cfg.Announce.NoWinnersMessage, data)
	} else {
		content = renderText(ctx, cfg.Announce.Message, data)
	}
	if content == "" {
		return
	}
	send := &discordgo.MessageSend{Content: content}
	if comps := jumpComponents(cfg, g); comps != nil {
		send.Components = comps
	}
	if cfg.Announce.PingWinners && len(winners) > 0 {
		ids := make([]string, len(winners))
		for i, w := range winners {
			ids[i] = event.FormatID(w)
		}
		send.AllowedMentions = &discordgo.MessageAllowedMentions{Users: ids}
	} else {
		send.AllowedMentions = &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	}
	if _, err := p.deps.Discord.SendMessage(event.FormatID(g.ChannelID), send); err != nil {
		p.deps.Log.Warn("giveaway: announcement failed", "giveaway", g.ID, "err", err)
	}
}

// publishEnded emits the GIVEAWAY_ENDED event so Automations can react. The
// first winner is resolved (best-effort) into .User / .Member for the flow scope.
func (p *Plugin) publishEnded(ctx context.Context, g store.Giveaway, winners []int64, entryCount int, rerolled bool) {
	d := p.deps
	if d.Bus == nil {
		return
	}
	gidStr := event.FormatID(g.GuildID)
	winnerStrs := make([]string, len(winners))
	for i, w := range winners {
		winnerStrs[i] = event.FormatID(w)
	}
	payload := event.GiveawayEnded{
		GuildID:     gidStr,
		GiveawayID:  g.ID,
		ChannelID:   event.FormatID(g.ChannelID),
		Prize:       g.Prize,
		WinnerCount: len(winners),
		WinnerIDs:   winnerStrs,
		EntryCount:  entryCount,
		Rerolled:    rerolled,
	}
	if g.MessageID != 0 {
		payload.MessageID = event.FormatID(g.MessageID)
	}
	if g.HostID != 0 {
		payload.HostID = event.FormatID(g.HostID)
	}
	if len(winnerStrs) > 0 {
		payload.User = event.User{ID: winnerStrs[0]}
		if m, err := d.Discord.GuildMember(gidStr, winnerStrs[0]); err == nil && m != nil && m.User != nil {
			payload.User = event.User{
				ID: m.User.ID, Username: m.User.Username, GlobalName: m.User.GlobalName,
				Avatar: m.User.Avatar, Bot: m.User.Bot,
			}
			payload.Member = &event.Member{
				User: payload.User, Nick: m.Nick, Roles: m.Roles,
				JoinedAt: m.JoinedAt.Format(time.RFC3339),
			}
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	envBytes, err := json.Marshal(event.Envelope{
		Type:    event.TypeGiveawayEnded,
		GuildID: gidStr,
		TS:      time.Now().UnixMilli(),
		Data:    data,
	})
	if err != nil {
		return
	}
	subject := event.Subject(event.TypeGiveawayEnded, gidStr)
	if err := d.Bus.Publish(ctx, subject, envBytes, ""); err != nil {
		d.Log.Warn("giveaway: publish ended failed", "subject", subject, "err", err)
	}
}

// markCancelled edits a cancelled giveaway's message to a dimmed cancelled state
// and removes the Enter button. Best-effort.
func (p *Plugin) markCancelled(ctx context.Context, cfg Config, g store.Giveaway) {
	if g.MessageID == 0 {
		return
	}
	name, memberCount := p.guildInfo(ctx, g.GuildID)
	data := scopeData(g, 0, nil, name, memberCount)
	em := &discordgo.MessageEmbed{
		Title:       fallback(renderText(ctx, cfg.Embed.Title, data), g.Prize),
		Description: "🚫 This giveaway was cancelled.",
		Color:       giveawayColor(g, cfg),
	}
	embeds := []*discordgo.MessageEmbed{em}
	noComponents := []discordgo.MessageComponent{}
	if _, err := p.deps.Discord.EditMessage(&discordgo.MessageEdit{
		Channel:    event.FormatID(g.ChannelID),
		ID:         event.FormatID(g.MessageID),
		Embeds:     &embeds,
		Components: &noComponents,
	}); err != nil {
		p.deps.Log.Debug("giveaway: mark cancelled", "giveaway", g.ID, "err", err)
	}
}

// userMentions renders user ids as mentions ("<@id>").
func userMentions(ids []int64) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = "<@" + event.FormatID(id) + ">"
	}
	return out
}
