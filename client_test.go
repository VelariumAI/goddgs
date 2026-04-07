package goddgs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testSolver struct {
	solves int
}

func (s *testSolver) Supports(signal BlockSignal) bool { return true }

func (s *testSolver) Solve(_ context.Context, _ string, _ BlockInfo, _ []byte) (*ChallengeSolution, error) {
	s.solves++
	return &ChallengeSolution{}, nil
}

func TestExtractVQD(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"quoted_double", `abc vqd="3-12345678901234567890123456789012" xyz`, "3-12345678901234567890123456789012"},
		{"quoted_single", `abc vqd='4-abcdef' xyz`, "4-abcdef"},
		{"url_style", `...&vqd=5-zzz123&bing_market=en-US...`, "5-zzz123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractVQD([]byte(tc.in))
			if got != tc.want {
				t.Fatalf("extractVQD() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseDJSResults(t *testing.T) {
	body := `DDG.pageLayout.load('d',[{"t":"One","u":"https://example.com/1","a":"Snippet 1"},{"t":"Two","u":"https://example.com/2","a":"Snippet 2"}]);`
	results, err := parseDJSResults([]byte(body))
	if err != nil {
		t.Fatalf("parseDJSResults error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Title != "One" || results[1].URL != "https://example.com/2" {
		t.Fatalf("unexpected parsed results: %#v", results)
	}
}

func TestParseHTMLResults(t *testing.T) {
	html := `<html><body><a class="result__a" href="https://a.example">A &amp; B</a><a class="result__a foo" href="https://b.example">B</a></body></html>`
	results := parseHTMLResults([]byte(html))
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].Title != "A & B" || results[1].URL != "https://b.example" {
		t.Fatalf("unexpected html parse result: %#v", results)
	}
}

func TestClientSearchDJS(t *testing.T) {
	t.Helper()
	var sawSearch bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`<html>vqd="3-token-abc"</html>`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			sawSearch = true
			q := r.URL.Query().Get("q")
			if q != "golang" {
				t.Fatalf("query = %q, want golang", q)
			}
			_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Go","u":"https://go.dev","a":"The Go programming language"}]);`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{
		DuckDuckGoBase: srv.URL,
		LinksBase:      srv.URL,
		HTMLBase:       srv.URL,
		RequestTimeout: 5 * time.Second,
		RetryMax:       2,
	})

	results, err := c.Search(context.Background(), "golang", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if !sawSearch {
		t.Fatal("expected search endpoint call")
	}
	if len(results) != 1 || !strings.Contains(results[0].URL, "go.dev") {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestClientSearchFallbackToHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd='3-token-xyz'`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			_, _ = w.Write([]byte(`not valid js payload`))
		case r.Method == http.MethodPost && r.URL.Path == "/html/":
			_, _ = w.Write([]byte(`<a class="result__a" href="https://fallback.example">Fallback</a>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL})
	results, err := c.Search(context.Background(), "x", SearchOptions{})
	if err != nil {
		t.Fatalf("Search fallback error: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://fallback.example" {
		t.Fatalf("unexpected fallback results: %#v", results)
	}
}

func TestRetryOnBlockedStatus(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd='3-token-xyz'`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			calls++
			if calls == 1 {
				w.WriteHeader(http.StatusTooManyRequests)
				_, _ = w.Write([]byte("rate limited"))
				return
			}
			_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Ok","u":"https://ok.example","a":"ok"}]);`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL, RetryMax: 3, RetryBaseDelay: 1 * time.Millisecond})
	results, err := c.Search(context.Background(), "x", SearchOptions{})
	if err != nil {
		t.Fatalf("Search retry error: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 d.js calls, got %d", calls)
	}
	if len(results) != 1 || results[0].Title != "Ok" {
		t.Fatalf("unexpected results after retry: %#v", results)
	}
}

