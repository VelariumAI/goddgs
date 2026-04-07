package goddgs

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestServiceHealthReady(t *testing.T) {
	p := &fakeProvider{name: "ddg", enabled: true, fn: func(_ context.Context, _ SearchRequest) ([]Result, error) {
		return []Result{{Title: "x", URL: "https://x"}}, nil
	}}
	engine, err := NewEngine(EngineOptions{Providers: []Provider{p}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	h := NewHTTPHandler(engine, Config{Timeout: 2 * time.Second}, nil)
	s := httptest.NewServer(h)
	defer s.Close()

	resp, err := http.Get(s.URL + "/healthz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz failed: %v code=%d", err, resp.StatusCode)
	}
	resp.Body.Close()
	resp, err = http.Get(s.URL + "/readyz")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz failed: %v code=%d", err, resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServiceSearchValidationAndSuccess(t *testing.T) {
	p := &fakeProvider{name: "ddg", enabled: true, fn: func(_ context.Context, _ SearchRequest) ([]Result, error) {
		return []Result{{Title: "Go", URL: "https://go.dev"}}, nil
	}}
	engine, err := NewEngine(EngineOptions{Providers: []Provider{p}})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	h := NewHTTPHandler(engine, Config{Timeout: 2 * time.Second}, nil)
	s := httptest.NewServer(h)
	defer s.Close()

	resp, err := http.Post(s.URL+"/v1/search", "application/json", bytes.NewBufferString("{"))
	if err != nil {
		t.Fatalf("post invalid json: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	body := map[string]any{"query": "golang", "max_results": 2, "region": "us-en"}
	raw, _ := json.Marshal(body)
	resp, err = http.Post(s.URL+"/v1/search", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("post search: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var parsed SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	resp.Body.Close()
	if parsed.Provider != "ddg" || len(parsed.Results) != 1 {
		t.Fatalf("unexpected response: %+v", parsed)
	}
}
