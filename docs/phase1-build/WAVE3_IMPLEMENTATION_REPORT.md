# Wave 3 implementation report

Date: 2026-10-10 (Asia/Shanghai). This report separates code, local verification, remote CI, staging, and production claims.

## Stage 0 — CI follow-up closure

Implementation commit: `ac46b6bf2411aa68af671897f6ebdb2e0786e323`.

- Fixed `GoModSum` parsing for Go's `go mod download -json` output, whose field contains a trailing comma. The prior silent `test` failure is now diagnosed explicitly.
- Pinned and SHA-256 verified the upstream MinIO license fetched from the same source commit as the Go module; source module checksums remain fixed.
- Updated the separate consumer module's Go directive to 1.26.9.
- Remote CI Run `37962333979` passed all five jobs, including real MinIO image construction/start and required S3, recovery and backup/restore tests. Security Run `37962334001` passed CodeQL, govulncheck and Secret Scan; Scorecard was skipped by its PR-event condition.
- No production API, database schema, or worker admission policy changed in this commit. The original failed Runs remain immutable.

## Package B — provider-independent HTTP contract (implemented; provider integration remains open)

New source: `internal/effects/http_provider.go`; new tests: `internal/effects/http_provider_test.go`; decision: `docs/adr/ADR-0005-provider-independent-http-contract.md`.

The port carries operation/idempotency/request/task identity, lease generation and attempt. It treats a transport timeout or reset after dispatch, 5xx, and malformed success as Unknown. A pre-dispatch cancellation is Not Sent. The local contract tests cover success, permanent and retryable rejection, 5xx, malformed response, redirect, timeout, connection reset, duplicate key replay, same-key/different-payload rejection, and reconciliation states. Provider response bodies are not copied into returned errors.

`internal/worker/handlers/side_effect.go` now returns Effect Ledger persistence failures together with the original uncertain outcome instead of silently discarding the `MarkUnknown` error.

This is a real local HTTP contract/fault-injection test, not a real payment/notification Provider integration. The production handler is not wired to this adapter. `REAL_PROVIDER_NOT_VERIFIED` remains true.

Implementation commit: `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf`. Clean Evidence `wave3-20261010T171000Z-9b360f5952bb` passed 14/14 against this commit. Its new provider and side-effect tests passed within full unit and full Race suites, plus the targeted Race ×10 run preceding the commit.

The same commit passed remote CI Run `37963245838` and Security Run `37963245883`. Remote CI includes PostgreSQL Integration, full Race, the actual source-built MinIO image, S3 Contract, host-process API/two-Worker recovery, PostgreSQL backup/restore, and external client. The local clean Evidence additionally recorded Failure Lab, Chaos 6×30, 100k pagination, vet, OTLP receiver, and all hashes. These are development/local validations, not staging or production evidence.

The full local Evidence environment was Windows AMD64, Go 1.26.9, Docker Engine 28.3.3, PostgreSQL 16.15, and an ephemeral source-built MinIO image with local image ID `sha256:7c340841a5a8e0f7d0013b6e5e8e03bde8beb9cb009608e37de6f79c9a19e2cc` (not a Registry Digest). The MinIO health endpoint returned HTTP 200. Backup/restore measured about 358 ms for dump and 420 ms for restore in this isolated fixture. Those timings are not an RPO/RTO commitment.

The 100,000-row keyset traversal completed 201 pages with zero duplicates; local middle-cursor P50/P95 was 4.123/4.411 ms and owner-filtered P50/P95 was 5.572/7.141 ms on the recorded fixture. The six Chaos scenarios passed 30/30 each. Recorded recovery percentiles are local harness measurements and not production SLOs. See the manifest and copied artifacts for query plans and exact configuration.

## Package C — local rate-limit bucket lifecycle fix and policy proposal

Review confirmed that `internal/api/router.go` kept a hard-bounded map of 10,000 verified Principal buckets but never evicted any. Once filled, even inactive identities remained unable to obtain a bucket until process restart. Commit `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` reclaims only a bucket whose available tokens would already have reached the configured burst under normal refill. `TestIngressLimiterReclaimsOnlyFullyRefilledIdleBuckets` checks just-before-refill denial and exact-refill reclamation; targeted Race ×100 and vet passed.

Clean Evidence `wave3-20261010T171600Z-cedbdc1edb09` passed 14/14 against the exact implementation commit, including full tests, full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab 12/12, Chaos, pagination, MinIO contract, process-recovery and backup/restore. CI Run `37965020508` and Security Run `37965020486` also passed for this commit (Scorecard was conditionally skipped).

This is only process-local bucket lifecycle management. It does not implement cluster-wide quotas. ADR-0006 records the decisions needed for shared quotas and cursor key rotation; no cursor format or production setting changed.

## Other packages

Package A and remaining parts of D/E are not yet accepted. The dual-Worker recovery is still an independent host-process test, not API/Worker containers; response-loss via an S3 network proxy, conflicting-content process recovery, cursor rotation, cluster rate-limit semantics, full trace/alert firing, SBOM/image scan and release smoke remain unverified. Their exact gaps and next actions are in `WAVE3_REMAINING_GAPS.md`; no production/staging claims are inferred from these local results.
