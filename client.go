package goddgs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultDuckDuckGoBase = "https://duckduckgo.com"
	defaultLinksBase      = "https://links.duckduckgo.com"
	defaultHTMLBase       = "https://html.duckduckgo.com"
	defaultUserAgent      = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0 Safari/537.36"
)

var (
	ErrNoVQD          = errors.New("goddgs: vqd token not found")
	ErrNoResults      = errors.New("goddgs: no results found")
	ErrUnexpectedBody = errors.New("goddgs: unexpected response body")
	ErrBlocked        = errors.New("goddgs: response appears blocked")
	// ErrCircuitOpen is returned when the circuit breaker has tripped after too
	// many consecutive block responses. Wait for CircuitBreakerCooldown before retrying.
	ErrCircuitOpen = errors.New("goddgs: circuit breaker open — session is burned, retry later")
)

type BlockedEvent struct {
	StatusCode  int
	Headers     http.Header
	BodySnippet string
	Detector    string
	Attempt     int
}

type CircuitState string

const (
	CircuitStateClosed CircuitState = "closed"
	CircuitStateOpen   CircuitState = "open"
)

type CircuitEvent struct {
	State             CircuitState
	Trigger           string
	Attempt           int
	ConsecutiveBlocks int
	Threshold         int
	Cooldown          time.Duration
	OpenUntil         time.Time
}

type BlockedError struct {
	Event BlockedEvent
}

func (e *BlockedError) Error() string {
	if e == nil {
		return ErrBlocked.Error()
	}
	if e.Event.StatusCode > 0 {
		return fmt.Sprintf("%s (status=%d detector=%s)", ErrBlocked.Error(), e.Event.StatusCode, e.Event.Detector)
	}
	return fmt.Sprintf("%s (detector=%s)", ErrBlocked.Error(), e.Event.Detector)
}

func (e *BlockedError) Unwrap() error { return ErrBlocked }

func IsBlocked(err error) bool { return errors.Is(err, ErrBlocked) }

type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

type SearchOptions struct {
	MaxResults int
	Region     string
	SafeSearch SafeSearch
	TimeRange  string
	Offset     int
}

type SafeSearch int

const (
	SafeModerate SafeSearch = iota
	SafeOff
	SafeStrict
)

func (s SafeSearch) ddgParam() string {
	switch s {
	case SafeOff:
		return "-2"
	case SafeStrict:
		return "1"
	default:
		return "-1"
	}
}

type Options struct {
	HTTPClient          *http.Client
	DuckDuckGoBase      string
	LinksBase           string
	HTMLBase            string
	UserAgent           string
	Referer             string
	Headers             map[string]string
	RequestTimeout      time.Duration
	RetryMax            int
	RetryBaseDelay      time.Duration
	RetryJitterFrac     float64
	MinRequestInterval  time.Duration
	VQDTTL              time.Duration
	DisableHTMLFallback bool
	BlockedStatusCodes  map[int]struct{}
	BlockedBodyPatterns []*regexp.Regexp
	OnBlocked           func(BlockedEvent)
	OnCircuit           func(CircuitEvent)

	// AntiBot enables advanced browser-compatible transport/session behavior.
	// Use NewAntiBotConfig() for
	// recommended defaults, or nil to keep the original behaviour.
	// When set and Options.HTTPClient is nil, the client is built with Chrome TLS
	// fingerprinting, browser-profile headers, and the configured cookie jar.
	AntiBot *AntiBotConfig
}

type tokenEntry struct {
	Value     string
	ExpiresAt time.Time
}

type Client struct {
	httpClient *http.Client
	ddgBase    string
	linksBase  string
	htmlBase   string
	ua         string
	referer    string
	headers    map[string]string
	retryMax   int
	baseDelay  time.Duration
	jitterFrac float64
	vqdTTL     time.Duration
	minGap     time.Duration
	noFallback bool
	blocked    map[int]struct{}
	blockedRe  []*regexp.Regexp
	onBlocked  func(BlockedEvent)
	onCircuit  func(CircuitEvent)

	mu       sync.RWMutex
	vqdCache map[string]tokenEntry
	rateMu   sync.Mutex
	lastReq  time.Time

	// Anti-bot runtime state (nil when AntiBot option is not set).
	antiBot *antiBotState
}

