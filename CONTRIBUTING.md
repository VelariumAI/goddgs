# Contributing

## Development Baseline

- Go `1.24+`
- Keep changes focused and test-backed.
- Preserve backward compatibility for stable exported APIs unless explicitly documented.

## Local Validation

Run before opening a PR:

```bash
make fmt
make vet
make test
make coverage
make build
```

## Pull Request Expectations

- Clear problem and solution summary.
- Tests added or updated for behavior changes.
- Documentation updated when API/runtime behavior changes.
- No secrets in source, logs, or CI files.
- Changelog updated for user-visible changes.
- PR description follows `.github/PULL_REQUEST_TEMPLATE.md`.

## Commit Style

Preferred format:

- `<type>(<scope>): <summary>`

Examples:

- `feat(engine): add provider failover diagnostics`
- `fix(client): refresh vqd token on blocked retry`
- `docs(readme): clarify solver capability boundaries`
