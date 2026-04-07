# API Reference

This document summarizes the primary exported API surface in package `goddgs`.

## Core Request/Response Types

- `SearchRequest`
  - `Query string`
  - `MaxResults int`
  - `Region string`
  - `SafeSearch SafeSearch`
  - `TimeRange string`
  - `Offset int`

- `SearchResponse`
  - `Results []Result`
  - `Provider string`
  - `FallbackUsed bool`
  - `Diagnostics Diagnostics`

- `Diagnostics`
  - `BlockInfo *BlockInfo`
  - `Attempts int`
  - `ProviderChain []string`
  - `Timings map[string]time.Duration`
  - `Errors []ProviderError`

## Engine

- `NewEngine(EngineOptions) (*Engine, error)`
- `(*Engine).Search(ctx, SearchRequest) (SearchResponse, error)`
- `(*Engine).EnabledProviders() []string`

`EngineOptions`:

- `Providers []Provider`
- `Hooks []EventHook`

## Providers

Interface:

- `Provider`
  - `Name() string`
  - `Enabled() bool`
  - `Search(ctx, SearchRequest) ([]Result, error)`

Constructors:

- `NewDDGProvider(client *Client)`
- `NewBraveProvider(apiKey string, hc *http.Client)`
- `NewTavilyProvider(apiKey string, hc *http.Client)`
- `NewSerpAPIProvider(apiKey string, hc *http.Client)`

## DDG Client

- `NewClient(opts Options) *Client`
- `(*Client).Search(ctx, query, SearchOptions) ([]Result, error)`
- `(*Client).SearchPages(ctx, query, perPage, pages, opts) ([]Result, error)`

`SearchOptions`:

- `MaxResults int`
- `Region string`
- `SafeSearch SafeSearch`
- `TimeRange string`
- `Offset int`

`Options` (selected fields):

- network and retry: `HTTPClient`, `RequestTimeout`, `RetryMax`, `RetryBaseDelay`, `RetryJitterFrac`
- DDG endpoints: `DuckDuckGoBase`, `LinksBase`, `HTMLBase`
- fallback/headers: `DisableHTMLFallback`, `UserAgent`, `Referer`, `Headers`
- block handling: `BlockedStatusCodes`, `BlockedBodyPatterns`, `OnBlocked`
- anti-bot integration: `AntiBot`, `OnCircuit`

## Anti-Bot Types

- `AntiBotConfig` (`NewAntiBotConfig()` defaults enabled)
- `ChallengeSolver` interface
- `ChallengeSolution`
- solver constructors:
  - `NewFlareSolverrSolver(endpoint string)`
  - `NewTwoCaptchaSolver(apiKey string)`
  - `NewCapSolverSolver(apiKey string)`

`AntiBotConfig` key controls:

- `UARotation`, `ChromeTLS`, `SessionWarmup`
- `AdaptiveRateLimit`, `AdaptiveBaseDelay`, `AdaptiveMaxDelay`
- `SessionInvalidateOnBlock`, `VQDInvalidateOnBlock`
- `ChallengeSolvers []ChallengeSolver`
- `CircuitBreakerThreshold`, `CircuitBreakerCooldown`

## Errors and Classification

- `SearchError` with `Kind ErrorKind`
- `ErrorKind` values include:
  - `blocked`, `rate_limited`, `provider_unavailable`, `parse`, `invalid_input`, `no_results`, `internal`
- `BlockedError` / `IsBlocked(err)` helpers
- `ErrCircuitOpen` for open-circuit fail-fast path

## Events and Metrics

- `Event` and `EventHook`
- `PrometheusCollector`
  - `NewPrometheusCollector(reg)`
  - `Hook(ev Event)`
  - `SetProviderEnabled(provider, enabled)`
  - `ObserveCircuitEvent(provider, ev)`

## Helpers

- `LoadConfigFromEnv() Config`
- `NewDefaultEngineFromConfig(cfg Config, hooks ...EventHook)`
- `NewHTTPClient(timeout, proxyURL)`
- `NewAntiBotHTTPClient(timeout, proxyPool)`