func NewClient(opts Options) *Client {
	timeout := opts.RequestTimeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	// Build anti-bot runtime state before creating the http.Client so the
	// anti-bot transport (Chrome TLS, cookie jar) is wired in from the start.
	var abState *antiBotState
	if opts.AntiBot != nil {
		st, abClient, err := buildAntiBotState(opts.AntiBot)
		if err == nil {
			abState = st
			// Only use the anti-bot http.Client when the caller didn't supply one.
			if opts.HTTPClient == nil && abClient != nil {
				abClient.Timeout = timeout
				opts.HTTPClient = abClient
			}
		}
	}

	h := opts.HTTPClient
	if h == nil {
		h = &http.Client{Timeout: timeout}
	}
	ddgBase := strings.TrimRight(opts.DuckDuckGoBase, "/")
	if ddgBase == "" {
		ddgBase = defaultDuckDuckGoBase
	}
	linksBase := strings.TrimRight(opts.LinksBase, "/")
	if linksBase == "" {
		linksBase = defaultLinksBase
	}
	htmlBase := strings.TrimRight(opts.HTMLBase, "/")
	if htmlBase == "" {
		htmlBase = defaultHTMLBase
	}
	ua := strings.TrimSpace(opts.UserAgent)
	if ua == "" {
		ua = defaultUserAgent
	}
	referer := strings.TrimSpace(opts.Referer)
	if referer == "" {
		referer = ddgBase + "/"
	}
	retryMax := opts.RetryMax
	if retryMax <= 0 {
		retryMax = 3
	}
	baseDelay := opts.RetryBaseDelay
	if baseDelay <= 0 {
		baseDelay = 250 * time.Millisecond
	}
	jitter := opts.RetryJitterFrac
	if jitter <= 0 {
		jitter = 0.2
	}
	vqdTTL := opts.VQDTTL
	if vqdTTL <= 0 {
		vqdTTL = 10 * time.Minute
	}
	blocked := opts.BlockedStatusCodes
	if blocked == nil {
		blocked = map[int]struct{}{401: {}, 403: {}, 407: {}, 429: {}, 500: {}, 502: {}, 503: {}, 504: {}}
	}
	blockedRe := opts.BlockedBodyPatterns
	if len(blockedRe) == 0 {
		blockedRe = []*regexp.Regexp{
			regexp.MustCompile(`(?i)captcha`),
			regexp.MustCompile(`(?i)verify you are human`),
			regexp.MustCompile(`(?i)automated traffic`),
			regexp.MustCompile(`(?i)rate limit`),
		}
	}

	headers := map[string]string{}
	for k, v := range opts.Headers {
		headers[k] = v
	}

	return &Client{
		httpClient: h,
		ddgBase:    ddgBase,
		linksBase:  linksBase,
		htmlBase:   htmlBase,
		ua:         ua,
		referer:    referer,
		headers:    headers,
		retryMax:   retryMax,
		baseDelay:  baseDelay,
		jitterFrac: jitter,
		vqdTTL:     vqdTTL,
		minGap:     opts.MinRequestInterval,
		noFallback: opts.DisableHTMLFallback,
		blocked:    blocked,
		blockedRe:  blockedRe,
		onBlocked:  opts.OnBlocked,
		onCircuit:  opts.OnCircuit,
		vqdCache:   map[string]tokenEntry{},
		antiBot:    abState,
	}
}

