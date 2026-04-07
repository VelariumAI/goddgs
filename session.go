package goddgs

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"
	"time"
)

// sessionManager maintains a persistent cookie jar and handles session warmup.
//
// A "warmed-up" session has visited the DDG homepage via a real GET request,
// acquiring the session cookies (__ddg1_, __ddg2_, etc.) that DuckDuckGo sets
// on first visit. Without these cookies, search requests may be blocked or
// return degraded results.
type sessionManager struct {
	mu         sync.Mutex
	jar        *cookiejar.Jar
	warmedUp   bool
	lastWarmup time.Time
	warmupTTL  time.Duration
}

func newSessionManager(warmupTTL time.Duration) *sessionManager {
	if warmupTTL <= 0 {
		warmupTTL = 20 * time.Minute
	}
	jar, _ := cookiejar.New(nil)
	return &sessionManager{jar: jar, warmupTTL: warmupTTL}
}

// CookieJar returns the session's cookie jar for use in http.Client.
func (s *sessionManager) CookieJar() http.CookieJar { return s.jar }

// NeedsWarmup reports whether a GET to the DDG homepage is required.
func (s *sessionManager) NeedsWarmup() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.warmedUp || time.Since(s.lastWarmup) > s.warmupTTL
}

// markWarmed records a successful warmup (called after the GET succeeds).
func (s *sessionManager) markWarmed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warmedUp = true
	s.lastWarmup = time.Now()
}

// Invalidate resets the session — clears cookies and warmup state.
// Called when a block is detected that may be session/cookie related.
func (s *sessionManager) Invalidate() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.warmedUp = false
	jar, _ := cookiejar.New(nil)
	s.jar = jar
}

// HasCookiesFor reports whether the jar holds any cookies for rawURL.
func (s *sessionManager) HasCookiesFor(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return len(s.jar.Cookies(u)) > 0
}

// warmupRequest is a callback signature used by Warmup to build each HTTP request.
// The caller supplies this so that browser-profile headers are applied consistently.
type warmupRequest func(ctx context.Context, method, rawURL string) (*http.Request, error)

// Warmup performs a GET to homeURL (and a second GET with a dummy query parameter)
// to cause DuckDuckGo to set its session cookies. It is idempotent — concurrent
// callers will each run independently but the result is harmless duplication.
func (s *sessionManager) Warmup(
	ctx context.Context,
	hc *http.Client,
	homeURL string,
	buildReq warmupRequest,
) error {
	// Re-check inside (may have warmed up between NeedsWarmup and here).
	s.mu.Lock()
	if s.warmedUp && time.Since(s.lastWarmup) <= s.warmupTTL {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// First GET — homepage root.
	if err := doWarmupGET(ctx, hc, homeURL, buildReq); err != nil {
		return err
	}
	// Second GET — homepage with a benign query so DDG fingerprints a "search session".
	_ = doWarmupGET(ctx, hc, homeURL+"?q=", buildReq) // best-effort

	s.markWarmed()
	return nil
}

func doWarmupGET(ctx context.Context, hc *http.Client, rawURL string, buildReq warmupRequest) error {
	req, err := buildReq(ctx, http.MethodGet, rawURL)
	if err != nil {
		return err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
