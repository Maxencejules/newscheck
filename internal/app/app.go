package app

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gingfrederik/docx"
	"newscheck/internal/discovery"
	"newscheck/internal/extract"
	"newscheck/internal/geo"
)

type Input struct {
	Query       string
	TimeRange   TimeRange
	Intent      Intent
	SearchPlans []SearchPlan

	// Country-driven discovery targets: (ISO2, language)
	Targets   []geo.DiscoveryTarget
	PivotLang string // "en" or "fr"
}

type TimeRange struct {
	From  time.Time
	To    time.Time
	Label string
}

type Intent struct {
	Topics    []string
	Regions   []string
	Countries []string
	Themes    []string
	Keywords  []string
}

type SearchPlan struct {
	Query   string
	Scope   string // "global" | "region:<name>" | "country:<name>"
	Focus   string // "topic:<x>" | "theme:<x>" | "mixed"
	Weight  int
	Explain string
}

func Run() error {
	in := bufio.NewReader(os.Stdin)

	// 1) Query input + validation
	var query string
	for {
		fmt.Println("Enter your topic (keywords/sentence/paragraph).")
		fmt.Println("Submit with a blank line.")
		fmt.Print("> ")

		q, err := readMultiline(in)
		if err != nil {
			return err
		}
		q = strings.TrimSpace(q)

		if ok, reason := validateQuery(q); !ok {
			fmt.Printf("Invalid input (%s). Please try again.\n\n", reason)
			continue
		}

		query = q
		break
	}

	// 2) Time window selection
	tr, err := selectTimeRange(in)
	if err != nil {
		return err
	}

	// 3) Search scope selection
	scopeMode, chosenCountry, err := selectSearchScope(in)
	if err != nil {
		return err
	}

	// 4) Intent extraction
	intent := ExtractIntent(query)

	// 5) Pivot language selection (translation later)
	pivot, err := selectPivotLanguage(in)
	if err != nil {
		return err
	}

	ctx := context.Background()

	// 6) Country detection + resolver chain:
	// - Manual overrides dataset (country_languages.json)
	// - Auto cache dataset (country_auto_cache.json) written automatically
	// - API fallback (RestCountries)
	// - In-memory cache layer (your geo.NewCache)
	cache := geo.NewCache("newscheck")

	ds, err := geo.NewDatasetResolver("data/country_languages.json")
	if err != nil {
		return err
	}

	autoStore, err := geo.NewAutoCacheStore("data/country_auto_cache.json")
	if err != nil {
		return err
	}

	api := geo.NewRestCountriesResolver()
	apiWithAuto := geo.NewAutoCacheResolver(autoStore, api)

	resolver := geo.NewHybridResolver(cache, ds, apiWithAuto)

	matcher, err := geo.NewCountryMatcher("data/country_languages.json")
	if err != nil {
		return err
	}

	var countryNames []string

	switch scopeMode {
	case ScopeAuto:
		// Auto (current behavior)
		// Find possibly multiple countries from the raw query (manual overrides only)
		countryNames = matcher.FindCountries(query)

		// If matcher found none, fall back to rule-based intent country hits (if any)
		if len(countryNames) == 0 && len(intent.Countries) > 0 {
			countryNames = append(countryNames, intent.Countries...)
		}

		// If still none: attempt automatic country resolution from query hints
		// This is what enables "any country -> local languages" without editing JSON.
		if len(countryNames) == 0 {
			hints := geo.ExtractCountryHints(query)
			for _, h := range hints {
				info, err := resolver.ResolveCountry(ctx, h)
				if err == nil && info.ISO2 != "" && len(info.Languages) > 0 {
					countryNames = append(countryNames, info.Name)
					break
				}
			}
		}

	case ScopeChosen:
		// User chose a specific country
		countryNames = []string{chosenCountry}
		// Clear intent countries/regions to prevent mixing if user explicitly chose one
		intent.Countries = nil
		intent.Regions = nil

	case ScopeGlobal:
		// Explicit global - force empty country list
		countryNames = []string{}
		// Also clear intent locations
		intent.Countries = nil
		intent.Regions = nil
	}

	// Resolve all countries (some may fail; we skip failed ones)
	resolved := make([]geo.CountryInfo, 0, len(countryNames))
	for _, name := range countryNames {
		info, err := resolver.ResolveCountry(ctx, name)
		if err == nil && info.ISO2 != "" {
			resolved = append(resolved, info)
		}
	}

	// Build discovery targets:
	// - For each resolved country: local langs + English
	// - If none: a safe fallback (US/en)
	targets := buildTargets(resolved)
	printTargets(countryNames, resolved, targets)

	// Generate search plans AFTER scope/targets are finalized
	plans := BuildSearchPlans(query, intent, resolved)

	input := Input{
		Query:       query,
		TimeRange:   tr,
		Intent:      intent,
		SearchPlans: plans,
		Targets:     targets,
		PivotLang:   pivot,
	}

	fmt.Println("\nRequest accepted:")
	fmt.Println("Time window:", input.TimeRange.Label)
	fmt.Println("Pivot lang :", input.PivotLang)

	fmt.Println("\nExtracted intent:")
	printIntent(input.Intent)

	fmt.Println("\nGenerated search plans:")
	printPlans(input.SearchPlans)

	// 7) Discovery (Google News RSS per (ISO2,lang) + curated RSS)
	gn := discovery.NewGoogleNews()

	rss := discovery.NewRSSFeeds([]string{
		"https://rss.nytimes.com/services/xml/rss/nyt/World.xml",
		"https://www.theguardian.com/world/rss",
		"https://feeds.bbci.co.uk/news/world/rss.xml",
		"https://www.aljazeera.com/xml/rss/all.xml",
	})

	candidates, err := runDiscoveryWithTargets(ctx, input.SearchPlans, input.TimeRange, input.Targets, gn, rss)
	if err != nil {
		return err
	}

	// Relevance filtering
	candidates = filterCandidates(candidates, query, intent, resolved)

	// Cross-source consensus scoring
	consensusScores := calculateConsensus(candidates)
	for i := range candidates {
		candidates[i].ConsensusScore = consensusScores[candidates[i].URL]
	}

	fmt.Printf("\nDiscovered %d candidate articles (after filtering)\n", len(candidates))
	for i := 0; i < mini(20, len(candidates)); i++ {
		c := candidates[i]
		consensusLabel := ""
		if c.ConsensusScore > 1 {
			consensusLabel = fmt.Sprintf(" [Consensus: %d]", c.ConsensusScore)
		}

		fmt.Printf("%2d) %s%s [Rel: %d]\n    %s\n    %s\n    %s\n",
			i+1, c.Title, consensusLabel, c.RelevanceScore, c.URL, c.PublishedAt.Format(time.RFC3339), c.Source)
	}

	// 8) Step 7: Fetch + Extract (Python worker) for top N
	fmt.Print("\nExtract how many articles now? (0 to skip, default 5): ")
	line, _ := in.ReadString('\n')
	line = strings.TrimSpace(line)

	n := 5
	if line != "" {
		var tmp int
		_, _ = fmt.Sscanf(line, "%d", &tmp)
		if tmp < 0 {
			tmp = 0
		}
		n = tmp
	}
	if n > len(candidates) {
		n = len(candidates)
	}

	var extractedArticles []extract.Article

	if n > 0 {
		worker := extract.NewWorker()
		for i := 0; i < n; i++ {
			u := candidates[i].URL
			fmt.Printf("\n[%d/%d] Extracting: %s\n", i+1, n, u)

			art, err := worker.Extract(ctx, u, input.PivotLang)
			if err != nil {
				fmt.Println("  - error:", err)
				continue
			}

			extractedArticles = append(extractedArticles, art)

			fmt.Println("  - title:", art.Title)
			fmt.Println("  - site :", art.Site)
			if art.Lang != nil {
				fmt.Println("  - lang :", *art.Lang)
			}
			fmt.Printf("  - text : %d chars\n", len(art.Text))

			preview := strings.TrimSpace(art.Text)
			if len(preview) > 250 {
				preview = preview[:250] + "..."
			}
			if preview != "" {
				fmt.Println("  - preview:", preview)
			}
		}
	}

	if len(extractedArticles) > 0 || len(candidates) > 0 {
		fmt.Println("\nGenerating reports...")
		if err := generateReports(extractedArticles, candidates); err != nil {
			fmt.Println("Error generating reports:", err)
		} else {
			fmt.Println("Reports generated: articles.docx, scores.docx")
		}

		if len(extractedArticles) > 0 {
			fmt.Println("\nGenerating coherent resume (Summary)...")
			worker := extract.NewWorker()
			if err := generateResume(ctx, worker, extractedArticles, query); err != nil {
				fmt.Printf("Error generating resume: %v\n", err)
			} else {
				fmt.Println("Resume generated: summaries/resume_....docx")
			}
		}
	}

	return nil
}

