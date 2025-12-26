package geo

import (
	"regexp"
	"strings"
	"unicode"
)

// Heuristic: detect likely country phrases from user query.
// We avoid calling the API for random words.
var (
	reMultiSpace  = regexp.MustCompile(`\s+`)
	reWordLike    = regexp.MustCompile(`[\pL]{3,}`) // at least one real word token
	reBadAllPunct = regexp.MustCompile(`^[\d\pP\pS\s]+$`)
)

func LooksResolvableCountryQuery(q string) bool {
	q = strings.TrimSpace(q)
	if q == "" {
		return false
	}
	if reBadAllPunct.MatchString(q) {
		return false
	}
	if !reWordLike.MatchString(q) {
		return false
	}

	// If the query has at least one capitalized token OR contains common country markers, we’ll allow API attempt.
	// (Works well for “Madagascar elections”, “Hungary politics”, etc.)
	if hasCapitalizedToken(q) {
		return true
	}

	l := strings.ToLower(q)
	if strings.Contains(l, " in ") || strings.Contains(l, " from ") || strings.Contains(l, " for ") {
		return true
	}

	// As fallback, allow if query is short-ish (user likely typed “Country + topic”)
	compact := reMultiSpace.ReplaceAllString(q, " ")
	if len([]rune(compact)) <= 40 {
		return true
	}
	return false
}

func hasCapitalizedToken(s string) bool {
	// Very simple: any token that starts with an uppercase letter and has >=3 letters
	toks := strings.Fields(s)
	for _, t := range toks {
		r := []rune(t)
		if len(r) < 3 {
			continue
		}
		// strip punctuation around token
		t = strings.TrimFunc(t, func(r rune) bool {
			return unicode.IsPunct(r) || unicode.IsSymbol(r)
		})
		r = []rune(t)
		if len(r) < 3 {
			continue
		}
		if unicode.IsUpper(r[0]) {
			// ensure it has letters
			hasLetter := false
			for _, x := range r {
				if unicode.IsLetter(x) {
					hasLetter = true
					break
				}
			}
			if hasLetter {
				return true
			}
		}
	}
	return false
}