func (c *Client) Search(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("goddgs: query is required")
	}
	max := opts.MaxResults
	if max <= 0 {
		max = 10
	}
	if opts.Region == "" {
		opts.Region = "us-en"
	}

	// Session warmup: acquire DDG session cookies before the first real search.
	if c.antiBot != nil && c.antiBot.session != nil && c.antiBot.session.NeedsWarmup() {
		_ = c.antiBot.session.Warmup(ctx, c.httpClient, c.ddgBase+"/", func(ctx context.Context, method, rawURL string) (*http.Request, error) {
			req, e := http.NewRequestWithContext(ctx, method, rawURL, nil)
			if e != nil {
				return nil, e
			}
			c.applyHeaders(req)
			return req, nil
		})
	}

	results, err := c.searchDJS(ctx, query, opts)
	if err == nil && len(results) > 0 {
		if len(results) > max {
			results = results[:max]
		}
		return results, nil
	}

	if c.noFallback {
		if err != nil {
			return nil, err
		}
		return nil, ErrNoResults
	}

	fallback, fbErr := c.searchHTML(ctx, query, opts)
	if fbErr != nil {
		if err != nil {
			return nil, fmt.Errorf("goddgs: d.js failed (%v), html fallback failed (%w)", err, fbErr)
		}
		return nil, fbErr
	}
	if len(fallback) == 0 {
		return nil, ErrNoResults
	}
	if len(fallback) > max {
		fallback = fallback[:max]
	}
	return fallback, nil
}

func (c *Client) SearchPages(ctx context.Context, query string, perPage, pages int, opts SearchOptions) ([]Result, error) {
	if pages <= 0 {
		pages = 1
	}
	if perPage <= 0 {
		perPage = 30
	}
	all := make([]Result, 0, perPage*pages)
	for i := 0; i < pages; i++ {
		curr := opts
		curr.MaxResults = perPage
		curr.Offset = opts.Offset + i*30
		results, err := c.Search(ctx, query, curr)
		if err != nil {
			if errors.Is(err, ErrNoResults) {
				break
			}
			return all, err
		}
		all = append(all, results...)
		if len(results) < perPage {
			break
		}
	}
	return all, nil
}

func (c *Client) searchDJS(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
	// getVQD is called inside the closure so that each retry attempt fetches a
	// fresh token when VQDInvalidateOnBlock has cleared the cache on a previous
	// attempt. Pre-capturing vqd once would leave retries with a burned token.
	body, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		vqd, e := c.getVQD(ctx, query)
		if e != nil {
			return nil, e
		}
		params := url.Values{
			"q":   {query},
			"vqd": {vqd},
			"l":   {opts.Region},
			"p":   {"1"},
			"s":   {strconv.Itoa(opts.Offset)},
			"df":  {opts.TimeRange},
			"ex":  {"-1"},
			"kp":  {opts.SafeSearch.ddgParam()},
		}
		req, e := http.NewRequestWithContext(ctx, http.MethodGet, c.linksBase+"/d.js?"+params.Encode(), nil)
		if e != nil {
			return nil, e
		}
		c.applyHeaders(req)
		req.Header.Set("Accept", "*/*")
		return req, nil
	})
	if err != nil {
		return nil, err
	}

	results, err := parseDJSResults(body)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, ErrNoResults
	}
	return results, nil
}

func (c *Client) searchHTML(ctx context.Context, query string, opts SearchOptions) ([]Result, error) {
	params := url.Values{
		"q":  {query},
		"s":  {strconv.Itoa(opts.Offset)},
		"kl": {opts.Region},
		"kp": {opts.SafeSearch.ddgParam()},
	}
	if opts.TimeRange != "" {
		params.Set("df", opts.TimeRange)
	}

	body, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.htmlBase+"/html/", strings.NewReader(params.Encode()))
		if e != nil {
			return nil, e
		}
		c.applyHeaders(req)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Referer", c.htmlBase+"/")
		return req, nil
	})
	if err != nil {
		return nil, err
	}

	results := parseHTMLResults(body)
	if len(results) == 0 {
		return nil, ErrNoResults
	}
	return results, nil
}

