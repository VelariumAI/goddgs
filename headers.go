package goddgs

import (
	"net/http"
)

// browserProfile holds the ordered set of HTTP headers a browser family sends.
type browserProfile struct {
	Family  string
	headers []hPair
}

type hPair struct{ Key, Value string }

// buildProfile returns the complete browser header profile for the given user-agent.
func buildProfile(ua string) browserProfile {
	switch uaFamily(ua) {
	case "firefox":
		return firefoxProfile(ua)
	case "safari":
		return safariProfile(ua)
	default: // chrome, edge
		return chromeProfile(ua)
	}
}

func chromeProfile(ua string) browserProfile {
	pairs := []hPair{
		{"User-Agent", ua},
		{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		{"Accept-Language", "en-US,en;q=0.9"},
		{"Accept-Encoding", "gzip, deflate, br"},
		{"Connection", "keep-alive"},
	}
	if secCHUA := SecCHUA(ua); secCHUA != "" {
		pairs = append(pairs,
			hPair{"Sec-CH-UA", secCHUA},
			hPair{"Sec-CH-UA-Mobile", SecCHUAMobile(ua)},
			hPair{"Sec-CH-UA-Platform", SecCHUAPlatform(ua)},
		)
	}
	return browserProfile{Family: "chrome", headers: pairs}
}

func firefoxProfile(ua string) browserProfile {
	return browserProfile{
		Family: "firefox",
		headers: []hPair{
			{"User-Agent", ua},
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.5"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
			{"TE", "trailers"},
		},
	}
}

func safariProfile(ua string) browserProfile {
	return browserProfile{
		Family: "safari",
		headers: []hPair{
			{"User-Agent", ua},
			{"Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"},
			{"Accept-Language", "en-US,en;q=0.9"},
			{"Accept-Encoding", "gzip, deflate, br"},
			{"Connection", "keep-alive"},
		},
	}
}

// applyProfile sets all browser-profile headers on req that are not already present,
// then applies any extra overrides (e.g. Sec-Fetch-* headers).
func applyProfile(req *http.Request, profile browserProfile, extra map[string]string) {
	for _, kv := range profile.headers {
		if req.Header.Get(kv.Key) == "" {
			req.Header.Set(kv.Key, kv.Value)
		}
	}
	for k, v := range extra {
		req.Header.Set(k, v)
	}
}

// secFetchNavigation returns Sec-Fetch-* headers for a top-level page navigation
// (browser address bar, link click, form submit).
// site must be "none", "same-origin", or "cross-site".
func secFetchNavigation(site string) map[string]string {
	return map[string]string{
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            site,
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
	}
}

// secFetchScript returns Sec-Fetch-* headers for a <script src="..."> resource load.
func secFetchScript() map[string]string {
	return map[string]string{
		"Sec-Fetch-Dest": "script",
		"Sec-Fetch-Mode": "no-cors",
		"Sec-Fetch-Site": "cross-site",
	}
}

// secFetchXHR returns Sec-Fetch-* headers for a cross-site XMLHttpRequest / fetch().
func secFetchXHR(site string) map[string]string {
	return map[string]string{
		"Sec-Fetch-Dest": "empty",
		"Sec-Fetch-Mode": "cors",
		"Sec-Fetch-Site": site,
	}
}
