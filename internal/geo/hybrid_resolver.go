package geo

import (
	"context"
	"errors"
)

type HybridResolver struct {
	Cache   *Cache
	Dataset Resolver // optional
	API     Resolver // optional
}

func NewHybridResolver(cache *Cache, dataset Resolver, api Resolver) *HybridResolver {
	return &HybridResolver{
		Cache:   cache,
		Dataset: dataset,
		API:     api,
	}
}

func (h *HybridResolver) ResolveCountry(ctx context.Context, name string) (CountryInfo, error) {
	key := normalizeKey(name)
	if key == "" {
		return CountryInfo{}, errors.New("empty country name")
	}

	// Ensure cache is loaded once
	if h.Cache != nil {
		_ = h.Cache.Load()
		if v, ok := h.Cache.Get(key); ok {
			return v, nil
		}
	}

	// 1) dataset
	if h.Dataset != nil {
		if v, err := h.Dataset.ResolveCountry(ctx, name); err == nil {
			if h.Cache != nil {
				_ = h.Cache.Put(key, v)
			}
			return v, nil
		}
	}

	// 2) api fallback
	if h.API != nil {
		v, err := h.API.ResolveCountry(ctx, name)
		if err == nil {
			if h.Cache != nil {
				_ = h.Cache.Put(key, v)
			}
			return v, nil
		}
		return CountryInfo{}, err
	}

	return CountryInfo{}, errors.New("no resolver available")
}
