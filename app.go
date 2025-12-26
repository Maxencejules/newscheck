package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"newscheck/internal/app"
	"newscheck/internal/discovery"
	"newscheck/internal/extract"
)

// App struct
type App struct {
	ctx     context.Context
	service *app.Service
}

// NewApp creates a new App application struct
func NewApp() *App {
	svc, err := app.NewService()
	if err != nil {
		fmt.Printf("Error initializing service: %v\n", err)
	}
	return &App{
		service: svc,
	}
}

// startup is called when the app starts. The context is saved
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// SearchParams exposed to frontend
type SearchParams struct {
	Query         string `json:"query"`
	Days          int    `json:"days"` // 1, 7, 30, or -1 (Custom)
	CustomFrom    string `json:"customFrom"` // YYYY-MM-DD
	CustomTo      string `json:"customTo"`   // YYYY-MM-DD
	Scope         int    `json:"scope"` // 0=Auto, 1=Chosen, 2=Global
	ChosenCountry string `json:"chosenCountry"`
	PivotLang     string `json:"pivotLang"`
}

// Search calls the backend service
func (a *App) Search(p SearchParams) (*app.SearchResult, error) {
	if a.service == nil {
		return nil, fmt.Errorf("backend service not initialized")
	}

	var from, to time.Time

	if p.Days == -1 {
		// Custom range
		var err error
		from, err = time.Parse("2006-01-02", p.CustomFrom)
		if err != nil {
			return nil, fmt.Errorf("invalid custom from date: %w", err)
		}
		to, err = time.Parse("2006-01-02", p.CustomTo)
		if err != nil {
			return nil, fmt.Errorf("invalid custom to date: %w", err)
		}
		// Adjust 'to' to end of day if it's just a date, or keep as is.
		// Usually discovery works better if 'to' is inclusive of the day.
		to = to.Add(23*time.Hour + 59*time.Minute)
	} else {
		// Standard ranges
		to = time.Now()
		from = to.AddDate(0, 0, -p.Days)
		if p.Days == 1 {
			from = to.Add(-24 * time.Hour)
		}
	}

	req := app.SearchRequest{
		Query:         p.Query,
		From:          from,
		To:            to,
		Scope:         app.SearchScope(p.Scope),
		ChosenCountry: p.ChosenCountry,
		PivotLang:     p.PivotLang,
	}

	return a.service.Search(a.ctx, req)
}

// ExtractParams exposed to frontend
type ExtractParams struct {
	URLs      []string `json:"urls"`
	PivotLang string   `json:"pivotLang"`
	Query     string   `json:"query"`
	ApiKey    string   `json:"apiKey"`
}

type ExtractResult struct {
	Articles []extract.Article `json:"articles"`
	Summary  string            `json:"summary"`
}

func (a *App) ExtractAndSummarize(p ExtractParams) (*ExtractResult, error) {
	if a.service == nil {
		return nil, fmt.Errorf("backend service not initialized")
	}
	articles, summary, err := a.service.ExtractAndSummarize(a.ctx, p.URLs, p.PivotLang, p.Query, p.ApiKey)
	if err != nil {
		return nil, err
	}
	return &ExtractResult{Articles: articles, Summary: summary}, nil
}

func (a *App) SaveArticleReport(articles []extract.Article) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "articles_report.docx",
		Title:           "Save Article Report",
		Filters: []runtime.FileFilter{
			{DisplayName: "Word Documents (*.docx)", Pattern: "*.docx"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // User cancelled
	}

	err = a.service.GenerateArticleReport(path, articles)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) SaveScoresReport(candidates []discovery.Candidate) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "scores_report.docx",
		Title:           "Save Scores Report",
		Filters: []runtime.FileFilter{
			{DisplayName: "Word Documents (*.docx)", Pattern: "*.docx"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // User cancelled
	}

	err = a.service.GenerateScoresReport(path, candidates)
	if err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) SaveResumeReport(summary string, query string, articles []extract.Article) (string, error) {
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: "resume_report.docx",
		Title:           "Save Resume Report",
		Filters: []runtime.FileFilter{
			{DisplayName: "Word Documents (*.docx)", Pattern: "*.docx"},
		},
	})
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil // User cancelled
	}

	err = a.service.GenerateResumeReport(path, summary, query, articles)
	if err != nil {
		return "", err
	}
	return path, nil
}
