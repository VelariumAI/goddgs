package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunFailurePath(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("GODDGS_TIMEOUT", "20ms")
	if err := run(); err == nil {
		t.Fatal("expected run() error under forced network failure")
	}
}

func TestRunSuccessPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd="3-test"`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Go","u":"https://go.dev","a":"lang"}]);`))
		case r.Method == http.MethodPost && r.URL.Path == "/html/":
			_, _ = w.Write([]byte(`<a class="result__a" href="https://go.dev">Go</a>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("GODDGS_DDG_BASE", srv.URL)
	t.Setenv("GODDGS_LINKS_BASE", srv.URL)
	t.Setenv("GODDGS_HTML_BASE", srv.URL)
	t.Setenv("GODDGS_TIMEOUT", "1s")
	if err := run(); err != nil {
		t.Fatalf("run success error: %v", err)
	}
}

func TestMainSuccessPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd="3-test"`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Go","u":"https://go.dev","a":"lang"}]);`))
		case r.Method == http.MethodPost && r.URL.Path == "/html/":
			_, _ = w.Write([]byte(`<a class="result__a" href="https://go.dev">Go</a>`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	t.Setenv("GODDGS_DDG_BASE", srv.URL)
	t.Setenv("GODDGS_LINKS_BASE", srv.URL)
	t.Setenv("GODDGS_HTML_BASE", srv.URL)
	t.Setenv("GODDGS_TIMEOUT", "1s")
	main()
}
