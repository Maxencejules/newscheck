package discovery

import "time"

type Candidate struct {
	Title       string
	URL         string
	Source      string
	PublishedAt time.Time
	FoundBy     string // which plan produced it
}

type Plan struct {
	Query string
	Scope string
}
