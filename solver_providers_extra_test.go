package goddgs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestTwoCaptchaSolverFlowAndSupports(t *testing.T) {
	var polls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/in.php"):
			_, _ = w.Write([]byte(`{"status":1,"request":"task123"}`))
		case strings.HasSuffix(r.URL.Path, "/res.php"):
			polls++
			if polls == 1 {
				_, _ = w.Write([]byte(`{"status":0,"request":"CAPCHA_NOT_READY"}`))
				return
			}
			_, _ = w.Write([]byte(`{"status":1,"request":"token-ok"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	s := NewTwoCaptchaSolver("k")
	s.BaseURL = srv.URL
	s.PollInterval = time.Millisecond
	s.PollTimeout = 100 * time.Millisecond

	if !s.Supports(BlockSignalCloudflare) || s.Supports(BlockSignalAkamai) {
		t.Fatal("supports matrix mismatch")
	}
	body := []byte(`<div data-sitekey="12345678901234567890"></div><script src="https://www.google.com/recaptcha/api2"></script>`)
	sol, err := s.Solve(context.Background(), "https://example.com", BlockInfo{Signal: BlockSignalReCAPTCHA}, body)
	if err != nil || sol == nil || sol.Token != "token-ok" {
		t.Fatalf("unexpected solve result: sol=%#v err=%v", sol, err)
	}
}

func TestCapSolverBuildTaskAndFlow(t *testing.T) {
	s := NewCapSolverSolver("k")
	if !s.Supports(BlockSignalGeneric) || s.Supports(BlockSignalAkamai) {
		t.Fatal("supports matrix mismatch")
	}
	if _, err := s.buildTask("https://x", "", "hcaptcha"); err == nil {
		t.Fatal("expected missing sitekey error")
	}
	if task, err := s.buildTask("https://x", "12345678901234567890", "turnstile"); err != nil || task["type"] != "AntiTurnstileTaskProxyless" {
		t.Fatalf("unexpected turnstile task: %#v err=%v", task, err)
	}

	var getCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/createTask":
			_, _ = w.Write([]byte(`{"errorId":0,"taskId":"tid1"}`))
		case "/getTaskResult":
			getCalls++
			if getCalls == 1 {
				_, _ = w.Write([]byte(`{"errorId":0,"status":"processing"}`))
				return
			}
			_, _ = w.Write([]byte(`{"errorId":0,"status":"ready","solution":{"gRecaptchaResponse":"tok"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	s.BaseURL = srv.URL
	s.PollInterval = time.Millisecond
	s.PollTimeout = 200 * time.Millisecond

	body := []byte(`<div data-sitekey="12345678901234567890"></div><script src="https://www.google.com/recaptcha/api2"></script>`)
	sol, err := s.Solve(context.Background(), "https://example.com", BlockInfo{Signal: BlockSignalReCAPTCHA}, body)
	if err != nil || sol == nil || sol.Token != "tok" {
		t.Fatalf("unexpected solve result: sol=%#v err=%v", sol, err)
	}
}

func TestFlareSolverrSolveAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req["cmd"] != "request.get" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"status":"ok","solution":{"userAgent":"UA","cookies":[{"name":"cf_clearance","value":"v","domain":"duckduckgo.com","path":"/"}]}}`))
	}))
	defer srv.Close()

	f := NewFlareSolverrSolver(srv.URL)
	if !f.Supports(BlockSignalCloudflare) || f.Supports(BlockSignalAkamai) {
		t.Fatal("supports matrix mismatch")
	}
	sol, err := f.Solve(context.Background(), "https://example.com", BlockInfo{}, nil)
	if err != nil || sol == nil || len(sol.Cookies) == 0 || sol.UserAgent != "UA" {
		t.Fatalf("unexpected flaresolverr solve: sol=%#v err=%v", sol, err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"error","message":"blocked"}`))
	}))
	defer bad.Close()
	f2 := NewFlareSolverrSolver(bad.URL)
	if _, err := f2.Solve(context.Background(), "https://example.com", BlockInfo{}, nil); err == nil {
		t.Fatal("expected challenge failed error")
	}
}

func TestDDGProviderSearchGuards(t *testing.T) {
	p := NewDDGProvider(nil)
	if _, err := p.Search(context.Background(), SearchRequest{Query: "x"}); err == nil {
		t.Fatal("expected nil client error")
	}
	p = NewDDGProvider(NewClient(Options{RetryMax: 1, DisableHTMLFallback: true}))
	if _, err := p.Search(context.Background(), SearchRequest{}); err == nil {
		t.Fatal("expected invalid input")
	}
}
