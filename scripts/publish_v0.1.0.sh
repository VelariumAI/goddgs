#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

VERSION="v0.1.0"
REMOTE_URL="git@github.com:velariumai/goddgs.git"

if ! git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  git init
fi

if ! git remote get-url origin >/dev/null 2>&1; then
  git remote add origin "$REMOTE_URL"
fi

make check

git add .
if ! git diff --cached --quiet; then
  git commit -m "release(v0.1.0): production-ready OSS launch"
fi

if ! git rev-parse "$VERSION" >/dev/null 2>&1; then
  git tag -a "$VERSION" -m "$VERSION"
fi

git branch -M main
git push -u origin main --tags

gh release create "$VERSION" \
  --repo velariumai/goddgs \
  --title "$VERSION" \
  --notes-file docs/releases/v0.1.0.md
