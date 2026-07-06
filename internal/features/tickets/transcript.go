package tickets

import (
	"bytes"
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"github.com/dia-bot/dia/internal/event"
	"github.com/dia-bot/dia/internal/plugin"
	"github.com/dia-bot/dia/internal/store"
	"github.com/dia-bot/dia/pkg/discordgo"
)

// transcriptMaxPages bounds transcript generation (100 messages/page).
const transcriptMaxPages = 10

// generateAndPostTranscript renders the ticket channel's history to a
// self-contained HTML file, posts it to the transcript/log channel (and DMs the
// opener when configured), and records its location on the ticket. Returns the
// posted file's URL (or "" when there was nowhere to post it).
func (p *Plugin) generateAndPostTranscript(ctx context.Context, d plugin.Deps, cfg Config, cat CategoryConfig, t store.Ticket, opener event.User) (string, error) {
	if t.ChannelID == 0 {
		return "", nil
	}
	msgs, err := p.fetchHistory(d, event.FormatID(t.ChannelID))
	if err != nil && len(msgs) == 0 {
		return "", err
	}
	gName := guildName(ctx, d, t.GuildID)
	htmlBytes := buildTranscriptHTML(t, gName, msgs)
	filename := fmt.Sprintf("ticket-%d.html", t.Number)

	dest := cfg.TranscriptChannel
	if dest == "" {
		dest = cfg.LogChannel
	}

	url := ""
	if dest != "" {
		content := fmt.Sprintf("Transcript for ticket #%d (%d messages).", t.Number, len(msgs))
		msg, perr := d.Discord.SendMessage(dest, &discordgo.MessageSend{
			Content:         content,
			Files:           []*discordgo.File{{Name: filename, ContentType: "text/html", Reader: bytes.NewReader(htmlBytes)}},
			AllowedMentions: &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}},
		})
		if perr == nil && msg != nil && len(msg.Attachments) > 0 {
			url = msg.Attachments[0].URL
		}
	}

	if cat.Transcript.DMOpener {
		_, _ = d.Discord.SendDMComplex(event.FormatID(t.OpenerID), &discordgo.MessageSend{
			Content: fmt.Sprintf("Here is the transcript for your ticket #%d in %s.", t.Number, gName),
			Files:   []*discordgo.File{{Name: filename, ContentType: "text/html", Reader: bytes.NewReader(htmlBytes)}},
		})
	}

	_ = d.Store.Tickets.SetTranscript(ctx, t.ID, url, len(msgs))
	return url, nil
}

// fetchHistory pages the channel's messages (oldest-first result), bounded by
// transcriptMaxPages.
func (p *Plugin) fetchHistory(d plugin.Deps, channelID string) ([]*discordgo.Message, error) {
	var all []*discordgo.Message
	before := ""
	for page := 0; page < transcriptMaxPages; page++ {
		batch, err := d.Discord.ChannelMessages(channelID, 100, before)
		if err != nil {
			return all, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		before = batch[len(batch)-1].ID // oldest in this (newest-first) batch
		if len(batch) < 100 {
			break
		}
	}
	// Reverse to chronological order.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

// buildTranscriptHTML renders a clean, standalone dark-theme transcript.
func buildTranscriptHTML(t store.Ticket, guildName string, msgs []*discordgo.Message) []byte {
	var b strings.Builder
	esc := html.EscapeString

	title := fmt.Sprintf("Ticket #%d transcript", t.Number)
	b.WriteString("<!doctype html><html lang=\"en\"><head><meta charset=\"utf-8\">")
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>" + esc(title) + "</title>")
	b.WriteString("<style>" + transcriptCSS + "</style></head><body>")

	b.WriteString("<header class=\"hdr\"><h1>" + esc(title) + "</h1><div class=\"meta\">")
	b.WriteString("<span>" + esc(guildName) + "</span>")
	if t.CategoryLabel != "" {
		b.WriteString("<span>" + esc(t.CategoryLabel) + "</span>")
	}
	b.WriteString("<span>Opener: " + esc(event.FormatID(t.OpenerID)) + "</span>")
	b.WriteString("<span>Opened: " + esc(t.OpenedAt.UTC().Format("2006-01-02 15:04 UTC")) + "</span>")
	if t.ClosedAt != nil {
		b.WriteString("<span>Closed: " + esc(t.ClosedAt.UTC().Format("2006-01-02 15:04 UTC")) + "</span>")
	}
	b.WriteString("<span>" + strconv.Itoa(len(msgs)) + " messages</span>")
	b.WriteString("</div></header><main>")

	if len(msgs) == 0 {
		b.WriteString("<p class=\"empty\">No messages were sent in this ticket.</p>")
	}
	for _, m := range msgs {
		author := "Unknown"
		if m.Author != nil {
			author = m.Author.Username
			if m.Author.GlobalName != "" {
				author = m.Author.GlobalName
			}
		}
		b.WriteString("<div class=\"msg\"><div class=\"row\"><span class=\"author\">" + esc(author) + "</span>")
		b.WriteString("<span class=\"ts\">" + esc(m.Timestamp.UTC().Format("2006-01-02 15:04")) + "</span></div>")
		if c := strings.TrimSpace(m.Content); c != "" {
			b.WriteString("<div class=\"content\">" + strings.ReplaceAll(esc(c), "\n", "<br>") + "</div>")
		}
		for _, em := range m.Embeds {
			if em == nil {
				continue
			}
			if em.Title != "" || em.Description != "" {
				b.WriteString("<div class=\"embed\">")
				if em.Title != "" {
					b.WriteString("<div class=\"etitle\">" + esc(em.Title) + "</div>")
				}
				if em.Description != "" {
					b.WriteString("<div class=\"edesc\">" + strings.ReplaceAll(esc(em.Description), "\n", "<br>") + "</div>")
				}
				b.WriteString("</div>")
			}
		}
		for _, a := range m.Attachments {
			if a == nil {
				continue
			}
			b.WriteString("<div class=\"att\"><a href=\"" + esc(a.URL) + "\">" + esc(a.Filename) + "</a></div>")
		}
		b.WriteString("</div>")
	}
	b.WriteString("</main><footer>Generated " + esc(time.Now().UTC().Format("2006-01-02 15:04 UTC")) + "</footer></body></html>")
	return []byte(b.String())
}

const transcriptCSS = `
:root{color-scheme:dark}
body{margin:0;background:#0a0a0c;color:#e8e8ea;font:14px/1.5 -apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif}
.hdr{padding:28px 32px;border-bottom:1px solid #26262b;background:#141417}
.hdr h1{margin:0 0 10px;font-size:20px}
.meta{display:flex;flex-wrap:wrap;gap:8px 18px;color:#9a9aa2;font-size:12px}
main{padding:20px 32px;max-width:900px}
.msg{padding:10px 0;border-bottom:1px solid #1c1c20}
.row{display:flex;align-items:baseline;gap:10px}
.author{font-weight:600;color:#ff8a8a}
.ts{color:#6a6a72;font-size:11px}
.content{margin-top:3px;white-space:pre-wrap;word-wrap:break-word}
.embed{margin-top:6px;padding:8px 12px;border-left:3px solid #ff6363;background:#141417;border-radius:4px}
.etitle{font-weight:600}
.edesc{color:#c4c4ca;margin-top:2px}
.att a{color:#8ab4ff}
.empty{color:#6a6a72}
footer{padding:18px 32px;color:#6a6a72;font-size:11px;border-top:1px solid #26262b}
`
