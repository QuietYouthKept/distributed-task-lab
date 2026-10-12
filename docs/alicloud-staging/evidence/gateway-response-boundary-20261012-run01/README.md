# Gateway response-boundary fix — 2026-10-12 run 01

Status: focused Worker handler tests PASS. Code was changed in the working tree before this command; source commit/tree record the clean base, and `tested-source-sha256.txt` binds the exact modified files that were executed.

- Base commit: `54df472514d92be62448d44e421db8c9e289866f`
- Base tree: `9f84e45cb008bacd662802bed3c586e822f92070`
- Go: `go1.26.9 windows/amd64`
- Exact command: `go test -p 1 ./internal/worker/handlers -run '^TestGatewayLLMEval|^TestStagingSimulationCompositeExecutesOnlyGatewayCandidate$' -count=1 -timeout=2m`
- Exit: 0.
- Container, PostgreSQL, Race, external Gateway, and cloud checks: NOT RUN.

## Change

The Gateway HTTP client now refuses redirects (`http.ErrUseLastResponse`) so a configured Gateway cannot redirect task input and credentials to another destination. Responses are limited to 64 KiB and parsed as exactly one JSON value. New deterministic tests cover a 307 redirect, a response exceeding the limit, and trailing JSON; selected existing gateway replay/resume and staging-sim candidate tests also passed.

This does not establish production provider safety or a live staging E2E. The gateway endpoint remains deployment-controlled, and `PRODUCTION_NOT_APPROVED` is unchanged. `SHA256SUMS` covers all files here except itself.
