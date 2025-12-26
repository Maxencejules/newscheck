package discovery

// Simple starter profiles.
// You can tweak these anytime (HL/GL/CEID influence what Google News returns).

func DefaultLanguageProfiles() map[string]LanguageProfile {
	return map[string]LanguageProfile{
		"en": {Code: "en", HL: "en-CA", GL: "CA", CEID: "CA:en"},
		"fr": {Code: "fr", HL: "fr-CA", GL: "CA", CEID: "CA:fr"},
		"es": {Code: "es", HL: "es-419", GL: "US", CEID: "US:es-419"}, // Latin America Spanish
		"pt": {Code: "pt", HL: "pt-BR", GL: "BR", CEID: "BR:pt-419"},  // Portuguese (Brazil-heavy)
	}
}
