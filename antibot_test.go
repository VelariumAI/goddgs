package goddgs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── UA pool ───────────────────────────────────────────────────────────────────

func TestUserAgentPoolWeightedDistribution(t *testing.T) {
	pool := NewUserAgentPool()
	counts := map[string]int{}
	const n = 10_000
	for range n {
		ua := pool.PickUA()
		fam := uaFamily(ua)
		counts[fam]++
	}
	// Chrome+Edge should dominate (>60%).
	chromium := counts["chrome"] + counts["edge"]
	if chromium < n*60/100 {
		t.Fatalf("expected chromium >60%% of picks, got %d/%d (%d%%)", chromium, n, chromium*100/n)
	}
	// All expected families must appear.
	for _, fam := range []string{"chrome", "safari", "firefox", "edge"} {
		if counts[fam] == 0 {
			t.Fatalf("family %q never picked in %d draws", fam, n)
		}
	}
}

func TestSecCHUAChrome(t *testing.T) {
	ua := `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`
	got := SecCHUA(ua)
	if !strings.Contains(got, `"Google Chrome";v="124"`) {
		t.Fatalf("SecCHUA() = %q, want Google Chrome v124", got)
	}
	if !strings.Contains(got, `"Chromium";v="124"`) {
		t.Fatalf("SecCHUA() = %q, want Chromium v124", got)
	}
}

func TestSecCHUAEdge(t *testing.T) {
	ua := `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0`
	got := SecCHUA(ua)
	if !strings.Contains(got, `"Microsoft Edge";v="124"`) {
		t.Fatalf("SecCHUA() for Edge = %q", got)
	}
}

func TestSecCHUASafariEmpty(t *testing.T) {
	ua := `Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15`
	if got := SecCHUA(ua); got != "" {
		t.Fatalf("SecCHUA() for Safari should be empty, got %q", got)
	}
}

func TestSecCHUAPlatform(t *testing.T) {
	cases := []struct{ ua, want string }{
		{`... (Windows NT 10.0; Win64; x64) ...`, `"Windows"`},
		{`... (Macintosh; Intel Mac OS X 14) ...`, `"macOS"`},
		{`... (iPhone; CPU iPhone OS ...) ...`, `"iOS"`},
		{`... (Linux; Android 14; ...) ...`, `"Android"`},
		{`... (X11; Linux x86_64) ...`, `"Linux"`},
	}
	for _, tc := range cases {
		if got := SecCHUAPlatform(tc.ua); got != tc.want {
			t.Errorf("SecCHUAPlatform(%q) = %q, want %q", tc.ua, got, tc.want)
		}
	}
}

// ── Header profiles ───────────────────────────────────────────────────────────

func TestChromeProfileHeadersPresent(t *testing.T) {
	ua := `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`
	profile := buildProfile(ua)
	req, _ := http.NewRequest(http.MethodGet, "https://duckduckgo.com/", nil)
	req.Header = make(http.Header)
	applyProfile(req, profile, secFetchNavigation("none"))

	for _, hdr := range []string{"User-Agent", "Accept", "Accept-Language", "Accept-Encoding",
		"Sec-CH-UA", "Sec-CH-UA-Mobile", "Sec-CH-UA-Platform",
		"Sec-Fetch-Dest", "Sec-Fetch-Mode", "Sec-Fetch-Site"} {
		if req.Header.Get(hdr) == "" {
			t.Errorf("Chrome profile missing header: %s", hdr)
		}
	}
}

func TestFirefoxProfileNoSecCHUA(t *testing.T) {
	ua := `Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0`
	profile := buildProfile(ua)
	req, _ := http.NewRequest(http.MethodGet, "https://duckduckgo.com/", nil)
	req.Header = make(http.Header)
	applyProfile(req, profile, secFetchNavigation("none"))

	if req.Header.Get("Sec-CH-UA") != "" {
		t.Error("Firefox profile should NOT include Sec-CH-UA")
	}
	if req.Header.Get("TE") == "" {
		t.Error("Firefox profile should include TE: trailers")
	}
}

