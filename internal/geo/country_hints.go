package geo

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	reToken = regexp.MustCompile(`[\pL\pM][\pL\pM'\-]{1,}`) // word-ish tokens
)

// ExtractCountryHints tries to pull likely country name candidates from a query.
// Example: "Madagascar elections" -> ["Madagascar"]
// Example: "latest political developments in South Africa" -> ["South Africa"]
func ExtractCountryHints(q string) []string {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil
	}

	// Strategy:
	// - Prefer sequences of Capitalized words: "South Africa", "United Kingdom"
	// - Otherwise fall back to the first long token

	// Tokenize (preserve original casing)
	rawTokens := reToken.FindAllString(q, -1)
	if len(rawTokens) == 0 {
		return nil
	}

	type span struct {
		start int
		end   int
	}
	spans := []span{}

	// Build spans of consecutive capitalized tokens
	i := 0
	for i < len(rawTokens) {
		if isCapWord(rawTokens[i]) {
			j := i + 1
			for j < len(rawTokens) && isCapWord(rawTokens[j]) {
				j++
			}
			// span i..j-1
			if j-i >= 1 {
				spans = append(spans, span{start: i, end: j})
			}
			i = j
		} else {
			i++
		}
	}

	candidates := []string{}
	seen := map[string]struct{}{}

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		candidates = append(candidates, s)
	}

	// Add longest spans first ("South Africa" before "Africa")
	// (simple: iterate spans and add multi-word first)
	for _, sp := range spans {
		if sp.end-sp.start >= 2 {
			add(strings.Join(rawTokens[sp.start:sp.end], " "))
		}
	}
	for _, sp := range spans {
		if sp.end-sp.start == 1 {
			add(rawTokens[sp.start])
		}
	}

	// Fallback: first long token (>=4 letters)
	if len(candidates) == 0 {
		for _, t := range rawTokens {
			if len([]rune(t)) >= 4 {
				add(t)
				break
			}
		}
	}

	// Keep top few
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	return candidates
}

func isCapWord(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	r := []rune(s)
	// skip leading punctuation
	k := 0
	for k < len(r) && (unicode.IsPunct(r[k]) || unicode.IsSymbol(r[k])) {
		k++
	}
	if k >= len(r) {
		return false
	}
	return unicode.IsUpper(r[k])
}
