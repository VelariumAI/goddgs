# Release Checklist

## Pre-release

- Update `CHANGELOG.md` for the new version.
- Confirm `README.md` examples run against current API.
- Run local quality checks:
  - `make fmt`
  - `make vet`
  - `make test`
  - `make coverage`
  - `make build`
- Verify CI is green on `main`.
- Ensure no secrets/tokens in repo history for this release.

## Versioning

- Use semver tags (`vX.Y.Z`).
- Create annotated tag:
  - `git tag -a vX.Y.Z -m "vX.Y.Z"`
- Push tag:
  - `git push origin vX.Y.Z`

## Post-release

- Verify GitHub release workflow completed.
- Validate module install:
  - `go get github.com/velariumai/goddgs@vX.Y.Z`
- Run smoke test with fresh module cache.
- Announce release notes.
