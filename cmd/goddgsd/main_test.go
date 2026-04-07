package main

import (
	"net/http"
	"os"
	"testing"
)

func TestGetenv(t *testing.T) {
	t.Setenv("GODDGS_ADDR", "")
	if v := getenv("GODDGS_ADDR", ":8080"); v != ":8080" {
		t.Fatalf("getenv default mismatch: %q", v)
	}
	t.Setenv("GODDGS_ADDR", " :9090 ")
	if v := getenv("GODDGS_ADDR", ":8080"); v != ":9090" {
		t.Fatalf("getenv trimmed mismatch: %q", v)
	}
}

func TestRunStartStopPath(t *testing.T) {
	stop := make(chan os.Signal, 1)
	stop <- os.Interrupt

	oldServe := serveFn
	serveFn = func(_ *http.Server) error { return http.ErrServerClosed }
	defer func() { serveFn = oldServe }()

	t.Setenv("GODDGS_DDG_BASE", "http://127.0.0.1:1")
	t.Setenv("GODDGS_LINKS_BASE", "http://127.0.0.1:1")
	t.Setenv("GODDGS_HTML_BASE", "http://127.0.0.1:1")
	t.Setenv("GODDGS_ADDR", ":0")
	if err := run(stop); err != nil {
		t.Fatalf("run error: %v", err)
	}
}
