package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

type RestCountriesResolver struct {
	Client *http.Client
}

func NewRestCountriesResolver() *RestCountriesResolver {
	return &RestCountriesResolver{
		Client: &http.Client{Timeout: 12 * time.Second},
	}
}

type rcCountry struct {
	Name struct {
		Common string `json:"common"`
	} `json:"name"`
	CCA2      string            `json:"cca2"`
	Languages map[string]string `json:"languages"`
}

func (r *RestCountriesResolver) ResolveCountry(ctx context.Context, name string) (CountryInfo, error) {
	q := strings.TrimSpace(name)
	if q == "" {
		return CountryInfo{}, errors.New("empty country name")
	}

	// Minimal fields for speed
	endpoint := fmt.Sprintf(
		"https://restcountries.com/v3.1/name/%s?fields=name,cca2,languages",
		url.PathEscape(q),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return CountryInfo{}, err
	}

	resp, err := r.Client.Do(req)
	if err != nil {
		return CountryInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return CountryInfo{}, errors.New("not found in api")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return CountryInfo{}, fmt.Errorf("api error: status %d", resp.StatusCode)
	}

	var results []rcCountry
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return CountryInfo{}, err
	}
	if len(results) == 0 {
		return CountryInfo{}, errors.New("not found in api")
	}

	// Pick best match:
	// 1) exact common name match (case-insensitive)
	// 2) otherwise first entry
	target := results[0]
	for _, c := range results {
		if strings.EqualFold(strings.TrimSpace(c.Name.Common), q) {
			target = c
			break
		}
	}

	langs := extractLangCodes(target.Languages)
	if len(langs) == 0 {
		// Sometimes the API might omit languages. Keep empty list, Hybrid will still add English baseline.
		langs = []string{}
	}

	info := CountryInfo{
		Name:      strings.TrimSpace(target.Name.Common),
		ISO2:      strings.ToUpper(strings.TrimSpace(target.CCA2)),
		Languages: langs,
	}

	if info.ISO2 == "" {
		return CountryInfo{}, errors.New("api returned empty iso2")
	}

	return info, nil
}

func extractLangCodes(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for code := range m {
		code = strings.ToLower(strings.TrimSpace(code))
		if code == "" {
			continue
		}
		// prefer ISO 639-1 style codes like "de", "pl", "hu"
		if len(code) < 2 || len(code) > 8 {
			continue
		}
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
