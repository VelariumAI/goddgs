package goddgs

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

type fakeSolver struct {
	support map[BlockSignal]bool
	err     error
	sol     *ChallengeSolution
}

func (f fakeSolver) Supports(s BlockSignal) bool { return f.support[s] }
func (f fakeSolver) Solve(context.Context, string, BlockInfo, []byte) (*ChallengeSolution, error) {
	return f.sol, f.err
}

func TestSearchRequestToSearchOptionsAndSafeParam(t *testing.T) {
	r := SearchRequest{MaxResults: 7, Region: "uk-en", SafeSearch: SafeStrict, TimeRange: "d", Offset: 30}
	o := r.toSearchOptions()
	if o.MaxResults != 7 || o.Region != "uk-en" || o.TimeRange != "d" || o.Offset != 30 {
		t.Fatalf("unexpected options: %#v", o)
	}
	if SafeOff.ddgParam() != "-2" || SafeStrict.ddgParam() != "1" || SafeModerate.ddgParam() != "-1" {
		t.Fatal("unexpected safesearch mapping")
	}
}

func TestBlockSignalStringAndSecFetchHelpers(t *testing.T) {
	if BlockSignalCloudflare.String() != "cloudflare" || BlockSignalNone.String() != "none" {
		t.Fatal("unexpected block signal string")
	}
	xhr := secFetchXHR("cross-site")
	if xhr["Sec-Fetch-Mode"] != "cors" {
		t.Fatalf("unexpected xhr sec-fetch: %#v", xhr)
	}
	sc := secFetchScript()
	if sc["Sec-Fetch-Dest"] != "script" {
		t.Fatalf("unexpected script sec-fetch: %#v", sc)
	}
}

func TestSearchErrorFormattingAndClassify(t *testing.T) {
	if (&SearchError{Kind: ErrKindInternal}).Error() == "" {
		t.Fatal("expected non-empty error")
	}
	se := &SearchError{Kind: ErrKindBlocked, Cause: errors.New("x")}
	if !strings.Contains(se.Error(), "blocked") {
		t.Fatalf("unexpected error text: %s", se.Error())
	}
	if !errors.Is(&SearchError{Kind: ErrKindInternal, Cause: ErrNoResults}, ErrNoResults) {
		t.Fatal("unwrap should expose cause")
	}

	c1 := classifyError("ddg", ErrNoVQD)
	if c1.Kind != ErrKindParse {
		t.Fatalf("kind=%s want parse", c1.Kind)
	}
	c2 := classifyError("ddg", ErrNoResults)
	if c2.Kind != ErrKindNoResults {
		t.Fatalf("kind=%s want no_results", c2.Kind)
	}
	c3 := classifyError("ddg", &BlockedError{Event: BlockedEvent{Detector: "cf"}})
	if c3.Kind != ErrKindBlocked || c3.Details["detector"] != "cf" {
		t.Fatalf("unexpected blocked classify: %#v", c3)
	}
	c4 := classifyError("ddg", errors.New("boom"))
	if c4.Kind != ErrKindProviderUnavailable {
		t.Fatalf("kind=%s want provider_unavailable", c4.Kind)
	}
}

func TestChainSolverAndChallengeHelpers(t *testing.T) {
	cs := &ChainSolver{Solvers: []ChallengeSolver{
		fakeSolver{support: map[BlockSignal]bool{BlockSignalCloudflare: true}, err: fmt.Errorf("fail")},
		fakeSolver{support: map[BlockSignal]bool{BlockSignalCloudflare: true}, sol: &ChallengeSolution{Token: "ok"}},
	}}
	if !cs.Supports(BlockSignalCloudflare) {
		t.Fatal("supports should be true")
	}
	sol, err := cs.Solve(context.Background(), "https://x", BlockInfo{Signal: BlockSignalCloudflare}, nil)
	if err != nil || sol == nil || sol.Token != "ok" {
		t.Fatalf("unexpected chain solve result: sol=%#v err=%v", sol, err)
	}
	if _, err := (&ChainSolver{}).Solve(context.Background(), "https://x", BlockInfo{Signal: BlockSignalCloudflare}, nil); err == nil {
		t.Fatal("expected no-solver error")
	}

	body := []byte(`<div data-sitekey="12345678901234567890"></div><script src="https://hcaptcha.com"></script>`)
	if sk := extractSiteKey(body); sk == "" {
		t.Fatal("expected site key")
	}
	if ct := challengeType(body, BlockSignalGeneric); ct != "hcaptcha" {
		t.Fatalf("unexpected challenge type: %s", ct)
	}
}

