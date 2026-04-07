# Changelog

## Unreleased

## v0.1.0 - 2026-04-07

- Added DDG-first resilient search client with typed block detection.
- Added provider failover engine with adapters for Brave, Tavily, and SerpAPI.
- Added `goddgs` CLI and `goddgsd` HTTP service runtimes.
- Added structured event hooks and Prometheus observability.
- Added anti-bot resilience hardening (fresh VQD retry, solver retry budget fix, circuit breaker fail-fast).
- Added OSS governance/release scaffolding and CI quality gates.
- Enforced total test coverage gate at `>=85.0%`.
