package social

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// RSS fetches and parses RSS 2.0 / Atom feeds with conditional-GET support,
// covering blogs, Reddit (…/.rss), Mastodon, podcasts and anything else that
// publishes a feed.
type RSS struct{}

// FeedItem is one entry, normalized across RSS and Atom.
type FeedItem struct {
	ID        string // guid / atom id, falling back to the link — the dedupe key
	Title     string
	Link      string
	Published string
}

// Feed is a parsed feed.
type Feed struct {
	Title string
	Items []FeedItem
}

// Fetch GETs a feed with conditional-GET validators. notModified=true means
// the feed is unchanged (feed is nil); otherwise the new validators are
// returned for the next poll.
func (r *RSS) Fetch(ctx context.Context, feedURL, etag, lastModified string) (feed *Feed, newETag, newLastModified string, notModified bool, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, "", "", false, err
	}
	req.Header.Set("User-Agent", "DiaBot/1.0 (+https://github.com/dia-bot/dia)")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, "", "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return nil, etag, lastModified, true, nil
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, "", "", false, err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, "", "", false, fmt.Errorf("fetch feed: status %d", resp.StatusCode)
	}
	f, err := ParseFeedXML(body)
	if err != nil {
		return nil, "", "", false, err
	}
	return f, resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), false, nil
}

// Validate fetches a feed once to confirm it parses, returning its title.
func (r *RSS) Validate(ctx context.Context, feedURL string) (string, error) {
	u, err := url.Parse(feedURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("not a valid http(s) URL")
	}
	feed, _, _, _, err := r.Fetch(ctx, feedURL, "", "")
	if err != nil {
		return "", err
	}
	title := strings.TrimSpace(feed.Title)
	if title == "" {
		title = u.Host
	}
	return title, nil
}

// rssDoc / atomDoc are the two feed shapes we accept.
type rssDoc struct {
	Channel struct {
		Title string `xml:"title"`
		Items []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			GUID    string `xml:"guid"`
			PubDate string `xml:"pubDate"`
		} `xml:"item"`
	} `xml:"channel"`
}

type atomDoc struct {
	Title   string `xml:"title"`
	Entries []struct {
		ID        string `xml:"id"`
		Title     string `xml:"title"`
		Published string `xml:"published"`
		Updated   string `xml:"updated"`
		Links     []struct {
			Rel  string `xml:"rel,attr"`
			Href string `xml:"href,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

// ParseFeedXML parses RSS 2.0 or Atom bytes into a normalized Feed.
func ParseFeedXML(body []byte) (*Feed, error) {
	root, err := rootElement(body)
	if err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	switch root {
	case "rss", "RDF":
		var doc rssDoc
		if err := xml.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("parse rss: %w", err)
		}
		f := &Feed{Title: strings.TrimSpace(doc.Channel.Title)}
		for _, it := range doc.Channel.Items {
			id := strings.TrimSpace(it.GUID)
			if id == "" {
				id = strings.TrimSpace(it.Link)
			}
			if id == "" {
				continue
			}
			f.Items = append(f.Items, FeedItem{
				ID: id, Title: strings.TrimSpace(it.Title),
				Link: strings.TrimSpace(it.Link), Published: it.PubDate,
			})
		}
		return f, nil
	case "feed":
		var doc atomDoc
		if err := xml.Unmarshal(body, &doc); err != nil {
			return nil, fmt.Errorf("parse atom: %w", err)
		}
		f := &Feed{Title: strings.TrimSpace(doc.Title)}
		for _, e := range doc.Entries {
			link := ""
			for _, l := range e.Links {
				if l.Rel == "" || l.Rel == "alternate" {
					link = l.Href
					break
				}
			}
			id := strings.TrimSpace(e.ID)
			if id == "" {
				id = strings.TrimSpace(link)
			}
			if id == "" {
				continue
			}
			pub := e.Published
			if pub == "" {
				pub = e.Updated
			}
			f.Items = append(f.Items, FeedItem{
				ID: id, Title: strings.TrimSpace(e.Title), Link: strings.TrimSpace(link), Published: pub,
			})
		}
		return f, nil
	default:
		return nil, fmt.Errorf("parse feed: unrecognized root element <%s>", root)
	}
}

// rootElement returns the document's root element local name.
func rootElement(body []byte) (string, error) {
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	for {
		tok, err := dec.Token()
		if err != nil {
			return "", err
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, nil
		}
	}
}
