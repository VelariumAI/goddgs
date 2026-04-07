# goddgs
[![CI](https://github.com/velariumai/goddgs/actions/workflows/ci.yml/badge.svg)](https://github.com/velariumai/goddgs/actions/workflows/ci.yml)
[![Release](https://github.com/velariumai/goddgs/actions/workflows/release.yml/badge.svg)](https://github.com/velariumai/goddgs/actions/workflows/release.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/velariumai/goddgs)](https://goreportcard.com/report/github.com/velariumai/goddgs)

`goddgs` is a production-oriented Go web search toolkit with:

- DDG-first search without requiring API keys.
- Typed provider failover engine (`ddg`, `brave`, `tavily`, `serpapi`).
- Bot/challenge signal detection and diagnostics.
- Optional challenge-solving integrations (FlareSolverr, 2captcha, CapSolver).
- CLI and HTTP service runtimes.
- Prometheus-compatible observability hooks.

## Install

```bash
go get github.com/velariumai/goddgs
```

## Compatibility

- Go `1.24+`

## What This Project Does

`goddgs` is designed for resilient search in real-world environments where providers can fail, throttle, or challenge traffic.

It includes:

- Provider-chain orchestration with typed errors and diagnostics.
- Retry/backoff logic, VQD token refresh, and adaptive controls.
- Circuit-breaker fail-fast behavior for burned sessions.
- Optional solver interfaces for challenge workflows.

Important reality:

- Challenge-solving behavior is environment-dependent and not guaranteed to succeed against all anti-bot systems.
- Use official providers for reliability-critical production traffic.

## Quick Start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/velariumai/goddgs"
)

func main() {
	cfg := goddgs.LoadConfigFromEnv()
	engine, err := goddgs.NewDefaultEngineFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := engine.Search(context.Background(), goddgs.SearchRequest{
		Query:      "golang structured logging",
		MaxResults: 5,
		Region:     "us-en",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("provider=%s fallback=%v results=%d\n", resp.Provider, resp.FallbackUsed, len(resp.Results))
}
```

## CLI

```bash
go run ./cmd/goddgs providers
go run ./cmd/goddgs search --q "golang" --max 5 --region us-en
go run ./cmd/goddgs doctor
```

## HTTP Service

```bash
go run ./cmd/goddgsd
```

Endpoints:

- `GET /healthz`
- `GET /readyz`
- `GET /metrics`
- `POST /v1/search`

Example:

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/search \
  -H 'content-type: application/json' \
  -d '{"query":"golang","max_results":3,"region":"us-en"}'
```

## Configuration

Core environment variables:

- `GODDGS_PROVIDER_ORDER` (default: `ddg,brave,tavily,serpapi`)
- `GODDGS_TIMEOUT` (default: `20s`)
- `GODDGS_MAX_RETRIES` (default: `3`)
- `GODDGS_DISABLE_HTML_FALLBACK` (`true|false`)
- `GODDGS_DDG_BASE` (optional override for DDG base URL)
- `GODDGS_LINKS_BASE` (optional override for links endpoint)
- `GODDGS_HTML_BASE` (optional override for html endpoint)
- `GODDGS_BRAVE_API_KEY`
- `GODDGS_TAVILY_API_KEY`
- `GODDGS_SERPAPI_API_KEY`
- `GODDGS_ADDR` (HTTP service bind, default `:8080`)

For full runtime and anti-bot configuration guidance, see docs below.

## Documentation

- [Documentation Index](docs/README.md)
- [Architecture](docs/ARCHITECTURE.md)
- [API Reference](docs/API_REFERENCE.md)
- [HTTP API](docs/HTTP_API.md)
- [CLI Reference](docs/CLI.md)
- [Configuration](docs/CONFIGURATION.md)
- [Anti-Bot and Solver Model](docs/ANTI_BOT_AND_SOLVERS.md)
- [Operations Runbook](docs/OPERATIONS.md)
- [Release Checklist](docs/RELEASE_CHECKLIST.md)

## Developer Workflow

```bash
make fmt
make vet
make test
make coverage
make build
```

Coverage policy:

- Total statement coverage is enforced at `>=85.0%` via `scripts/check_coverage.sh` and CI.

## License

MIT
