#!/usr/bin/env bash
set -euo pipefail

THRESHOLD="${1:-85.0}"

if ! [[ "$THRESHOLD" =~ ^[0-9]+([.][0-9]+)?$ ]]; then
  echo "invalid threshold: $THRESHOLD" >&2
  exit 2
fi

go test ./... -coverprofile=coverage.out >/dev/null
TOTAL_LINE="$(go tool cover -func=coverage.out | tail -n1)"
TOTAL="$(echo "$TOTAL_LINE" | awk '{print $3}' | tr -d '%')"

echo "coverage total=${TOTAL}% threshold=${THRESHOLD}%"
awk -v total="$TOTAL" -v threshold="$THRESHOLD" 'BEGIN { exit !(total+0 >= threshold+0) }'
