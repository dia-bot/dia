package social

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// Bluesky reads the public AppView (no auth): handle → DID resolution at
// subscription time and author-feed polling for new posts.
type Bluesky struct{}

const bskyAppView = "https://public.api.bsky.app/xrpc"

// ResolveHandle resolves a handle ("name.bsky.social") to its DID — the
// stable account id a subscription stores (handles can change).
func (b *Bluesky) ResolveHandle(ctx context.Context, handle string) (string, error) {
	handle = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(handle), "@"))
	var out struct {
		DID string `json:"did"`
	}
	err := getJSON(ctx, bskyAppView+"/com.atproto.identity.resolveHandle?handle="+url.QueryEscape(handle), nil, &out)
	if err != nil {
		return "", fmt.Errorf("bluesky handle %q: %w", handle, err)
	}
	return out.DID, nil
}

// BlueskyPost is one original post from an author feed.
type BlueskyPost struct {
	URI       string // at://did/app.bsky.feed.post/rkey — the dedupe key
	Text      string
	CreatedAt string
	Handle    string // author handle at poll time (builds the web URL)
	Avatar    string
}

// AuthorFeed returns the actor's recent original posts (replies and reposts
// filtered out), newest first.
func (b *Bluesky) AuthorFeed(ctx context.Context, actor string, limit int) ([]BlueskyPost, error) {
	q := url.Values{
		"actor":  {actor},
		"limit":  {fmt.Sprint(limit)},
		"filter": {"posts_no_replies"},
	}
	var out struct {
		Feed []struct {
			Post struct {
				URI    string `json:"uri"`
				Author struct {
					Handle string `json:"handle"`
					Avatar string `json:"avatar"`
				} `json:"author"`
				Record struct {
					Text      string `json:"text"`
					CreatedAt string `json:"createdAt"`
				} `json:"record"`
			} `json:"post"`
			Reason map[string]any `json:"reason"` // set on reposts
		} `json:"feed"`
	}
	if err := getJSON(ctx, bskyAppView+"/app.bsky.feed.getAuthorFeed?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	posts := make([]BlueskyPost, 0, len(out.Feed))
	for _, f := range out.Feed {
		if f.Reason != nil {
			continue // repost of someone else's post
		}
		posts = append(posts, BlueskyPost{
			URI:       f.Post.URI,
			Text:      f.Post.Record.Text,
			CreatedAt: f.Post.Record.CreatedAt,
			Handle:    f.Post.Author.Handle,
			Avatar:    f.Post.Author.Avatar,
		})
	}
	return posts, nil
}

// PostURL builds the bsky.app web link for a post URI.
func PostURL(handle, uri string) string {
	rkey := uri
	if i := strings.LastIndex(uri, "/"); i >= 0 {
		rkey = uri[i+1:]
	}
	return "https://bsky.app/profile/" + handle + "/post/" + rkey
}

// ProfileURL builds the bsky.app web link for an account.
func ProfileURL(handle string) string { return "https://bsky.app/profile/" + handle }