func TestApplyProfileDoesNotOverrideExisting(t *testing.T) {
	ua := `Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`
	profile := buildProfile(ua)
	req, _ := http.NewRequest(http.MethodGet, "https://duckduckgo.com/", nil)
	req.Header = make(http.Header)
	req.Header.Set("Accept-Language", "de-DE,de;q=0.9")
	applyProfile(req, profile, nil)
	if req.Header.Get("Accept-Language") != "de-DE,de;q=0.9" {
		t.Error("applyProfile must not overwrite pre-set headers")
	}
}

// ── Block detection ───────────────────────────────────────────────────────────

func TestDetectBlockSignalCloudflareHeader(t *testing.T) {
	h := http.Header{}
	h.Set("cf-mitigated", "challenge")
	info := DetectBlockSignal(403, h, nil)
	if info.Signal != BlockSignalCloudflare {
		t.Fatalf("expected Cloudflare, got %s", info.Signal)
	}
}

func TestDetectBlockSignalReCAPTCHABody(t *testing.T) {
	body := []byte(`<html><body>Please complete www.google.com/recaptcha challenge</body></html>`)
	info := DetectBlockSignal(200, http.Header{}, body)
	if info.Signal != BlockSignalReCAPTCHA {
		t.Fatalf("expected ReCAPTCHA, got %s", info.Signal)
	}
}

func TestDetectBlockSignalAkamaiServer(t *testing.T) {
	h := http.Header{}
	h.Set("server", "AkamaiGHost")
	info := DetectBlockSignal(403, h, nil)
	if info.Signal != BlockSignalAkamai {
		t.Fatalf("expected Akamai, got %s", info.Signal)
	}
}

func TestDetectBlockSignal429Generic(t *testing.T) {
	info := DetectBlockSignal(429, http.Header{}, nil)
	if info.Signal != BlockSignalGeneric {
		t.Fatalf("expected Generic for 429, got %s", info.Signal)
	}
	if info.DetectorKey != "http_429" {
		t.Fatalf("expected key http_429, got %s", info.DetectorKey)
	}
}

func TestDetectBlockSignalNone(t *testing.T) {
	info := DetectBlockSignal(200, http.Header{}, []byte(`{"results": []}`))
	if info.IsDetected() {
		t.Fatalf("expected no block signal, got %s / %s", info.Signal, info.DetectorKey)
	}
}

func TestRetryAfterSecondsNumeric(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "42")
	if got := RetryAfterSeconds(h); got != 42 {
		t.Fatalf("RetryAfterSeconds() = %d, want 42", got)
	}
}

func TestRetryAfterSecondsAbsent(t *testing.T) {
	if got := RetryAfterSeconds(http.Header{}); got != 0 {
		t.Fatalf("RetryAfterSeconds() = %d, want 0", got)
	}
}

// ── Session manager ───────────────────────────────────────────────────────────

func TestSessionManagerNeedsWarmup(t *testing.T) {
	s := newSessionManager(10 * time.Minute)
	if !s.NeedsWarmup() {
		t.Fatal("fresh session should need warmup")
	}
	s.markWarmed()
	if s.NeedsWarmup() {
		t.Fatal("just-warmed session should not need warmup")
	}
}

func TestSessionManagerInvalidate(t *testing.T) {
	s := newSessionManager(10 * time.Minute)
	s.markWarmed()
	s.Invalidate()
	if !s.NeedsWarmup() {
		t.Fatal("invalidated session should need warmup")
	}
}

