package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gingfrederik/docx"
	"newscheck/internal/discovery"
	"newscheck/internal/extract"
	"newscheck/internal/geo"
)

type Service struct {
	Resolver *geo.HybridResolver
	Matcher  *geo.CountryMatcher
	GN       *discovery.GoogleNews
	RSS      *discovery.RSSFeeds
	Worker   *extract.Worker
}

func NewService() (*Service, error) {
	cache := geo.NewCache("newscheck")
	ds, err := geo.NewDatasetResolver("data/country_languages.json")
	if err != nil {
		return nil, err
	}
	autoStore, err := geo.NewAutoCacheStore("data/country_auto_cache.json")
	if err != nil {
		return nil, err
	}
	api := geo.NewRestCountriesResolver()
	apiWithAuto := geo.NewAutoCacheResolver(autoStore, api)
	resolver := geo.NewHybridResolver(cache, ds, apiWithAuto)

	matcher, err := geo.NewCountryMatcher("data/country_languages.json")
	if err != nil {
		return nil, err
	}

	return &Service{
		Resolver: resolver,
		Matcher:  matcher,
		GN:       discovery.NewGoogleNews(),
		RSS:      discovery.NewRSSFeeds([]string{
			"https://rss.nytimes.com/services/xml/rss/nyt/World.xml",
			"https://www.theguardian.com/world/rss",
			"https://feeds.bbci.co.uk/news/world/rss.xml",
			"https://www.aljazeera.com/xml/rss/all.xml",
		}),
		Worker: extract.NewWorker(),
	}, nil
}

type SearchRequest struct {
	Query         string
	From          time.Time
	To            time.Time
	Scope         SearchScope
	ChosenCountry string
	PivotLang     string
}

type SearchResult struct {
	Candidates []discovery.Candidate `json:"Candidates"`
	Intent     Intent                `json:"Intent"`
	Plans      []SearchPlan          `json:"Plans"`
	Targets    []geo.DiscoveryTarget `json:"Targets"`
}

func (s *Service) Search(ctx context.Context, req SearchRequest) (*SearchResult, error) {
	// 1. Intent
	intent := ExtractIntent(req.Query)

	// 2. Country Resolution
	var countryNames []string
	switch req.Scope {
	case ScopeAuto:
		countryNames = s.Matcher.FindCountries(req.Query)
		if len(countryNames) == 0 && len(intent.Countries) > 0 {
			countryNames = append(countryNames, intent.Countries...)
		}
		if len(countryNames) == 0 {
			hints := geo.ExtractCountryHints(req.Query)
			for _, h := range hints {
				info, err := s.Resolver.ResolveCountry(ctx, h)
				if err == nil && info.ISO2 != "" && len(info.Languages) > 0 {
					countryNames = append(countryNames, info.Name)
					break
				}
			}
		}
	case ScopeChosen:
		countryNames = []string{req.ChosenCountry}
		intent.Countries = nil
		intent.Regions = nil
	case ScopeGlobal:
		countryNames = []string{}
		intent.Countries = nil
		intent.Regions = nil
	}

	resolved := make([]geo.CountryInfo, 0, len(countryNames))
	for _, name := range countryNames {
		info, err := s.Resolver.ResolveCountry(ctx, name)
		if err == nil && info.ISO2 != "" {
			resolved = append(resolved, info)
		}
	}

	// 3. Build Targets
	targets := buildTargets(resolved)

	// 4. Build Plans
	plans := BuildSearchPlans(req.Query, intent, resolved)

	// 5. Discovery
	tr := TimeRange{From: req.From, To: req.To}
	candidates, err := runDiscoveryWithTargets(ctx, plans, tr, targets, s.GN, s.RSS)
	if err != nil {
		return nil, err
	}

	// 6. Filter & Score
	candidates = filterCandidates(candidates, req.Query, intent, resolved)
	consensus := calculateConsensus(candidates)
	for i := range candidates {
		candidates[i].ConsensusScore = consensus[candidates[i].URL]
	}

	return &SearchResult{
		Candidates: candidates,
		Intent:     intent,
		Plans:      plans,
		Targets:    targets,
	}, nil
}

