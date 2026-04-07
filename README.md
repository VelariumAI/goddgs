# goddgs
[![CI](https://github.com/velariumai/goddgs/actions/workflows/ci.yml/badge.svg)](https://github.com/velariumai/goddgs/actions/workflows/ci.yml)
[![Release](https://github.com/velariumai/goddgs/actions/workflows/release.yml/badge.svg)](https://github.com/velariumai/goddgs/actions/workflows/release.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/velariumai/goddgs.svg)](https://pkg.go.dev/github.com/velariumai/goddgs)
[![Go Report Card](https://goreportcard.com/badge/github.com/velariumai/goddgs)](https://goreportcard.com/report/github.com/velariumai/goddgs)
[![Coverage](https://img.shields.io/badge/coverage-85.0%25-brightgreen)](./scripts/check_coverage.sh)

`goddgs` is a production-oriented Go search library and runtime toolkit.

It provides:
- DDG-based no-key search as the default path.
- Typed challenge/block detection and diagnostics.
- Optional failover to official providers (`Brave`, `Tavily`, `SerpAPI`) when keys are configured.
- A high-level failover engine, CLI, and HTTP service.
- Structured event hooks and Prometheus metrics.

## Install

```bash
go get github.com/velariumai/goddgs
```

## Repository Layout

- `cmd/goddgs`: CLI (`search`, `providers`, `doctor`)
- `cmd/goddgsd`: HTTP service (`/healthz`, `/readyz`, `/metrics`, `/v1/search`)
- `examples/basic-search`: minimal executable integration example
- `docs/OPERATIONS.md`: runtime tuning and incident runbook
- `docs/RELEASE_CHECKLIST.md`: maintainer release workflow

## Go Compatibility

- Go `1.24+`

## Quick Start (Library)

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/velariumai/goddgs"
)

func main() {
	cfg := goddgs.LoadConfigFromEnv() // DDG-only works without any API keys.
	engine, err := goddgs.NewDefaultEngineFromConfig(cfg)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := engine.Search(context.Background(), goddgs.SearchRequest{
		Query:      "golang error handling best practices",
		MaxResults: 5,
		Region:     "us-en",
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("provider=%s fallback=%v\n", resp.Provider, resp.FallbackUsed)
	for i, r := range resp.Results {
		fmt.Printf("%d. %s\n   %s\n", i+1, r.Title, r.URL)
	}
}
```

## Environment Configuration

- `GODDGS_BRAVE_API_KEY`
- `GODDGS_TAVILY_API_KEY`
- `GODDGS_SERPAPI_API_KEY`
- `GODDGS_PROVIDER_ORDER` (comma-separated, default: `ddg,brave,tavily,serpapi`)
- `GODDGS_TIMEOUT` (duration, default: `20s`)
- `GODDGS_MAX_RETRIES` (default: `3`)
- `GODDGS_DISABLE_HTML_FALLBACK` (`true|false`)
- `GODDGS_ADDR` (HTTP service listen addr, default: `:8080`)

## CLI

```bash
# Show enabled providers
go run ./cmd/goddgs providers

# Run search
go run ./cmd/goddgs search --q "golang" --max 5 --region us-en

# JSON output
go run ./cmd/goddgs search --q "golang" --json

# Environment diagnostics + probe
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

Example request:

```bash
curl -sS -X POST http://127.0.0.1:8080/v1/search \
  -H 'content-type: application/json' \
  -d '{"query":"golang","max_results":3,"region":"us-en"}'
```

## Observability

Use event hooks via `EngineOptions.Hooks`.

Prometheus metrics exposed by `goddgsd`:
- `goddgs_requests_total{provider,status}`
- `goddgs_request_duration_seconds{provider}`
- `goddgs_block_events_total{provider,signal}`
- `goddgs_fallback_transitions_total{provider,kind}`
- `goddgs_provider_enabled{provider}`
- `goddgs_circuit_events_total{provider,state,trigger}`
- `goddgs_circuit_open{provider}`

You can also wire low-level DDG circuit events directly:

```go
collector := goddgs.NewPrometheusCollector(nil)
ddgClient := goddgs.NewClient(goddgs.Options{
  AntiBot: goddgs.NewAntiBotConfig(),
  OnCircuit: func(ev goddgs.CircuitEvent) {
    collector.ObserveCircuitEvent("ddg", ev)
  },
})
_ = ddgClient
```

Circuit event triggers:
- `threshold_reached`: breaker tripped open after consecutive block responses.
- `fail_fast`: request short-circuited while breaker is open.
- `success_reset`: breaker closed after a successful response.

## Circuit Breaker Tuning

Recommended starting values:
- `CircuitBreakerThreshold=5`
- `CircuitBreakerCooldown=60s`

How to tune:
- Raise threshold (for example `8-10`) if your environment has occasional transient 403/429 bursts and recovery is usually quick.
- Lower threshold (for example `3`) if sessions frequently burn and repeated retries waste time/cost.
- Increase cooldown (`120s-300s`) when blocks are sticky (same IP/session repeatedly challenged).
- Decrease cooldown (`15s-30s`) when your proxy pool rotates aggressively and sessions recover fast.

Operational guidance:
- Watch `goddgs_circuit_events_total{trigger="threshold_reached"}` and `...{trigger="fail_fast"}` trends together.
- If `fail_fast` is high but downstream success remains low, route sooner to keyed providers.
- Keep `VQDInvalidateOnBlock=true` and `SessionInvalidateOnBlock=true` for fastest DDG recovery attempts.

## Security and Compliance Notes

- DDG endpoints used by the no-key provider are unofficial and may change.
- This package does not implement challenge-defeat behavior.
- Recommended production behavior is classify-and-failover using configured official providers.

## Developer Workflow

```bash
make fmt
make vet
make test
make coverage
make build
```

For releases, follow [docs/RELEASE_CHECKLIST.md](docs/RELEASE_CHECKLIST.md).

## License

MIT
