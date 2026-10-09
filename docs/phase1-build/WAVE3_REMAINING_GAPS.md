# Wave 3 remaining gaps

## P0 — cross-container storage and recovery

- Add a disposable Compose topology with PostgreSQL, MinIO, API and at least two separately built Worker containers on a task-specific network, each with independent filesystems.
- Demonstrate shared input/output reads, Put response loss after persistence, same-key/same-content reuse, same-key/different-content conflict, worker crash after Put/before ACK, lease reclaim, and stale-generation fencing.
- Preserve exact object bytes/hash and database result-reference evidence. Existing CI recovery is host-process based, not container topology proof.

## P0 — external side effects

- New provider-independent HTTP contract client and tests are local-verified but need clean-commit full suite and Evidence.
- Integrate only after selecting a provider and documenting provider-specific idempotency, retry, signature, timeout, receipt, and reconciliation behavior. Keep `REAL_PROVIDER_NOT_VERIFIED` until an approved sandbox E2E.
- Add PostgreSQL integration coverage proving unknown-outcome persistence failure is visible and cannot be mistaken for a recorded reconciliation state.
- Admin manual resolution still needs end-to-end authenticated HTTP authorization, tenant/owner isolation, request-ID audit and concurrent update tests.

## P1 — API consistency and security

- Complete OpenAPI request/response schema drift checks beyond route, auth, errors and local reference validity.
- Cursor HMAC rotation needs key IDs, bounded old-key acceptance windows, TTL and compatibility decision. Do not enable indefinite historical key acceptance or change production defaults before an owner-approved ADR.
- Current ingress rate limiting is per process. Decide global quota semantics, shared store, partition behavior, Retry-After and whether local overload protection remains layered; no shared implementation is yet enabled.
- Production IdP/JWKS operations, production role mapping, and external issuer rotation remain unverified (`EXTERNAL_IDP_NOT_VERIFIED`).

## P1 — observability and operations

- Local OTLP receiver proves an exported span, not the API → database → async Worker → ArtifactStore → fenced ACK path. Add a real Collector and trace-link evidence with secret redaction.
- Prometheus currently scrapes the API; add Worker scraping and thresholds only after service owners approve actionable alert policy. Validate actual alert firing and recovery. No Alertmanager/on-call loop is certified.
- Owner-filtered pagination plan needs representative high/low owner-selectivity EXPLAIN ANALYZE, buffers and write-cost comparison before index changes.
- Backup/restore is a local rehearsal only. Automate protected/offsite backups and isolated recovery checks; cloud DR and RPO/RTO remain unverified.
- Retention/GC remains to be designed as dry-run first with reference checks, bounded batches, audit and resumability.

## P2 — release and supply chain

- Build API/Worker images with source commit metadata, least privilege, read-only-root feasibility, health checks, signal/drain tests and reproducible dependency inputs.
- Produce a source/image-linked SBOM and container vulnerability scan artifact. Go vulnerability scanning and CodeQL do not cover image OS/package findings.
- Add RC, migration compatibility, rolling/worker drain, rollback and recovery smoke procedures. Staging and production approvals remain separate states.
- Maintain `Production Worker Fail-Closed`; no production task allowlist, real provider, hosted S3/KMS, external IdP, or release approval is granted here.
