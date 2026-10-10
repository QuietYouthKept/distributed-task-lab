# Wave 2 E2E architecture and current boundary

## Intended verified chain

`OIDC verifier → API RBAC/resource scope → application service → PostgreSQL task + security audit → independent Worker claim/fencing → shared ArtifactStore → fenced ACK → task result/audit → trace`

The code currently has local OIDC/JWKS and PostgreSQL authorization/audit tests. Store is injectable into RAG, deterministic Eval, Gateway Eval, and Replay. In development, S3 can be explicitly selected; worker startup verifies the configured bucket before creating the pool. The production task admission policy remains an empty allowlist and `Config.ValidateWorker` rejects production startup until the full execution path is accepted.

## Runtime topology status

| Component | Current status |
|---|---|
| PostgreSQL | Real PostgreSQL 16 integration and Race tests pass on host port 5433. |
| MinIO | Authenticated adapter contract passed under Race against a temporary pinned-image container with one-run random credentials (Evidence `wave2-20261009T123051207Z-0e8f75573f67`); not a dual-worker run or production-vendor/TLS validation. |
| API | A real API executable was launched as an independent host process for the development-profile recovery E2E; it is not a containerized/production topology test. |
| Worker A / B | Two independent Worker executables consumed an API-created task. The test killed the lease owner after shared MinIO Put while a PostgreSQL row lock blocked ACK; the other process reclaimed after lease expiry, reused the artifact, and ACKed at attempt/version 2. |
| Network | `tasklab-wave2-net` exists; no new resources or unrelated containers were altered. |
| Trace | A local OTLP/HTTP receiver test decodes exported protobuf and verifies service/span identity under Race. No staging collector or full API-to-worker trace was verified. |

## Failure chain not yet demonstrated

The process-level failure chain is recorded in Evidence `wave2-20261009T143117144Z-24d8857f8d90`, using the deterministic Eval development handler, an isolated PostgreSQL database, and the temporary MinIO service. It proves one artifact-recovery path, not every handler or a production execution path. The production execution allowlist remains empty and `ValidateWorker` still fails closed. Do not claim end-to-end exactly-once: the system remains at-least-once with lease fencing and idempotent artifact reuse.

Still unverified: stale-generation rejection after the competing worker has reclaimed, a response-loss proxy around S3 Put/ACK, content-conflict recovery, production-approved task types, a production IdP/provider, cross-container deployment, and staging trace continuity.
