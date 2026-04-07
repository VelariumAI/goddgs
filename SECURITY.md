# Security Policy

## Reporting

Report vulnerabilities privately to maintainers.
Do not open public issues for sensitive security reports.

## Scope

Security reports are especially relevant for:

- request routing and provider key handling,
- HTTP service exposure and error payloads,
- third-party solver/provider integration paths,
- dependency vulnerabilities.

## Secrets Handling

- Never commit API keys or tokens.
- Use environment variables for credentials.
- Rotate credentials immediately if exposure is suspected.

## Responsible Use

`goddgs` includes optional anti-bot and challenge-solver integrations.
Deployers are responsible for compliant and lawful operation in their jurisdiction and target environment.
