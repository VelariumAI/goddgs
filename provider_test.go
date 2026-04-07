package goddgs

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

type rewriteRoundTripper struct {
	base   http.RoundTripper
	target *url.URL
}

func (r rewriteRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.URL.Scheme = r.target.Scheme
	cloned.URL.Host = r.target.Host
	cloned.Host = r.target.Host
	return r.base.RoundTrip(cloned)
}

func newRewrittenClient(server *httptest.Server) *http.Client {
	u, _ := url.Parse(server.URL)
	base := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // test-only
	return &http.Client{Transport: rewriteRoundTripper{base: base, target: u}}
}

func TestBraveProviderSuccess(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "k" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":"bad key"}`)
			return
		}
		_, _ = io.WriteString(w, `{"web":{"results":[{"title":"Go","url":"https://go.dev","description":"Golang"}]}}`)
	}))
	defer srv.Close()

	p := NewBraveProvider("k", newRewrittenClient(srv))
	res, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 2})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(res) != 1 || res[0].URL != "https://go.dev" {
		t.Fatalf("unexpected results: %+v", res)
	}
}

func TestTavilyProviderSuccess(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.WriteString(w, `{"results":[{"title":"Go","url":"https://go.dev","content":"docs"}]}`)
	}))
	defer srv.Close()

	p := NewTavilyProvider("k", newRewrittenClient(srv))
	res, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 2})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(res) != 1 || res[0].Snippet != "docs" {
		t.Fatalf("unexpected results: %+v", res)
	}
}

func TestSerpAPIProviderSuccess(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"organic_results":[{"title":"Go","link":"https://go.dev","snippet":"docs"}]}`)
	}))
	defer srv.Close()

	p := NewSerpAPIProvider("k", newRewrittenClient(srv))
	res, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 2})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(res) != 1 || res[0].Title != "Go" {
		t.Fatalf("unexpected results: %+v", res)
	}
}

func TestProviderRateLimitedClassification(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `rate`)
	}))
	defer srv.Close()

	for _, tc := range []struct {
		name string
		p    Provider
	}{
		{"brave", NewBraveProvider("k", newRewrittenClient(srv))},
		{"tavily", NewTavilyProvider("k", newRewrittenClient(srv))},
		{"serpapi", NewSerpAPIProvider("k", newRewrittenClient(srv))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.p.Search(context.Background(), SearchRequest{Query: "q"})
			var se *SearchError
			if err == nil || !errorAsSearch(err, &se) || se.Kind != ErrKindRateLimited {
				t.Fatalf("expected rate limited SearchError, got %v", err)
			}
		})
	}
}

func TestProviderDisabled(t *testing.T) {
	for _, p := range []Provider{NewBraveProvider("", nil), NewTavilyProvider("", nil), NewSerpAPIProvider("", nil)} {
		if p.Enabled() {
			t.Fatalf("provider %s should be disabled without key", p.Name())
		}
		_, err := p.Search(context.Background(), SearchRequest{Query: "x"})
		if err == nil || !strings.Contains(err.Error(), "provider disabled") {
			t.Fatalf("expected provider disabled error, got: %v", err)
		}
	}
}

func errorAsSearch(err error, target **SearchError) bool {
	if err == nil {
		return false
	}
	se, ok := err.(*SearchError)
	if ok {
		*target = se
		return true
	}
	return false
}

func TestRewriterRoundTripKeepsPathAndQuery(t *testing.T) {
	hit := ""
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = r.URL.Path + "?" + r.URL.RawQuery
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	client := newRewrittenClient(srv)
	req, _ := http.NewRequest(http.MethodGet, "https://api.search.brave.com/res/v1/web/search?q=go", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do error: %v", err)
	}
	resp.Body.Close()
	if hit != "/res/v1/web/search?q=go" {
		t.Fatalf("unexpected rewritten path/query: %s", hit)
	}
}

func ExampleNewDefaultEngineFromConfig() {
	cfg := Config{ProviderOrder: []string{"ddg"}, Timeout: 5_000_000_000, MaxRetries: 2}
	eng, err := NewDefaultEngineFromConfig(cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(len(eng.EnabledProviders()) > 0)
	// Output: true
}
