package geo

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type DatasetEntry struct {
	ISO2      string   `json:"iso2"`
	Languages []string `json:"languages"`
	Aliases   []string `json:"aliases"`
}

type DatasetResolver struct {
	byKey map[string]CountryInfo // normalized country/alias -> info
}

func NewDatasetResolver(datasetPath string) (*DatasetResolver, error) {
	data, err := os.ReadFile(filepath.Clean(datasetPath))
	if err != nil {
		return nil, err
	}

	raw := map[string]DatasetEntry{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	byKey := map[string]CountryInfo{}
	for name, e := range raw {
		info := CountryInfo{
			Name:      strings.TrimSpace(name),
			ISO2:      strings.ToUpper(strings.TrimSpace(e.ISO2)),
			Languages: normalizeLangs(e.Languages),
		}
		// main name
		byKey[normalizeKey(name)] = info
		// aliases
		for _, a := range e.Aliases {
			if strings.TrimSpace(a) == "" {
				continue
			}
			byKey[normalizeKey(a)] = info
		}
	}

	return &DatasetResolver{byKey: byKey}, nil
}

func (d *DatasetResolver) ResolveCountry(ctx context.Context, name string) (CountryInfo, error) {
	_ = ctx
	key := normalizeKey(name)
	if key == "" {
		return CountryInfo{}, errors.New("empty country name")
	}
	if v, ok := d.byKey[key]; ok {
		return v, nil
	}
	return CountryInfo{}, errors.New("not found in dataset")
}

func normalizeLangs(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			continue
		}
		// keep short codes like "en", "fr", "de", "pl", "hu"
		if len(s) < 2 || len(s) > 8 {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
