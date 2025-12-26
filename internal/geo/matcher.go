package geo

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CountryMatcher finds country mentions using the dataset file.
// It supports multiple matches in one query.
type CountryMatcher struct {
	phrases []string          // normalized phrases, sorted by length desc
	toCanon map[string]string // phrase -> canonical name
}

func NewCountryMatcher(datasetPath string) (*CountryMatcher, error) {
	data, err := os.ReadFile(filepath.Clean(datasetPath))
	if err != nil {
		return nil, err
	}

	// country_languages.json format:
	// {
	//   "Canada": {"iso2":"CA","languages":["en","fr"],"aliases":[...]} ,
	//   ...
	// }
	raw := map[string]DatasetEntry{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	toCanon := map[string]string{}
	phrases := make([]string, 0, len(raw)*2)

	for canon, entry := range raw {
		canon = strings.TrimSpace(canon)
		if canon == "" || strings.TrimSpace(entry.ISO2) == "" {
			continue
		}

		add := func(s string) {
			s = strings.TrimSpace(s)
			if s == "" {
				return
			}
			k := normalizeKey(s)
			if k == "" {
				return
			}
			if _, exists := toCanon[k]; !exists {
				toCanon[k] = canon
				phrases = append(phrases, k)
			}
		}

		add(canon)
		for _, a := range entry.Aliases {
			add(a)
		}
	}

	// Prefer longer phrases first to avoid "United" matching before "United States"
	sort.Slice(phrases, func(i, j int) bool {
		if len(phrases[i]) == len(phrases[j]) {
			return phrases[i] < phrases[j]
		}
		return len(phrases[i]) > len(phrases[j])
	})

	return &CountryMatcher{phrases: phrases, toCanon: toCanon}, nil
}

func (m *CountryMatcher) FindCountries(text string) []string {
	t := " " + normalizeKey(text) + " "
	seen := map[string]struct{}{}
	out := []string{}

	for _, p := range m.phrases {
		needle := " " + p + " "
		if strings.Contains(t, needle) {
			canon := m.toCanon[p]
			if _, ok := seen[canon]; ok {
				continue
			}
			seen[canon] = struct{}{}
			out = append(out, canon)
		}
	}
	return out
}
