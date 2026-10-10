# Wave 3 remaining gaps

This list is current as of implementation snapshot `0a5482d657ae431751c205abdf01632ebe89cee6`. It supersedes earlier progress bullets; historical implementation and failure records remain in the evidence index.

## P0 — no locally reproduced blocker identified

The final 14-gate clean-worktree suite and four live cross-container gates passed. No local data-loss, stale-fencing, duplicate-completion or known P0 failure reproduced in this verification. This is a bounded local result, not a claim that all production failure modes are covered.

## P1 — remaining capability, external-dependency and production gaps (outside scoped engineering evidence closeout)

**Resolved delivery blocker:** Final docs-only HEAD `ad52365b` has been pushed. CI `38029233771`, Security `38029233767` (including govulncheck) and Supply Chain `38029233773` all passed on the first attempt at that exact SHA. The earlier local network EOF/push failures remain historical; the current hosted verification gap is closed. This does not close the remaining P1 capability and production gaps listed below.
- **Credential incident scope:** the local MinIO credential was rotated and container data volume retained. User-saved terminal logs were not located, and external reuse cannot be ruled out. Rotate any reuse and review any copies outside the inspected repository/E: folders. Do not claim past logs or chat disclosures were withdrawn.
- **External identity operations:** production IdP/JWKS issuer, audience/claim mapping, key rotation, outage behavior and operator ownership need a real configured provider. Local verifier/JWKS tests are not an IdP staging exercise (`EXTERNAL_IDP_NOT_VERIFIED`).
- **External side effects:** choose a payment/notification provider and obtain an approved sandbox. Verify provider-specific signatures, idempotency, response-loss recovery, reconciliation and manual resolution authorization. Keep `REAL_PROVIDER_NOT_VERIFIED`; no production effect is enabled.
- **Production Worker allowlist:** production task execution remains fail-closed/empty until task types and side-effect policies are reviewed and approved.
- **Storage and recovery:** choose cloud S3 vendor/region, TLS/CA, workload identity, KMS, bucket policy and retention. MinIO tests do not establish other S3 implementations. Build protected offsite backup/restore and measure approved RPO/RTO in isolated cloud staging.
- **End-to-end observability:** connect API request → DB-persisted context → separate Worker → MinIO/fenced ACK → actual OTLP Collector and verify a received Trace ID with redaction. The Worker continuation test and local OTLP receiver are separate tests. Production alert rules, Alertmanager delivery, runbooks, SLO and on-call ownership remain unverified.
- **API drift:** 15 stable v1 operations are route/auth enumerated. Only CreateTask request/payload/response field parity is checked against Go DTOs. Thirteen operations lack response-body schemas: `GET /v1/tasks`, `GET /v1/tasks/{id}`, `GET /v1/tasks/{id}/events`, `POST /v1/tasks/{id}/cancel`, `POST /v1/tasks/{id}/requeue`, `GET /v1/dlq`, `GET /v1/stats`, `GET /v1/queues`, `PUT /v1/queues/{name}`, `POST /v1/queues/{name}/pause`, `POST /v1/queues/{name}/resume`, `GET /v1/effects`, and `POST /v1/effects/{id}/resolve`. Legacy unversioned and health aliases are intentionally outside the v1 operation set but should be documented if they remain public.
- **Pagination key rotation and global rate limiting:** cursor key IDs, old-key overlap/TTL and revocation semantics need a decision. Ingress limiting is process-local; cross-instance quota semantics, shared-store failure policy and Retry-After behavior need approval and implementation. Existing per-process bucket reclamation is not global fairness.
- **Supply-chain release maturity:** the revised Node 24 action pins and `ubuntu-24.04` runner passed the updated hosted workflows at `ad52365b`. Image signing, registry publication, deployment/rollback rehearsal, production image approval and residual Worker `GO-2026-5932` UNKNOWN advisory triage remain separate from this hosted workflow acceptance.

## P2 — operational maturity

- Perform representative high/low owner-selectivity `EXPLAIN ANALYZE BUFFERS`, index write-cost and retention-size measurements before changing database indexes.
- Design dry-run-first retention/GC with reference checks, bounded batches, audit, resumability and concurrent-worker safety.
- Pin local Prometheus/Grafana overlay images, add alert rules and test firing/recovery; establish production service discovery, TLS/authentication and on-call routes.
- Add migration compatibility, rolling worker drain, rollback and recovery smoke in staging.
- Review image hardening (read-only root, capability drops, health/drain behavior), provenance/signing and reproducible release inputs.
- Establish production SLO, capacity target and workload/resource evidence. The final local pagination and Chaos latency samples are not production SLOs.

## External dependency / approval boundary

Production IdP/JWKS; real Provider sandbox and reconciliation authority; cloud S3/KMS/IAM; offsite backup; production deployment and staging credentials; approved Worker task types; SLO/RPO/RTO and on-call owner; release approval. Until those are supplied and verified, keep `PRODUCTION_NOT_APPROVED` and do not open the production Worker allowlist.
