package goddgs

import (
	"fmt"
	"math/rand"
	"net/url"
	"sync"
	"time"
)

// RotationStrategy controls how ProxyPool selects the next proxy.
type RotationStrategy int

const (
	RotateRoundRobin RotationStrategy = iota // cycle through proxies in order
	RotateRandom                             // uniform random selection
	RotateWeighted                           // weighted random based on Weight field
)

// ProxyEntry is a single proxy URL with health-tracking state.
type ProxyEntry struct {
	URL    string
	Weight int // relative weight for RotateWeighted (default 1)

	mu               sync.Mutex
	consecutiveFails int
	lastFail         time.Time
	requests         int64
	failures         int64
}

// ProxyStats is an immutable snapshot of a ProxyEntry's metrics.
type ProxyStats struct {
	URL              string
	Requests         int64
	Failures         int64
	ConsecutiveFails int
	InCooldown       bool
}

// ProxyPool manages a set of HTTP/HTTPS/SOCKS5 proxies with automatic health
// tracking and configurable rotation strategy.
type ProxyPool struct {
	entries  []*ProxyEntry
	strategy RotationStrategy
	cooldown time.Duration // skip a proxy for this long after maxFails consecutive failures
	maxFails int           // consecutive failures that trigger cooldown

	mu      sync.Mutex
	rrIndex int // next round-robin position
	total   int // sum of weights (for RotateWeighted)
}

// NewProxyPool creates a pool from a list of proxy URLs.
// Each URL must be in the form "http://host:port", "https://...", or "socks5://...".
// Returns an error if any URL is unparseable.
func NewProxyPool(proxyURLs []string, strategy RotationStrategy) (*ProxyPool, error) {
	p := &ProxyPool{
		strategy: strategy,
		cooldown: 60 * time.Second,
		maxFails: 3,
	}
	for _, raw := range proxyURLs {
		if _, err := url.Parse(raw); err != nil {
			return nil, fmt.Errorf("proxypool: invalid proxy url %q: %w", raw, err)
		}
		e := &ProxyEntry{URL: raw, Weight: 1}
		p.entries = append(p.entries, e)
		p.total++
	}
	return p, nil
}

// SetCooldown adjusts the health-failure policy.
func (p *ProxyPool) SetCooldown(cooldown time.Duration, maxConsecFails int) {
	p.mu.Lock()
	p.cooldown = cooldown
	p.maxFails = maxConsecFails
	p.mu.Unlock()
}

// SetWeight sets the selection weight of a proxy by URL (used for RotateWeighted).
func (p *ProxyPool) SetWeight(proxyURL string, weight int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if e.URL == proxyURL {
			p.total += weight - e.Weight
			e.Weight = weight
			return
		}
	}
}

// Next returns the next available proxy according to the rotation strategy.
// Returns nil if all proxies are currently in cooldown.
func (p *ProxyPool) Next() *ProxyEntry {
	p.mu.Lock()
	defer p.mu.Unlock()

	avail := p.available()
	if len(avail) == 0 {
		return nil
	}
	switch p.strategy {
	case RotateRandom:
		return avail[rand.Intn(len(avail))]
	case RotateWeighted:
		return p.weightedPick(avail)
	default: // RotateRoundRobin
		e := avail[p.rrIndex%len(avail)]
		p.rrIndex++
		return e
	}
}

// available returns entries not in cooldown. Caller holds p.mu.
func (p *ProxyPool) available() []*ProxyEntry {
	out := make([]*ProxyEntry, 0, len(p.entries))
	for _, e := range p.entries {
		e.mu.Lock()
		ok := e.consecutiveFails < p.maxFails || time.Since(e.lastFail) > p.cooldown
		e.mu.Unlock()
		if ok {
			out = append(out, e)
		}
	}
	return out
}

func (p *ProxyPool) weightedPick(entries []*ProxyEntry) *ProxyEntry {
	total := 0
	for _, e := range entries {
		total += e.Weight
	}
	if total == 0 {
		return entries[rand.Intn(len(entries))]
	}
	n := rand.Intn(total)
	for _, e := range entries {
		n -= e.Weight
		if n < 0 {
			return e
		}
	}
	return entries[len(entries)-1]
}

// MarkSuccess records a successful request through entry.
func (p *ProxyPool) MarkSuccess(e *ProxyEntry) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.consecutiveFails = 0
	e.requests++
	e.mu.Unlock()
}

// MarkFailed records a failed request through entry and may put it in cooldown.
func (p *ProxyPool) MarkFailed(e *ProxyEntry) {
	if e == nil {
		return
	}
	e.mu.Lock()
	e.consecutiveFails++
	e.lastFail = time.Now()
	e.requests++
	e.failures++
	e.mu.Unlock()
}

// Stats returns an immutable snapshot of every proxy's health metrics.
func (p *ProxyPool) Stats() []ProxyStats {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]ProxyStats, len(p.entries))
	for i, e := range p.entries {
		e.mu.Lock()
		out[i] = ProxyStats{
			URL:              e.URL,
			Requests:         e.requests,
			Failures:         e.failures,
			ConsecutiveFails: e.consecutiveFails,
			InCooldown:       e.consecutiveFails >= p.maxFails && time.Since(e.lastFail) <= p.cooldown,
		}
		e.mu.Unlock()
	}
	return out
}

// Len returns the total number of proxies in the pool.
func (p *ProxyPool) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.entries)
}
