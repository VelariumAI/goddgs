package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	_ = r.Close()
	return buf.String()
}

func TestUsagePrints(t *testing.T) {
	out := captureStdout(t, usage)
	if out == "" {
		t.Fatal("expected usage output")
	}
}

func TestRunDoctorFailurePath(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("GODDGS_TIMEOUT", "20ms")
	code := runDoctor()
	if code != 3 {
		t.Fatalf("runDoctor code=%d want 3", code)
	}
}

func TestRunSearchFailurePath(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("GODDGS_TIMEOUT", "20ms")
	code := runSearch([]string{"--q", "golang"})
	if code != 2 {
		t.Fatalf("runSearch code=%d want 2", code)
	}
}

func newDDGTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
}

func TestRunSearchSuccessPath(t *testing.T) {
	srv := newDDGTestServer()
	defer srv.Close()
	t.Setenv("GODDGS_DDG_BASE", srv.URL)
	t.Setenv("GODDGS_LINKS_BASE", srv.URL)
	t.Setenv("GODDGS_HTML_BASE", srv.URL)
	t.Setenv("GODDGS_TIMEOUT", "1s")
	if code := runSearch([]string{"--q", "golang", "--json"}); code != 0 {
		t.Fatalf("runSearch success code=%d want 0", code)
	}
}

func TestRunDoctorSuccessPath(t *testing.T) {
	srv := newDDGTestServer()
	defer srv.Close()
	t.Setenv("GODDGS_DDG_BASE", srv.URL)
	t.Setenv("GODDGS_LINKS_BASE", srv.URL)
	t.Setenv("GODDGS_HTML_BASE", srv.URL)
	t.Setenv("GODDGS_TIMEOUT", "1s")
	if code := runDoctor(); code != 0 {
		t.Fatalf("runDoctor success code=%d want 0", code)
	}
}
