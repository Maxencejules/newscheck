package discovery

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MultiSourceDiscovery combines multiple news sources
type MultiSourceDiscovery struct {
	GoogleNews  *GoogleNews
	directFeeds map[string][]string // country -> RSS feed URLs
	client      *http.Client
}

func NewMultiSourceDiscovery() *MultiSourceDiscovery {
	return &MultiSourceDiscovery{
		GoogleNews:  NewGoogleNews(),
		directFeeds: getDirectFeedsByCountry(),
		client:      &http.Client{Timeout: 20 * time.Second},
	}
}

// Discover searches multiple sources and deduplicates
func (m *MultiSourceDiscovery) Discover(ctx context.Context, p Plan, lang LanguageProfile, from, to time.Time, limit int) ([]Candidate, error) {
	var allCandidates []Candidate
	seenURLs := make(map[string]bool)

	// 1. Try Google News first (filtered for real URLs only)
	fmt.Printf("  Searching Google News RSS...\n")
	gnCandidates, err := m.GoogleNews.Discover(ctx, p, lang, from, to, limit*2)
	if err != nil {
		fmt.Printf("  Warning: Google News failed: %v\n", err)
	} else {
		for _, c := range gnCandidates {
			normalizedURL := normalizeURL(c.URL)
			if !seenURLs[normalizedURL] {
				seenURLs[normalizedURL] = true
				allCandidates = append(allCandidates, c)
			}
		}
		fmt.Printf("  Found %d articles from Google News\n", len(allCandidates))
	}

	// 2. If we don't have enough results, try direct feeds for this country
	if len(allCandidates) < limit/2 {
		countryCode := lang.GL // e.g., "CA"
		if feeds, ok := m.directFeeds[countryCode]; ok {
			fmt.Printf("  Searching direct publisher feeds for %s...\n", countryCode)

			keywords := extractSearchKeywords(p.Query)
			for _, feedURL := range feeds {
				if len(allCandidates) >= limit {
					break
				}

				candidates, err := m.fetchDirectFeed(ctx, feedURL, keywords, from, to, limit)
				if err != nil {
					continue // Skip failed feeds
				}

				for _, c := range candidates {
					normalizedURL := normalizeURL(c.URL)
					if !seenURLs[normalizedURL] {
						seenURLs[normalizedURL] = true
						allCandidates = append(allCandidates, c)
					}
				}
			}
			fmt.Printf("  Total articles after direct feeds: %d\n", len(allCandidates))
		}
	}

	// Limit to requested number
	if len(allCandidates) > limit {
		allCandidates = allCandidates[:limit]
	}

	return allCandidates, nil
}

// fetchDirectFeed fetches and filters articles from a direct RSS feed
func (m *MultiSourceDiscovery) fetchDirectFeed(ctx context.Context, feedURL string, keywords []string, from, to time.Time, limit int) ([]Candidate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 newscheck/0.1")
	req.Header.Set("Accept", "application/rss+xml, application/xml")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("http %d", resp.StatusCode)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed rssFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, err
	}

	// Extract publisher name from URL
	parsedURL, _ := url.Parse(feedURL)
	publisherName := parsedURL.Host
	if publisherName == "" {
		publisherName = "Direct RSS"
	}

	var candidates []Candidate
	for _, item := range feed.Channel.Items {
		// Parse date
		pub, ok := parseGoogleRSSDate(item.PubDate)
		if !ok {
			continue
		}

		// Filter by date range
		if pub.Before(from) || pub.After(to) {
			continue
		}

		// Filter by keywords in title and description
		titleLower := strings.ToLower(item.Title)
		descLower := strings.ToLower(item.Description)
		matchCount := 0
		for _, kw := range keywords {
			if strings.Contains(titleLower, kw) || strings.Contains(descLower, kw) {
				matchCount++
			}
		}

		// Require at least 1 keyword match for relevance
		if len(keywords) > 0 && matchCount == 0 {
			continue
		}

		// Get the actual article URL
		articleURL := strings.TrimSpace(item.Link)
		if articleURL == "" {
			continue
		}

		// Skip if it's a Google News wrapper
		if strings.Contains(articleURL, "news.google.com") {
			continue
		}

		candidates = append(candidates, Candidate{
			Title:       strings.TrimSpace(item.Title),
			URL:         articleURL,
			Source:      publisherName,
			PublishedAt: pub,
			FoundBy:     fmt.Sprintf("Direct RSS: %s", publisherName),
		})

		if len(candidates) >= limit {
			break
		}
	}

	return candidates, nil
}

// getDirectFeedsByCountry returns major news RSS feeds by country
func getDirectFeedsByCountry() map[string][]string {
	return map[string][]string{
		"CA": { // Canada
			"https://www.cbc.ca/webfeed/rss/rss-topstories",
			"https://www.cbc.ca/webfeed/rss/rss-business",
			"https://www.ctvnews.ca/rss/ctvnews-ca-top-stories-public-rss-1.822009",
			"https://globalnews.ca/canada/feed/",
		},
		"US": { // United States
			"https://feeds.npr.org/1001/rss.xml",
			"https://rss.nytimes.com/services/xml/rss/nyt/HomePage.xml",
			"https://rss.nytimes.com/services/xml/rss/nyt/Business.xml",
		},
		"GB": { // United Kingdom
			"https://feeds.bbci.co.uk/news/rss.xml",
			"https://feeds.bbci.co.uk/news/business/rss.xml",
			"https://www.theguardian.com/world/rss",
		},
		"FR": { // France
			"https://www.lemonde.fr/rss/une.xml",
			"https://www.france24.com/en/rss",
		},
		"DE": { // Germany
			"https://www.dw.com/en/rss",
		},
		"AU": { // Australia
			"https://www.abc.net.au/news/feed/51120/rss.xml",
		},
		// Add more countries as needed
	}
}

func extractSearchKeywords(query string) []string {
	query = strings.ToLower(query)
	words := strings.Fields(query)

	stopWords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "by": true, "from": true,
		"is": true, "are": true, "was": true, "were": true, "be": true,
	}

	var keywords []string
	for _, word := range words {
		if !stopWords[word] && len(word) > 2 {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

func normalizeURL(urlStr string) string {
	// Remove query parameters and fragments for deduplication
	urlStr = strings.TrimSpace(urlStr)
	if i := strings.Index(urlStr, "?"); i > 0 {
		urlStr = urlStr[:i]
	}
	if i := strings.Index(urlStr, "#"); i > 0 {
		urlStr = urlStr[:i]
	}
	return strings.ToLower(urlStr)
}
