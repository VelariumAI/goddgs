package goddgs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDDGProviderSuccessNoResultsAndBlocked(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/":
				_, _ = w.Write([]byte(`vqd="3-token"`))
			case r.Method == http.MethodGet && r.URL.Path == "/d.js":
				_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Go","u":"https://go.dev","a":"lang"}]);`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL, DisableHTMLFallback: true, RetryMax: 1, RequestTimeout: 2 * time.Second})
		p := NewDDGProvider(c)
		res, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 1, Region: "us-en"})
		if err != nil {
			t.Fatalf("search err: %v", err)
		}
		if len(res) != 1 || res[0].URL != "https://go.dev" {
			t.Fatalf("unexpected results: %#v", res)
		}
	})

	t.Run("no_results", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/":
				_, _ = w.Write([]byte(`vqd="3-token"`))
			case r.Method == http.MethodGet && r.URL.Path == "/d.js":
				_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"x","u":"","a":""}]);`))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL, DisableHTMLFallback: true, RetryMax: 1})
		p := NewDDGProvider(c)
		_, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 1, Region: "us-en"})
		se, ok := err.(*SearchError)
		if !ok || se.Kind != ErrKindNoResults {
			t.Fatalf("expected no results SearchError, got %v", err)
		}
	})

	t.Run("blocked", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/":
				_, _ = w.Write([]byte(`vqd="3-token"`))
			case r.Method == http.MethodGet && r.URL.Path == "/d.js":
				w.Header().Set("cf-mitigated", "challenge")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("challenge"))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer srv.Close()

		c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL, DisableHTMLFallback: true, RetryMax: 1})
		p := NewDDGProvider(c)
		_, err := p.Search(context.Background(), SearchRequest{Query: "golang", MaxResults: 1, Region: "us-en"})
		se, ok := err.(*SearchError)
		if !ok || se.Kind != ErrKindBlocked {
			t.Fatalf("expected blocked SearchError, got %v", err)
		}
		if se.Details == nil || se.Details["detector"] == "" {
			t.Fatalf("expected detector details, got %#v", se)
		}
	})
}
