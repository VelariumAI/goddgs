package goddgs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider is a search backend that can be chained by Engine.
type Provider interface {
	Name() string
	Enabled() bool
	Search(ctx context.Context, req SearchRequest) ([]Result, error)
}

type DDGProvider struct {
	client *Client
}

func NewDDGProvider(client *Client) *DDGProvider { return &DDGProvider{client: client} }
func (p *DDGProvider) Name() string              { return "ddg" }
func (p *DDGProvider) Enabled() bool             { return p != nil && p.client != nil }
func (p *DDGProvider) Search(ctx context.Context, req SearchRequest) ([]Result, error) {
	if p == nil || p.client == nil {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: "ddg", Cause: fmt.Errorf("nil ddg client")}
	}
	if strings.TrimSpace(req.Query) == "" {
		return nil, &SearchError{Kind: ErrKindInvalidInput, Provider: "ddg", Cause: fmt.Errorf("query is required")}
	}
	results, err := p.client.Search(ctx, req.Query, req.toSearchOptions())
	if err != nil {
		return nil, classifyError("ddg", err)
	}
	if len(results) == 0 {
		return nil, &SearchError{Kind: ErrKindNoResults, Provider: "ddg", Cause: ErrNoResults}
	}
	return results, nil
}

type BraveProvider struct {
	apiKey string
	hc     *http.Client
}

func NewBraveProvider(apiKey string, hc *http.Client) *BraveProvider {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &BraveProvider{apiKey: strings.TrimSpace(apiKey), hc: hc}
}
func (p *BraveProvider) Name() string  { return "brave" }
func (p *BraveProvider) Enabled() bool { return p != nil && p.apiKey != "" }

func (p *BraveProvider) Search(ctx context.Context, req SearchRequest) ([]Result, error) {
	if !p.Enabled() {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: false, Cause: fmt.Errorf("provider disabled")}
	}
	u, _ := url.Parse("https://api.search.brave.com/res/v1/web/search")
	q := u.Query()
	q.Set("q", req.Query)
	if req.MaxResults > 0 {
		q.Set("count", fmt.Sprintf("%d", req.MaxResults))
	}
	u.RawQuery = q.Encode()
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &SearchError{Kind: ErrKindInternal, Provider: p.Name(), Cause: err}
	}
	hreq.Header.Set("Accept", "application/json")
	hreq.Header.Set("X-Subscription-Token", p.apiKey)
	resp, err := p.hc.Do(hreq)
	if err != nil {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: true, Cause: err}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &SearchError{Kind: ErrKindRateLimited, Provider: p.Name(), Temporary: true, Cause: fmt.Errorf("http %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 500 {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: true, Cause: fmt.Errorf("http %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 400 {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: false, Cause: fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
	}
	var payload struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, &SearchError{Kind: ErrKindParse, Provider: p.Name(), Temporary: true, Cause: err}
	}
	out := make([]Result, 0, len(payload.Web.Results))
	for _, r := range payload.Web.Results {
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		out = append(out, Result{Title: strings.TrimSpace(r.Title), URL: strings.TrimSpace(r.URL), Snippet: strings.TrimSpace(r.Description)})
	}
	if len(out) == 0 {
		return nil, &SearchError{Kind: ErrKindNoResults, Provider: p.Name(), Cause: ErrNoResults}
	}
	return out, nil
}

type TavilyProvider struct {
	apiKey string
	hc     *http.Client
}

