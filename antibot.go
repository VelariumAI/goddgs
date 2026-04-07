package goddgs

import (
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

// AntiBotConfig holds advanced browser-compatible transport/session options.
// Use NewAntiBotConfig() to obtain a fully-enabled default configuration.
//
// All techniques are independent and can be toggled individually.
type AntiBotConfig struct {
	// UARotation picks a fresh, market-share-weighted user-agent from a pool of
	// 24 realistic browser UAs on every request. Also keeps Sec-CH-UA, platform,
	// and mobile hints consistent with the chosen UA.
	UARotation bool

	// ChromeTLS uses the utls library to perform TLS handshakes with Chrome's
	// exact ClientHello specification, making the JA3 / JA4 fingerprint
	// aligned with real Chrome browser behavior.
	ChromeTLS bool

	// SessionWarmup performs a GET to the DDG homepage before the first search
	// to acquire session cookies (__ddg1_, __ddg2_, etc.). Without these cookies
	// some DDG search endpoints return degraded or blocked responses.
	SessionWarmup bool

	// WarmupTTL controls how long a warmed-up session is considered fresh.
	// When the TTL expires, the next search re-warms the session.
	// Default: 20 minutes.
	WarmupTTL time.Duration

	// ProxyPool is an optional proxy pool for IP rotation.
	// When set, each request is routed through the next available proxy,
	// with automatic failover and per-proxy health tracking.
	ProxyPool *ProxyPool

	// AdaptiveRateLimit enables a rate limiter that increases the inter-request
	// delay when blocks are detected and gradually relaxes back to base on
	// consecutive successes.
	AdaptiveRateLimit bool

	// AdaptiveBaseDelay is the minimum (and initial) inter-request delay.
	// Default: 300 ms.
	AdaptiveBaseDelay time.Duration

	// AdaptiveMaxDelay is the ceiling for the adaptive delay.
	// Default: 30 s.
	AdaptiveMaxDelay time.Duration

	// GaussianJitter adds normally-distributed randomness to request timing,
	// making inter-request intervals statistically indistinguishable from
	// human browsing. Stddev is AdaptiveBaseDelay × 0.25 by default.
	GaussianJitter bool

	// SessionInvalidateOnBlock resets the cookie jar and warmup state whenever
	// a block signal is detected. This triggers a fresh session on the next
	// request, which can help recover from session-based throttling.
	SessionInvalidateOnBlock bool

	// VQDInvalidateOnBlock clears the cached VQD token for the affected query
	// when a block is detected, forcing a fresh token fetch on retry.
	VQDInvalidateOnBlock bool

	// ChallengeSolvers is an ordered list of CAPTCHA/challenge solvers invoked
	// when a block signal is detected. The first solver that Supports() the
	// signal and succeeds wins; subsequent solvers are not tried. Configure
	// with the most reliable solver first (e.g. FlareSolverr, then 2captcha).
	//
	// Example: NewFlareSolverrSolver(""), NewTwoCaptchaSolver(key)
	ChallengeSolvers []ChallengeSolver

	// CircuitBreakerThreshold is the number of consecutive block responses
	// that opens the circuit breaker, causing all further attempts to fail
	// immediately with ErrCircuitOpen until the cooldown expires.
	// Default: 5. Set to 0 to disable the circuit breaker.
	CircuitBreakerThreshold int

	// CircuitBreakerCooldown is how long the circuit stays open once tripped.
	// Default: 60 s.
	CircuitBreakerCooldown time.Duration
}

// NewAntiBotConfig returns an AntiBotConfig with all recommended techniques enabled
// and sensible defaults. This is the recommended starting point.
func NewAntiBotConfig() *AntiBotConfig {
	return &AntiBotConfig{
		UARotation:               true,
		ChromeTLS:                true,
		SessionWarmup:            true,
		WarmupTTL:                20 * time.Minute,
		AdaptiveRateLimit:        true,
		AdaptiveBaseDelay:        300 * time.Millisecond,
		AdaptiveMaxDelay:         30 * time.Second,
		GaussianJitter:           true,
		SessionInvalidateOnBlock: true,
		VQDInvalidateOnBlock:     true,
		CircuitBreakerThreshold:  5,
		CircuitBreakerCooldown:   60 * time.Second,
	}
}

// circuitBreaker trips open after a run of consecutive block responses and
// keeps the client from hammering a burned session. It resets on the first
// successful response after the cooldown expires.
type circuitBreaker struct {
	threshold   int
	cooldown    time.Duration
	consecutive int
	openUntil   time.Time
	mu          sync.Mutex
}

type circuitSnapshot struct {
	Threshold   int
	Cooldown    time.Duration
	Consecutive int
	OpenUntil   time.Time
}

func newCircuitBreaker(threshold int, cooldown time.Duration) *circuitBreaker {
	if threshold <= 0 {
		threshold = 5
	}
	if cooldown <= 0 {
		cooldown = 60 * time.Second
	}
	return &circuitBreaker{threshold: threshold, cooldown: cooldown}
}

// IsOpen returns true when the breaker is tripped and requests should fail fast.
func (cb *circuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return !cb.openUntil.IsZero() && time.Now().Before(cb.openUntil)
}

// RecordBlock increments the consecutive-block counter and opens the breaker
// once the threshold is reached.
func (cb *circuitBreaker) RecordBlock() (opened bool, snap circuitSnapshot) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutive++
	opened = false
	if cb.consecutive >= cb.threshold {
		wasOpen := !cb.openUntil.IsZero() && time.Now().Before(cb.openUntil)
		cb.openUntil = time.Now().Add(cb.cooldown)
		opened = !wasOpen
	}
	snap = circuitSnapshot{
		Threshold:   cb.threshold,
		Cooldown:    cb.cooldown,
		Consecutive: cb.consecutive,
		OpenUntil:   cb.openUntil,
	}
	return opened, snap
}

