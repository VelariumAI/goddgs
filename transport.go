package goddgs

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	utls "github.com/refraction-networking/utls"
)

// antiBotTransport is an http.RoundTripper that:
//
//  1. Applies a full browser header profile (Accept, Accept-Language,
//     Accept-Encoding, Sec-CH-UA, Sec-Fetch-*, etc.) matching the active UA.
//  2. Performs TLS handshakes using Chrome-style ClientHello behavior via utls.
//  3. Rotates the active proxy from a ProxyPool on each request (if configured).
type antiBotTransport struct {
	// baseTransport is the underlying http.Transport; its Proxy func reads
	// activeProxy on every dial so we can rotate between requests.
	baseTransport *http.Transport

	// uaPool, if non-nil, rotates the UA on every request. When nil the
	// fixed UA (set on the request by applyHeaders) is used as-is.
	uaPool *UserAgentPool

	// proxyPool drives proxy rotation. activeProxy is updated atomically
	// before each round-trip so baseTransport.Proxy picks it up.
	proxyPool   *ProxyPool
	activeProxy atomic.Pointer[ProxyEntry]

	// lastProxy tracks the entry used for the most recent request so the
	// caller can call MarkSuccess / MarkFailed on the right entry.
	lastProxyMu sync.Mutex
	lastProxy   *ProxyEntry
}

// newAntiBotTransport builds an antiBotTransport.
// If proxyPool is nil, requests go direct. Chrome TLS is always enabled.
func newAntiBotTransport(proxyPool *ProxyPool) *antiBotTransport {
	t := &antiBotTransport{proxyPool: proxyPool}

	t.baseTransport = &http.Transport{
		// Chrome-style TLS behavior via utls.
		DialTLSContext: t.chromeTLSDial,

		// Proxy is resolved per-request so proxy rotation takes effect on each dial.
		Proxy: t.proxyFunc,

		// Mimic Chrome's connection-level behaviour.
		ForceAttemptHTTP2:     false, // utls conn is not *tls.Conn; H2 needs separate setup
		MaxIdleConns:          50,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
	}
	return t
}

// chromeTLSDial performs a TLS handshake using Chrome's ClientHello spec via utls.
// The resulting TLS fingerprint (JA3/JA4) is identical to real Chrome traffic.
func (t *antiBotTransport) chromeTLSDial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	d := net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
	}
	conn, err := d.DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	uconn := utls.UClient(conn, &utls.Config{
		ServerName:         host,
		InsecureSkipVerify: false,
	}, utls.HelloChrome_Auto)

	if err := uconn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}
	return uconn, nil
}

// proxyFunc is the http.Transport.Proxy callback. It reads the currently active
// proxy entry (set by pickAndStoreProxy before each request).
func (t *antiBotTransport) proxyFunc(_ *http.Request) (*url.URL, error) {
	if t.proxyPool == nil {
		return nil, nil
	}
	e := t.activeProxy.Load()
	if e == nil {
		return nil, nil
	}
	return url.Parse(e.URL)
}

// pickAndStoreProxy selects the next proxy from the pool and stores it so
// proxyFunc returns it for the upcoming dial.
func (t *antiBotTransport) pickAndStoreProxy() *ProxyEntry {
	if t.proxyPool == nil {
		return nil
	}
	e := t.proxyPool.Next()
	t.activeProxy.Store(e)
	t.lastProxyMu.Lock()
	t.lastProxy = e
	t.lastProxyMu.Unlock()
	return e
}

// LastProxy returns the ProxyEntry used in the most recent RoundTrip call.
// The caller should call ProxyPool.MarkSuccess / MarkFailed on this entry.
func (t *antiBotTransport) LastProxy() *ProxyEntry {
	t.lastProxyMu.Lock()
	defer t.lastProxyMu.Unlock()
	return t.lastProxy
}

// RoundTrip implements http.RoundTripper.
func (t *antiBotTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 1. Pick next proxy before the dial happens.
	t.pickAndStoreProxy()

	// 2. Clone the request so we don't mutate the caller's headers.
	r := req.Clone(req.Context())
	if r.Header == nil {
		r.Header = make(http.Header)
	}

	// 3. Resolve the active user-agent.
	ua := r.Header.Get("User-Agent")
	if t.uaPool != nil {
		// Rotation mode: override with a freshly picked UA.
		ua = t.uaPool.PickUA()
		r.Header.Set("User-Agent", ua)
	} else if ua == "" {
		ua = defaultUserAgent
		r.Header.Set("User-Agent", ua)
	}

	// 4. Apply the full browser profile + endpoint-specific Sec-Fetch-* headers.
	profile := buildProfile(ua)
	secFetch := inferSecFetch(r)
	applyProfile(r, profile, secFetch)

	return t.baseTransport.RoundTrip(r)
}

// inferSecFetch determines the correct Sec-Fetch-* headers by examining the
// request URL and method, mimicking what a real browser sends for each endpoint.
func inferSecFetch(req *http.Request) map[string]string {
	host := strings.ToLower(req.URL.Host)

	switch {
	// links.duckduckgo.com/d.js — loaded as a <script> resource.
	case strings.HasPrefix(host, "links.") && strings.HasSuffix(req.URL.Path, ".js"):
		return secFetchScript()

	// POST form submissions (VQD fetch, HTML search).
	case req.Method == http.MethodPost:
		if strings.Contains(host, "duckduckgo.com") {
			return secFetchNavigation("same-origin")
		}
		return secFetchNavigation("cross-site")

	// All other GETs — treat as top-level page navigation.
	default:
		return secFetchNavigation("none")
	}
}
