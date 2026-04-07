package goddgs

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// BlockSignal categorises the type of bot-detection challenge detected in a response.
type BlockSignal int

const (
	BlockSignalNone       BlockSignal = iota
	BlockSignalCloudflare             // Cloudflare Bot Management / IUAM challenge
	BlockSignalReCAPTCHA              // Google reCAPTCHA or hCaptcha
	BlockSignalAkamai                 // Akamai Bot Manager
	BlockSignalPerimeterX             // PerimeterX / HUMAN Security
	BlockSignalDataDome               // DataDome
	BlockSignalGeneric                // Rate-limit or generic bot challenge
)

func (s BlockSignal) String() string {
	switch s {
	case BlockSignalCloudflare:
		return "cloudflare"
	case BlockSignalReCAPTCHA:
		return "recaptcha"
	case BlockSignalAkamai:
		return "akamai"
	case BlockSignalPerimeterX:
		return "perimeterx"
	case BlockSignalDataDome:
		return "datadome"
	case BlockSignalGeneric:
		return "generic"
	default:
		return "none"
	}
}

// BlockInfo holds the result of a block-signal detection pass.
type BlockInfo struct {
	Signal      BlockSignal
	DetectorKey string // machine-readable sub-key identifying the specific signal
}

// IsDetected reports whether any block signal was found.
func (b BlockInfo) IsDetected() bool { return b.Signal != BlockSignalNone }

// ── body-pattern table ────────────────────────────────────────────────────────

type bodyPattern struct {
	re     *regexp.Regexp
	signal BlockSignal
	key    string
}

var bodyPatterns = []bodyPattern{
	// Cloudflare
	{regexp.MustCompile(`(?i)just a moment`), BlockSignalCloudflare, "cf_iuam"},
	{regexp.MustCompile(`(?i)checking your browser`), BlockSignalCloudflare, "cf_browser_check"},
	{regexp.MustCompile(`(?i)enable javascript and cookies`), BlockSignalCloudflare, "cf_js_required"},
	{regexp.MustCompile(`(?i)cloudflare\s+ray\s+id`), BlockSignalCloudflare, "cf_ray_id_body"},
	{regexp.MustCompile(`(?i)__cf_bm`), BlockSignalCloudflare, "cf_bm_cookie"},
	// reCAPTCHA / hCaptcha
	{regexp.MustCompile(`(?i)www\.google\.com/recaptcha`), BlockSignalReCAPTCHA, "recaptcha"},
	{regexp.MustCompile(`(?i)hcaptcha\.com`), BlockSignalReCAPTCHA, "hcaptcha"},
	{regexp.MustCompile(`(?i)g-recaptcha`), BlockSignalReCAPTCHA, "g_recaptcha"},
	// Akamai
	{regexp.MustCompile(`(?i)ak_bmsc`), BlockSignalAkamai, "akamai_bmsc"},
	{regexp.MustCompile(`(?i)_abck`), BlockSignalAkamai, "akamai_abck"},
	// PerimeterX / HUMAN
	{regexp.MustCompile(`(?i)perimeterx`), BlockSignalPerimeterX, "perimeterx"},
	{regexp.MustCompile(`(?i)px_captcha`), BlockSignalPerimeterX, "px_captcha"},
	{regexp.MustCompile(`(?i)_pxde`), BlockSignalPerimeterX, "px_de"},
	{regexp.MustCompile(`(?i)human\.security`), BlockSignalPerimeterX, "human_security"},
	// DataDome
	{regexp.MustCompile(`(?i)datadome\.co`), BlockSignalDataDome, "datadome"},
	{regexp.MustCompile(`(?i)__dd_s`), BlockSignalDataDome, "datadome_session"},
	// Generic challenges
	{regexp.MustCompile(`(?i)access denied`), BlockSignalGeneric, "access_denied"},
	{regexp.MustCompile(`(?i)automated traffic`), BlockSignalGeneric, "automated_traffic"},
	{regexp.MustCompile(`(?i)verify you are human`), BlockSignalGeneric, "verify_human"},
	{regexp.MustCompile(`(?i)rate.?limit`), BlockSignalGeneric, "rate_limit"},
	{regexp.MustCompile(`(?i)too many requests`), BlockSignalGeneric, "too_many_requests"},
	{regexp.MustCompile(`(?i)\bcaptcha\b`), BlockSignalGeneric, "captcha"},
	{regexp.MustCompile(`(?i)unusual traffic`), BlockSignalGeneric, "unusual_traffic"},
	{regexp.MustCompile(`(?i)suspicious activity`), BlockSignalGeneric, "suspicious_activity"},
	{regexp.MustCompile(`(?i)please complete the security check`), BlockSignalGeneric, "security_check"},
}

// DetectBlockSignal inspects an HTTP response for bot-detection signals.
// It checks response headers first (fast path), then scans the body.
func DetectBlockSignal(statusCode int, headers http.Header, body []byte) BlockInfo {
	if info := detectByHeaders(statusCode, headers); info.IsDetected() {
		return info
	}
	return detectByBody(body)
}

func detectByHeaders(statusCode int, h http.Header) BlockInfo {
	// Cloudflare
	if h.Get("cf-mitigated") != "" {
		return BlockInfo{BlockSignalCloudflare, "cf_mitigated"}
	}
	if h.Get("cf-ray") != "" && (statusCode == 403 || statusCode == 503) {
		return BlockInfo{BlockSignalCloudflare, "cf_ray"}
	}
	// Akamai
	if strings.Contains(h.Get("server"), "AkamaiGHost") {
		return BlockInfo{BlockSignalAkamai, "akamai_server"}
	}
	if h.Get("x-check-cacheable") != "" && statusCode == 403 {
		return BlockInfo{BlockSignalAkamai, "akamai_403"}
	}
	// DataDome
	if strings.Contains(strings.ToLower(h.Get("x-datadome")), "blocked") {
		return BlockInfo{BlockSignalDataDome, "datadome_header"}
	}
	// PerimeterX
	if h.Get("x-px-client-uuid") != "" {
		return BlockInfo{BlockSignalPerimeterX, "px_header"}
	}
	// Generic
	if statusCode == 429 {
		return BlockInfo{BlockSignalGeneric, "http_429"}
	}
	return BlockInfo{}
}

func detectByBody(body []byte) BlockInfo {
	s := string(body)
	for _, bp := range bodyPatterns {
		if bp.re.MatchString(s) {
			return BlockInfo{bp.signal, bp.key}
		}
	}
	return BlockInfo{}
}

// RetryAfterSeconds parses the Retry-After response header and returns the
// suggested delay in seconds, or 0 if the header is absent or unparseable.
func RetryAfterSeconds(headers http.Header) int {
	v := strings.TrimSpace(headers.Get("Retry-After"))
	if v == "" {
		return 0
	}
	// Numeric value (seconds)
	var secs int
	if _, err := fmt.Sscanf(v, "%d", &secs); err == nil && secs > 0 {
		return secs
	}
	// HTTP-date form (RFC 7231) e.g. "Wed, 21 Oct 2015 07:28:00 GMT".
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0
		}
		return int(d.Round(time.Second).Seconds())
	}
	// Unparseable value.
	return 0
}
