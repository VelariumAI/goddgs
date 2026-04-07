package goddgs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestBlockedErrorFormatting(t *testing.T) {
	if (&BlockedError{}).Error() == "" {
		t.Fatal("empty blocked error")
	}
	e := &BlockedError{Event: BlockedEvent{StatusCode: 403, Detector: "cf"}}
	if e.Error() == "" || !IsBlocked(e) {
		t.Fatalf("unexpected blocked error behavior: %v", e)
	}
}

func TestBlockSignalStringAllValues(t *testing.T) {
	cases := []BlockSignal{BlockSignalNone, BlockSignalCloudflare, BlockSignalReCAPTCHA, BlockSignalAkamai, BlockSignalPerimeterX, BlockSignalDataDome, BlockSignalGeneric}
	for _, c := range cases {
		if c.String() == "" {
			t.Fatalf("empty string for %v", c)
		}
	}
}

func TestObservabilityHookAndProviderGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	c := NewPrometheusCollector(reg)
	c.SetProviderEnabled("ddg", true)
	if got := testutil.ToFloat64(c.providerUp.WithLabelValues("ddg")); got != 1 {
		t.Fatalf("providerUp=%.0f want 1", got)
	}
	c.SetProviderEnabled("ddg", false)
	if got := testutil.ToFloat64(c.providerUp.WithLabelValues("ddg")); got != 0 {
		t.Fatalf("providerUp=%.0f want 0", got)
	}

	c.Hook(Event{Type: EventProviderEnd, Provider: "ddg", Duration: 10 * time.Millisecond, Success: true})
	c.Hook(Event{Type: EventProviderEnd, Provider: "ddg", Duration: 20 * time.Millisecond, Success: false})
	c.Hook(Event{Type: EventBlocked, Provider: "ddg", Block: &BlockInfo{Signal: BlockSignalCloudflare}})
	c.Hook(Event{Type: EventFallback, Provider: "ddg", ErrKind: ErrKindRateLimited})

	if got := testutil.ToFloat64(c.requestsTotal.WithLabelValues("ddg", "success")); got != 1 {
		t.Fatalf("success count=%.0f", got)
	}
	if got := testutil.ToFloat64(c.requestsTotal.WithLabelValues("ddg", "error")); got != 1 {
		t.Fatalf("error count=%.0f", got)
	}
	if got := testutil.ToFloat64(c.blocksTotal.WithLabelValues("ddg", "cloudflare")); got != 1 {
		t.Fatalf("blocks count=%.0f", got)
	}
	if got := testutil.ToFloat64(c.fallbacks.WithLabelValues("ddg", string(ErrKindRateLimited))); got != 1 {
		t.Fatalf("fallbacks count=%.0f", got)
	}
}

func TestServiceAdditionalBranches(t *testing.T) {
	// Ready with nil engine.
	h := NewHTTPHandler(nil, Config{Timeout: 20 * time.Millisecond}, nil)
	s := httptest.NewServer(h)
	defer s.Close()

	resp, _ := http.Get(s.URL + "/readyz")
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("ready nil engine code=%d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Get(s.URL + "/v1/search")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("method code=%d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, _ = http.Post(s.URL+"/v1/search", "application/json", bytes.NewBufferString(`{"query":"x"}`))
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("nil engine search code=%d", resp.StatusCode)
	}
	resp.Body.Close()

	// Empty query branch.
	p := &fakeProvider{name: "ddg", enabled: true, fn: func(context.Context, SearchRequest) ([]Result, error) { return nil, nil }}
	eng, _ := NewEngine(EngineOptions{Providers: []Provider{p}})
	h2 := NewHTTPHandler(eng, Config{Timeout: 20 * time.Millisecond}, nil)
	s2 := httptest.NewServer(h2)
	defer s2.Close()
	resp, _ = http.Post(s2.URL+"/v1/search", "application/json", bytes.NewBufferString(`{"query":"   "}`))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("empty query code=%d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestServiceErrorMappingBranches(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code int
	}{
		// Engine currently wraps exhausted provider chains as ErrKindNoResults.
		{"invalid", &SearchError{Kind: ErrKindInvalidInput, Provider: "x", Cause: errors.New("bad")}, http.StatusNotFound},
		{"none", &SearchError{Kind: ErrKindNoResults, Provider: "x", Cause: ErrNoResults}, http.StatusNotFound},
		{"rate", &SearchError{Kind: ErrKindRateLimited, Provider: "x", Cause: errors.New("429")}, http.StatusNotFound},
		{"blocked", &BlockedError{Event: BlockedEvent{StatusCode: 403, Detector: "status"}}, http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakeProvider{name: "ddg", enabled: true, fn: func(context.Context, SearchRequest) ([]Result, error) {
				return nil, tc.err
			}}
			eng, _ := NewEngine(EngineOptions{Providers: []Provider{p}})
			h := NewHTTPHandler(eng, Config{Timeout: 20 * time.Millisecond}, nil)
			s := httptest.NewServer(h)
			defer s.Close()
			resp, _ := http.Post(s.URL+"/v1/search", "application/json", bytes.NewBufferString(`{"query":"golang"}`))
			if resp.StatusCode != tc.code {
				t.Fatalf("status=%d want=%d", resp.StatusCode, tc.code)
			}
			resp.Body.Close()
		})
	}
}

