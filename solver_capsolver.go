package goddgs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CapSolverSolver resolves CAPTCHA challenges via the capsolver.com API.
// Supports: reCAPTCHA v2/v3, hCaptcha, and Cloudflare Turnstile.
//
// Get an API key at https://capsolver.com/.
type CapSolverSolver struct {
	// APIKey is the CapSolver account client key.
	APIKey string

	// BaseURL defaults to https://api.capsolver.com.
	BaseURL string

	// PollInterval is how long to wait between result polls. Default: 3s.
	PollInterval time.Duration

	// PollTimeout is the maximum total wait for a solution. Default: 120s.
	PollTimeout time.Duration

	hc *http.Client
}

// NewCapSolverSolver creates a solver backed by capsolver.com.
func NewCapSolverSolver(apiKey string) *CapSolverSolver {
	return &CapSolverSolver{
		APIKey:       apiKey,
		BaseURL:      "https://api.capsolver.com",
		PollInterval: 3 * time.Second,
		PollTimeout:  120 * time.Second,
		hc:           &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *CapSolverSolver) Supports(signal BlockSignal) bool {
	switch signal {
	case BlockSignalCloudflare, BlockSignalReCAPTCHA, BlockSignalGeneric:
		return true
	}
	return false
}

func (c *CapSolverSolver) Solve(ctx context.Context, pageURL string, info BlockInfo, body []byte) (*ChallengeSolution, error) {
	ctype := challengeType(body, info.Signal)
	siteKey := extractSiteKey(body)

	taskID, err := c.createTask(ctx, pageURL, siteKey, ctype)
	if err != nil {
		return nil, fmt.Errorf("capsolver: create task: %w", err)
	}

	token, err := c.getTaskResult(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("capsolver: get result: %w", err)
	}
	return &ChallengeSolution{Token: token}, nil
}

// ── task creation ──────────────────────────────────────────────────────────────

func (c *CapSolverSolver) createTask(ctx context.Context, pageURL, siteKey, ctype string) (string, error) {
	taskObj, err := c.buildTask(pageURL, siteKey, ctype)
	if err != nil {
		return "", err
	}

	payload, _ := json.Marshal(map[string]any{
		"clientKey": c.APIKey,
		"task":      taskObj,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.BaseURL+"/createTask", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		ErrorID          int    `json:"errorId"`
		ErrorCode        string `json:"errorCode"`
		ErrorDescription string `json:"errorDescription"`
		TaskID           string `json:"taskId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode create response: %w", err)
	}
	if result.ErrorID != 0 {
		return "", fmt.Errorf("%s: %s", result.ErrorCode, result.ErrorDescription)
	}
	return result.TaskID, nil
}

func (c *CapSolverSolver) buildTask(pageURL, siteKey, ctype string) (map[string]any, error) {
	switch ctype {
	case "hcaptcha":
		if siteKey == "" {
			return nil, fmt.Errorf("no hcaptcha site key found")
		}
		return map[string]any{
			"type":    "HCaptchaTaskProxyless",
			"websiteURL": pageURL,
			"websiteKey": siteKey,
		}, nil
	case "recaptchav3":
		if siteKey == "" {
			return nil, fmt.Errorf("no recaptcha v3 site key found")
		}
		return map[string]any{
			"type":       "ReCaptchaV3TaskProxyless",
			"websiteURL": pageURL,
			"websiteKey": siteKey,
			"pageAction": "verify",
			"minScore":   0.5,
		}, nil
	case "turnstile":
		if siteKey == "" {
			return nil, fmt.Errorf("no turnstile site key found")
		}
		return map[string]any{
			"type":       "AntiTurnstileTaskProxyless",
			"websiteURL": pageURL,
			"websiteKey": siteKey,
		}, nil
	default:
		// reCAPTCHA v2 / unknown
		if siteKey == "" {
			return nil, fmt.Errorf("no site key found in challenge page")
		}
		return map[string]any{
			"type":       "ReCaptchaV2TaskProxyless",
			"websiteURL": pageURL,
			"websiteKey": siteKey,
		}, nil
	}
}

// ── result polling ─────────────────────────────────────────────────────────────

func (c *CapSolverSolver) getTaskResult(ctx context.Context, taskID string) (string, error) {
	deadline := time.Now().Add(c.PollTimeout)
	endpoint := c.BaseURL + "/getTaskResult"

	for {
		if time.Now().After(deadline) {
			return "", fmt.Errorf("poll timeout after %s", c.PollTimeout)
		}

		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(c.PollInterval):
		}

		payload, _ := json.Marshal(map[string]any{
			"clientKey": c.APIKey,
			"taskId":    taskID,
		})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
			bytes.NewReader(payload))
		if err != nil {
			return "", err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.hc.Do(req)
		if err != nil {
			return "", err
		}

		var result struct {
			ErrorID          int    `json:"errorId"`
			ErrorCode        string `json:"errorCode"`
			ErrorDescription string `json:"errorDescription"`
			Status           string `json:"status"`
			Solution         struct {
				GRecaptchaResponse string `json:"gRecaptchaResponse"`
				Token              string `json:"token"` // Turnstile
			} `json:"solution"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			return "", fmt.Errorf("decode poll response: %w", err)
		}
		resp.Body.Close()

		if result.ErrorID != 0 {
			return "", fmt.Errorf("%s: %s", result.ErrorCode, result.ErrorDescription)
		}
		if result.Status == "processing" {
			continue
		}
		if result.Status == "ready" {
			// Prefer gRecaptchaResponse; fall back to token (Turnstile)
			if t := result.Solution.GRecaptchaResponse; t != "" {
				return t, nil
			}
			if t := result.Solution.Token; t != "" {
				return t, nil
			}
			return "", fmt.Errorf("ready but solution token is empty")
		}
		return "", fmt.Errorf("unexpected status: %s", result.Status)
	}
}