func generateResume(ctx context.Context, w *extract.Worker, articles []extract.Article, query string) error {
	if err := os.MkdirAll("summaries", 0755); err != nil {
		return fmt.Errorf("creating summaries dir: %w", err)
	}

	// Aggregate texts
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("User Query: %s\n\n", query))
	sb.WriteString("Source Articles:\n")
	for _, art := range articles {
		sb.WriteString(fmt.Sprintf("Title: %s\nSource: %s\nText:\n%s\n\n", art.Title, art.Site, art.Text))
	}

	fullText := sb.String()

	// Call summarizer
	summary, err := w.Summarize(ctx, fullText)
	if err != nil {
		return err
	}

	// Save to DOCX
	f := docx.NewFile()

	// Header
	p := f.AddParagraph()
	run := p.AddText("Global Intelligence Resume")
	run.Size(20)
	// run.Bold()

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

	timestamp := time.Now().Format("2006-01-02_15-04")
	filename := fmt.Sprintf("summaries/resume_%s.docx", timestamp)
	if err := f.Save(filename); err != nil {
		return err
	}

	return nil
}

func generateReports(articles []extract.Article, candidates []discovery.Candidate) error {
	// Create output directories
	if err := os.MkdirAll("reports", 0755); err != nil {
		return fmt.Errorf("creating reports dir: %w", err)
	}
	if err := os.MkdirAll("scores", 0755); err != nil {
		return fmt.Errorf("creating scores dir: %w", err)
	}

	// 1. Articles DOCX
	if len(articles) > 0 {
		f := docx.NewFile()

		titleP := f.AddParagraph()
		titleRun := titleP.AddText("Extracted Articles Report")
		titleRun.Size(20)
		f.AddParagraph() // Spacer

		for _, art := range articles {
			// Title
			p := f.AddParagraph()
			run := p.AddText(art.Title)
			// run.Bold() // Not supported in this lib version apparently
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

		timestamp := time.Now().Format("2006-01-02_15-04")
		filename := fmt.Sprintf("reports/articles_%s.docx", timestamp)
		if err := f.Save(filename); err != nil {
			return err
		}
		fmt.Printf("Saved article report to: %s\n", filename)
	}

	// 2. Scores DOCX
	if len(candidates) > 0 {
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
			// run.Bold()

			p = f.AddParagraph()
			run = p.AddText(c.URL)
			run.Size(10)

			// Scale relevance to look more standard (it was raw points before)
			// Assuming raw score rarely exceeds ~20-30 in current logic, let's just present it clearly or normalize if we knew max.
			// Current logic: +10 per keyword match, +5 country, +2 recency.
			// Let's cap visual display at 100 or just show "Score: X".
			// A "perfect" match might be ~2 keywords + country + recent = 27.
			// Let's show it as "Relevance Score: X (Raw)".

			consensusDesc := "Low"
			if c.ConsensusScore >= 2 { consensusDesc = "Medium" }
			if c.ConsensusScore >= 4 { consensusDesc = "High" }
			if c.ConsensusScore >= 6 { consensusDesc = "Very High" }

			p = f.AddParagraph()
			run = p.AddText(fmt.Sprintf("Relevance: %d | Consensus: %d (%s)", c.RelevanceScore, c.ConsensusScore, consensusDesc))
			run.Color("008000")

			f.AddParagraph() // Spacer
		}

		timestamp := time.Now().Format("2006-01-02_15-04")
		filename := fmt.Sprintf("scores/scores_%s.docx", timestamp)
		if err := f.Save(filename); err != nil {
			return err
		}
		fmt.Printf("Saved scores report to: %s\n", filename)
	}

	return nil
}

