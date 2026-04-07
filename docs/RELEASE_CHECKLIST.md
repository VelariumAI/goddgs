# Release Checklist

## Pre-Release

- Update `CHANGELOG.md`.
- Ensure docs reflect current runtime behavior and flags.
- Run local quality gates:

```bash
make fmt
make vet
make test
make coverage
make build
```

- Confirm CI is green on `main`.
- Confirm no secret material in git history or release artifacts.

## Versioning

- Use semantic versions (`vX.Y.Z`).
- Create annotated tag:

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

## Release Notes

- Create release notes under `docs/releases/`.
- Include highlights, reliability notes, and compatibility constraints.

## Post-Release

- Verify workflow completion for CI and release.
- Validate install:

```bash
go get github.com/velariumai/goddgs@vX.Y.Z
```

- Perform smoke test from a clean module cache.
