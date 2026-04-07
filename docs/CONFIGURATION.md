# Configuration

## Environment Variables

Core:

- `GODDGS_PROVIDER_ORDER`: comma-separated provider order.
  - Default: `ddg,brave,tavily,serpapi`
- `GODDGS_TIMEOUT`: request timeout duration.
  - Default: `20s`
- `GODDGS_MAX_RETRIES`: retry attempts for DDG client.
  - Default: `3`
- `GODDGS_DISABLE_HTML_FALLBACK`: disables DDG `/html/` fallback.
  - Values: `true|false`

DDG endpoint overrides (useful for testing/internal proxies):

- `GODDGS_DDG_BASE`
- `GODDGS_LINKS_BASE`
- `GODDGS_HTML_BASE`

Provider keys:

- `GODDGS_BRAVE_API_KEY`
- `GODDGS_TAVILY_API_KEY`
- `GODDGS_SERPAPI_API_KEY`

Service:

- `GODDGS_ADDR`
  - Default: `:8080`

## Provider Strategy Recommendations

- Default order is suitable for no-key startup (`ddg` first).
- For stricter reliability, place keyed providers earlier in `GODDGS_PROVIDER_ORDER`.
- Keep `ddg` in chain for cost control and broad availability fallback.

## Runtime Tuning Guidance

- Increase timeout for unstable mobile networks.
- Increase retries only if latency budget allows.
- Disable HTML fallback only if you require strict endpoint behavior.

For anti-bot tuning values (`CircuitBreakerThreshold`, etc.), see [OPERATIONS.md](OPERATIONS.md).
