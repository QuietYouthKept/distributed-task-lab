# Wave 2 remaining gaps

## P0 — before any production Worker admission

- Development-profile process E2E proves an API-created deterministic Eval task is written to shared MinIO, the owner is killed after Put while ACK is blocked, the second Worker reclaims at generation 2 and reuses the artifact, and a stale write using generation 1 is rejected. Remaining: S3/ACK response-loss proxy, conflicting-content recovery, cross-container topology, and production-approved execution types; this is not a production Worker acceptance.
- Gateway receipts need concurrent duplicate-case integration semantics verified against the provider's idempotency behavior; no payment/notification sandbox or reconciliation authority was supplied.
- Production Worker is intentionally fail-closed. No production task type is approved by the execution allowlist.

## P1 — release engineering and API assurance

- Required MinIO CI service builds from a source commit and verifies Go module checksums plus the upstream license checksum, with ephemeral credentials. Run `37960805439` exposed a Go JSON parsing bug in the build helper; the fix passed local source-build verification and remote CI Runs `37962333979` and `37963245838`, including actual Docker image build/start, S3 Contract, two-Worker recovery and backup/restore. This verifies CI at `ac46b6b` and `9b360f5`, not production S3; see [remote CI failure repair](remote-ci-failure-repair.md).
- A separate Go consumer module now verifies the public client surface locally and in CI. OpenAPI tests verify stable routes, methods, roles, auth/error contracts, and recursively resolve every self-contained local component reference; a source-generated route/schema drift gate remains outstanding.
- Cross-process API rate-limit behavior and cursor key rotation are outstanding. A real PostgreSQL 100k-row keyset benchmark now exists; its unfiltered middle-cursor plan uses a bounded index range, while the owner-filtered cursor plan still removes about 9.5k candidate rows on the recorded fixture. Reassess that plan against representative owner distributions before making latency/SLO claims.
- A local OTLP/HTTP receiver now decodes and verifies exported spans; a staging collector assertion and alert firing remain outstanding. Basic structured-log redaction for configured credentials/common credential forms is tested locally, but staging log-sink verification and arbitrary unregistered/opaque secret handling remain unverified. A local PostgreSQL least-privilege integration test verifies a restricted runtime role, but production/hosted role provisioning remains unverified. A local custom-format PostgreSQL dump/restore rehearsal against disposable databases and a temporary MinIO artifact store passed; production backup automation, encrypted offsite retention, scheduled restore cadence, and measured RPO/RTO remain outstanding.
- Standardized E2E dependency lifecycle scripts beyond the environment preflight are incomplete.

## P2 — operational readiness

- Production S3 vendor compatibility, TLS/CA policy, encryption/KMS, lifecycle/retention, workload identity and registry/image signing need platform decisions.
- External IdP production issuer, audience, claim mapping and rotation/availability runbook need owner approval.
- On-call ownership, SLO, RPO/RTO, disaster-recovery cadence and retention enforcement have no verified operational exercise.

## Execution-context notes

- Earlier collector runs without MinIO variables remain immutable as NOT VERIFIED. The latest clean run used newly generated one-run credentials against a temporary pinned MinIO container and passed the authenticated adapter contract, API/two-Worker crash recovery, and database-restore/artifact-reference check. This verifies the local development path only, not production S3 or cloud disaster recovery.
- Existing PostgreSQL/MinIO and unrelated strategyops containers are shared environment resources. Do not stop, reconfigure, or remove them as part of this work.