func TestBlockedErrorAndCallback(t *testing.T) {
	var events []BlockedEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd='3-token-xyz'`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			w.Header().Set("cf-mitigated", "challenge")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("challenge page"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{
		DuckDuckGoBase:      srv.URL,
		LinksBase:           srv.URL,
		HTMLBase:            srv.URL,
		RetryMax:            1,
		DisableHTMLFallback: true,
		OnBlocked: func(e BlockedEvent) {
			events = append(events, e)
		},
	})
	_, err := c.Search(context.Background(), "x", SearchOptions{})
	if err == nil {
		t.Fatal("expected blocked error")
	}
	if !IsBlocked(err) {
		t.Fatalf("expected blocked classification, got: %v", err)
	}
	var be *BlockedError
	if !errors.As(err, &be) {
		// Search wraps d.js + html fallback errors; still must classify as blocked.
		if !strings.Contains(err.Error(), "response appears blocked") {
			t.Fatalf("expected blocked signal in error, got: %v", err)
		}
	}
	if len(events) == 0 {
		t.Fatal("expected OnBlocked callback")
	}
	if events[0].StatusCode != http.StatusForbidden {
		t.Fatalf("unexpected blocked status: %d", events[0].StatusCode)
	}
}

func TestSearchPagesPagination(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd='3-token-xyz'`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			calls++
			offset := r.URL.Query().Get("s")
			_, _ = w.Write([]byte(fmt.Sprintf(`DDG.pageLayout.load('d',[{"t":"R%s","u":"https://e/%s","a":"S"}]);`, offset, offset)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(Options{DuckDuckGoBase: srv.URL, LinksBase: srv.URL, HTMLBase: srv.URL})
	got, err := c.SearchPages(context.Background(), "x", 1, 3, SearchOptions{})
	if err != nil {
		t.Fatalf("SearchPages error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("search calls = %d, want 3", calls)
	}
	if len(got) != 3 {
		t.Fatalf("result len = %d, want 3", len(got))
	}
	if got[1].Title != "R30" || got[2].Title != "R60" {
		t.Fatalf("unexpected pagination offsets in results: %#v", got)
	}
}

func TestSearchDJSVQDFreshPerRetryAfterInvalidation(t *testing.T) {
	var vqdPosts int
	var djsCalls int
	firstVQD := "3-stale"
	secondVQD := "3-fresh"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK) // optional warmup
		case r.Method == http.MethodPost && r.URL.Path == "/":
			vqdPosts++
			if vqdPosts == 1 {
				_, _ = w.Write([]byte(`vqd="` + firstVQD + `"`))
				return
			}
			_, _ = w.Write([]byte(`vqd="` + secondVQD + `"`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			djsCalls++
			vqd := r.URL.Query().Get("vqd")
			if djsCalls == 1 {
				if vqd != firstVQD {
					t.Fatalf("first attempt vqd = %q, want %q", vqd, firstVQD)
				}
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("challenge"))
				return
			}
			if vqd != secondVQD {
				t.Fatalf("retry attempt vqd = %q, want refreshed %q", vqd, secondVQD)
			}
			_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Ok","u":"https://ok.example","a":"ok"}]);`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := NewAntiBotConfig()
	cfg.ChromeTLS = false
	cfg.SessionWarmup = false
	cfg.AdaptiveRateLimit = false
	cfg.VQDInvalidateOnBlock = true

	c := NewClient(Options{
		DuckDuckGoBase: srv.URL,
		LinksBase:      srv.URL,
		HTMLBase:       srv.URL,
		RetryMax:       3,
		RetryBaseDelay: 1 * time.Millisecond,
		AntiBot:        cfg,
	})

	results, err := c.Search(context.Background(), "golang", SearchOptions{MaxResults: 1})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if vqdPosts < 2 {
		t.Fatalf("expected fresh vqd fetch after block, posts=%d", vqdPosts)
	}
}

func TestSolverSuccessDoesNotConsumeRetryBudget(t *testing.T) {
	var djsCalls int
	solver := &testSolver{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd="3-token"`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			djsCalls++
			if djsCalls == 1 {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte("blocked"))
				return
			}
			_, _ = w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Ok","u":"https://ok.example","a":"ok"}]);`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := NewAntiBotConfig()
	cfg.ChromeTLS = false
	cfg.SessionWarmup = false
	cfg.AdaptiveRateLimit = false
	cfg.VQDInvalidateOnBlock = false
	cfg.ChallengeSolvers = []ChallengeSolver{solver}

	c := NewClient(Options{
		DuckDuckGoBase: srv.URL,
		LinksBase:      srv.URL,
		HTMLBase:       srv.URL,
		RetryMax:       1, // only passes with i-- retry slot preservation
		RetryBaseDelay: 1 * time.Millisecond,
		AntiBot:        cfg,
	})

	results, err := c.Search(context.Background(), "golang", SearchOptions{MaxResults: 1})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if solver.solves != 1 {
		t.Fatalf("solver solves = %d, want 1", solver.solves)
	}
	if djsCalls != 2 {
		t.Fatalf("d.js calls = %d, want 2", djsCalls)
	}
}

func TestCircuitBreakerTripAndFailFast(t *testing.T) {
	var djsCalls int
	var circuitEvents []CircuitEvent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && r.URL.Path == "/":
			_, _ = w.Write([]byte(`vqd="3-token"`))
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			djsCalls++
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("blocked"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := NewAntiBotConfig()
	cfg.ChromeTLS = false
	cfg.SessionWarmup = false
	cfg.AdaptiveRateLimit = false
	cfg.VQDInvalidateOnBlock = false
	cfg.CircuitBreakerThreshold = 2
	cfg.CircuitBreakerCooldown = time.Minute

	c := NewClient(Options{
		DuckDuckGoBase:      srv.URL,
		LinksBase:           srv.URL,
		HTMLBase:            srv.URL,
		RetryMax:            3,
		RetryBaseDelay:      1 * time.Millisecond,
		DisableHTMLFallback: true,
		AntiBot:             cfg,
		OnCircuit: func(ev CircuitEvent) {
			circuitEvents = append(circuitEvents, ev)
		},
	})

	_, err := c.Search(context.Background(), "golang", SearchOptions{MaxResults: 1})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}
	if djsCalls != 2 {
		t.Fatalf("expected breaker to trip after threshold, d.js calls=%d want 2", djsCalls)
	}

	if len(circuitEvents) < 2 {
		t.Fatalf("expected open + fail_fast circuit events, got %d", len(circuitEvents))
	}
	foundOpen := false
	foundFailFast := false
	for _, ev := range circuitEvents {
		if ev.State == CircuitStateOpen && ev.Trigger == "threshold_reached" {
			foundOpen = true
		}
		if ev.State == CircuitStateOpen && ev.Trigger == "fail_fast" {
			foundFailFast = true
		}
	}
	if !foundOpen || !foundFailFast {
		t.Fatalf("missing circuit events, open=%v fail_fast=%v events=%#v", foundOpen, foundFailFast, circuitEvents)
	}
}
