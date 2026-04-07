package goddgs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSearchPagesDefaultsAndEarlyStop(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd="3-token"`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			calls++
			if calls == 1 {
				_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"One","u":"https://one.example","a":"s"}]);`))
				return
			}
			_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"x","u":"","a":""}]);`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL, DisableHTMLFallback: true, RetryMax: 1})
	res, err := c.SearchPages(context.Background(), "golang", 0, 0, SearchOptions{})
	if err != nil {
		t.Fatalf("SearchPages err: %v", err)
	}
	if len(res) != 1 || res[0].URL != "https://one.example" {
		t.Fatalf("unexpected results: %#v", res)
	}
	if calls < 1 {
		t.Fatal("expected at least one d.js call")
	}
}