func TestSessionWarmupSetsJar(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "__ddg1_", Value: "test123"})
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s := newSessionManager(10 * time.Minute)
	hc := &http.Client{Jar: s.CookieJar()}

	err := s.Warmup(context.Background(), hc, srv.URL+"/", func(ctx context.Context, method, rawURL string) (*http.Request, error) {
		return http.NewRequestWithContext(ctx, method, rawURL, nil)
	})
	if err != nil {
		t.Fatalf("Warmup error: %v", err)
	}
	if !s.HasCookiesFor(srv.URL) {
		t.Fatal("warmup should have stored cookies in the jar")
	}
	if s.NeedsWarmup() {
		t.Fatal("session should be marked as warmed after Warmup()")
	}
}

// ── Proxy pool ────────────────────────────────────────────────────────────────

func TestProxyPoolRoundRobin(t *testing.T) {
	pool, _ := NewProxyPool([]string{
		"http://proxy1:8080",
		"http://proxy2:8080",
		"http://proxy3:8080",
	}, RotateRoundRobin)

	seen := map[string]int{}
	for range 9 {
		e := pool.Next()
		if e == nil {
			t.Fatal("Next() returned nil")
		}
		seen[e.URL]++
	}
	for _, url := range []string{"http://proxy1:8080", "http://proxy2:8080", "http://proxy3:8080"} {
		if seen[url] != 3 {
			t.Errorf("proxy %s: expected 3 picks in 9, got %d", url, seen[url])
		}
	}
}

func TestProxyPoolCooldown(t *testing.T) {
	pool, _ := NewProxyPool([]string{"http://bad:8080", "http://good:8080"}, RotateRoundRobin)
	pool.SetCooldown(10*time.Second, 1) // 1 fail → cooldown

	bad := pool.entries[0]
	pool.MarkFailed(bad)

	// bad proxy should be skipped; all picks should return good.
	for range 5 {
		e := pool.Next()
		if e != nil && e.URL == "http://bad:8080" {
			t.Fatal("cooled-down proxy should not be returned by Next()")
		}
	}
}

func TestProxyPoolMarkSuccessResets(t *testing.T) {
	pool, _ := NewProxyPool([]string{"http://p:8080"}, RotateRoundRobin)
	pool.SetCooldown(10*time.Second, 2)

	e := pool.entries[0]
	pool.MarkFailed(e)
	pool.MarkSuccess(e)

	e.mu.Lock()
	fails := e.consecutiveFails
	e.mu.Unlock()
	if fails != 0 {
		t.Fatalf("MarkSuccess should reset consecutiveFails to 0, got %d", fails)
	}
}

func TestProxyPoolStats(t *testing.T) {
	pool, _ := NewProxyPool([]string{"http://p1:8080", "http://p2:8080"}, RotateRandom)
	pool.MarkFailed(pool.entries[0])
	pool.MarkSuccess(pool.entries[1])

	stats := pool.Stats()
	if len(stats) != 2 {
		t.Fatalf("len(Stats()) = %d, want 2", len(stats))
	}
	if stats[0].Failures != 1 {
		t.Errorf("p1 failures = %d, want 1", stats[0].Failures)
	}
	if stats[1].Failures != 0 {
		t.Errorf("p2 failures = %d, want 0", stats[1].Failures)
	}
}

// ── Adaptive rate limiter ─────────────────────────────────────────────────────

func TestAdaptiveRateLimiterGrows(t *testing.T) {
	rl := newAdaptiveRateLimiter(100*time.Millisecond, 10*time.Second)
	before := rl.Current()
	rl.OnBlock()
	after := rl.Current()
	if after <= before {
		t.Fatalf("OnBlock() should increase delay: %v → %v", before, after)
	}
}

func TestAdaptiveRateLimiterShrinks(t *testing.T) {
	rl := newAdaptiveRateLimiter(100*time.Millisecond, 10*time.Second)
	rl.OnBlock()
	rl.OnBlock() // grow twice
	grown := rl.Current()
	rl.OnSuccess()
	after := rl.Current()
	if after >= grown {
		t.Fatalf("OnSuccess() should decrease delay: %v → %v", grown, after)
	}
}