// ===== Targets =====

func buildTargets(resolved []geo.CountryInfo) []geo.DiscoveryTarget {
	if len(resolved) == 0 {
		return []geo.DiscoveryTarget{{ISO2: "US", Lang: "en"}}
	}

	seen := map[string]struct{}{}
	out := make([]geo.DiscoveryTarget, 0, 8)

	for _, c := range resolved {
		for _, t := range geo.BuildDiscoveryTargets(c, true) { // true => include English always
			key := t.ISO2 + "|" + t.Lang
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, t)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].ISO2 == out[j].ISO2 {
			return out[i].Lang < out[j].Lang
		}
		return out[i].ISO2 < out[j].ISO2
	})
	return out
}

func printTargets(countryNames []string, resolved []geo.CountryInfo, targets []geo.DiscoveryTarget) {
	fmt.Println("\nDetected countries:", strings.Join(countryNames, ", "))
	for _, c := range resolved {
		fmt.Printf("Resolved: %s (%s) langs=%v\n", c.Name, c.ISO2, c.Languages)
	}
	if len(resolved) == 0 {
		fmt.Println("Resolved: (none) -> fallback discovery target: US/en")
	}

	fmt.Println("\nDiscovery targets (ISO2/lang):")
	for _, t := range targets {
		fmt.Printf("- %s/%s\n", t.ISO2, t.Lang)
	}
}

