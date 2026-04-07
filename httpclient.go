package goddgs

import (
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"time"
)

// NewHTTPClient creates a plain http.Client with an optional proxy.
func NewHTTPClient(timeout time.Duration, proxyURL string) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy url: %w", err)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	return &http.Client{Timeout: timeout, Transport: transport}, nil
}

// NewAntiBotHTTPClient creates an http.Client pre-configured with the full
// anti-bot stack: Chrome TLS fingerprint, browser-profile headers, UA rotation,
// and a persistent cookie jar. It is equivalent to building a Client with
// NewAntiBotConfig() but returns a raw http.Client for use outside of Client.
//
// proxyPool is optional — pass nil for direct connections.
func NewAntiBotHTTPClient(timeout time.Duration, proxyPool *ProxyPool) (*http.Client, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	jar, _ := cookiejar.New(nil)

	t := newAntiBotTransport(proxyPool)
	t.uaPool = NewUserAgentPool()

	return &http.Client{
		Transport: t,
		Jar:       jar,
		Timeout:   timeout,
	}, nil
}
