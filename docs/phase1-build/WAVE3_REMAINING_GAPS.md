# Wave 3 remaining gaps

## P0 — cross-container storage and recovery

- A disposable Compose topology exercises PostgreSQL, source-built MinIO, API and two separately built Worker containers on a run-scoped network with independent worker filesystems. Final clean implementation-bound run `tasklab-e2e-10de3fd9335c` passed four live gates at `e74e59f`: response loss in the S3 contract and actual Worker PUT path, API/two-Worker crash recovery, and simultaneous same-key writes by two Worker containers. Per-log SHA256 values are recorded; credentials are excluded.
- Remaining: validate other S3-compatible/cloud implementations and real network/storage fault behavior; current results are local Docker/MinIO evidence only. Preserve exact object bytes/hash and DB result-reference checks. No exactly-once claim is made; remote CI recovery is host-process based and does not replace local container topology evidence.

## P0 — external side effects

- New provider-independent HTTP contract client and tests are local-verified but need clean-commit full suite and Evidence.
- Integrate only after selecting a provider and documenting provider-specific idempotency, retry, signature, timeout, receipt, and reconciliation behavior. Keep `REAL_PROVIDER_NOT_VERIFIED` until an approved sandbox E2E.
- Add PostgreSQL integration coverage proving unknown-outcome persistence failure is visible and cannot be mistaken for a recorded reconciliation state.
- Admin manual resolution still needs end-to-end authenticated HTTP authorization, tenant/owner isolation, request-ID audit and concurrent update tests.

## P1 — API consistency and security

- OpenAPI now declares the `POST /v1/tasks` 201 response schema and checks the create request, nested payload envelope, and create response property sets against their Go DTO JSON tags. Other request/response bodies are still not covered by systematic DTO/schema drift checks; no whole-API generated drift gate is claimed.
- Cursor HMAC rotation needs key IDs, bounded old-key acceptance windows, TTL and compatibility decision. Do not enable indefinite historical key acceptance or change production defaults before an owner-approved ADR.
- Current ingress rate limiting is per process. Decide global quota semantics, shared store, partition behavior, Retry-After and whether local overload protection remains layered; no shared implementation is yet enabled.
- Process-local limiter bucket reclamation now removes only inactive buckets whose tokens would already have fully refilled, preventing the 10,000-subject capacity from staying saturated until restart. This does not verify cross-instance fairness or outage policy.
- Production IdP/JWKS operations, production role mapping, and external issuer rotation remain unverified (`EXTERNAL_IDP_NOT_VERIFIED`).

## P1 — observability and operations

- A deterministic Worker test now proves persisted W3C `traceparent` extraction, parent/trace continuity into the execution span, and removal of internal propagation fields from handler payload. The local OTLP receiver separately proves span export. Still missing: API → PostgreSQL persisted context → separate Worker process → ArtifactStore/fenced ACK → real Collector end-to-end trace evidence, including secret redaction.
- Optional local Compose observability now scrapes API and Worker successfully; API scraping retains Bearer auth via a runtime secret file. It uses a single named development Worker and mutable Prometheus/Grafana tags. Production replica/service discovery, TLS/authentication, pinned images, alert thresholds, actual alert firing/recovery, and Alertmanager/on-call ownership remain unverified.
- Owner-filtered pagination plan needs representative high/low owner-selectivity EXPLAIN ANALYZE, buffers and write-cost comparison before index changes.
- Backup/restore is a local rehearsal only. Automate protected/offsite backups and isolated recovery checks; cloud DR and RPO/RTO remain unverified.
- Retention/GC remains to be designed as dry-run first with reference checks, bounded batches, audit and resumability.

## P2 — release and supply chain

- Build API/Worker images with source commit metadata, least privilege, read-only-root feasibility, health checks, signal/drain tests and reproducible dependency inputs.
- Local source/image-linked CycloneDX SBOM generation and full-severity Trivy scanning are implemented and verified at `75bce219fb329d1960bc66e25fcda37b3898f099`; exact artifacts/reports are preserved at `E:\distributed-task-lab-temp\wave3-artifacts\package-e-75bce21`. API reported zero findings; Worker retains one `UNKNOWN` module advisory (`GO-2026-5932`, `golang.org/x/crypto/openpgp` v0.57.0) that govulncheck classifies as not apparently called by imported code. No HIGH/CRITICAL findings were present. Remote Actions execution is NOT VERIFIED because the commits are unpushed. Signing/publication, release/rollback smoke, and production approval remain open.
- Add RC, migration compatibility, rolling/worker drain, rollback and recovery smoke procedures. Staging and production approvals remain separate states.
- Maintain `Production Worker Fail-Closed`; no production task allowlist, real provider, hosted S3/KMS, external IdP, or release approval is granted here.