// ===== Discovery =====

func runDiscoveryWithTargets(
	ctx context.Context,
	plans []SearchPlan,
	tr TimeRange,
	targets []geo.DiscoveryTarget,
	gn *discovery.GoogleNews,
	rss *discovery.RSSFeeds,
) ([]discovery.Candidate, error) {

	toPlan := func(p SearchPlan) discovery.Plan {
		return discovery.Plan{Query: p.Query, Scope: p.Scope}
	}

	maxPlans := 10
	if len(plans) < maxPlans {
		maxPlans = len(plans)
	}

	all := make([]discovery.Candidate, 0, 400)

	for _, t := range targets {
		hl, gl, ceid := geo.BuildGoogleNewsParams(t.ISO2, t.Lang)
		if hl == "" || gl == "" || ceid == "" {
			continue
		}
		profile := discovery.LanguageProfile{
			Code: t.Lang,
			HL:   hl,
			GL:   gl,
			CEID: ceid,
		}

		for i := 0; i < maxPlans; i++ {
			found, err := gn.Discover(ctx, toPlan(plans[i]), profile, tr.From, tr.To, 25)
			if err == nil {
				all = append(all, found...)
			}
		}
	}

	for i := 0; i < maxPlans; i++ {
		found, err := rss.Discover(ctx, toPlan(plans[i]), tr.From, tr.To, 10)
		if err == nil {
			all = append(all, found...)
		}
	}

	return dedupeCandidates(all), nil
}

func dedupeCandidates(in []discovery.Candidate) []discovery.Candidate {
	seen := map[string]discovery.Candidate{}
	for _, c := range in {
		u := strings.TrimSpace(c.URL)
		if u == "" {
			continue
		}
		if prev, ok := seen[u]; ok {
			if c.PublishedAt.After(prev.PublishedAt) {
				seen[u] = c
			}
			continue
		}
		seen[u] = c
	}

	out := make([]discovery.Candidate, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].PublishedAt.After(out[j].PublishedAt)
	})
	return out
}

// ===== Pivot selection =====

func selectPivotLanguage(r *bufio.Reader) (string, error) {
	for {
		fmt.Println("\nTranslate everything to (pivot language):")
		fmt.Println("1) English (en)")
		fmt.Println("2) French  (fr)")
		fmt.Print("> ")

		choice, _ := r.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			return "en", nil
		case "2":
			return "fr", nil
		default:
			fmt.Println("Invalid choice. Please select 1–2.")
		}
	}
}

// ===== Printing helpers =====

func printIntent(i Intent) {
	if len(i.Topics) > 0 {
		fmt.Println("Topics   :", strings.Join(i.Topics, ", "))
	}
	if len(i.Regions) > 0 {
		fmt.Println("Regions  :", strings.Join(i.Regions, ", "))
	}
	if len(i.Countries) > 0 {
		fmt.Println("Countries:", strings.Join(i.Countries, ", "))
	}
	if len(i.Themes) > 0 {
		fmt.Println("Themes   :", strings.Join(i.Themes, ", "))
	}
	if len(i.Keywords) > 0 {
		fmt.Println("Keywords :", strings.Join(i.Keywords, ", "))
	}
}

