# Contributing

## Development
- Go 1.24+
- Run `make fmt && make vet && make test && make coverage && make build` before PRs.
- Keep public APIs backward compatible for minor releases.

## PR checklist
- Tests added/updated.
- Docs updated for behavior changes.
- No secrets in code or CI.
- Changelog updated for user-visible changes.
- Pull request description follows `.github/PULL_REQUEST_TEMPLATE.md`.
