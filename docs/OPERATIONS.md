# Operations Guide

## SLO-Oriented View

Primary production objectives:

- low request failure rate,
- bounded p95 latency,
- stable fallback behavior under provider disruptions.

## Key Runtime Controls

- `CircuitBreakerThreshold` (default: `5`)
- `CircuitBreakerCooldown` (default: `60s`)
- `VQDInvalidateOnBlock` (recommended: `true`)
- `SessionInvalidateOnBlock` (recommended: `true`)

## Metrics

- `goddgs_requests_total{provider,status}`
- `goddgs_request_duration_seconds{provider}`
- `goddgs_block_events_total{provider,signal}`
- `goddgs_fallback_transitions_total{provider,kind}`
- `goddgs_provider_enabled{provider}`
- `goddgs_circuit_events_total{provider,state,trigger}`
- `goddgs_circuit_open{provider}`

## Alerting Suggestions

- Circuit open sustained for DDG provider over N minutes.
- Fallback transitions exceed baseline by threshold multiplier.
- Provider error rate above budget.

## Incident Runbook

### High block rate

- Validate block signal mix via `goddgs_block_events_total`.
- Confirm breaker transitions (`threshold_reached`, `fail_fast`).
- Increase fallback preference toward keyed providers.

### Persistent fail-fast

- Inspect `goddgs_circuit_open{provider="ddg"}`.
- Increase cooldown if immediate retries remain ineffective.
- Rotate session/proxy resources if available.

### Elevated latency

- Check retries/timeouts and provider ordering.
- Reduce retry count if timeout amplification is observed.
- Prioritize lower-latency provider in chain where acceptable.
