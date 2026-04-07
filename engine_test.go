package goddgs

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

type fakeProvider struct {
	name    string
	enabled bool
	fn      func(context.Context, SearchRequest) ([]Result, error)
}

func (p *fakeProvider) Name() string  { return p.name }
func (p *fakeProvider) Enabled() bool { return p.enabled }
func (p *fakeProvider) Search(ctx context.Context, req SearchRequest) ([]Result, error) {
	return p.fn(ctx, req)
}

func TestEngineFallbackFromBlockedToNextProvider(t *testing.T) {
	p1 := &fakeProvider{name: "ddg", enabled: true, fn: func(context.Context, SearchRequest) ([]Result, error) {
		return nil, &SearchError{Kind: ErrKindBlocked, Provider: "ddg", Temporary: true, Cause: ErrBlocked}
	}}
	p2 := &fakeProvider{name: "brave", enabled: true, fn: func(context.Context, SearchRequest) ([]Result, error) {
		return []Result{{Title: "ok", URL: "https://ok"}}, nil
	}}
	eng, err := NewEngine(EngineOptions{Providers: []Provider{p1, p2}})
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}
	res, err := eng.Search(context.Background(), SearchRequest{Query: "test", MaxResults: 5, Region: "us-en"})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if res.Provider != "brave" || !res.FallbackUsed || len(res.Results) != 1 {
		t.Fatalf("unexpected fallback result: %+v", res)
	}
	if res.Diagnostics.Attempts != 2 {
		t.Fatalf("attempts=%d, want 2", res.Diagnostics.Attempts)
	}
}

func TestEngineNoEnabledProviders(t *testing.T) {
	eng, err := NewEngine(EngineOptions{Providers: []Provider{&fakeProvider{name: "x", enabled: false, fn: func(context.Context, SearchRequest) ([]Result, error) {
		return nil, nil
	}}}})
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}
	_, serr := eng.Search(context.Background(), SearchRequest{Query: "q"})
	var se *SearchError
	if !errors.As(serr, &se) || se.Kind != ErrKindProviderUnavailable {
		t.Fatalf("expected provider_unavailable, got: %v", serr)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("GODDGS_BRAVE_API_KEY", "b")
	t.Setenv("GODDGS_TAVILY_API_KEY", "t")
	t.Setenv("GODDGS_SERPAPI_API_KEY", "s")
	t.Setenv("GODDGS_DDG_BASE", "https://ddg.example")
	t.Setenv("GODDGS_LINKS_BASE", "https://links.example")
	t.Setenv("GODDGS_HTML_BASE", "https://html.example")
	t.Setenv("GODDGS_PROVIDER_ORDER", "ddg,brave")
	t.Setenv("GODDGS_TIMEOUT", "7s")
	t.Setenv("GODDGS_MAX_RETRIES", "9")
	t.Setenv("GODDGS_DISABLE_HTML_FALLBACK", "true")
	cfg := LoadConfigFromEnv()
	if cfg.BraveAPIKey != "b" || cfg.TavilyAPIKey != "t" || cfg.SerpAPIKey != "s" {
		t.Fatalf("unexpected keys: %+v", cfg)
	}
	if cfg.DuckDuckGoBase != "https://ddg.example" || cfg.LinksBase != "https://links.example" || cfg.HTMLBase != "https://html.example" {
		t.Fatalf("unexpected ddg endpoints: %+v", cfg)
	}
	if len(cfg.ProviderOrder) != 2 || cfg.ProviderOrder[1] != "brave" {
		t.Fatalf("unexpected provider order: %+v", cfg.ProviderOrder)
	}
	if cfg.Timeout != 7*time.Second || cfg.MaxRetries != 9 || !cfg.DisableHTMLFallback {
		t.Fatalf("unexpected numeric/bool config: %+v", cfg)
	}
}

func TestRetryAfterSecondsHTTPDate(t *testing.T) {
	h := http.Header{}
	target := time.Now().Add(12 * time.Second).UTC().Format(httpTimeFormat)
	h.Set("Retry-After", target)
	secs := RetryAfterSeconds(h)
	if secs < 1 || secs > 20 {
		t.Fatalf("RetryAfterSeconds HTTP-date out of expected range: %d", secs)
	}
}

const httpTimeFormat = "Mon, 02 Jan 2006 15:04:05 GMT"
