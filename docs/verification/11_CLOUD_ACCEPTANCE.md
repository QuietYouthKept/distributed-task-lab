# Cloud Acceptance Plan (Prepared, Not Executed)

This matrix is a safe staged route for a future explicitly approved cloud run. It does not authorize resource creation and does not claim cloud readiness or acceptance.

| Level | Preconditions | Action / oracle | Evidence and current status |
|---|---|---|---|
| Local isolated baseline | PostgreSQL 16 image provenance verified; isolated Compose project; adequate Linux host budget | Migrations/roles, DB state/lease/fence/retry/effect/audit/limiter assertions | Current SHA NOT_RUN. Use existing PostgreSQL-only scripts and exact digest; no other DB. |
| Single Linux ECS | Approved single ECS budget, SSH-only access, private DB/object endpoints, secrets provisioned outside Git | Migration, least privilege, one staging-sim eval/gateway task through API→DB→Worker→Gateway→Artifact→fenced ACK; include auth negative and crash/reclaim | NOT_RUN; requires cloud purchase and access approval. Production Allowlist stays empty. |
| Two ECS | Second approved instance and private network, distinct Worker process, bounded run/cleanup | Cross-host concurrent claim, crash, lease reclaim, stale ACK, shared limiter and cursor keyring | NOT_RUN; single-host or logical-worker evidence is insufficient. |
| Operations/recovery | Independent DB + object restore targets, OTLP collector/alert receiver, image registry and cost approval | Backup/restore hashes/references, Normal→Firing→Delivered→Resolved, digest rollback, resource inventory | NOT_RUN; no OSS/RDS/ECS resources created. |

Use the existing cloud deployment material under `docs/alicloud-staging/`, `infra/alicloud/`, and `scripts/alicloud/`; validate those exact artifacts before any paid run. User authorization is required for Push, ECS/OSS creation, RAM-role binding, and any Apply/Destroy. No operation in this plan was performed.