func (c *Client) getVQD(ctx context.Context, query string) (string, error) {
	cacheKey := query + "|" + c.ua
	now := time.Now()

	c.mu.RLock()
	if entry, ok := c.vqdCache[cacheKey]; ok && entry.Value != "" && now.Before(entry.ExpiresAt) {
		c.mu.RUnlock()
		return entry.Value, nil
	}
	c.mu.RUnlock()

	values := url.Values{"q": {query}}
	body, err := c.doWithRetry(ctx, func() (*http.Request, error) {
		req, e := http.NewRequestWithContext(ctx, http.MethodPost, c.ddgBase+"/", strings.NewReader(values.Encode()))
		if e != nil {
			return nil, e
		}
		c.applyHeaders(req)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req, nil
	})
	if err != nil {
		return "", err
	}

	vqd := extractVQD(body)
	if vqd == "" {
		return "", ErrNoVQD
	}

	c.mu.Lock()
	c.vqdCache[cacheKey] = tokenEntry{Value: vqd, ExpiresAt: now.Add(c.vqdTTL)}
	c.mu.Unlock()
	return vqd, nil
}

func (c *Client) doWithRetry(ctx context.Context, buildReq func() (*http.Request, error)) ([]byte, error) {
	var lastErr error
	for i := 0; i < c.retryMax; i++ {
		// Circuit breaker: fail fast when the session is known-burned.
		if c.antiBot != nil && c.antiBot.circuit != nil && c.antiBot.circuit.IsOpen() {
			if c.onCircuit != nil {
				snap := c.antiBot.circuit.Snapshot()
				c.onCircuit(CircuitEvent{
					State:             CircuitStateOpen,
					Trigger:           "fail_fast",
					Attempt:           i + 1,
					ConsecutiveBlocks: snap.Consecutive,
					Threshold:         snap.Threshold,
					Cooldown:          snap.Cooldown,
					OpenUntil:         snap.OpenUntil,
				})
			}
			return nil, ErrCircuitOpen
		}

		// Rate limiting: adaptive (anti-bot) takes priority over fixed minGap.
		if c.antiBot != nil && c.antiBot.rateLimit != nil {
			if err := c.antiBot.rateLimit.Wait(ctx); err != nil {
				return nil, err
			}
		} else {
			if err := c.waitForTurn(ctx); err != nil {
				return nil, err
			}
		}

		req, err := buildReq()
		if err != nil {
			return nil, err
		}

		// lastBody is hoisted so the block handler can forward it to the solver.
		var lastBody []byte

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastBody = body
			if readErr != nil {
				lastErr = readErr
			} else if _, blocked := c.blocked[resp.StatusCode]; blocked {
				hdrs := cloneHeader(resp.Header)
				lastErr = &BlockedError{Event: BlockedEvent{
					StatusCode:  resp.StatusCode,
					Headers:     hdrs,
					BodySnippet: snippet(body, 280),
					Detector:    "status",
					Attempt:     i + 1,
				}}
				// Respect Retry-After before the next attempt.
				if secs := RetryAfterSeconds(hdrs); secs > 0 && i < c.retryMax-1 {
					if err2 := sleepContext(ctx, time.Duration(secs)*time.Second); err2 != nil {
						return nil, err2
					}
				}
			} else if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if det := c.blockedDetector(body); det != "" {
					lastErr = &BlockedError{Event: BlockedEvent{
						StatusCode:  resp.StatusCode,
						Headers:     cloneHeader(resp.Header),
						BodySnippet: snippet(body, 280),
						Detector:    det,
						Attempt:     i + 1,
					}}
				} else {
					// Success path: notify anti-bot systems.
					if c.antiBot != nil {
						if c.antiBot.rateLimit != nil {
							c.antiBot.rateLimit.OnSuccess()
						}
						if c.antiBot.circuit != nil {
							if closed, snap := c.antiBot.circuit.RecordSuccess(); closed && c.onCircuit != nil {
								c.onCircuit(CircuitEvent{
									State:             CircuitStateClosed,
									Trigger:           "success_reset",
									Attempt:           i + 1,
									ConsecutiveBlocks: snap.Consecutive,
									Threshold:         snap.Threshold,
									Cooldown:          snap.Cooldown,
									OpenUntil:         snap.OpenUntil,
								})
							}
						}
						if c.antiBot.cfg.ProxyPool != nil && c.antiBot.transport != nil {
							c.antiBot.cfg.ProxyPool.MarkSuccess(c.antiBot.transport.LastProxy())
						}
					}
					return body, nil
				}
			} else if resp.StatusCode == 429 || resp.StatusCode >= 500 {
				lastErr = fmt.Errorf("http %d", resp.StatusCode)
			} else {
				return nil, fmt.Errorf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
			}
		}

		// Block-response handling: fire callback, notify anti-bot systems,
		// and attempt challenge solving before the normal backoff/retry.
		if be, ok := lastErr.(*BlockedError); ok {
			if c.onBlocked != nil {
				c.onBlocked(be.Event)
			}
			if c.antiBot != nil {
				if c.antiBot.rateLimit != nil {
					c.antiBot.rateLimit.OnBlock()
				}
				if c.antiBot.circuit != nil {
					if opened, snap := c.antiBot.circuit.RecordBlock(); opened && c.onCircuit != nil {
						c.onCircuit(CircuitEvent{
							State:             CircuitStateOpen,
							Trigger:           "threshold_reached",
							Attempt:           i + 1,
							ConsecutiveBlocks: snap.Consecutive,
							Threshold:         snap.Threshold,
							Cooldown:          snap.Cooldown,
							OpenUntil:         snap.OpenUntil,
						})
					}
				}
				if c.antiBot.cfg.ProxyPool != nil && c.antiBot.transport != nil {
					c.antiBot.cfg.ProxyPool.MarkFailed(c.antiBot.transport.LastProxy())
				}
				if c.antiBot.cfg.VQDInvalidateOnBlock {
					c.clearAllVQD()
				}
				if c.antiBot.cfg.SessionInvalidateOnBlock && c.antiBot.session != nil {
					c.antiBot.session.Invalidate()
				}

				// Attempt challenge solving. On success apply the credentials
				// and retry immediately without burning a retry slot: the solve
				// itself already constituted a real attempt at the challenge.
				if c.antiBot.solver != nil && req != nil {
					info := DetectBlockSignal(be.Event.StatusCode, be.Event.Headers, lastBody)
					if sol, solErr := c.antiBot.solver.Solve(ctx, req.URL.String(), info, lastBody); solErr == nil && sol != nil {
						c.applySolution(sol)
						i-- // don't count this iteration against the retry budget
						continue
					}
				}
			}
		}

		if i == c.retryMax-1 {
			break
		}
		if err := c.sleepBackoff(ctx, i); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

