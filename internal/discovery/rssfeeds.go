package discovery

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
)

type RSSFeeds struct {
	Client *http.Client
	Feeds  []string
}

func NewRSSFeeds(feeds []string) *RSSFeeds {
	return &RSSFeeds{
		Client: &http.Client{Timeout: 15 * time.Second},
		Feeds:  feeds,
	}
}

func (r *RSSFeeds) Discover(ctx context.Context, p Plan, from, to time.Time, limit int) ([]Candidate, error) {
	// RSS feeds are not queryable like search, so we pull and filter locally by keywords.
	// For now: basic contains-any-keyword match on title.
	keywords := strings.Fields(strings.ToLower(p.Query))
	if len(keywords) == 0 {
		return nil, nil
	}

	parser := gofeed.NewParser()
	out := make([]Candidate, 0, limit)

	for _, feedURL := range r.Feeds {
		if len(out) >= limit {
			break
		}

		req, err := http.NewRequestWithContext(ctx, "GET", feedURL, nil)
		if err != nil {
			continue
		}
		resp, err := r.Client.Do(req)
		if err != nil {
			continue
		}
		feed, err := parser.Parse(resp.Body)
		resp.Body.Close()
		if err != nil {
			continue
		}

		for _, it := range feed.Items {
			if len(out) >= limit {
				break
			}
			title := strings.ToLower(strings.TrimSpace(it.Title))

			if !matchesAnyKeyword(title, keywords) {
				continue
			}

			var pub time.Time
			if it.PublishedParsed != nil {
				pub = *it.PublishedParsed
			} else if it.UpdatedParsed != nil {
				pub = *it.UpdatedParsed
			} else {
				continue
			}

			if pub.Before(from) || pub.After(to) {
				continue
			}

			out = append(out, Candidate{
				Title:       strings.TrimSpace(it.Title),
				URL:         strings.TrimSpace(it.Link),
				Source:      strings.TrimSpace(feed.Title),
				PublishedAt: pub,
				FoundBy:     p.Scope + " | " + p.Query,
			})
		}
	}

	return out, nil
}

func matchesAnyKeyword(text string, keywords []string) bool {
	for _, k := range keywords {
		k = strings.TrimSpace(k)
		if len(k) < 3 {
			continue
		}
		if strings.Contains(text, k) {
			return true
		}
	}
	return false
}
