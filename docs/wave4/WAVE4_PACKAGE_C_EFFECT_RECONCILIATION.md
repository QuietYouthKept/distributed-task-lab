# Wave 4 Package C — Effect Identity and Reconciliation

Status: `PASS — LOCAL PACKAGE ACCEPTANCE`

Implementation commit: `8e76d2f5a90b614a99f16cfa8b9d623a314c4667` (tree `5c7e81eb49683fed9a0b9ab3d46ccd91a309274c`), based on Package B documentation commit `51970121a888ed40274ee132ef3fc554c0df0aec`. This package hardens the durable effect ledger and provider contract tests. It does not wire the experimental HTTP provider into production task execution; `PRODUCTION_NOT_APPROVED` and the production Worker allowlist remain unchanged.

## Root cause and changes

The PostgreSQL ledger previously allowed `RecordIntent` to overwrite `request_hash` on conflict for the same `(task_id, operation_key, generation)`. It also accepted a different `effect_id`, kind, or policy while silently retaining the old row's identity. That meant one stable idempotency identity could be rebound to changed request content. The upsert now only advances the attempt when effect ID, kind, policy, and request hash all match; conflicting identity returns an error without changing the row.

`RecordReceipt` previously allowed a later write to replace provider request and response hashes after status was already `applied`. It now permits first receipt recording from `intent`/`unknown`, and permits only an exact duplicate once applied. An incompatible receipt is rejected and the original durable evidence is preserved.

The HTTP provider's reconciliation contract previously accepted terminal `completed`/`failed` states with an incomplete receipt or a status inconsistent with the state. Terminal reconciliation now requires both operation and receipt identity and a matching terminal receipt status; malformed/inconsistent responses remain `unknown` and cannot be mistaken for success or failure.

## Verification

On the clean implementation commit, all of the following passed:

- `go test ./... -count=1` with a fresh isolated PostgreSQL 16 database.
- `go test -race ./... -count=1` with the same isolated PostgreSQL test setup.
- `go vet ./...`.
- `REQUIRE_TEST_DATABASE=1 go test ./internal/storage/postgres -count=1 -v`.
- PostgreSQL regression tests for stable intent identity, immutable applied receipts, a real local HTTP response-loss/reconciliation path, and concurrent receipt-vs-manual-resolution finalization.
- The explicit backup/restore drill was `SKIPPED` because its opt-in environment flag was not enabled; it is not represented as passing.

For the provider response-loss test, the isolated HTTP server records the operation, drops the POST response, then answers a lookup under the same idempotency key. The test stores the operation intent before the call, records the unknown outcome, reconciles the original receipt, and verifies that the ledger becomes applied without changing the request fingerprint. The manual-resolution race verifies exactly one terminal writer wins and that a manual-resolution winner has its audit record.

Evidence and SHA-256 list: `E:\distributed-task-lab-temp\wave4-artifacts\package-c-final-8e76d2f-20261010`. The database password was generated per run, kept only in the test process/container environment, and is not recorded. The database data mount and test logs are on E:.

## Limits

- The test HTTP server is a controlled fault-injection provider, not a real payment/notification vendor. Vendor-specific idempotency retention, authentication, rate limits, dispute semantics, and reconciliation SLAs remain unverified.
- `SideEffectHandler` still uses the development receipt-file adapter; the HTTP `Provider` interface is not wired into production execution. Enabling a real provider requires an explicit product/security decision and a complete provider-specific apply/reconcile adapter.
- Current handler request fingerprint covers kind, operation key, and target. Any future provider adapter that sends additional business fields must include canonicalized provider request content in the fingerprint before those fields can be enabled.
- Hosted CI has not been run for the Wave 4 branch. No external-provider exactly-once or production readiness claim is made.
