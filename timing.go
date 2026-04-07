package goddgs

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"
)

// gaussianDuration draws a duration from a Gaussian (Normal) distribution using
// the Box-Muller transform. The result is NOT clamped — call clampDuration after.
func gaussianDuration(mean, stddev time.Duration) time.Duration {
	u1 := rand.Float64()
	if u1 == 0 {
		u1 = 1e-10 // avoid log(0)
	}
	u2 := rand.Float64()
	z := math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
	return time.Duration(float64(mean) + float64(stddev)*z)
}

func clampDuration(d, min, max time.Duration) time.Duration {
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}

// GaussianSleep sleeps for a duration drawn from N(mean, stddev²), clamped to
// [minDelay, maxDelay]. It respects context cancellation.
func GaussianSleep(ctx context.Context, mean, stddev, minDelay, maxDelay time.Duration) error {
	d := clampDuration(gaussianDuration(mean, stddev), minDelay, maxDelay)
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// AdaptiveRateLimiter enforces a minimum gap between requests and automatically
// adjusts that gap in response to block/success signals.
//
// On block:   current = min(current × growFactor,  maxDelay)
// On success: current = max(current × shrinkFactor, baseDelay)
//
// Wait() blocks until the gap since the last request is satisfied, using
// Gaussian jitter around the current delay.
type AdaptiveRateLimiter struct {
	mu           sync.Mutex
	base         time.Duration
	current      time.Duration
	max          time.Duration
	growFactor   float64 // multiplier applied on block  (e.g. 2.0)
	shrinkFactor float64 // multiplier applied on success (e.g. 0.85)
	jitterFrac   float64 // stddev as fraction of current delay (e.g. 0.25)
	lastReq      time.Time
}

// newAdaptiveRateLimiter creates a limiter with the given base and max delays.
func newAdaptiveRateLimiter(base, max time.Duration) *AdaptiveRateLimiter {
	return &AdaptiveRateLimiter{
		base:         base,
		current:      base,
		max:          max,
		growFactor:   2.0,
		shrinkFactor: 0.85,
		jitterFrac:   0.25,
	}
}

// Wait blocks until the inter-request gap is satisfied. It applies Gaussian
// jitter around the current delay and respects context cancellation.
func (r *AdaptiveRateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	delay := r.current
	stddev := time.Duration(float64(delay) * r.jitterFrac)
	jittered := clampDuration(gaussianDuration(delay, stddev), r.base/2, r.max*2)

	now := time.Now()
	var wait time.Duration
	if !r.lastReq.IsZero() {
		elapsed := now.Sub(r.lastReq)
		if elapsed < jittered {
			wait = jittered - elapsed
		}
	}
	r.lastReq = now.Add(wait) // reserve the slot optimistically
	r.mu.Unlock()

	if wait <= 0 {
		return nil
	}
	t := time.NewTimer(wait)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// OnBlock increases the inter-request delay (called when a block is detected).
func (r *AdaptiveRateLimiter) OnBlock() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.current = clampDuration(time.Duration(float64(r.current)*r.growFactor), r.base, r.max)
}

// OnSuccess gradually relaxes the delay back toward base (called on clean responses).
func (r *AdaptiveRateLimiter) OnSuccess() {
	r.mu.Lock()
	defer r.mu.Unlock()
	shrunk := time.Duration(float64(r.current) * r.shrinkFactor)
	if shrunk < r.base {
		shrunk = r.base
	}
	r.current = shrunk
}

// Current returns the current configured inter-request delay.
func (r *AdaptiveRateLimiter) Current() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.current
}
