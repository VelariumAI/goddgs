package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/velariumai/goddgs"
)

func main() {
	os.Exit(run(os.Args))
}

func run(argv []string) int {
	if len(argv) < 2 {
		usage()
		return 2
	}
	switch argv[1] {
	case "search":
		return runSearch(argv[2:])
	case "providers":
		return runProviders()
	case "doctor":
		return runDoctor()
	default:
		usage()
		return 2
	}
}

func usage() {
	fmt.Println("goddgs commands: search, providers, doctor")
}

func runSearch(args []string) int {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	q := fs.String("q", "", "query")
	max := fs.Int("max", 10, "max results")
	region := fs.String("region", "us-en", "region")
	asJSON := fs.Bool("json", false, "json output")
	if err := fs.Parse(args); err != nil {
		fmt.Println(err)
		return 2
	}
	if strings.TrimSpace(*q) == "" {
		fmt.Println("query is required (--q)")
		return 2
	}
	cfg := goddgs.LoadConfigFromEnv()
	engine, err := goddgs.NewDefaultEngineFromConfig(cfg)
	if err != nil {
		fmt.Println("engine init error:", err)
		return 4
	}
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()
	res, err := engine.Search(ctx, goddgs.SearchRequest{Query: *q, MaxResults: *max, Region: *region})
	if err != nil {
		if goddgs.IsBlocked(err) {
			fmt.Println("blocked by target protection:", err)
			return 2
		}
		var se *goddgs.SearchError
		if errors.As(err, &se) {
			fmt.Println(se.Error())
			switch se.Kind {
			case goddgs.ErrKindInvalidInput:
				return 2
			case goddgs.ErrKindNoResults:
				return 2
			}
		}
		fmt.Println("search error:", err)
		return 4
	}
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
		return 0
	}
	fmt.Printf("Provider: %s (fallback=%v)\n", res.Provider, res.FallbackUsed)
	for i, r := range res.Results {
		fmt.Printf("%d. %s\n   %s\n", i+1, r.Title, r.URL)
	}
	return 0
}

func runProviders() int {
	cfg := goddgs.LoadConfigFromEnv()
	engine, err := goddgs.NewDefaultEngineFromConfig(cfg)
	if err != nil {
		fmt.Println("engine init error:", err)
		return 4
	}
	enabled := engine.EnabledProviders()
	fmt.Println("Enabled providers:", strings.Join(enabled, ", "))
	return 0
}

func runDoctor() int {
	cfg := goddgs.LoadConfigFromEnv()
	fmt.Println("Timeout:", cfg.Timeout)
	fmt.Println("Max retries:", cfg.MaxRetries)
	fmt.Println("Provider order:", strings.Join(cfg.ProviderOrder, ","))
	fmt.Println("Brave key set:", cfg.BraveAPIKey != "")
	fmt.Println("Tavily key set:", cfg.TavilyAPIKey != "")
	fmt.Println("SerpAPI key set:", cfg.SerpAPIKey != "")
	engine, err := goddgs.NewDefaultEngineFromConfig(cfg)
	if err != nil {
		fmt.Println("engine init error:", err)
		return 4
	}
	ctx, cancel := context.WithTimeout(context.Background(), min(cfg.Timeout, 10*time.Second))
	defer cancel()
	_, err = engine.Search(ctx, goddgs.SearchRequest{Query: "golang", MaxResults: 1, Region: "us-en"})
	if err != nil {
		fmt.Println("probe failed:", err)
		return 3
	}
	fmt.Println("probe ok")
	return 0
}

func min(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