func NewTavilyProvider(apiKey string, hc *http.Client) *TavilyProvider {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &TavilyProvider{apiKey: strings.TrimSpace(apiKey), hc: hc}
}
func (p *TavilyProvider) Name() string  { return "tavily" }
func (p *TavilyProvider) Enabled() bool { return p != nil && p.apiKey != "" }
func (p *TavilyProvider) Search(ctx context.Context, req SearchRequest) ([]Result, error) {
	if !p.Enabled() {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: false, Cause: fmt.Errorf("provider disabled")}
	}
	payload := fmt.Sprintf(`{"api_key":%q,"query":%q,"max_results":%d}`, p.apiKey, req.Query, maxOrDefault(req.MaxResults, 10))
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", strings.NewReader(payload))
	if err != nil {
		return nil, &SearchError{Kind: ErrKindInternal, Provider: p.Name(), Cause: err}
	}
	hreq.Header.Set("Content-Type", "application/json")
	hreq.Header.Set("Accept", "application/json")
	resp, err := p.hc.Do(hreq)
	if err != nil {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: true, Cause: err}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &SearchError{Kind: ErrKindRateLimited, Provider: p.Name(), Temporary: true, Cause: fmt.Errorf("http %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 500 {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: true, Cause: fmt.Errorf("http %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 400 {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: false, Cause: fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
	}
	var parsed struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &SearchError{Kind: ErrKindParse, Provider: p.Name(), Temporary: true, Cause: err}
	}
	out := make([]Result, 0, len(parsed.Results))
	for _, r := range parsed.Results {
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		out = append(out, Result{Title: strings.TrimSpace(r.Title), URL: strings.TrimSpace(r.URL), Snippet: strings.TrimSpace(r.Content)})
	}
	if len(out) == 0 {
		return nil, &SearchError{Kind: ErrKindNoResults, Provider: p.Name(), Cause: ErrNoResults}
	}
	return out, nil
}

type SerpAPIProvider struct {
	apiKey string
	hc     *http.Client
}

func NewSerpAPIProvider(apiKey string, hc *http.Client) *SerpAPIProvider {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &SerpAPIProvider{apiKey: strings.TrimSpace(apiKey), hc: hc}
}
func (p *SerpAPIProvider) Name() string  { return "serpapi" }
func (p *SerpAPIProvider) Enabled() bool { return p != nil && p.apiKey != "" }
func (p *SerpAPIProvider) Search(ctx context.Context, req SearchRequest) ([]Result, error) {
	if !p.Enabled() {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: false, Cause: fmt.Errorf("provider disabled")}
	}
	u, _ := url.Parse("https://serpapi.com/search.json")
	q := u.Query()
	q.Set("engine", "duckduckgo")
	q.Set("q", req.Query)
	q.Set("api_key", p.apiKey)
	u.RawQuery = q.Encode()
	hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, &SearchError{Kind: ErrKindInternal, Provider: p.Name(), Cause: err}
	}
	resp, err := p.hc.Do(hreq)
	if err != nil {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: true, Cause: err}
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, &SearchError{Kind: ErrKindRateLimited, Provider: p.Name(), Temporary: true, Cause: fmt.Errorf("http %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 500 {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: true, Cause: fmt.Errorf("http %d", resp.StatusCode)}
	}
	if resp.StatusCode >= 400 {
		return nil, &SearchError{Kind: ErrKindProviderUnavailable, Provider: p.Name(), Temporary: false, Cause: fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))}
	}
	var parsed struct {
		Organic []struct {
			Title   string `json:"title"`
			Link    string `json:"link"`
			Snippet string `json:"snippet"`
		} `json:"organic_results"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, &SearchError{Kind: ErrKindParse, Provider: p.Name(), Temporary: true, Cause: err}
	}
	out := make([]Result, 0, len(parsed.Organic))
	for _, r := range parsed.Organic {
		if strings.TrimSpace(r.Link) == "" {
			continue
		}
		out = append(out, Result{Title: strings.TrimSpace(r.Title), URL: strings.TrimSpace(r.Link), Snippet: strings.TrimSpace(r.Snippet)})
	}
	if len(out) == 0 {
		return nil, &SearchError{Kind: ErrKindNoResults, Provider: p.Name(), Cause: ErrNoResults}
	}
	return out, nil
}

func maxOrDefault(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}
