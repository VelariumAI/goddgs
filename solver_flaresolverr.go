package goddgs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// FlareSolverrSolver uses a running FlareSolverr instance to bypass Cloudflare
// Bot Management: IUAM ("Just a moment…"), Turnstile, and JS challenges.
//
// FlareSolverr runs a real Chrome browser internally, so it handles any
// JS-based challenge Cloudflare issues. Run it with:
//
//	docker run -d -p 8191:8191 ghcr.io/flaresolverr/flaresolverr:latest
//
// The cf_clearance cookie it returns is tied to the UserAgent it used, so
// the client MUST send that exact UA in all subsequent requests.
type FlareSolverrSolver struct {
	// Endpoint is the FlareSolverr API URL (default: http://localhost:8191/v1).
	Endpoint string

	// MaxTimeout is the maximum milliseconds FlareSolverr may spend solving.
	// Default: 60 000 (60 s). Complex challenges may need more.
	MaxTimeout int

	hc *http.Client
}

// NewFlareSolverrSolver creates a solver for a FlareSolverr instance.
// endpoint defaults to http://localhost:8191/v1 when empty.
func NewFlareSolverrSolver(endpoint string) *FlareSolverrSolver {
	if endpoint == "" {
		endpoint = "http://localhost:8191/v1"
	}
	return &FlareSolverrSolver{
		Endpoint:   endpoint,
		MaxTimeout: 60_000,
		hc:         &http.Client{Timeout: 120 * time.Second},
	}
}

func (f *FlareSolverrSolver) Supports(signal BlockSignal) bool {
	// FlareSolverr can handle Cloudflare challenges and generic JS challenges.
	return signal == BlockSignalCloudflare || signal == BlockSignalGeneric
}

func (f *FlareSolverrSolver) Solve(ctx context.Context, pageURL string, _ BlockInfo, _ []byte) (*ChallengeSolution, error) {
	// ── build request ─────────────────────────────────────────────────────────
	type fsReq struct {
		Cmd        string `json:"cmd"`
		URL        string `json:"url"`
		MaxTimeout int    `json:"maxTimeout"`
	}
	payload, _ := json.Marshal(fsReq{
		Cmd:        "request.get",
		URL:        pageURL,
		MaxTimeout: f.MaxTimeout,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// ── call FlareSolverr ─────────────────────────────────────────────────────
	resp, err := f.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: %w", err)
	}
	defer resp.Body.Close()

	// ── parse response ────────────────────────────────────────────────────────
	var result struct {
		Status  string `json:"status"`
		Message string `json:"message"`
		Solution struct {
			Cookies []struct {
				Name     string  `json:"name"`
				Value    string  `json:"value"`
				Domain   string  `json:"domain"`
				Path     string  `json:"path"`
				Expires  float64 `json:"expiry"`
				Secure   bool    `json:"secure"`
				HTTPOnly bool    `json:"httpOnly"`
				SameSite string  `json:"sameSite"`
			} `json:"cookies"`
			UserAgent string `json:"userAgent"`
		} `json:"solution"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("flaresolverr: decode: %w", err)
	}
	if result.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr: challenge failed: %s", result.Message)
	}

	sol := &ChallengeSolution{UserAgent: result.Solution.UserAgent}
	for _, c := range result.Solution.Cookies {
		sol.Cookies = append(sol.Cookies, &http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		})
	}
	if len(sol.Cookies) == 0 && sol.UserAgent == "" {
		return nil, fmt.Errorf("flaresolverr: solution contained no usable credentials")
	}
	return sol, nil
}