func printPlans(plans []SearchPlan) {
	for idx, p := range plans {
		fmt.Printf("%2d) [%s] (%s, w=%d) %s\n", idx+1, p.Scope, p.Focus, p.Weight, p.Query)
		if p.Explain != "" {
			fmt.Printf("    - %s\n", p.Explain)
		}
	}
}

// ===== Step 5: Search plan generation =====

func BuildSearchPlans(original string, intent Intent, forcedCountries []geo.CountryInfo) []SearchPlan {
	base := normalizeQuery(original)

	// If forced countries exist (from Choose Country mode), override intent scopes
	var scopes []string
	if len(forcedCountries) > 0 {
		for _, c := range forcedCountries {
			scopes = append(scopes, "country:"+c.ISO2)
		}
	} else {
		scopes = buildScopes(intent)
	}

	plans := []SearchPlan{}

	for _, scope := range scopes {
		plans = append(plans, SearchPlan{
			Query:   base,
			Scope:   scope,
			Focus:   "mixed",
			Weight:  100,
			Explain: "original user query",
		})
	}

	if len(intent.Keywords) > 0 {
		kw := strings.Join(intent.Keywords, " ")
		for _, scope := range scopes {
			plans = append(plans, SearchPlan{
				Query:   kw,
				Scope:   scope,
				Focus:   "mixed",
				Weight:  85,
				Explain: "top extracted keywords",
			})
		}
	}

	for _, topic := range intent.Topics {
		for _, scope := range scopes {
			plans = append(plans, SearchPlan{
				Query:   fmt.Sprintf("%s %s", base, strings.ToLower(topic)),
				Scope:   scope,
				Focus:   "topic:" + topic,
				Weight:  80,
				Explain: "topic expansion",
			})
		}
	}

	for _, theme := range intent.Themes {
		for _, scope := range scopes {
			plans = append(plans, SearchPlan{
				Query:   fmt.Sprintf("%s %s", base, strings.ToLower(theme)),
				Scope:   scope,
				Focus:   "theme:" + theme,
				Weight:  75,
				Explain: "theme expansion",
			})
		}
	}

	if len(intent.Countries) == 0 && len(intent.Regions) > 0 {
		countries := countriesForRegions(intent.Regions)
		for _, c := range countries {
			plans = append(plans, SearchPlan{
				Query:   fmt.Sprintf("%s %s", base, strings.ToLower(c)),
				Scope:   "country:" + c,
				Focus:   "mixed",
				Weight:  70,
				Explain: "country expansion from region",
			})
		}
	}

	plans = dedupePlans(plans)
	sort.Slice(plans, func(i, j int) bool {
		if plans[i].Weight == plans[j].Weight {
			if plans[i].Scope == plans[j].Scope {
				return plans[i].Query < plans[j].Query
			}
			return plans[i].Scope < plans[j].Scope
		}
		return plans[i].Weight > plans[j].Weight
	})

	if len(plans) > 40 {
		plans = plans[:40]
	}
	return plans
}

func buildScopes(intent Intent) []string {
	var scopes []string
	for _, r := range intent.Regions {
		scopes = append(scopes, "region:"+r)
	}
	for _, c := range intent.Countries {
		scopes = append(scopes, "country:"+c)
	}
	if len(scopes) == 0 {
		scopes = []string{"global"}
	}
	return uniqueSorted(scopes)
}

func calculateConsensus(candidates []discovery.Candidate) map[string]int {
	scores := make(map[string]int)
	if len(candidates) < 2 {
		return scores
	}

	// Pre-process titles into sets of tokens
	type doc struct {
		url    string
		tokens map[string]struct{}
	}

	docs := make([]doc, len(candidates))
	for i, c := range candidates {
		// Use extractKeywords to get significant tokens
		tokens := extractKeywords(strings.ToLower(c.Title))
		set := make(map[string]struct{})
		for _, t := range tokens {
			set[t] = struct{}{}
		}
		docs[i] = doc{c.URL, set}
	}

	// Compare every pair
	for i := 0; i < len(docs); i++ {
		for j := 0; j < len(docs); j++ {
			if i == j {
				continue
			}

			// Calculate overlap (Jaccard-ish)
			common := 0
			for t := range docs[i].tokens {
				if _, ok := docs[j].tokens[t]; ok {
					common++
				}
			}

			// Threshold: if they share significant keywords, assume they cover the same topic
			if common >= 2 {
				scores[docs[i].url]++
			}
		}
	}
	return scores
}

