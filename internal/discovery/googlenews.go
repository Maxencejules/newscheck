package discovery

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type LanguageProfile struct {
	Code string // "en", "fr", "es", "pt"
	HL   string // e.g. "en-CA"
	GL   string // e.g. "CA"
	CEID string // e.g. "CA:en"
}

type GoogleNews struct {
	Client *http.Client
}

func NewGoogleNews() *GoogleNews {
	return &GoogleNews{
		Client: &http.Client{Timeout: 20 * time.Second},
	}
}

// ---------- RSS structs ----------
type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	GUID        string    `xml:"guid"`
	PubDate     string    `xml:"pubDate"`
	Description string    `xml:"description"`
	Source      rssSource `xml:"source"`
}

type rssSource struct {
	URL  string `xml:"url,attr"`
	Text string `xml:",chardata"`
}

// Matches href="..." or href='...'
var reHrefAny = regexp.MustCompile(`(?i)\bhref\s*=\s*(?:"([^"]+)"|'([^']+)')`)

// Matches URLs in plain text
var reURLPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

func (g *GoogleNews) Discover(ctx context.Context, p Plan, lang LanguageProfile, from, to time.Time, limit int) ([]Candidate, error) {
	q := buildScopedQuery(p.Query, p.Scope)

	u := fmt.Sprintf(
		"https://news.google.com/rss/search?q=%s&hl=%s&gl=%s&ceid=%s",
		url.QueryEscape(q),
		url.QueryEscape(lang.HL),
		url.QueryEscape(lang.GL),
		url.QueryEscape(lang.CEID),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	// More browser-like UA
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 newscheck/0.1 (+personal use)")
	req.Header.Set("Accept", "application/rss+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.1")

	resp, err := g.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("google news rss http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var feed rssFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, err
	}

	out := make([]Candidate, 0, limit)
	skipped := 0
	for _, it := range feed.Channel.Items {
		if len(out) >= limit {
			break
		}

		pub, ok := parseGoogleRSSDate(it.PubDate)
		if !ok {
			continue
		}
		if pub.Before(from) || pub.After(to) {
			continue
		}

		googleURL := strings.TrimSpace(it.Link)

		// Try multiple strategies to extract the real publisher URL
		publisherURL := extractPublisherURL(it, googleURL)

		// Skip if we couldn't resolve to a real article URL
		// If we can't unwrap it here, pass the wrapper URL to the worker which can handle redirects/unwrapping
		if publisherURL == "" {
			if isGoogleNewsWrapper(googleURL) {
				publisherURL = googleURL
			} else {
				skipped++
				continue
			}
		}

		out = append(out, Candidate{
			Title:       strings.TrimSpace(it.Title),
			URL:         publisherURL,
			Source:      "Google News RSS (" + lang.Code + ")",
			PublishedAt: pub,
			FoundBy:     fmt.Sprintf("%s | %s", p.Scope, p.Query),
		})
	}

	// Log how many were skipped
	if skipped > 0 {
		fmt.Printf("  (skipped %d Google News wrappers that couldn't be resolved)\n", skipped)
	}

	return out, nil
}

// isGoogleNewsWrapper checks if the URL is a Google News wrapper that needs resolution
func isGoogleNewsWrapper(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if host != "news.google.com" && host != "google.com" && host != "www.google.com" {
		return false
	}
	// Check for article paths
	return strings.Contains(parsed.Path, "/rss/articles/") || strings.Contains(parsed.Path, "/articles/")
}

// extractPublisherURL tries multiple strategies to find the real article URL
func extractPublisherURL(item rssItem, googleURL string) string {
	// Strategy 1: Extract from description HTML (MOST RELIABLE - contains actual article link)
	if item.Description != "" {
		if url := extractFromDescription(item.Description); url != "" {
			return url
		}
	}

	// Strategy 2: Check GUID (sometimes contains article URL)
	if item.GUID != "" {
		if url := extractFromGUID(item.GUID); url != "" {
			return url
		}
	}

	// Strategy 3: Parse the Google News link itself for encoded URLs
	if url := extractFromGoogleNewsURL(googleURL); url != "" {
		return url
	}

	// Strategy 4: Check the <source url="..."> attribute (LAST - usually just homepage)
	// Only use this as absolute last resort since it's often just the publisher domain
	if item.Source.URL != "" {
		sourceURL := strings.TrimSpace(item.Source.URL)
		// Check if it looks like a full article URL (has path beyond just domain)
		if isValidPublisherURL(sourceURL) && hasArticlePath(sourceURL) {
			return sourceURL
		}
	}

	return ""
}

// extractFromDescription extracts publisher URL from the HTML description field
func extractFromDescription(desc string) string {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return ""
	}

	// Unescape HTML entities (Google sometimes double-encodes)
	for i := 0; i < 3; i++ {
		unescaped := html.UnescapeString(desc)
		if unescaped == desc {
			break
		}
		desc = unescaped
	}

	// Strategy A: Look for href attributes in anchor tags
	matches := reHrefAny.FindAllStringSubmatch(desc, -1)
	for _, m := range matches {
		href := ""
		if len(m) >= 2 && strings.TrimSpace(m[1]) != "" {
			href = strings.TrimSpace(m[1])
		} else if len(m) >= 3 && strings.TrimSpace(m[2]) != "" {
			href = strings.TrimSpace(m[2])
		}
		if href != "" && isValidPublisherURL(href) {
			return href
		}
	}

	// Strategy B: Look for plain URLs in text (sometimes Google includes them)
	urlMatches := reURLPattern.FindAllString(desc, -1)
	for _, urlStr := range urlMatches {
		urlStr = strings.TrimRight(urlStr, `.,;:!?)'"`)
		if isValidPublisherURL(urlStr) {
			return urlStr
		}
	}

	return ""
}