// applySolution injects the credentials returned by a ChallengeSolver into the
// active session so that subsequent requests carry them automatically.
func (c *Client) applySolution(sol *ChallengeSolution) {
	if sol == nil {
		return
	}
	// Inject cookies into the HTTP client's jar.
	if len(sol.Cookies) > 0 && c.httpClient.Jar != nil {
		// Cookies have domain set by the solver; build a URL for the jar API.
		byDomain := make(map[string][]*http.Cookie)
		for _, ck := range sol.Cookies {
			d := ck.Domain
			if d == "" {
				d = "duckduckgo.com"
			}
			byDomain[d] = append(byDomain[d], ck)
		}
		for domain, cks := range byDomain {
			scheme := "https"
			u := &url.URL{Scheme: scheme, Host: domain, Path: "/"}
			c.httpClient.Jar.SetCookies(u, cks)
		}
	}
	// Update the active User-Agent so it matches the solved session.
	// Cloudflare ties cf_clearance to the exact UA used during solving.
	if sol.UserAgent != "" {
		c.ua = sol.UserAgent
		if c.antiBot != nil && c.antiBot.transport != nil && c.antiBot.transport.uaPool != nil {
			// Pin the transport to the solver's UA by replacing the pool with
			// a single-entry pool so rotation keeps the correct fingerprint.
			c.antiBot.transport.uaPool = singleUAPool(sol.UserAgent)
		}
	}
}

