package discovery

import "time"

type Candidate struct {
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	Source         string    `json:"source"`
	PublishedAt    time.Time `json:"published_at"`
	FoundBy        string    `json:"found_by"`
	RelevanceScore int       `json:"relevance_score"`
	ConsensusScore int       `json:"consensus_score"`
}

type Plan struct {
	Query string
	Scope string
}
