package goddgs

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
)

// ChallengeSolution is the credential set returned by a solver after it
// successfully resolves a bot-detection challenge. Apply it to the session
// before retrying the blocked request.
type ChallengeSolution struct {
	// Cookies are injected into the client's cookie jar.
	// For Cloudflare this typically includes cf_clearance.
	Cookies []*http.Cookie

	// UserAgent must be used for all subsequent requests when non-empty.
	// Cloudflare ties cf_clearance to the exact UA the challenge was solved with.
	UserAgent string

	// Token is a raw CAPTCHA response string (g-recaptcha-response,
	// g-turnstile-response, etc.) for flows that need to POST it somewhere.
	Token string
}

// ChallengeSolver resolves a detected bot-detection challenge and returns
// credentials that allow the blocked session to continue.
type ChallengeSolver interface {
	// Supports reports whether this solver handles the given block signal.
	Supports(signal BlockSignal) bool

	// Solve attempts to resolve the challenge for pageURL.
	// body is the blocked response body; it may be nil.
	// Implementations must respect ctx cancellation.
	Solve(ctx context.Context, pageURL string, info BlockInfo, body []byte) (*ChallengeSolution, error)
}

// ChainSolver tries a list of solvers in order and returns the first success.
// Configure it with the most reliable/fastest solver first.
type ChainSolver struct {
	Solvers []ChallengeSolver
}

func (c *ChainSolver) Supports(signal BlockSignal) bool {
	for _, s := range c.Solvers {
		if s.Supports(signal) {
			return true
		}
	}
	return false
}

func (c *ChainSolver) Solve(ctx context.Context, pageURL string, info BlockInfo, body []byte) (*ChallengeSolution, error) {
	var lastErr error
	for _, s := range c.Solvers {
		if !s.Supports(info.Signal) {
			continue
		}
		sol, err := s.Solve(ctx, pageURL, info, body)
		if err == nil && sol != nil {
			return sol, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return nil, fmt.Errorf("chain: all solvers failed for %s: %w", info.Signal, lastErr)
	}
	return nil, fmt.Errorf("chain: no solver registered for %s", info.Signal)
}

// ── site-key extraction ───────────────────────────────────────────────────────

// siteKeyPatterns are tried in order on a challenge page body to extract the
// CAPTCHA widget's data-sitekey value (used by reCAPTCHA, hCaptcha, Turnstile).
var siteKeyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`data-sitekey=["']([0-9A-Za-z_\-]{20,})["']`),
	regexp.MustCompile(`["']sitekey["']:\s*["']([0-9A-Za-z_\-]{20,})["']`),
	regexp.MustCompile(`sitekey=([0-9A-Za-z_\-]{20,})`),
	// Cloudflare Turnstile embeds the sitekey differently
	regexp.MustCompile(`cf-chl-widget-[a-z0-9]+.*?data-sitekey=["']([0-9A-Za-z_\-]{20,})["']`),
}

// extractSiteKey tries to parse a CAPTCHA site key from a challenge page body.
// Returns "" if no key is found.
func extractSiteKey(body []byte) string {
	s := string(body)
	for _, re := range siteKeyPatterns {
		if m := re.FindStringSubmatch(s); len(m) > 1 {
			return m[1]
		}
	}
	return ""
}

// challengeType returns the CAPTCHA variant hinted by the page body.
// Values: "recaptchav2", "recaptchav3", "hcaptcha", "turnstile", "".
func challengeType(body []byte, signal BlockSignal) string {
	s := string(body)
	switch {
	case regexp.MustCompile(`hcaptcha\.com`).MatchString(s):
		return "hcaptcha"
	case regexp.MustCompile(`turnstile`).MatchString(s), signal == BlockSignalCloudflare:
		return "turnstile"
	case regexp.MustCompile(`recaptcha/api2`).MatchString(s):
		return "recaptchav2"
	case regexp.MustCompile(`recaptcha/enterprise`).MatchString(s):
		return "recaptchav3"
	case regexp.MustCompile(`google\.com/recaptcha`).MatchString(s):
		return "recaptchav2"
	}
	return ""
}