func TestHTTPClientsAndProxyPoolHelpers(t *testing.T) {
	hc, err := NewHTTPClient(0, "")
	if err != nil || hc == nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	if _, err := NewHTTPClient(0, "://bad"); err == nil {
		t.Fatal("expected invalid proxy url error")
	}
	ab, err := NewAntiBotHTTPClient(0, nil)
	if err != nil || ab == nil || ab.Jar == nil {
		t.Fatalf("NewAntiBotHTTPClient failed: %v", err)
	}

	pool, err := NewProxyPool([]string{"http://p1:8080", "http://p2:8080"}, RotateWeighted)
	if err != nil {
		t.Fatalf("NewProxyPool error: %v", err)
	}
	pool.SetWeight("http://p1:8080", 10)
	if pool.Len() != 2 {
		t.Fatalf("len=%d want 2", pool.Len())
	}
	seen := 0
	for i := 0; i < 20; i++ {
		if e := pool.Next(); e != nil {
			seen++
		}
	}
	if seen == 0 {
		t.Fatal("expected weighted next selections")
	}
}

func TestTransportProxyAndInferFetch(t *testing.T) {
	pool, _ := NewProxyPool([]string{"http://proxy1:8080"}, RotateRoundRobin)
	tr := newAntiBotTransport(pool)
	if tr.pickAndStoreProxy() == nil {
		t.Fatal("expected proxy selection")
	}
	u, err := tr.proxyFunc(nil)
	if err != nil || u == nil || !strings.Contains(u.String(), "proxy1") {
		t.Fatalf("unexpected proxy func output: %v %v", u, err)
	}
	if tr.LastProxy() == nil {
		t.Fatal("expected last proxy")
	}

	r1, _ := http.NewRequest(http.MethodGet, "https://links.duckduckgo.com/d.js", nil)
	if inferSecFetch(r1)["Sec-Fetch-Dest"] != "script" {
		t.Fatal("expected script fetch")
	}
	r2, _ := http.NewRequest(http.MethodPost, "https://duckduckgo.com/", nil)
	if inferSecFetch(r2)["Sec-Fetch-Site"] != "same-origin" {
		t.Fatal("expected same-origin post fetch")
	}
}

func TestTimingAndContextHelpers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := GaussianSleep(ctx, 20*time.Millisecond, 5*time.Millisecond, 1*time.Millisecond, 50*time.Millisecond); err == nil {
		t.Fatal("expected cancellation")
	}
	if err := sleepContext(context.Background(), 1*time.Millisecond); err != nil {
		t.Fatalf("sleepContext error: %v", err)
	}
}

func TestClientApplySolutionAndWaitGap(t *testing.T) {
	cfg := NewAntiBotConfig()
	cfg.ChromeTLS = false
	cfg.UARotation = true
	cfg.SessionWarmup = false
	cfg.AdaptiveRateLimit = false
	c := NewClient(Options{AntiBot: cfg, MinRequestInterval: 1 * time.Millisecond})
	sol := &ChallengeSolution{
		UserAgent: "Mozilla/5.0 TestUA",
		Cookies:   []*http.Cookie{{Name: "cf_clearance", Value: "abc", Domain: "duckduckgo.com", Path: "/"}},
	}
	c.applySolution(sol)
	if c.ua != "Mozilla/5.0 TestUA" {
		t.Fatalf("ua not applied: %q", c.ua)
	}
	if c.antiBot == nil || c.antiBot.transport == nil || c.antiBot.transport.uaPool == nil {
		t.Fatal("expected transport ua pool")
	}
	if got := c.antiBot.transport.uaPool.PickUA(); got != "Mozilla/5.0 TestUA" {
		t.Fatalf("expected pinned UA, got %q", got)
	}
	if err := c.waitForTurn(context.Background()); err != nil {
		t.Fatalf("waitForTurn first error: %v", err)
	}
	if err := c.waitForTurn(context.Background()); err != nil {
		t.Fatalf("waitForTurn second error: %v", err)
	}
	if got := snippet([]byte("abcdef"), 3); got != "abc..." {
		t.Fatalf("snippet got %q", got)
	}
}