func (s *Service) ExtractAndSummarize(ctx context.Context, urls []string, pivotLang string, query string, apiKey string) ([]extract.Article, string, error) {
	var extracted []extract.Article

	for _, u := range urls {
		art, err := s.Worker.Extract(ctx, u, pivotLang)
		if err != nil {
			fmt.Printf("Extract error for %s: %v\n", u, err) // Log to stdout for now
			continue
		}
		extracted = append(extracted, art)
	}

	var summary string
	if len(extracted) > 0 {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("User Query: %s\n\n", query))
		sb.WriteString("Source Articles:\n")
		for _, art := range extracted {
			sb.WriteString(fmt.Sprintf("Title: %s\nSource: %s\nText:\n%s\n\n", art.Title, art.Site, art.Text))
		}
		fullText := sb.String()

		var err error
		summary, err = s.Worker.Summarize(ctx, fullText, apiKey)
		if err != nil {
			return extracted, "", err
		}
	}

	return extracted, summary, nil
}

func (s *Service) GenerateArticleReport(path string, articles []extract.Article) error {
	f := docx.NewFile()

	titleP := f.AddParagraph()
	titleRun := titleP.AddText("Extracted Articles Report")
	titleRun.Size(20)
	f.AddParagraph() // Spacer

	for _, art := range articles {
		// Title
		p := f.AddParagraph()
		run := p.AddText(art.Title)
		run.Size(16)

		// Metadata
		p = f.AddParagraph()
		pub := ""
		if art.PublishedAt != nil {
			pub = *art.PublishedAt
		}
		run = p.AddText(fmt.Sprintf("Source: %s | Date: %s", art.Site, pub))
		run.Size(10)
		run.Color("808080")

		// URL
		p = f.AddParagraph()
		run = p.AddText(art.FinalURL)
		run.Size(10)
		run.Color("0000FF")

		// Simple text splitting by double newlines for paragraphs
		paragraphs := strings.Split(art.Text, "\n\n")
		for _, txt := range paragraphs {
			txt = strings.TrimSpace(txt)
			if txt != "" {
				f.AddParagraph().AddText(txt)
			}
		}
		f.AddParagraph().AddText("--------------------------------------------------")
	}

	return f.Save(path)
}

func (s *Service) GenerateScoresReport(path string, candidates []discovery.Candidate) error {
	f := docx.NewFile()

	// Header
	p := f.AddParagraph()
	run := p.AddText("Relevance & Consensus Scores Report")
	run.Size(18)

	// Explanations
	p = f.AddParagraph()
	p.AddText("Understanding the Scores:")

	p = f.AddParagraph()
	p.AddText("- Relevance Score (0-100): Indicates how closely the article matches your specific query keywords and country intent. Higher is better.")

	p = f.AddParagraph()
	p.AddText("- Consensus Score: Represents cross-source validation. It counts how many *other* independent sources are covering essentially the same story (based on keyword overlap). A higher score suggests a major, verified event.")

	f.AddParagraph() // Spacer
	f.AddParagraph().AddText("--------------------------------------------------")
	f.AddParagraph() // Spacer

	for _, c := range candidates {
		p = f.AddParagraph()
		run = p.AddText(c.Title)

		p = f.AddParagraph()
		run = p.AddText(c.URL)
		run.Size(10)

		consensusDesc := "Low"
		if c.ConsensusScore >= 2 { consensusDesc = "Medium" }
		if c.ConsensusScore >= 4 { consensusDesc = "High" }
		if c.ConsensusScore >= 6 { consensusDesc = "Very High" }

		p = f.AddParagraph()
		run = p.AddText(fmt.Sprintf("Relevance: %d | Consensus: %d (%s)", c.RelevanceScore, c.ConsensusScore, consensusDesc))
		run.Color("008000")

		f.AddParagraph() // Spacer
	}

	return f.Save(path)
}

func (s *Service) GenerateResumeReport(path string, summary string, query string, articles []extract.Article) error {
	f := docx.NewFile()

	// Header
	p := f.AddParagraph()
	run := p.AddText("Global Intelligence Resume")
	run.Size(20)

	p = f.AddParagraph()
	p.AddText(fmt.Sprintf("Query: %s", query))

	f.AddParagraph() // Spacer

	// Summary Content
	p = f.AddParagraph()
	p.AddText(summary)

	f.AddParagraph() // Spacer
	f.AddParagraph().AddText("--------------------------------------------------")
	f.AddParagraph() // Spacer

	p = f.AddParagraph()
	p.AddText("Based on sources:")
	for _, art := range articles {
		f.AddParagraph().AddText(fmt.Sprintf("- %s (%s)", art.Title, art.Site))
	}

	return f.Save(path)
}