func filterCandidates(candidates []discovery.Candidate, query string, intent Intent, countries []geo.CountryInfo) []discovery.Candidate {
	if len(candidates) == 0 {
		return candidates
	}

	// Normalize query terms for simple matching
	qTerms := extractKeywords(strings.ToLower(query))

	// Add intent keywords
	for _, k := range intent.Keywords {
		qTerms = append(qTerms, strings.ToLower(k))
	}

	// If explicit countries, add them to boost match
	countryTerms := []string{}
	for _, c := range countries {
		countryTerms = append(countryTerms, strings.ToLower(c.Name))
	}

	type scored struct {
		c     discovery.Candidate
		score int
	}

	var scoredCandidates []scored

	for _, c := range candidates {
		score := 0
		title := strings.ToLower(c.Title)

		// 1. Title keyword match (high weight)
		for _, term := range qTerms {
			if strings.Contains(title, term) {
				score += 10
			}
		}

		// 2. Country match (medium weight)
		for _, cName := range countryTerms {
			if strings.Contains(title, cName) {
				score += 5
			}
		}

		// 3. Recency boost (simple)
		if time.Since(c.PublishedAt) < 24*time.Hour {
			score += 2
		}

		// Threshold: at least one keyword match or very strong other signals
		if score > 0 {
			// Update the candidate's score
			c.RelevanceScore = score
			scoredCandidates = append(scoredCandidates, scored{c, score})
		}
	}

	// Sort by score descending
	sort.Slice(scoredCandidates, func(i, j int) bool {
		return scoredCandidates[i].score > scoredCandidates[j].score
	})

	out := make([]discovery.Candidate, len(scoredCandidates))
	for i, sc := range scoredCandidates {
		out[i] = sc.c
	}

	// If filtering removed everything but we had candidates, return top original ones as fallback?
	// Or stricter: return empty. Let's return empty to reduce noise as requested.
	return out
}

func normalizeQuery(q string) string {
	q = strings.ToLower(q)
	q = strings.ReplaceAll(q, "\n", " ")
	q = strings.Join(strings.Fields(q), " ")
	return q
}