// clearAllVQD wipes the entire VQD token cache, forcing fresh tokens on next use.
func (c *Client) clearAllVQD() {
	c.mu.Lock()
	c.vqdCache = make(map[string]tokenEntry)
	c.mu.Unlock()
}

// sleepContext sleeps for d or until ctx is cancelled.
func sleepContext(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (c *Client) waitForTurn(ctx context.Context) error {
	if c.minGap <= 0 {
		return nil
	}
	c.rateMu.Lock()
	defer c.rateMu.Unlock()

	now := time.Now()
	if c.lastReq.IsZero() {
		c.lastReq = now
		return nil
	}
	wait := c.lastReq.Add(c.minGap).Sub(now)
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
	c.lastReq = time.Now()
	return nil
}

func (c *Client) sleepBackoff(ctx context.Context, attempt int) error {
	backoff := c.baseDelay * time.Duration(1<<attempt)
	jitter := 1 + ((rand.Float64()*2 - 1) * c.jitterFrac)
	d := time.Duration(float64(backoff) * jitter)
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

func (c *Client) blockedDetector(body []byte) string {
	s := string(body)
	for _, re := range c.blockedRe {
		if re.MatchString(s) {
			return re.String()
		}
	}
	return ""
}

func (c *Client) applyHeaders(req *http.Request) {
	req.Header.Set("User-Agent", c.ua)
	if req.Header.Get("Referer") == "" {
		req.Header.Set("Referer", c.referer)
	}
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}
}

func extractVQD(body []byte) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`vqd=['\"]([^'\"]+)['\"]`),
		regexp.MustCompile(`vqd=([^&'\"\s]+)`),
	}
	for _, re := range patterns {
		m := re.FindSubmatch(body)
		if len(m) > 1 {
			return string(m[1])
		}
	}
	return ""
}

func parseDJSResults(body []byte) ([]Result, error) {
	s := string(body)
	start := strings.Index(s, "[{")
	end := strings.LastIndex(s, "}]")
	if start < 0 || end < 0 || end < start {
		return nil, ErrUnexpectedBody
	}
	payload := s[start : end+2]

	var raw []struct {
		Title string `json:"t"`
		URL   string `json:"u"`
		Body  string `json:"a"`
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(raw))
	for _, r := range raw {
		if strings.TrimSpace(r.URL) == "" {
			continue
		}
		results = append(results, Result{
			Title:   strings.TrimSpace(r.Title),
			URL:     strings.TrimSpace(r.URL),
			Snippet: strings.TrimSpace(r.Body),
		})
	}
	return results, nil
}

var (
	htmlResultRe = regexp.MustCompile(`(?s)<a[^>]*class="[^"]*result__a[^"]*"[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	htmlTagRe    = regexp.MustCompile(`(?s)<[^>]+>`)
)

func parseHTMLResults(body []byte) []Result {
	matches := htmlResultRe.FindAllSubmatch(body, -1)
	results := make([]Result, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		href := strings.TrimSpace(html.UnescapeString(string(m[1])))
		title := strings.TrimSpace(html.UnescapeString(htmlTagRe.ReplaceAllString(string(m[2]), "")))
		if href == "" || title == "" {
			continue
		}
		results = append(results, Result{Title: title, URL: href})
	}
	return results
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, vv := range h {
		cp := make([]string, len(vv))
		copy(cp, vv)
		out[k] = cp
	}
	return out
}

func snippet(body []byte, max int) string {
	if max <= 0 {
		max = 200
	}
	s := strings.TrimSpace(strings.ReplaceAll(string(body), "\n", " "))
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
