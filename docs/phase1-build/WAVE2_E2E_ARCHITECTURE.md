# Wave 2 E2E architecture and current boundary

## Intended verified chain

`OIDC verifier → API RBAC/resource scope → application service → PostgreSQL task + security audit → independent Worker claim/fencing → shared ArtifactStore → fenced ACK → task result/audit → trace`

The code currently has local OIDC/JWKS and PostgreSQL authorization/audit tests. Store is injectable into RAG, deterministic Eval, Gateway Eval, and Replay. In development, S3 can be explicitly selected; worker startup verifies the configured bucket before creating the pool. The production task admission policy remains an empty allowlist and `Config.ValidateWorker` rejects production startup until the full execution path is accepted.

## Runtime topology status

| Component | Current status |
|---|---|
| PostgreSQL | Real PostgreSQL 16 integration and Race tests pass on host port 5433. |
| MinIO | Two endpoints health-checked (Windows 9000 and Docker 19000); authenticated contract was not run because credentials were absent from the execution process. |
| API | Real code and local contract tests; not launched as a container for Wave 2 E2E. |
| Worker A / B | Handler-level shared Store tests use independent handler instances; no two independent worker processes were run. |
| Network | `tasklab-wave2-net` exists; no new resources or unrelated containers were altered. |
| Trace | Existing instrumentation is not proof of a received OTLP trace; no external receiver assertion was performed. |

## Failure chain not yet demonstrated

No Wave 2 run killed a worker after S3 Put and before database ACK. The handler artifact writes are content-bound/conditional and the PostgreSQL repository still owns fencing, but cross-process unknown-outcome recovery is not yet proven as a complete system. Do not claim end-to-end exactly-once. Expected guarantee remains at-least-once execution plus lease fencing and external idempotency/reconciliation.
