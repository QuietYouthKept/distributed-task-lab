# Wave 4 Package D — Cross-Instance Admission & Cursor Rotation

## Result

`PARTIAL` — the isolated implementation candidate and local verification are complete. Product approval for a global quota and shared-store outage policy is still required before enablement. Both features remain off by default; the existing API token-bucket and v1 cursor behavior remain unchanged.

## Cross-instance admission

`internal/storage/postgres/shared_ingress_limiter.go` implements an optional PostgreSQL-backed token bucket. `api_ingress_rate_limit_buckets` has a fixed 10,000-slot pool. A keyed HMAC-SHA256 digest identifies a principal without storing the subject itself. Each decision uses PostgreSQL `statement_timestamp()` as the refill authority and a row-locked update; slot acquisition uses `FOR UPDATE SKIP LOCKED`. A partial unique index prevents two rows from representing one principal. New-principal races retry a bounded three times. When every slot is held by partially depleted buckets, admission is denied instead of growing storage without bound.

The shared gate is after authentication and before the route. Shared errors return sanitized 503 by default in the explicitly enabled configuration. An operator can explicitly choose fail-open; in that mode, the existing per-process limiter still protects local overload. Shared denials return 429 and `Retry-After`. The API-level option is disabled by default and validates its rate, burst, failure policy, and HMAC key.

`deployments/e2e/admission-compose.yaml` starts PostgreSQL and two separately built API containers. The runner creates ephemeral credentials, polls readiness, launches concurrent requests split across the two replicas, collects logs, destroys only its Compose project/volume, and hashes the evidence. Clean implementation commit `5258c5add74354809d53a9bf966775077fd492d5` run `admission-daf1761e5397` admitted exactly 16 of 64 concurrent calls and rate-limited 48; the shared bucket therefore did not reset at the API process boundary. The strict PostgreSQL Race test also checks distinct principal buckets, independent limiter objects, slot bound, and absence of the raw principal. No hosted/staging/global traffic behavior or contention SLO has been tested.

The first E2E attempt revealed an adjacent readiness defect: readiness still expected migration version/count 11 after migration 012 was added. `latestSchemaVersion` is now 12, and the successful clean two-container run exercised readiness against the full migrated schema. The initial failure and its logs are preserved in the preceding `admission-7e3c24c900a7` E: run directory.

## Cursor rotation

`internal/api/handlers/cursor_keyring.go` is an opt-in v2 codec. It issues with one active key and verifies only active plus explicitly bounded previous keys. Tokens bind owner subject, normalized filter, page size, keyset tuple, issue time, and expiry. TTL must be between one second and seven days. Previous-key and v1 legacy acceptance require finite `accept_until` boundaries; unknown/revoked keys, expired tokens, altered bindings, invalid future issue times, and MAC tampering are rejected. MAC comparison uses `hmac.Equal`. Stateless replay remains allowed during validity because pagination is read-only.

The original v1 codec and constructor remain the default for compatibility. Enabling the ring without the optional legacy key/cutoff intentionally rejects v1 cursors. Unit tests cover overlap before/after cutoff, legacy compatibility cutoff, expiration, changed owner/filter/page size, tampering, unknown key, invalid ring settings, and concurrent decode. This is not a rolling-deployment or secret-distribution test.

## Verification

- `go test ./internal/api/... ./internal/config ./internal/storage/postgres ./internal/e2e -count=1` — PASS (without a configured PostgreSQL URL, DB-gated package tests self-skip; separate live DB evidence below is authoritative).
- `go test -race ./internal/storage/postgres -count=1` — PASS on isolated PostgreSQL 16 with `TEST_DATABASE_URL` and `REQUIRE_TEST_DATABASE=1`; backup/restore drill is explicitly opt-in and was SKIPPED.
- `go test -race ./internal/api/... ./internal/config ./internal/e2e -count=1` — PASS; `go vet` for the same packages — PASS.
- `go test -race ./internal/e2e -run '^TestSharedIngressAdmissionAcrossIndependentAPIContainers$' -count=1 -v` — PASS in the isolated two-API/PostgreSQL Compose runner on clean implementation commit `5258c5a`.
- Cursor rotation handler tests — PASS in the API Race run above.
- First two-container attempt — FAIL before request testing, because the readiness migration version constant lagged migration 012. Diagnostic bundle is preserved; it is not presented as a passing run.

Exact run logs, manifest and SHA256 files: `E:\\distributed-task-lab-temp\\wave4-artifacts\\shared-admission-e2e-runs\\admission-daf1761e5397` (clean-commit PASS), `...\\admission-235f3ca88873` (pre-commit implementation smoke), and `...\\admission-7e3c24c900a7` (failed setup). Clean-commit PostgreSQL Race: `E:\\distributed-task-lab-temp\\wave4-artifacts\\package-d-postgres-race-clean-5258c5a-20261010`; API/config Race + vet: `E:\\distributed-task-lab-temp\\wave4-artifacts\\package-d-api-race-vet-5258c5a-20261010`. A prior command that set `DATABASE_URL` instead of the package's `TEST_DATABASE_URL` exited 0 with DB tests skipped; its files remain in `package-d-strict-postgres-20261010` and `package-d-postgres-race-20261010` and must not be counted as PASS.

## Decisions still needed and limitations

- Product owner must choose quota subject/scope, rate/burst, and fail-open versus fail-closed by endpoint class. No approval is inferred from these tests.
- Fixed 10k slot capacity is a candidate bound, not a measured maximum user count. Saturation rejects new partial buckets; operations need capacity and cleanup policy before large deployments.
- PostgreSQL write contention, outage/recovery behavior, fail-open/fail-closed live database fault test, and multi-region clock/DB behavior are not verified.
- Cursor key-ring TTL/overlap and secret distribution require an operator/owner decision. Rolling mixed-version deployment, external secret manager, and hosted tests are not verified.
- Production remains `PRODUCTION_NOT_APPROVED`; no production setting was enabled.
