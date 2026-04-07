# CLI Reference

Binary: `goddgs` (or `go run ./cmd/goddgs`).

## Commands

- `providers`
  - Prints currently enabled providers from environment config.

- `search`
  - Flags:
    - `--q` query (required)
    - `--max` max results (default `10`)
    - `--region` region code (default `us-en`)
    - `--json` output full JSON response

- `doctor`
  - Prints active configuration summary.
  - Executes a probe search (`golang`) with bounded timeout.

## Exit Codes

- `0`: success
- `2`: invalid input / usage / no results / blocked classification
- `3`: doctor probe failed
- `4`: engine initialization or provider/runtime error

## Examples

```bash
go run ./cmd/goddgs providers
go run ./cmd/goddgs search --q "golang" --json
go run ./cmd/goddgs doctor
```
