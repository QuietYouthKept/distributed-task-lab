# P0-D Gateway Simulator Contract Status

Date: 2026-10-11 (Asia/Shanghai)

## Scope

The only locally admitted candidate remains `eval/gateway` under `staging-sim`.
The simulator never calls a paid model provider; generated cost is zero except
for the explicit `cost_exceeded` fault fixture. All model, usage, latency, and
cost values are simulated. This verifies protocol handling, not real inference.

## Implemented fault modes

`GATEWAY_SIMULATOR_MODE` selects one bounded deterministic mode:

| Mode | Behavior | Verified locally |
|---|---|---|
| `normal` | successful deterministic response; request-id fingerprint replay | Yes |
| `explicit_failure` | HTTP 200 with a valid `failed` case result | Yes |
| `rate_limit` | HTTP 429 with `Retry-After: 1` | Yes |
| `http_500` | HTTP 500 | Yes |
| `timeout` | completes simulated work, then delays response; client deadline can expire | Yes |
| `response_loss` | completes and stores in-memory result, then closes the connection | Yes |
| `malformed_response` | HTTP 200 with malformed JSON | Yes |
| `invalid_usage` | HTTP 200 with negative token count | Yes |
| `cost_exceeded` | HTTP 200 with a deliberately excessive simulated cost | Yes |

The handler tests reject malformed JSON, invalid status, negative usage, and
negative cost. A valid explicit case failure is recorded as a failed case. The
worker's configured cost cap is checked after receipt persistence; no real cost
is incurred here.

## Idempotency and restart boundary

Within one simulator process, the request ID is the idempotency key and the
fingerprint is SHA-256 over the exact request JSON bytes. Repeating an identical
request replays its result; reusing the request ID with a different body returns
HTTP 409. The bounded ledger is in memory only. A dedicated restart test proves
that a new simulator process starts with an empty ledger and executes the same
request again. Therefore this fixture does **not** establish idempotency across
simulator restart, and response-loss recovery across a process restart remains
unverified. Connection refusal and persisted idempotency state are not yet
implemented as simulator features.

## Validation evidence

On the current implementation worktree, using Go 1.26.9:

- `go test ./cmd/gateway-simulator -count=1 -v` — PASS.
- `go test ./internal/worker/handlers -run 'TestGatewayLLMEval(RejectsMalformedAndInvalidUsageResponses|RecordsExplicitCaseFailure)$' -count=1 -v` — PASS.
- These are unit/HTTP handler tests with in-process test servers, not Compose,
  PostgreSQL, MinIO, multi-process, or full task-chain tests.
- The test run folder is recorded outside the repository under
  `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\gateway-contract-20261011T130500+0800`.

## Status

`P0_D_LOCAL_IMPLEMENTED: PARTIAL` — deterministic contract and fault modes are
implemented, but simulator restart durability and connection-refusal injection
are absent.

`P0_D_LOCAL_VERIFIED: PARTIAL` — unit/handler behavior above is verified only.
Worker-to-Gateway behavior in a separate container, database receipt durability,
unknown-result reconciliation, and Gateway restart recovery remain unverified.

`PRODUCTION_NOT_APPROVED` remains unchanged.
