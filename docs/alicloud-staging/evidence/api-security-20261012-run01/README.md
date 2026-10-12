# API security source audit and package tests — 2026-10-12 run 01

Status: bounded API unit tests PASS. Static review found the checked owner/role/request-boundary controls below; it did not include a live PostgreSQL cross-tenant test. No production code changed in this audit.

- Source commit: `2f66ddc2e83947aa6525b82125409aa9cddff042`
- Source tree: `9eb2d65735842957af1d8c747ff35807edb767fd`
- Go: `go1.26.9 windows/amd64`
- Exact command: `go test -p 1 ./internal/api/... -count=1 -timeout=2m`
- Exit: 0; API, contract, and handlers packages passed.
- Race, database integration, full suite, and remote CI: NOT RUN.

## Static checks

- `internal/storage/postgres/task_repository.go`: `GetTask`, task listing, cancellation/requeue, and event reads add `created_by` filtering for non-admin principals. Admin scope is an explicit role override. `TestTaskOwnerScopeAndIdempotencyIsolation` is the PostgreSQL integration regression test, but it was not run locally in this resource-constrained session.
- `internal/api/handlers/decode.go`: one JSON value only; unknown struct fields rejected.
- `internal/api/router.go`: request body limit and request timeout are applied before auth/handlers; API server has bounded read/header/write/idle timeouts.
- `internal/api/router.go`: verified OIDC roles gate task writes, queue/effect administration, and operator reads. Production configuration forbids static token auth and requires OIDC; production worker remains fail-closed.
- No untrusted request URL was found flowing to an HTTP client in the reviewed API/auth paths. Configured JWKS and provider endpoints remain deployment-controlled; this was a source scan, not a full SSRF proof.

These observations are not a security certification or hosted identity-provider acceptance. `tested-source-sha256.txt` binds the reviewed key files. `SHA256SUMS` covers the evidence files except itself.
