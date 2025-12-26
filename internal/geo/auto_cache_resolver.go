package geo

import "context"

type AutoCacheResolver struct {
	store *AutoCacheStore
	next  CountryResolver
}

func NewAutoCacheResolver(store *AutoCacheStore, next CountryResolver) *AutoCacheResolver {
	return &AutoCacheResolver{store: store, next: next}
}

func (r *AutoCacheResolver) ResolveCountry(ctx context.Context, name string) (CountryInfo, error) {
	// Check auto-cache by the exact name key we stored
	if e, ok := r.store.Get(name); ok && e.ISO2 != "" && len(e.Languages) > 0 {
		return CountryInfo{
			Name:      name,
			ISO2:      e.ISO2,
			Languages: normalizeLangs(e.Languages),
		}, nil
	}

	// Fallback to next resolver (API)
	info, err := r.next.ResolveCountry(ctx, name)
	if err != nil {
		return CountryInfo{}, err
	}

	// Write-through cache
	_ = r.store.Upsert(info.Name, DatasetEntry{
		ISO2:      info.ISO2,
		Languages: info.Languages,
		Aliases:   []string{},
	})

	return info, nil
}
