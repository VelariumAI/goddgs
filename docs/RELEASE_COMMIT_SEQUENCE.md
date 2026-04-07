# Release Commit Sequence

Use this sequence when you want a clean, reviewable release history.

## Suggested commit buckets

1. `chore(repo)`: repository scaffolding, workflows, policy files.
2. `feat(core)`: engine, providers, CLI, service runtime.
3. `feat(reliability)`: resilience logic, anti-bot controls, observability.
4. `test(quality)`: coverage and regression test expansions.
5. `docs(release)`: release notes and operational documentation.

## Example commands

```bash
git add .
git commit -m "release(vX.Y.Z): production-ready public release"
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin main --tags
```
