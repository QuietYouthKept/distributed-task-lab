# Wave 2 remaining gaps

## P0 — before any production Worker admission

- Authenticated MinIO/S3 conditional-write contract and denial tests using the actual configured service.
- Two independent Worker processes consuming API-created PostgreSQL tasks and shared input/output artifacts; kill-after-put-before-ACK, lease expiry, reclaim, stale-generation rejection, response-loss and content-conflict scenarios.
- Gateway receipts need concurrent duplicate-case integration semantics verified against the provider's idempotency behavior; no payment/notification sandbox or reconciliation authority was supplied.
- Production Worker is intentionally fail-closed. No production task type is approved by the execution allowlist.

## P1 — release engineering and API assurance

- MinIO CI service is not yet wired into the required workflow; this run did not verify remote Actions.
- Complete generated OpenAPI drift enforcement and externally compiled Go client module test are not demonstrated by the current run.
- 100k-row keyset benchmark/EXPLAIN evidence, cross-process API rate-limit behavior, and cursor key rotation are outstanding.
- OTLP receiver assertion, secret-redaction tests, alert firing, DB least-privilege tests and backup/restore rehearsal remain outstanding.
- Standardized E2E dependency lifecycle scripts beyond the environment preflight are incomplete.

## P2 — operational readiness

- Production S3 vendor compatibility, TLS/CA policy, encryption/KMS, lifecycle/retention, workload identity and registry/image signing need platform decisions.
- External IdP production issuer, audience, claim mapping and rotation/availability runbook need owner approval.
- On-call ownership, SLO, RPO/RTO, disaster-recovery cadence and retention enforcement have no verified operational exercise.

## Execution-context blocker

Both MinIO health endpoints returned HTTP 200 and Docker service/network resources existed, but MinIO credential variables were absent from the Codex process environment. No credentials were extracted from Docker metadata or logs. Authenticated MinIO and cross-container E2E remain NOT VERIFIED; request safe re-injection of the already provisioned variables in the execution shell rather than guessing credentials or changing shared resources.
