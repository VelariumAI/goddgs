package goddgs

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// APIError is a stable HTTP error payload for goddgs service endpoints.
type APIError struct {
	Error string `json:"error"`
	Kind  string `json:"kind,omitempty"`
}

type serviceSearchRequest struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
	Region     string `json:"region"`
}

// NewHTTPHandler builds a production HTTP API handler.
// Endpoints: /healthz, /readyz, /v1/search and optional /metrics.
func NewHTTPHandler(engine *Engine, cfg Config, metricsHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if engine == nil || len(engine.EnabledProviders()) == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("no providers"))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	if metricsHandler != nil {
		mux.Handle("/metrics", metricsHandler)
	}
	mux.HandleFunc("/v1/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		if engine == nil {
			writeAPIErr(w, http.StatusBadGateway, APIError{Error: "engine unavailable", Kind: string(ErrKindProviderUnavailable)})
			return
		}
		var req serviceSearchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIErr(w, http.StatusBadRequest, APIError{Error: "invalid json", Kind: string(ErrKindInvalidInput)})
			return
		}
		if strings.TrimSpace(req.Query) == "" {
			writeAPIErr(w, http.StatusBadRequest, APIError{Error: "query is required", Kind: string(ErrKindInvalidInput)})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), cfg.Timeout)
		defer cancel()
		resp, err := engine.Search(ctx, SearchRequest{Query: req.Query, MaxResults: req.MaxResults, Region: req.Region})
		if err != nil {
			status := http.StatusBadGateway
			kind := ErrKindInternal
			if IsBlocked(err) {
				status = http.StatusTooManyRequests
				kind = ErrKindBlocked
			}
			if se, ok := err.(*SearchError); ok {
				kind = se.Kind
				switch se.Kind {
				case ErrKindInvalidInput:
					status = http.StatusBadRequest
				case ErrKindNoResults:
					status = http.StatusNotFound
				case ErrKindRateLimited, ErrKindBlocked:
					status = http.StatusTooManyRequests
				}
			}
			writeAPIErr(w, status, APIError{Error: err.Error(), Kind: string(kind)})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}

func writeAPIErr(w http.ResponseWriter, status int, b APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(b)
}
