package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

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

func main() {
	// Test Google News RSS
	query := "Canada US trade agreement"
	u := fmt.Sprintf(
		"https://news.google.com/rss/search?q=%s&hl=en-CA&gl=CA&ceid=CA:en",
		url.QueryEscape(query),
	)

	client := &http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 newscheck/0.1")

	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	var feed rssFeed
	xml.Unmarshal(raw, &feed)

	// Print first 3 items to see what we're getting
	for i, item := range feed.Channel.Items {
		if i >= 3 {
			break
		}

		fmt.Printf("\n=== ITEM %d ===\n", i+1)
		fmt.Printf("Title: %s\n", item.Title)
		fmt.Printf("Link: %s\n", item.Link)
		fmt.Printf("GUID: %s\n", item.GUID)
		fmt.Printf("Source URL: %s\n", item.Source.URL)
		fmt.Printf("Source Text: %s\n", item.Source.Text)
		fmt.Printf("Description (first 200 chars): %s\n",
			truncate(item.Description, 200))
		fmt.Printf("Description (full):\n%s\n", item.Description)
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
