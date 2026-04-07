# Operations Guide

## Key Runtime Controls

- `CircuitBreakerThreshold` (default: `5`)
- `CircuitBreakerCooldown` (default: `60s`)
- `VQDInvalidateOnBlock` (recommended: `true`)
- `SessionInvalidateOnBlock` (recommended: `true`)

## Observability

Prometheus metrics:

- `goddgs_requests_total{provider,status}`
- `goddgs_request_duration_seconds{provider}`
- `goddgs_block_events_total{provider,signal}`
- `goddgs_fallback_transitions_total{provider,kind}`
- `goddgs_provider_enabled{provider}`
- `goddgs_circuit_events_total{provider,state,trigger}`
- `goddgs_circuit_open{provider}`

## Runbook

### High block rate

- Check `goddgs_block_events_total` by `signal`.
- If cloud/challenge signals spike, increase fallback priority to keyed providers.
- Confirm circuit breaker is tripping (look at `threshold_reached`).

### Persistent fail-fast

- Check `goddgs_circuit_open{provider="ddg"}`.
- Increase cooldown if immediate retries always fail.
- Rotate network/proxy session if available.

### Provider fallback pressure

- Inspect `goddgs_fallback_transitions_total` trend.
- If DDG is unstable for your traffic profile, prefer official providers first.
