package goddgs

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BraveAPIKey  string
	TavilyAPIKey string
	SerpAPIKey   string

	DuckDuckGoBase string
	LinksBase      string
	HTMLBase       string

	ProviderOrder []string
	Timeout       time.Duration
	MaxRetries    int

	DisableHTMLFallback bool
}

func LoadConfigFromEnv() Config {
	cfg := Config{
		BraveAPIKey:         strings.TrimSpace(os.Getenv("GODDGS_BRAVE_API_KEY")),
		TavilyAPIKey:        strings.TrimSpace(os.Getenv("GODDGS_TAVILY_API_KEY")),
		SerpAPIKey:          strings.TrimSpace(os.Getenv("GODDGS_SERPAPI_API_KEY")),
		DuckDuckGoBase:      strings.TrimSpace(os.Getenv("GODDGS_DDG_BASE")),
		LinksBase:           strings.TrimSpace(os.Getenv("GODDGS_LINKS_BASE")),
		HTMLBase:            strings.TrimSpace(os.Getenv("GODDGS_HTML_BASE")),
		ProviderOrder:       []string{"ddg", "brave", "tavily", "serpapi"},
		Timeout:             20 * time.Second,
		MaxRetries:          3,
		DisableHTMLFallback: strings.EqualFold(strings.TrimSpace(os.Getenv("GODDGS_DISABLE_HTML_FALLBACK")), "true"),
	}
	if s := strings.TrimSpace(os.Getenv("GODDGS_PROVIDER_ORDER")); s != "" {
		parts := strings.Split(s, ",")
		order := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.ToLower(strings.TrimSpace(p))
			if p != "" {
				order = append(order, p)
			}
		}
		if len(order) > 0 {
			cfg.ProviderOrder = order
		}
	}
	if s := strings.TrimSpace(os.Getenv("GODDGS_TIMEOUT")); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			cfg.Timeout = d
		}
	}
	if s := strings.TrimSpace(os.Getenv("GODDGS_MAX_RETRIES")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			cfg.MaxRetries = n
		}
	}
	return cfg
}

func NewDefaultEngineFromConfig(cfg Config, hooks ...EventHook) (*Engine, error) {
	hc := &http.Client{Timeout: cfg.Timeout}
	ddg := NewClient(Options{
		HTTPClient:          hc,
		RetryMax:            cfg.MaxRetries,
		DisableHTMLFallback: cfg.DisableHTMLFallback,
		DuckDuckGoBase:      cfg.DuckDuckGoBase,
		LinksBase:           cfg.LinksBase,
		HTMLBase:            cfg.HTMLBase,
	})
	providers := map[string]Provider{
		"ddg":     NewDDGProvider(ddg),
		"brave":   NewBraveProvider(cfg.BraveAPIKey, hc),
		"tavily":  NewTavilyProvider(cfg.TavilyAPIKey, hc),
		"serpapi": NewSerpAPIProvider(cfg.SerpAPIKey, hc),
	}
	ordered := make([]Provider, 0, len(cfg.ProviderOrder))
	seen := map[string]struct{}{}
	for _, name := range cfg.ProviderOrder {
		p, ok := providers[name]
		if !ok {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		ordered = append(ordered, p)
	}
	if _, ok := seen["ddg"]; !ok {
		ordered = append([]Provider{providers["ddg"]}, ordered...)
	}
	return NewEngine(EngineOptions{Providers: ordered, Hooks: hooks})
}
