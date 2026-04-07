# HTTP API

`goddgsd` exposes a minimal API surface.

## Endpoints

- `GET /healthz`
  - `200 OK` with body `ok`

- `GET /readyz`
  - `200 OK` with body `ready` when engine exists and has enabled providers
  - `503 Service Unavailable` with body `no providers` otherwise

- `GET /metrics`
  - Prometheus handler output when configured

- `POST /v1/search`

## /v1/search Request

```json
{
  "query": "golang",
  "max_results": 5,
  "region": "us-en"
}
```

Fields:

- `query` (required)
- `max_results` (optional)
- `region` (optional)

## /v1/search Response

Returns `SearchResponse` JSON on success.

`Content-Type: application/json`

## Error Payload

```json
{
  "error": "error message",
  "kind": "error_kind"
}
```

## Status Mapping (Current Behavior)

- `405` for non-POST `/v1/search` requests.
- `400` for invalid JSON or empty query.
- `502` if engine is unavailable.
- `429` for blocked/rate-limited mapped errors.
- `404` for no-results mapped errors.
- `502` for other provider/internal failures.

Note: when all providers are exhausted, engine returns a normalized no-results error, which maps to `404` in service.
