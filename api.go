package goddgs

import "time"

// SearchRequest is the provider-agnostic input contract.
type SearchRequest struct {
	Query      string
	MaxResults int
	Region     string
	SafeSearch SafeSearch
	TimeRange  string
	Offset     int
}

// SearchResponse is the provider-agnostic output contract.
type SearchResponse struct {
	Results      []Result
	Provider     string
	FallbackUsed bool
	Diagnostics  Diagnostics
}

// Diagnostics contains execution metadata for observability and debugging.
type Diagnostics struct {
	BlockInfo     *BlockInfo
	Attempts      int
	ProviderChain []string
	Timings       map[string]time.Duration
	Errors        []ProviderError
}

// ProviderError captures one provider failure in the chain.
type ProviderError struct {
	Provider string            `json:"provider"`
	Kind     ErrorKind         `json:"kind"`
	Message  string            `json:"message"`
	Details  map[string]string `json:"details,omitempty"`
}

func (r SearchRequest) toSearchOptions() SearchOptions {
	return SearchOptions{
		MaxResults: r.MaxResults,
		Region:     r.Region,
		SafeSearch: r.SafeSearch,
		TimeRange:  r.TimeRange,
		Offset:     r.Offset,
	}
}