func dedupePlans(plans []SearchPlan) []SearchPlan {
	seen := map[string]SearchPlan{}
	for _, p := range plans {
		key := p.Scope + "|" + p.Focus + "|" + p.Query
		if existing, ok := seen[key]; ok {
			if p.Weight > existing.Weight {
				seen[key] = p
			}
			continue
		}
		seen[key] = p
	}
	out := make([]SearchPlan, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	return out
}

func countriesForRegions(regions []string) []string {
	set := map[string]struct{}{}
	for _, r := range regions {
		switch r {
		case "South America":
			for _, c := range []string{"Argentina", "Bolivia", "Brazil", "Chile", "Colombia", "Ecuador", "Guyana", "Paraguay", "Peru", "Suriname", "Uruguay", "Venezuela"} {
				set[c] = struct{}{}
			}
		case "Caribbean":
			for _, c := range []string{"Haiti", "Jamaica", "Dominican Rep.", "Cuba", "Trinidad", "Barbados", "Bahamas"} {
				set[c] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(set))
	for c := range set {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// ===== Step 4: Intent extraction (rule-based) =====

func ExtractIntent(text string) Intent {
	t := strings.ToLower(text)

	regionsFound := matchAny(t, regionLexicon)
	countriesFound := matchAny(t, countryLexicon)
	topicsFound := matchAny(t, topicLexicon)
	themesFound := matchAny(t, themeLexicon)

	keywords := extractKeywords(t)

	return Intent{
		Topics:    uniqueSorted(topicsFound),
		Regions:   uniqueSorted(regionsFound),
		Countries: uniqueSorted(countriesFound),
		Themes:    uniqueSorted(themesFound),
		Keywords:  keywords,
	}
}

var regionLexicon = map[string][]string{
	"South America": {"south america", "latin america", "latam"},
	"Caribbean":     {"caribbean", "west indies"},
	"North America": {"north america"},
	"Europe":        {"europe", "eu"},
	"Africa":        {"africa"},
	"Middle East":   {"middle east"},
	"Asia":          {"asia"},
	"World":         {"world", "global", "international"},
}

var countryLexicon = map[string][]string{
	"Argentina": {"argentina"},
	"Bolivia":   {"bolivia"},
	"Brazil":    {"brazil"},
	"Chile":     {"chile"},
	"Colombia":  {"colombia"},
	"Ecuador":   {"ecuador"},
	"Guyana":    {"guyana"},
	"Paraguay":  {"paraguay"},
	"Peru":      {"peru"},
	"Suriname":  {"suriname"},
	"Uruguay":   {"uruguay"},
	"Venezuela": {"venezuela"},

	"Haiti":          {"haiti"},
	"Jamaica":        {"jamaica"},
	"Dominican Rep.": {"dominican republic", "dr"},
	"Cuba":           {"cuba"},
	"Trinidad":       {"trinidad", "trinidad and tobago"},
	"Barbados":       {"barbados"},
	"Bahamas":        {"bahamas"},
}

var topicLexicon = map[string][]string{
	"Politics": {"politic", "government", "parliament", "congress", "president", "prime minister", "minister"},
	"Economy":  {"economy", "inflation", "gdp", "recession", "interest rate", "central bank", "imf", "debt"},
	"Security": {"security", "military", "attack", "terror", "violence", "cartel", "gang"},
	"Health":   {"health", "outbreak", "virus", "hospital", "public health"},
	"Tech":     {"technology", "tech", "ai", "cyber", "hacker", "data breach"},
}

var themeLexicon = map[string][]string{
	"Elections":      {"election", "vote", "ballot", "runoff", "campaign"},
	"Protests":       {"protest", "demonstration", "strike", "unrest", "riot"},
	"Sanctions":      {"sanction"},
	"Corruption":     {"corruption", "bribery", "embezzle"},
	"Courts":         {"court", "supreme court", "ruling", "judge"},
	"Legislation":    {"bill", "law", "legislation", "act"},
	"Foreign policy": {"diplomacy", "treaty", "summit", "un", "oas"},
}

func matchAny(text string, lex map[string][]string) []string {
	var hits []string
	for label, patterns := range lex {
		for _, p := range patterns {
			if strings.Contains(text, p) {
				hits = append(hits, label)
				break
			}
		}
	}
	return hits
}

var stopwords = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "or": {}, "to": {}, "of": {}, "in": {}, "on": {}, "for": {}, "with": {},
	"is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {}, "being": {}, "this": {}, "that": {}, "these": {}, "those": {},
	"what": {}, "who": {}, "where": {}, "when": {}, "why": {}, "how": {}, "latest": {}, "major": {}, "developments": {}, "development": {},
}

func extractKeywords(text string) []string {
	re := regexp.MustCompile(`[^\pL\pN]+`)
	raw := re.Split(text, -1)

	counts := map[string]int{}
	for _, tok := range raw {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if len([]rune(tok)) < 3 {
			continue
		}
		if _, ok := stopwords[tok]; ok {
			continue
		}
		counts[tok]++
	}

	type kv struct {
		k string
		v int
	}
	var all []kv
	for k, v := range counts {
		all = append(all, kv{k: k, v: v})
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].v == all[j].v {
			return all[i].k < all[j].k
		}
		return all[i].v > all[j].v
	})

	N := 12
	if len(all) < N {
		N = len(all)
	}
	out := make([]string, 0, N)
	for i := 0; i < N; i++ {
		out = append(out, all[i].k)
	}
	return out
}

func uniqueSorted(in []string) []string {
	m := map[string]struct{}{}
	for _, s := range in {
		m[s] = struct{}{}
	}
	out := make([]string, 0, len(m))
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// ===== Search Scope selection =====

type SearchScope int

const (
	ScopeAuto SearchScope = iota
	ScopeChosen
	ScopeGlobal
)

func selectSearchScope(r *bufio.Reader) (SearchScope, string, error) {
	for {
		fmt.Println("\nSearch scope:")
		fmt.Println("1) Auto-detect from text (default)")
		fmt.Println("2) Choose country")
		fmt.Println("3) Global (worldwide)")
		fmt.Print("> ")

		choice, _ := r.ReadString('\n')
		choice = strings.TrimSpace(choice)

		if choice == "" {
			return ScopeAuto, "", nil
		}

		switch choice {
		case "1":
			return ScopeAuto, "", nil
		case "2":
			fmt.Println("Enter country name (e.g. 'Bulgaria'):")
			fmt.Print("> ")
			c, _ := r.ReadString('\n')
			c = strings.TrimSpace(c)
			if c == "" {
				fmt.Println("Empty country, falling back to Auto.")
				return ScopeAuto, "", nil
			}
			return ScopeChosen, c, nil
		case "3":
			return ScopeGlobal, "", nil
		default:
			fmt.Println("Invalid choice. Please select 1-3.")
		}
	}
}

// ===== Time window selection =====

func selectTimeRange(r *bufio.Reader) (TimeRange, error) {
	now := time.Now()
	for {
		fmt.Println("\nSelect time window:")
		fmt.Println("1) Last 24 hours")
		fmt.Println("2) Last 7 days")
		fmt.Println("3) Last 30 days")
		fmt.Println("4) Custom (YYYY-MM-DD to YYYY-MM-DD)")
		fmt.Print("> ")

		choice, _ := r.ReadString('\n')
		choice = strings.TrimSpace(choice)

		switch choice {
		case "1":
			return TimeRange{From: now.Add(-24 * time.Hour), To: now, Label: "Last 24 hours"}, nil
		case "2":
			return TimeRange{From: now.AddDate(0, 0, -7), To: now, Label: "Last 7 days"}, nil
		case "3":
			return TimeRange{From: now.AddDate(0, 0, -30), To: now, Label: "Last 30 days"}, nil
		case "4":
			return readCustomRange(r)
		default:
			fmt.Println("Invalid choice. Please select 1–4.")
		}
	}
}

func readCustomRange(r *bufio.Reader) (TimeRange, error) {
	for {
		fmt.Print("From date (YYYY-MM-DD): ")
		fromStr, _ := r.ReadString('\n')
		fmt.Print("To date (YYYY-MM-DD): ")
		toStr, _ := r.ReadString('\n')

		fromStr = strings.TrimSpace(fromStr)
		toStr = strings.TrimSpace(toStr)

		from, err1 := time.Parse("2006-01-02", fromStr)
		to, err2 := time.Parse("2006-01-02", toStr)

		if err1 != nil || err2 != nil {
			fmt.Println("Invalid date format. Try again.")
			continue
		}
		if from.After(to) {
			fmt.Println("From date must be before To date.")
			continue
		}
		return TimeRange{From: from, To: to, Label: fmt.Sprintf("Custom (%s → %s)", fromStr, toStr)}, nil
	}
}

// ===== Input helpers =====

func readMultiline(r *bufio.Reader) (string, error) {
	var lines []string
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			line = strings.TrimRight(line, "\r\n")
			if strings.TrimSpace(line) != "" {
				lines = append(lines, line)
			}
			break
		}

		line = strings.TrimRight(line, "\r\n")
		trim := strings.TrimSpace(line)

		if trim == "" {
			if len(lines) > 0 {
				break
			}
			fmt.Print("> ")
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

var (
	reDigitsPunctOnly = regexp.MustCompile(`^[\d\pP\pS\s]+$`)
	reWordToken       = regexp.MustCompile(`\pL{3,}`)
)

func validateQuery(q string) (bool, string) {
	q = strings.TrimSpace(q)
	if q == "" {
		return false, "empty"
	}
	if reDigitsPunctOnly.MatchString(q) {
		return false, "no words detected"
	}
	if !reWordToken.MatchString(q) {
		return false, "no real word token found"
	}

	total := 0
	letters := 0
	for _, r := range q {
		if unicode.IsSpace(r) {
			continue
		}
		total++
		if unicode.IsLetter(r) {
			letters++
		}
	}
	if total == 0 {
		return false, "empty"
	}
	if float64(letters)/float64(total) < 0.30 {
		return false, "too many non-letter characters"
	}

	words := strings.Fields(q)
	if len(words) < 2 {
		if m := reWordToken.FindString(q); len([]rune(m)) >= 4 {
			return true, ""
		}
		return false, "too few words"
	}
	return true, ""
}

func mini(a, b int) int {
	if a < b {
		return a
	}
	return b
}
