# Wave 3 implementation report

Date: 2026-10-10 (Asia/Shanghai). This report separates code, local verification, remote CI, staging, and production claims.

## Stage 0 — CI follow-up closure

Implementation commit: `ac46b6bf2411aa68af671897f6ebdb2e0786e323`.

- Fixed `GoModSum` parsing for Go's `go mod download -json` output, whose field contains a trailing comma. The prior silent `test` failure is now diagnosed explicitly.
- Pinned and SHA-256 verified the upstream MinIO license fetched from the same source commit as the Go module; source module checksums remain fixed.
- Updated the separate consumer module's Go directive to 1.26.9.
- Remote CI Run `37962333979` passed all five jobs, including real MinIO image construction/start and required S3, recovery and backup/restore tests. Security Run `37962334001` passed CodeQL, govulncheck and Secret Scan; Scorecard was skipped by its PR-event condition.
- No production API, database schema, or worker admission policy changed in this commit. The original failed Runs remain immutable.

## Package B — provider-independent HTTP contract (in progress)

New source: `internal/effects/http_provider.go`; new tests: `internal/effects/http_provider_test.go`; decision: `docs/adr/ADR-0005-provider-independent-http-contract.md`.

The port carries operation/idempotency/request/task identity, lease generation and attempt. It treats a transport timeout or reset after dispatch, 5xx, and malformed success as Unknown. A pre-dispatch cancellation is Not Sent. The local contract tests cover success, permanent and retryable rejection, 5xx, malformed response, redirect, timeout, connection reset, duplicate key replay, same-key/different-payload rejection, and reconciliation states. Provider response bodies are not copied into returned errors.

`internal/worker/handlers/side_effect.go` now returns Effect Ledger persistence failures together with the original uncertain outcome instead of silently discarding the `MarkUnknown` error.

This is a real local HTTP contract/fault-injection test, not a real payment/notification Provider integration. The production handler is not wired to this adapter. `REAL_PROVIDER_NOT_VERIFIED` remains true.

## Other packages

Packages A, C, D and the remaining parts of E are not yet accepted. Their exact gaps and next actions are in `WAVE3_REMAINING_GAPS.md`; no production/staging claims are inferred from the prior Wave 2 evidence.
