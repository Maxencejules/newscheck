package geo

import (
	"sort"
	"strings"
)

type DiscoveryTarget struct {
	ISO2 string // "HU"
	Lang string // Google News language code, usually ISO-639-1 like "hu"
}

// toGoogleNewsLang normalizes language codes for Google News.
// RestCountries may return ISO-639-3 codes (ex: "bul"), but Google News expects ISO-639-1 (ex: "bg").
// This is not meant to be exhaustive; it covers common ISO-639-3 -> ISO-639-1 cases.
func toGoogleNewsLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "bul":
		return "bg"
	case "zho", "chi":
		return "zh"
	case "jpn":
		return "ja"
	case "kor":
		return "ko"
	case "ron", "rum":
		return "ro"
	case "ces", "cze":
		return "cs"
	case "deu", "ger":
		return "de"
	case "fra", "fre":
		return "fr"
	case "spa":
		return "es"
	case "por":
		return "pt"
	case "nld", "dut":
		return "nl"
	case "pol":
		return "pl"
	case "hun":
		return "hu"
	case "ukr":
		return "uk"
	case "srp":
		return "sr"
	case "hrv":
		return "hr"
	case "slk", "slo":
		return "sk"
	case "slv":
		return "sl"
	case "lit":
		return "lt"
	case "lav":
		return "lv"
	case "est":
		return "et"
	case "ell", "gre":
		return "el"
	case "tur":
		return "tr"
	}
	return code
}

func BuildDiscoveryTargets(country CountryInfo, includeEnglish bool) []DiscoveryTarget {
	iso2 := strings.ToUpper(strings.TrimSpace(country.ISO2))
	if iso2 == "" {
		return nil
	}

	seen := map[string]struct{}{}
	langs := make([]string, 0, len(country.Languages)+1)

	add := func(l string) {
		l = toGoogleNewsLang(l)
		if l == "" {
			return
		}
		if _, ok := seen[l]; ok {
			return
		}
		seen[l] = struct{}{}
		langs = append(langs, l)
	}

	for _, l := range country.Languages {
		add(l)
	}
	if includeEnglish {
		add("en")
	}

	sort.Strings(langs)

	out := make([]DiscoveryTarget, 0, len(langs))
	for _, l := range langs {
		out = append(out, DiscoveryTarget{ISO2: iso2, Lang: l})
	}
	return out
}

// BuildGoogleNewsParams generates hl/gl/ceid generically from ISO2 + language.
// Example: ISO2=HU, lang=hu -> hl=hu-HU, gl=HU, ceid=HU:hu
func BuildGoogleNewsParams(iso2, lang string) (hl, gl, ceid string) {
	iso2 = strings.ToUpper(strings.TrimSpace(iso2))
	lang = toGoogleNewsLang(lang)
	if iso2 == "" || lang == "" {
		return "", "", ""
	}
	return lang + "-" + iso2, iso2, iso2 + ":" + lang
}
