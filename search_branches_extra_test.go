package goddgs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchNoFallbackNoResultsBranch(t *testing.T) {
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
	_, err := c.Search(context.Background(), "golang", SearchOptions{MaxResults: 3})
	if err == nil || err != ErrNoResults {
		t.Fatalf("expected ErrNoResults, got %v", err)
	}
}

func TestSearchDJSAndHTMLBothFailBranch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd="3-token"`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			_, _ = w.Write([]byte(`not a djs payload`))
		case r.Method == http.MethodPost && r.URL.Path == "/html/":
			_, _ = w.Write([]byte(`<html><body>no results</body></html>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL, RetryMax: 1})
	_, err := c.Search(context.Background(), "golang", SearchOptions{MaxResults: 3})
	if err == nil || !strings.Contains(err.Error(), "html fallback failed") {
		t.Fatalf("expected combined fallback error, got %v", err)
	}
}
