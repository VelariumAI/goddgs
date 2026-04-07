package goddgs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// TwoCaptchaSolver resolves CAPTCHA challenges via the 2captcha.com API.
// Supports: reCAPTCHA v2/v3, hCaptcha, and Cloudflare Turnstile.
//
// Get an API key at https://2captcha.com/. Balance is checked per-solve;
// the caller receives an error if the balance is insufficient.
type TwoCaptchaSolver struct {
	// APIKey is the 2captcha account API key.
	APIKey string

	// BaseURL defaults to https://2captcha.com. Override for self-hosted
	// or compatible (anti-captcha, etc.) endpoints.
	BaseURL string

	// PollInterval is how long to wait between result polls. Default: 5s.
	PollInterval time.Duration

	// PollTimeout is the maximum total wait for a solution. Default: 120s.
	PollTimeout time.Duration

	hc *http.Client
}

// NewTwoCaptchaSolver creates a solver backed by 2captcha.com.
func NewTwoCaptchaSolver(apiKey string) *TwoCaptchaSolver {
	return &TwoCaptchaSolver{
		APIKey:       apiKey,
		BaseURL:      "https://2captcha.com",
		PollInterval: 5 * time.Second,
		PollTimeout:  120 * time.Second,
		hc:           &http.Client{Timeout: 30 * time.Second},
	}
}

func (t *TwoCaptchaSolver) Supports(signal BlockSignal) bool {
	switch signal {
	case BlockSignalCloudflare, BlockSignalReCAPTCHA, BlockSignalGeneric:
		return true
	}
	return false
}

func (t *TwoCaptchaSolver) Solve(ctx context.Context, pageURL string, info BlockInfo, body []byte) (*ChallengeSolution, error) {
	ctype := challengeType(body, info.Signal)
	siteKey := extractSiteKey(body)

	var taskID string
	var err error

	switch ctype {
	case "hcaptcha":
		taskID, err = t.submitHCaptcha(ctx, pageURL, siteKey)
	case "recaptchav3":
		taskID, err = t.submitReCAPTCHAv3(ctx, pageURL, siteKey)
	case "turnstile":
		taskID, err = t.submitTurnstile(ctx, pageURL, siteKey)
	default:
		// recaptchav2 or unknown — try reCAPTCHA v2
		if siteKey == "" {
			return nil, fmt.Errorf("2captcha: no site key found in challenge page")
		}
		taskID, err = t.submitReCAPTCHAv2(ctx, pageURL, siteKey)
	}
	if err != nil {
		return nil, fmt.Errorf("2captcha: submit: %w", err)
	}

	token, err := t.pollResult(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("2captcha: poll: %w", err)
	}
	return &ChallengeSolution{Token: token}, nil
}

// ── task submission ────────────────────────────────────────────────────────────

func (t *TwoCaptchaSolver) submitReCAPTCHAv2(ctx context.Context, pageURL, siteKey string) (string, error) {
	params := url.Values{
		"key":       {t.APIKey},
		"method":    {"userrecaptcha"},
		"googlekey": {siteKey},
		"pageurl":   {pageURL},
		"json":      {"1"},
	}
	return t.submitTask(ctx, params)
}

func (t *TwoCaptchaSolver) submitReCAPTCHAv3(ctx context.Context, pageURL, siteKey string) (string, error) {
	params := url.Values{
		"key":       {t.APIKey},
		"method":    {"userrecaptcha"},
		"googlekey": {siteKey},
		"pageurl":   {pageURL},
		"version":   {"v3"},
		"action":    {"verify"},
		"min_score": {"0.5"},
		"json":      {"1"},
	}
	return t.submitTask(ctx, params)
}

func (t *TwoCaptchaSolver) submitHCaptcha(ctx context.Context, pageURL, siteKey string) (string, error) {
	if siteKey == "" {
		return "", fmt.Errorf("no hcaptcha site key found")
	}
	params := url.Values{
		"key":     {t.APIKey},
		"method":  {"hcaptcha"},
		"sitekey": {siteKey},
		"pageurl": {pageURL},
		"json":    {"1"},
	}
	return t.submitTask(ctx, params)
}

func (t *TwoCaptchaSolver) submitTurnstile(ctx context.Context, pageURL, siteKey string) (string, error) {
	if siteKey == "" {
		return "", fmt.Errorf("no turnstile site key found")
	}
	params := url.Values{
		"key":     {t.APIKey},
		"method":  {"turnstile"},
		"sitekey": {siteKey},
		"pageurl": {pageURL},
		"json":    {"1"},
	}
	return t.submitTask(ctx, params)
}

func (t *TwoCaptchaSolver) submitTask(ctx context.Context, params url.Values) (string, error) {
	endpoint := t.BaseURL + "/in.php"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := t.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		Status  int    `json:"status"`
		Request string `json:"request"` // task ID on success, error code on failure
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode submit response: %w", err)
	}
	if result.Status != 1 {
		return "", fmt.Errorf("api error: %s", result.Request)
	}
	return result.Request, nil
}

// ── result polling ─────────────────────────────────────────────────────────────

func (t *TwoCaptchaSolver) pollResult(ctx context.Context, taskID string) (string, error) {
	deadline := time.Now().Add(t.PollTimeout)
	endpoint := t.BaseURL + "/res.php"

	for {
		// Respect poll timeout independently of ctx.
		if time.Now().After(deadline) {
			return "", fmt.Errorf("poll timeout after %s", t.PollTimeout)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(t.PollInterval):
		}

		params := url.Values{
			"key":    {t.APIKey},
			"action": {"get"},
			"id":     {taskID},
			"json":   {"1"},
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet,
			endpoint+"?"+params.Encode(), nil)
		if err != nil {
			return "", err
		}

		resp, err := t.hc.Do(req)
		if err != nil {
			return "", err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result struct {
			Status  int    `json:"status"`
			Request string `json:"request"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			return "", fmt.Errorf("decode poll response: %w", err)
		}

		switch result.Request {
		case "CAPCHA_NOT_READY": // 2captcha typo in their API
			continue
		case "":
			if result.Status == 0 {
				return "", fmt.Errorf("solve failed: empty response")
			}
		}

		if result.Status == 1 {
			return result.Request, nil
		}
		return "", fmt.Errorf("solve failed: %s", result.Request)
	}
}