func TestAdaptiveRateLimiterFloorsAtBase(t *testing.T) {
	rl := newAdaptiveRateLimiter(100*time.Millisecond, 10*time.Second)
	for range 100 {
		rl.OnSuccess()
	}
	if rl.Current() < 100*time.Millisecond {
		t.Fatalf("rate limiter must not drop below base: got %v", rl.Current())
	}
}

func TestAdaptiveRateLimiterCapsAtMax(t *testing.T) {
	rl := newAdaptiveRateLimiter(100*time.Millisecond, 2*time.Second)
	for range 100 {
		rl.OnBlock()
	}
	if rl.Current() > 2*time.Second {
		t.Fatalf("rate limiter must not exceed max: got %v", rl.Current())
	}
}

func TestAdaptiveRateLimiterWaitRespectsCancellation(t *testing.T) {
	rl := newAdaptiveRateLimiter(5*time.Second, 30*time.Second)
	rl.OnBlock() // push delay high

	// First Wait() is free (no prior lastReq). Consume it to establish lastReq.
	_ = rl.Wait(context.Background())

	// Second Wait() must now block for ≥5 s → context should cancel it.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if err := rl.Wait(ctx); err == nil {
		t.Fatal("Wait() should return error when context expires before delay elapses")
	}
}

// ── AntiBotConfig integration ─────────────────────────────────────────────────

func TestAntiBotClientSessionWarmupOnSearch(t *testing.T) {
	warmupCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// d.js search results — must be checked before the generic GET warmup case.
		case r.Method == http.MethodGet && r.URL.Path == "/d.js":
			w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"T","u":"https://e.test","a":"S"}]);`))
		// Warmup GETs (homepage and /?q=).
		case r.Method == http.MethodGet:
			warmupCalls++
			http.SetCookie(w, &http.Cookie{Name: "__ddg1_", Value: "tok"})
			w.WriteHeader(http.StatusOK)
		// VQD token fetch.
		case r.Method == http.MethodPost && r.URL.Path == "/":
			w.Write([]byte(`vqd="3-tok"`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := NewAntiBotConfig()
	cfg.ChromeTLS = false // httptest server doesn't speak Chrome TLS

	c := NewClient(Options{
		DuckDuckGoBase: srv.URL,
		LinksBase:      srv.URL,
		HTMLBase:       srv.URL,
		RetryMax:       1,
		AntiBot:        cfg,
	})

	_, err := c.Search(context.Background(), "test", SearchOptions{MaxResults: 1})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if warmupCalls == 0 {
		t.Fatal("expected session warmup GET call(s)")
	}
}

func TestAntiBotVQDInvalidatedOnBlock(t *testing.T) {
	blockOnce := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/":
			w.WriteHeader(http.StatusOK) // warmup
		case r.Method == http.MethodPost && r.URL.Path == "/":
			w.Write([]byte(`vqd="3-fresh"`))
		case r.URL.Path == "/d.js":
			if blockOnce {
				blockOnce = false
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Write([]byte(`DDG.pageLayout.load('d',[{"t":"Ok","u":"https://ok.test","a":""}]);`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := NewAntiBotConfig()
	cfg.ChromeTLS = false
	cfg.VQDInvalidateOnBlock = true
	cfg.SessionWarmup = false
	cfg.AdaptiveRateLimit = false

	c := NewClient(Options{
		DuckDuckGoBase: srv.URL,
		LinksBase:      srv.URL,
		HTMLBase:       srv.URL,
		RetryMax:       3,
		RetryBaseDelay: 1 * time.Millisecond,
		AntiBot:        cfg,
	})

	results, err := c.Search(context.Background(), "test", SearchOptions{MaxResults: 1})
	if err != nil {
		t.Fatalf("Search() error after VQD invalidation: %v", err)
	}
	if len(results) == 0 || results[0].URL != "https://ok.test" {
		t.Fatalf("unexpected results: %#v", results)
	}
}
