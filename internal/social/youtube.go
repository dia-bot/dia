package social

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// YouTube manages WebSub (PubSubHubbub) push subscriptions for channel upload
// feeds and, when an API key is present, enriches pushes via the Data API
// (live vs upload, titles, @handle resolution). The push itself is keyless.
type YouTube struct {
	apiKey   string
	callback string // <PUBLIC_WEBHOOK_BASE_URL>/webhooks/youtube
	secret   string // keys the per-channel hub.secret HMAC
}

// NewYouTube builds a YouTube client.
func NewYouTube(apiKey, callback, secret string) *YouTube {
	return &YouTube{apiKey: apiKey, callback: callback, secret: secret}
}

// HasAPIKey reports whether Data API enrichment is available.
func (y *YouTube) HasAPIKey() bool { return y.apiKey != "" }

// SecretFor derives the hub.secret for one channel's WebSub subscription; the
// hub then signs pushes with it (X-Hub-Signature), which VerifySignature checks.
func (y *YouTube) SecretFor(channelID string) string {
	mac := hmac.New(sha256.New, []byte(y.secret))
	mac.Write([]byte("websub:" + channelID))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks a WebSub push's X-Hub-Signature
// ("sha1=…"/"sha256=…") against the channel's derived secret. An absent
// header fails: every subscription is created with hub.secret set.
func (y *YouTube) VerifySignature(h http.Header, channelID string, body []byte) bool {
	sig := h.Get("X-Hub-Signature")
	algo, hexDigest, ok := strings.Cut(sig, "=")
	if !ok {
		return false
	}
	secret := []byte(y.SecretFor(channelID))
	var want []byte
	switch algo {
	case "sha256":
		mac := hmac.New(sha256.New, secret)
		mac.Write(body)
		want = mac.Sum(nil)
	case "sha1":
		mac := hmac.New(sha1.New, secret)
		mac.Write(body)
		want = mac.Sum(nil)
	default:
		return false
	}
	got, err := hex.DecodeString(hexDigest)
	if err != nil {
		return false
	}
	return hmac.Equal(want, got)
}

// TopicFor returns the WebSub topic URL for a channel's upload feed.
func TopicFor(channelID string) string {
	return "https://www.youtube.com/xml/feeds/videos.xml?channel_id=" + url.QueryEscape(channelID)
}

// Subscribe (re)subscribes the callback to a channel's upload feed at Google's
// hub. Subscriptions lease out after ~10 days, so callers renew periodically;
// re-subscribing is idempotent.
func (y *YouTube) Subscribe(ctx context.Context, channelID string) error {
	return y.hub(ctx, "subscribe", channelID)
}

// Unsubscribe removes the callback's subscription for a channel.
func (y *YouTube) Unsubscribe(ctx context.Context, channelID string) error {
	return y.hub(ctx, "unsubscribe", channelID)
}

func (y *YouTube) hub(ctx context.Context, mode, channelID string) error {
	form := url.Values{
		"hub.callback":      {y.callback + "?channel_id=" + url.QueryEscape(channelID)},
		"hub.topic":         {TopicFor(channelID)},
		"hub.mode":          {mode},
		"hub.verify":        {"async"},
		"hub.secret":        {y.SecretFor(channelID)},
		"hub.lease_seconds": {"828000"}, // hub max (~9.6 days)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://pubsubhubbub.appspot.com/subscribe", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("websub %s %s: status %d", mode, channelID, resp.StatusCode)
	}
	return nil
}

// YouTubeChannel is the resolved channel a subscription follows.
type YouTubeChannel struct {
	ID    string
	Title string
}

// ResolveChannel resolves user input — a UC… channel id or an @handle — to a
// channel. Handles need the Data API key; a bare channel id resolves without
// one (the title then falls back to the id until enriched).
func (y *YouTube) ResolveChannel(ctx context.Context, input string) (YouTubeChannel, error) {
	input = strings.TrimSpace(input)
	// Tolerate pasted channel URLs.
	if i := strings.Index(input, "/channel/"); i >= 0 {
		input = strings.Trim(input[i+len("/channel/"):], "/")
	}
	isID := strings.HasPrefix(input, "UC") && len(input) == 24
	if !y.HasAPIKey() {
		if isID {
			return YouTubeChannel{ID: input, Title: input}, nil
		}
		return YouTubeChannel{}, fmt.Errorf("resolving @handles needs YOUTUBE_API_KEY; paste the UC… channel id instead")
	}
	q := url.Values{"part": {"snippet"}, "key": {y.apiKey}}
	if isID {
		q.Set("id", input)
	} else {
		q.Set("forHandle", strings.TrimPrefix(input, "@"))
	}
	var out struct {
		Items []struct {
			ID      string `json:"id"`
			Snippet struct {
				Title string `json:"title"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := getJSON(ctx, "https://www.googleapis.com/youtube/v3/channels?"+q.Encode(), nil, &out); err != nil {
		return YouTubeChannel{}, err
	}
	if len(out.Items) == 0 {
		return YouTubeChannel{}, fmt.Errorf("youtube channel %q not found", input)
	}
	return YouTubeChannel{ID: out.Items[0].ID, Title: out.Items[0].Snippet.Title}, nil
}

// YouTubeVideo is the enrichment snapshot for one pushed video.
type YouTubeVideo struct {
	Live      bool // currently live (premiere or livestream)
	Upcoming  bool // scheduled, not yet live
	Title     string
	Thumbnail string
}

// VideoInfo classifies a pushed video via the Data API (1 quota unit). Without
// an API key it returns ok=false and callers treat the push as a plain upload.
func (y *YouTube) VideoInfo(ctx context.Context, videoID string) (YouTubeVideo, bool, error) {
	if !y.HasAPIKey() {
		return YouTubeVideo{}, false, nil
	}
	q := url.Values{"part": {"snippet"}, "id": {videoID}, "key": {y.apiKey}}
	var out struct {
		Items []struct {
			Snippet struct {
				Title                string `json:"title"`
				LiveBroadcastContent string `json:"liveBroadcastContent"` // live|upcoming|none
				Thumbnails           struct {
					High struct {
						URL string `json:"url"`
					} `json:"high"`
				} `json:"thumbnails"`
			} `json:"snippet"`
		} `json:"items"`
	}
	if err := getJSON(ctx, "https://www.googleapis.com/youtube/v3/videos?"+q.Encode(), nil, &out); err != nil {
		return YouTubeVideo{}, false, err
	}
	if len(out.Items) == 0 {
		return YouTubeVideo{}, false, nil
	}
	s := out.Items[0].Snippet
	return YouTubeVideo{
		Live:      s.LiveBroadcastContent == "live",
		Upcoming:  s.LiveBroadcastContent == "upcoming",
		Title:     s.Title,
		Thumbnail: s.Thumbnails.High.URL,
	}, true, nil
}

// RecentEntries fetches a channel's current upload feed (the WebSub topic
// itself), so a new subscription can prime its seen ledger — the hub replays
// recent entries on subscribe, which must not flood the channel with old
// videos.
func (y *YouTube) RecentEntries(ctx context.Context, channelID string) ([]FeedEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, TopicFor(channelID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("youtube feed %s: status %d", channelID, resp.StatusCode)
	}
	return ParseFeed(body)
}

// FeedEntry is one video entry in a WebSub push.
type FeedEntry struct {
	VideoID   string
	ChannelID string
	Title     string
	Link      string
	Author    string
}

// ParseFeed extracts the video entries from a WebSub Atom push. Deleted-entry
// notifications carry no <entry> elements and yield an empty slice.
func ParseFeed(body []byte) ([]FeedEntry, error) {
	var feed struct {
		Entries []struct {
			VideoID   string `xml:"http://www.youtube.com/xml/schemas/2015 videoId"`
			ChannelID string `xml:"http://www.youtube.com/xml/schemas/2015 channelId"`
			Title     string `xml:"title"`
			Link      struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
			Author struct {
				Name string `xml:"name"`
			} `xml:"author"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, err
	}
	out := make([]FeedEntry, 0, len(feed.Entries))
	for _, e := range feed.Entries {
		if e.VideoID == "" {
			continue
		}
		link := e.Link.Href
		if link == "" {
			link = "https://www.youtube.com/watch?v=" + e.VideoID
		}
		out = append(out, FeedEntry{
			VideoID: e.VideoID, ChannelID: e.ChannelID,
			Title: e.Title, Link: link, Author: e.Author.Name,
		})
	}
	return out, nil
}