// extractFromGUID tries to extract a URL from the GUID field
func extractFromGUID(guid string) string {
	guid = strings.TrimSpace(guid)
	if guid == "" {
		return ""
	}

	// Sometimes GUID contains the actual article URL
	if strings.HasPrefix(guid, "http://") || strings.HasPrefix(guid, "https://") {
		if isValidPublisherURL(guid) {
			return guid
		}
	}

	// Sometimes GUID contains a URL after some prefix
	urlMatches := reURLPattern.FindAllString(guid, -1)
	for _, urlStr := range urlMatches {
		urlStr = strings.TrimRight(urlStr, `.,;:!?)'"`)
		if isValidPublisherURL(urlStr) {
			return urlStr
		}
	}

	return ""
}

// extractFromGoogleNewsURL tries to extract embedded URLs from Google News wrapper URLs
func extractFromGoogleNewsURL(googleURL string) string {
	// Google News URLs sometimes contain the publisher URL in query params
	parsed, err := url.Parse(googleURL)
	if err != nil {
		return ""
	}

	// Check common query parameters
	for _, param := range []string{"url", "u", "link", "q"} {
		if val := parsed.Query().Get(param); val != "" {
			if isValidPublisherURL(val) {
				return val
			}
		}
	}

	return ""
}

// isValidPublisherURL checks if a URL is a valid external publisher URL (not Google)
func isValidPublisherURL(urlStr string) bool {
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return false
	}

	// Must start with http:// or https://
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		return false
	}

	// Parse to get host
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Host))
	if host == "" {
		return false
	}

	// Strip port
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}

	// Reject Google domains
	googleDomains := []string{
		"google.com",
		"www.google.com",
		"news.google.com",
		"google.ca",
		"google.co.uk",
		"google.fr",
	}

	for _, gd := range googleDomains {
		if host == gd || strings.HasSuffix(host, "."+gd) {
			return false
		}
	}

	// Reject obviously invalid URLs
	if strings.Contains(urlStr, "javascript:") {
		return false
	}
	if strings.Contains(urlStr, "mailto:") {
		return false
	}

	return true
}

// hasArticlePath checks if URL has a path beyond just the domain (not a homepage)
func hasArticlePath(urlStr string) bool {
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	path := strings.Trim(parsed.Path, "/")

	// If there's a path, it's likely an article
	if path != "" {
		return true
	}

	// If there are query parameters, might be an article
	if parsed.RawQuery != "" {
		return true
	}

	// Just a domain = homepage
	return false
}

// Google News RSS pubDate is usually RFC1123Z, but we handle a couple common variants.
func parseGoogleRSSDate(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}

	layouts := []string{
		time.RFC1123Z, // "Mon, 02 Jan 2006 15:04:05 -0700"
		time.RFC1123,  // "Mon, 02 Jan 2006 15:04:05 MST"
		time.RFC822Z,
		time.RFC822,
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func buildScopedQuery(q, scope string) string {
	q = strings.TrimSpace(q)
	if scope == "" || scope == "global" {
		return q
	}
	if strings.HasPrefix(scope, "region:") {
		return q + " " + strings.TrimPrefix(scope, "region:")
	}
	if strings.HasPrefix(scope, "country:") {
		return q + " " + strings.TrimPrefix(scope, "country:")
	}
	return q
}
