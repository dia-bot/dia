package tickets

import (
	"context"
	"fmt"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// autoCloseLoop sweeps for inactive tickets once a minute and warns / closes
// them per their category's auto-close settings. The worker owns no scheduler;
// it just polls, and every mutation is single-flight (MarkWarned / CloseTicket
// are conditional) so multiple worker replicas can't double-act.
func (p *Plugin) autoCloseLoop(ctx context.Context, d plugin.Deps) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.sweepAutoClose(ctx, d)
		}
	}
}

func (p *Plugin) sweepAutoClose(ctx context.Context, d plugin.Deps) {
	// A worker goroutine gets no panic recovery from the host; guard the sweep.
	defer func() {
		if r := recover(); r != nil {
			d.Log.Error("tickets: auto-close sweep panicked", "recover", r)
		}
	}()

	due, err := d.Store.Tickets.DueAutoClose(ctx, 25)
	if err != nil {
		d.Log.Warn("tickets: scan due auto-close", "err", err)
		return
	}
	for _, t := range due {
		// Stage 1: warn, if a warning grace is configured and none sent yet.
		if t.AutoWarnMinutes > 0 && t.CloseWarnedAt == nil {
			if ok, _ := d.Store.Tickets.MarkWarned(ctx, t.ID); ok && t.ChannelID != 0 {
				p.postInactivityWarning(ctx, d, t)
			}
			continue
		}
		// Stage 2: after a sent warning, wait out the grace before closing.
		if t.AutoWarnMinutes > 0 && t.CloseWarnedAt != nil {
			if time.Since(*t.CloseWarnedAt) < time.Duration(t.AutoWarnMinutes)*time.Minute {
				continue
			}
		}
		cfg, cat := p.resolveTicketConfig(ctx, d, t.GuildID, t)
		p.performClose(ctx, d, cfg, cat, t, event.User{}, 0, "Closed automatically due to inactivity.", "auto")
	}

	// Close requests whose auto-accept deadline passed close on behalf of the
	// requesting staff member. CloseTicket is conditional, so an opener clicking
	// Keep-open in the same instant wins at most once.
	reqs, err := d.Store.Tickets.DueCloseRequests(ctx, 25)
	if err != nil {
		d.Log.Warn("tickets: scan due close requests", "err", err)
		return
	}
	for _, t := range reqs {
		cfg, cat := p.resolveTicketConfig(ctx, d, t.GuildID, t)
		actor := event.User{ID: event.FormatID(t.CloseRequestedBy)}
		reason := t.CloseRequestReason
		if reason == "" {
			reason = "Close request accepted automatically."
		}
		p.performClose(ctx, d, cfg, cat, t, actor, t.CloseRequestedBy, reason, "close_request")
	}
}

// postInactivityWarning posts the category's composed auto-close warning (or
// the built-in line) in the ticket channel, pinging the opener.
func (p *Plugin) postInactivityWarning(ctx context.Context, d plugin.Deps, t store.Ticket) {
	opener := event.FormatID(t.OpenerID)
	send := &discordgo.MessageSend{
		AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}, Users: []string{opener}},
	}
	_, cat := p.resolveTicketConfig(ctx, d, t.GuildID, t)
	if !cat.AutoClose.WarnMessage.Empty() {
		tv := viewOf(t)
		sc := ticketScope(event.FormatID(t.GuildID), guildName(ctx, d, t.GuildID), openerUser(t), cat, &tv)
		send.Content, send.Embeds = renderSpec(cat.AutoClose.WarnMessage, sc, brandColor)
		if rows := renderSpecRows(cat.AutoClose.WarnMessage, sc, t.ID, nil); len(rows) > 0 {
			send.Components = rows
		}
	}
	if send.Content == "" && len(send.Embeds) == 0 {
		send.Content = fmt.Sprintf("<@%s> This ticket will close in %d minute(s) due to inactivity. Send a message to keep it open.",
			opener, t.AutoWarnMinutes)
	}
	_, _ = d.Discord.SendMessage(event.FormatID(t.ChannelID), send)
}
