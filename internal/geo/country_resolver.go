package geo

import "context"

// CountryResolver is the common interface implemented by:
// - DatasetResolver
// - RestCountriesResolver
// - HybridResolver
// - AutoCacheResolver
type CountryResolver interface {
	ResolveCountry(ctx context.Context, name string) (CountryInfo, error)
}
