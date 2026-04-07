package goddgs

import (
	"math/rand"
	"regexp"
	"strings"
)

// UAEntry is a user-agent string with its browser family and selection weight.
type UAEntry struct {
	UA     string
	Family string // "chrome", "edge", "firefox", "safari"
	Weight int
}

// UserAgentPool provides weighted random selection of realistic browser user-agents.
// Weights approximate global browser market share.
type UserAgentPool struct {
	entries []UAEntry
	total   int
}

// NewUserAgentPool returns a pool pre-populated with realistic browser UAs
// weighted by approximate global market share (Chrome ~65%, Safari ~19%,
// Firefox ~4%, Edge ~4%).
func NewUserAgentPool() *UserAgentPool {
	p := &UserAgentPool{}
	for _, e := range defaultUAs {
		p.entries = append(p.entries, e)
		p.total += e.Weight
	}
	return p
}

//nolint:lll
var defaultUAs = []UAEntry{
	// ── Chrome / Windows (~35%) ───────────────────────────────────────────────
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`, "chrome", 15},
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36`, "chrome", 10},
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36`, "chrome", 6},
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36`, "chrome", 3},
	// ── Chrome / macOS (~15%) ────────────────────────────────────────────────
	{`Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`, "chrome", 12},
	{`Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36`, "chrome", 7},
	{`Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36`, "chrome", 4},
	// ── Chrome / Linux (~5%) ─────────────────────────────────────────────────
	{`Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36`, "chrome", 4},
	{`Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36`, "chrome", 3},
	// ── Chrome / Android (~8%) ───────────────────────────────────────────────
	{`Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.82 Mobile Safari/537.36`, "chrome", 4},
	{`Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.6367.82 Mobile Safari/537.36`, "chrome", 3},
	{`Mozilla/5.0 (Linux; Android 13; Pixel 7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.6312.99 Mobile Safari/537.36`, "chrome", 2},
	// ── Edge / Windows (~4%) ─────────────────────────────────────────────────
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36 Edg/124.0.0.0`, "edge", 4},
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/123.0.0.0 Safari/537.36 Edg/123.0.0.0`, "edge", 2},
	// ── Safari / macOS (~9%) ─────────────────────────────────────────────────
	{`Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4_1) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Safari/605.1.15`, "safari", 9},
	{`Mozilla/5.0 (Macintosh; Intel Mac OS X 14_3) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3.1 Safari/605.1.15`, "safari", 5},
	{`Mozilla/5.0 (Macintosh; Intel Mac OS X 13_6_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.2 Safari/605.1.15`, "safari", 3},
	// ── Safari / iPhone (~8%) ────────────────────────────────────────────────
	{`Mozilla/5.0 (iPhone; CPU iPhone OS 17_4_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4.1 Mobile/15E148 Safari/604.1`, "safari", 6},
	{`Mozilla/5.0 (iPhone; CPU iPhone OS 17_3_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.3 Mobile/15E148 Safari/604.1`, "safari", 3},
	// ── Firefox / Windows (~3%) ──────────────────────────────────────────────
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:125.0) Gecko/20100101 Firefox/125.0`, "firefox", 3},
	{`Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:124.0) Gecko/20100101 Firefox/124.0`, "firefox", 2},
	// ── Firefox / macOS + Linux (~1%) ────────────────────────────────────────
	{`Mozilla/5.0 (Macintosh; Intel Mac OS X 14.4; rv:125.0) Gecko/20100101 Firefox/125.0`, "firefox", 2},
	{`Mozilla/5.0 (X11; Linux x86_64; rv:125.0) Gecko/20100101 Firefox/125.0`, "firefox", 1},
}

// Pick returns a random UAEntry, weighted by browser market share.
func (p *UserAgentPool) Pick() UAEntry {
	n := rand.Intn(p.total)
	for _, e := range p.entries {
		n -= e.Weight
		if n < 0 {
			return e
		}
	}
	return p.entries[len(p.entries)-1]
}

// PickUA returns just the user-agent string.
func (p *UserAgentPool) PickUA() string { return p.Pick().UA }

// chromeMajorRe extracts the major version number from a Chrome/Edge UA string.
var chromeMajorRe = regexp.MustCompile(`Chrome/(\d+)\.`)

// SecCHUA derives the Sec-CH-UA header value for Chromium-based user-agents.
// Returns "" for non-Chromium browsers (Firefox, Safari).
func SecCHUA(ua string) string {
	m := chromeMajorRe.FindStringSubmatch(ua)
	if m == nil {
		return ""
	}
	v := m[1]
	if strings.Contains(ua, "Edg/") {
		return `"Chromium";v="` + v + `", "Not A(Brand";v="24", "Microsoft Edge";v="` + v + `"`
	}
	return `"Chromium";v="` + v + `", "Not A(Brand";v="24", "Google Chrome";v="` + v + `"`
}

// SecCHUAMobile returns "?1" for mobile user-agents, "?0" otherwise.
func SecCHUAMobile(ua string) string {
	if strings.Contains(ua, "Mobile") {
		return "?1"
	}
	return "?0"
}

// SecCHUAPlatform returns the Sec-CH-UA-Platform hint for the given user-agent.
func SecCHUAPlatform(ua string) string {
	switch {
	case strings.Contains(ua, "Windows"):
		return `"Windows"`
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"):
		return `"iOS"`
	case strings.Contains(ua, "Android"):
		return `"Android"`
	case strings.Contains(ua, "Macintosh"), strings.Contains(ua, "Mac OS X"):
		return `"macOS"`
	case strings.Contains(ua, "Linux"):
		return `"Linux"`
	default:
		return `"Unknown"`
	}
}

// uaFamily infers the browser family from a user-agent string.
// singleUAPool returns a UserAgentPool that always yields the given UA string.
// Used to pin the transport to the exact UA returned by a challenge solver,
// since cf_clearance cookies are tied to the UA used during solving.
func singleUAPool(ua string) *UserAgentPool {
	return &UserAgentPool{
		entries: []UAEntry{{UA: ua, Weight: 1}},
		total:   1,
	}
}

func uaFamily(ua string) string {
	switch {
	case strings.Contains(ua, "Firefox"):
		return "firefox"
	case strings.Contains(ua, "Edg/"):
		return "edge"
	case strings.Contains(ua, "Safari") && !strings.Contains(ua, "Chrome"):
		return "safari"
	default:
		return "chrome"
	}
}
