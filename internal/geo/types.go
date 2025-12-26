package geo

import "context"

type CountryInfo struct {
	Name      string   `json:"name"`
	ISO2      string   `json:"iso2"`
	Languages []string `json:"languages"`
}

type Resolver interface {
	ResolveCountry(ctx context.Context, name string) (CountryInfo, error)
}