// RecordSuccess resets the consecutive counter and closes the breaker.
func (cb *circuitBreaker) RecordSuccess() (closed bool, snap circuitSnapshot) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	closed = !cb.openUntil.IsZero() && time.Now().Before(cb.openUntil)
	cb.consecutive = 0
	cb.openUntil = time.Time{}
	snap = circuitSnapshot{
		Threshold:   cb.threshold,
		Cooldown:    cb.cooldown,
		Consecutive: cb.consecutive,
		OpenUntil:   cb.openUntil,
	}
	return closed, snap
}

// Snapshot returns the current breaker state for diagnostics.
func (cb *circuitBreaker) Snapshot() circuitSnapshot {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return circuitSnapshot{
		Threshold:   cb.threshold,
		Cooldown:    cb.cooldown,
		Consecutive: cb.consecutive,
		OpenUntil:   cb.openUntil,
	}
}

// antiBotState holds the runtime objects built from an AntiBotConfig.
// It is embedded in Client and referenced during every request.
type antiBotState struct {
	cfg       *AntiBotConfig
	session   *sessionManager
	transport *antiBotTransport
	rateLimit *AdaptiveRateLimiter
	solver    *ChainSolver    // wraps cfg.ChallengeSolvers; nil if empty
	circuit   *circuitBreaker // nil when CircuitBreakerThreshold == 0
}

// buildAntiBotState initialises all anti-bot subsystems from the config and
// returns the runtime state plus the http.Client that should be used for requests.
// Returns (nil, nil, nil) when cfg is nil (no anti-bot; caller uses defaults).
func buildAntiBotState(cfg *AntiBotConfig) (*antiBotState, *http.Client, error) {
	if cfg == nil {
		return nil, nil, nil
	}

	st := &antiBotState{cfg: cfg}

	// ── Session / cookie jar ──────────────────────────────────────────────────
	var jar http.CookieJar
	if cfg.SessionWarmup {
		st.session = newSessionManager(cfg.WarmupTTL)
		jar = st.session.CookieJar()
	} else {
		jar, _ = cookiejar.New(nil)
	}

	// ── Adaptive rate limiter ─────────────────────────────────────────────────
	if cfg.AdaptiveRateLimit {
		base := cfg.AdaptiveBaseDelay
		if base <= 0 {
			base = 300 * time.Millisecond
		}
		max := cfg.AdaptiveMaxDelay
		if max <= 0 {
			max = 30 * time.Second
		}
		st.rateLimit = newAdaptiveRateLimiter(base, max)
	}

	// ── Anti-bot transport (Chrome TLS + header injection + proxy rotation) ───
	if cfg.ChromeTLS || cfg.UARotation || cfg.ProxyPool != nil {
		t := newAntiBotTransport(cfg.ProxyPool)
		if cfg.UARotation {
			t.uaPool = NewUserAgentPool()
		}
		st.transport = t
	}

	// ── http.Client ───────────────────────────────────────────────────────────
	var roundTripper http.RoundTripper
	if st.transport != nil {
		roundTripper = st.transport
	}

	hc := &http.Client{
		Jar:       jar,
		Transport: roundTripper, // nil → http.DefaultTransport
		Timeout:   15 * time.Second,
	}

	// ── Challenge solver chain ────────────────────────────────────────────────
	if len(cfg.ChallengeSolvers) > 0 {
		st.solver = &ChainSolver{Solvers: cfg.ChallengeSolvers}
	}

	// ── Circuit breaker ───────────────────────────────────────────────────────
	if cfg.CircuitBreakerThreshold != 0 {
		st.circuit = newCircuitBreaker(cfg.CircuitBreakerThreshold, cfg.CircuitBreakerCooldown)
	}

	return st, hc, nil
}
