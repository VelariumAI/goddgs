package goddgs

import (
	"context"
	"testing"
	"time"
)

func TestNewCircuitBreakerDefaults(t *testing.T) {
	cb := newCircuitBreaker(0, 0)
	s := cb.Snapshot()
	if s.Threshold != 5 || s.Cooldown != 60*time.Second {
		t.Fatalf("unexpected defaults: %+v", s)
	}
}

func TestChallengeTypeVariants(t *testing.T) {
	if got := challengeType([]byte("recaptcha/enterprise"), BlockSignalGeneric); got != "recaptchav3" {
		t.Fatalf("got %q", got)
	}
	if got := challengeType([]byte("turnstile"), BlockSignalCloudflare); got != "turnstile" {
		t.Fatalf("got %q", got)
	}
	if got := challengeType([]byte("google.com/recaptcha"), BlockSignalGeneric); got != "recaptchav2" {
		t.Fatalf("got %q", got)
	}
}

func TestEngineEmitNilHook(t *testing.T) {
	p := &fakeProvider{name: "ddg", enabled: true, fn: func(context.Context, SearchRequest) ([]Result, error) {
		return []Result{{Title: "x", URL: "https://x"}}, nil
	}}
	eng, err := NewEngine(EngineOptions{Providers: []Provider{p}, Hooks: []EventHook{nil}})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	if _, err := eng.Search(context.Background(), SearchRequest{Query: "ok"}); err != nil {
		t.Fatalf("search error: %v", err)
	}
}
