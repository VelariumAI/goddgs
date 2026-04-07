# Anti-Bot and Solver Model

## What Is Implemented

`goddgs` includes an optional anti-bot subsystem with:

- challenge/bot-signal detection (`DetectBlockSignal`),
- adaptive retry/session controls,
- optional challenge solver integrations:
  - FlareSolverr,
  - 2captcha,
  - CapSolver.

The client can apply solver output (cookies/user-agent/token) and retry requests within configured limits.

## Important Limitations

- Success is not guaranteed across all targets or bot platforms.
- Challenge systems change frequently and are environment-dependent.
- Some solver paths require paid third-party services and proper credentials.
- Operational and legal responsibility remains with the deploying team.

## Recommended Production Pattern

1. Use DDG as low-cost/default path.
2. Enable anti-bot resilience for recovery attempts.
3. Keep official providers configured for reliability fallback.
4. Monitor block and circuit metrics continuously.

## Core Controls

From `AntiBotConfig`:

- `SessionWarmup`
- `SessionInvalidateOnBlock`
- `VQDInvalidateOnBlock`
- `AdaptiveRateLimit`
- `CircuitBreakerThreshold`
- `CircuitBreakerCooldown`
- `ChallengeSolvers`

## Operational Signals to Watch

- `goddgs_block_events_total{provider,signal}`
- `goddgs_circuit_events_total{provider,state,trigger}`
- `goddgs_circuit_open{provider}`
- `goddgs_fallback_transitions_total{provider,kind}`