func TestWriteAPIErrPayload(t *testing.T) {
	r := httptest.NewRecorder()
	writeAPIErr(r, http.StatusBadRequest, APIError{Error: "bad", Kind: "invalid_input"})
	if r.Code != http.StatusBadRequest {
		t.Fatalf("code=%d", r.Code)
	}
	var out APIError
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Error != "bad" || out.Kind != "invalid_input" {
		t.Fatalf("payload=%+v", out)
	}
}

func TestTwoCaptchaExtraBranches(t *testing.T) {
	s := NewTwoCaptchaSolver("k")
	if _, err := s.submitHCaptcha(context.Background(), "https://x", ""); err == nil {
		t.Fatal("expected missing hcaptcha key")
	}
	if _, err := s.submitTurnstile(context.Background(), "https://x", ""); err == nil {
		t.Fatal("expected missing turnstile key")
	}

	var lastMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastMethod = r.FormValue("method")
		_, _ = w.Write([]byte(`{"status":1,"request":"id"}`))
	}))
	defer srv.Close()
	s.BaseURL = srv.URL
	if _, err := s.submitReCAPTCHAv3(context.Background(), "https://x", "12345678901234567890"); err != nil || lastMethod != "userrecaptcha" {
		t.Fatalf("v3 submit failed method=%s err=%v", lastMethod, err)
	}
	if _, err := s.submitHCaptcha(context.Background(), "https://x", "12345678901234567890"); err != nil || lastMethod != "hcaptcha" {
		t.Fatalf("hcaptcha submit failed method=%s err=%v", lastMethod, err)
	}
	if _, err := s.submitTurnstile(context.Background(), "https://x", "12345678901234567890"); err != nil || lastMethod != "turnstile" {
		t.Fatalf("turnstile submit failed method=%s err=%v", lastMethod, err)
	}
}

func TestCapSolverAdditionalBranches(t *testing.T) {
	s := NewCapSolverSolver("k")
	if _, err := s.buildTask("https://x", "", "recaptchav3"); err == nil {
		t.Fatal("expected missing v3 key")
	}
	if _, err := s.buildTask("https://x", "", "turnstile"); err == nil {
		t.Fatal("expected missing turnstile key")
	}
	if _, err := s.buildTask("https://x", "", ""); err == nil {
		t.Fatal("expected missing default site key")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/getTaskResult" {
			_, _ = w.Write([]byte(`{"errorId":0,"status":"ready","solution":{"token":"turnstile-token"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"errorId":0,"taskId":"id"}`))
	}))
	defer srv.Close()
	s.BaseURL = srv.URL
	s.PollInterval = time.Millisecond
	s.PollTimeout = 100 * time.Millisecond
	if tok, err := s.getTaskResult(context.Background(), "id"); err != nil || tok != "turnstile-token" {
		t.Fatalf("token branch failed tok=%q err=%v", tok, err)
	}
}

func TestNewFlareSolverrDefaultEndpoint(t *testing.T) {
	f := NewFlareSolverrSolver("")
	if f.Endpoint == "" {
		t.Fatal("expected default endpoint")
	}
}
