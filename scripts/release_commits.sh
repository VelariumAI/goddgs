#!/usr/bin/env bash
set -euo pipefail

# Prints a suggested non-interactive release commit sequence.
# Review with `git status` before running any commit command.
cat <<'PLAN'
1) chore(repo): establish public OSS scaffolding and governance
2) feat(core): deliver production search engine, providers, cli, and daemon
3) feat(reliability): add block diagnostics, circuit telemetry, and ops runbook
4) test(quality): raise coverage to 85% and enforce threshold in CI
5) docs(release): add release checklist and commit playbook
PLAN
