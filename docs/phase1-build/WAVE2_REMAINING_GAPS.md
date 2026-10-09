# Wave 2 remaining gaps

## P0 — before any production Worker admission

- Development-profile process E2E now proves an API-created deterministic Eval task is written to shared MinIO, the owner is killed after Put while ACK is blocked, and the second Worker reclaims after lease expiry and reuses the artifact. Remaining: stale-generation rejection after reclaim, S3/ACK response-loss proxy, conflicting-content recovery, cross-container topology, and production-approved execution types; this is not a production Worker acceptance.
- Gateway receipts need concurrent duplicate-case integration semantics verified against the provider's idempotency behavior; no payment/notification sandbox or reconciliation authority was supplied.
- Production Worker is intentionally fail-closed. No production task type is approved by the execution allowlist.

## P1 — release engineering and API assurance

- Required MinIO CI service is configured with a pinned digest and ephemeral credentials; remote Actions has not been run.
- A separate Go consumer module now verifies the public client surface locally and in CI. Full generated OpenAPI drift enforcement remains outstanding.
- Cross-process API rate-limit behavior and cursor key rotation are outstanding. A real PostgreSQL 100k-row keyset benchmark now exists; its unfiltered middle-cursor plan uses a bounded index range, while the owner-filtered cursor plan still removes about 9.5k candidate rows on the recorded fixture. Reassess that plan against representative owner distributions before making latency/SLO claims.
- A local OTLP/HTTP receiver now decodes and verifies exported spans; a staging collector assertion, secret-redaction tests and alert firing remain outstanding. A local PostgreSQL least-privilege integration test now verifies a restricted runtime role, but production/hosted role provisioning remains unverified; backup/restore rehearsal remains outstanding.
- Standardized E2E dependency lifecycle scripts beyond the environment preflight are incomplete.

## P2 — operational readiness

- Production S3 vendor compatibility, TLS/CA policy, encryption/KMS, lifecycle/retention, workload identity and registry/image signing need platform decisions.
- External IdP production issuer, audience, claim mapping and rotation/availability runbook need owner approval.
- On-call ownership, SLO, RPO/RTO, disaster-recovery cadence and retention enforcement have no verified operational exercise.

## Execution-context notes

- Earlier collector runs without MinIO variables remain immutable as NOT VERIFIED. A later clean run used newly generated one-run credentials against a temporary pinned MinIO container and passed the authenticated adapter contract; this still does not verify dual-worker E2E or production S3.
- Existing PostgreSQL/MinIO and unrelated strategyops containers are shared environment resources. Do not stop, reconfigure, or remove them as part of this work.
