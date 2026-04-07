# Architecture

## High-Level Components

- `Client`: DDG-facing HTTP client with retry, fallback, block detection, and optional anti-bot state.
- `Engine`: provider-chain orchestrator that normalizes failures and returns typed diagnostics.
- `Provider` adapters: `ddg`, `brave`, `tavily`, `serpapi` implementations behind a common interface.
- `Service`: HTTP API (`/v1/search`) and health/readiness/metrics endpoints.
- `CLI`: operational and local-debug entrypoint (`search`, `providers`, `doctor`).

## Request Flow

1. Caller submits `SearchRequest` to `Engine.Search`.
2. Engine iterates enabled providers in configured order.
3. First successful provider returns `SearchResponse`.
4. Failures are classified (`SearchError`) and captured in diagnostics.
5. If all providers fail, engine returns a typed exhaustion error.

## DDG Client Runtime

`Client.Search` attempts:

1. `d.js` path (primary).
2. optional `/html/` fallback (unless disabled).

Resilience controls include:

- VQD cache and invalidation on block.
- Retry with backoff and jitter.
- Optional adaptive rate limiter.
- Optional circuit breaker (`ErrCircuitOpen`).
- Optional session invalidation and warmup.

## Anti-Bot and Solvers

The anti-bot subsystem supports:

- Browser-like header profiles and user-agent rotation.
- Chrome-like TLS behavior via `utls` transport path.
- Optional proxy-pool rotation.
- Block-signal detection and challenge solver chain.

See [ANTI_BOT_AND_SOLVERS.md](ANTI_BOT_AND_SOLVERS.md) for details and limits.

## Observability

- Internal event hooks (`EventHook`) for search/provider lifecycle and block/fallback events.
- Prometheus collector with provider, block, fallback, and circuit metrics.

## Key Design Decisions

- Preserve a typed error model for stable automation and API clients.
- Prefer failover over brittle single-provider dependency.
- Keep optional anti-bot behavior pluggable and explicit.
- Keep HTTP service thin over engine interfaces.
