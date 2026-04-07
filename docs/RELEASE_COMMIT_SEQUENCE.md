# Release Commit Sequence (v0.1.0)

Use this sequence to produce a clean, reviewable public release history.

## 1) Foundation / Repo Hygiene

```bash
git add .editorconfig .gitignore Makefile .github docs README.md CONTRIBUTING.md SECURITY.md CODE_OF_CONDUCT.md CHANGELOG.md LICENSE

git commit -m "chore(repo): establish public OSS scaffolding and governance"
```

## 2) Core Engine + Runtime

```bash
git add *.go cmd/goddgs cmd/goddgsd examples/basic-search

git commit -m "feat(core): deliver production search engine, providers, cli, and daemon"
```

## 3) Reliability + Observability

```bash
git add antibot.go detect.go observability_prometheus.go docs/OPERATIONS.md

git commit -m "feat(reliability): add block diagnostics, circuit telemetry, and ops runbook"
```

## 4) Test Hardening + Coverage Gate

```bash
git add *_test.go scripts/check_coverage.sh .github/workflows/ci.yml

git commit -m "test(quality): raise coverage to 85% and enforce threshold in CI"
```

## 5) Release Prep

```bash
git add docs/RELEASE_CHECKLIST.md docs/RELEASE_COMMIT_SEQUENCE.md

git commit -m "docs(release): add release checklist and commit playbook"
```

## 6) Tag + Publish

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin main --tags
```
