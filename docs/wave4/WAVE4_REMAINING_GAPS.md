# Wave 4 remaining gaps

## P0 — release safety

- **Old binary rollback:** direct rollback to Wave 3 API/Worker remains blocked on schema12 by the embedded migration-history and readiness guards. The populated schema11→12 upgrade, Wave 4 roll-forward restart, and complete isolated pre-migration DB+referenced-artifact restore passed locally on `0d3f48d`; see `WAVE4_MIGRATION_ROLLBACK_COMPATIBILITY.md`. This does not make old binaries compatible with schema12, prove simultaneous mixed-version operations, or constitute a production restore/cutover rehearsal.
- **Hosted final-SHA validation:** previous source `8ec4fbd` passed CI, Security, and Supply Chain. The new populated-migration harness commit `0d3f48d` and its follow-up docs have not yet been pushed/validated at the time of this edit; do not transfer prior hosted status to those descendants. After push, record exact new run IDs/SHA here.
- **Operational recovery:** one isolated backup/restore passed, but no scheduled backup, offsite retention, restore operator, RPO/RTO, or production cutover evidence exists.

The new migration-specific restore test uses a second isolated PostgreSQL database and a second MinIO service. It verifies the fixture's referenced input/result objects and their SHA256, but does not model a production-scale bucket snapshot, versioning/retention, cross-region copy, or real cutover.

## P1 — controlled capabilities

- **Global admission / cursor rotation:** feature candidates remain disabled/opt-in; product policy for principal scope, quotas, limiter-store failure and key overlap is absent. Mixed-version rollout and storage contention/outage are not proven.
- **Retention:** current inventory is bounded and read-only. Purging, archive, resumable deletion, audit, worker-safe deletion and object reachability remain unimplemented or unapproved.
- **Observability:** development rules and a local target-down lifecycle pass; production SLOs, owners, Alertmanager paging, acknowledgement, runbooks, retention and noise policy are absent.
- **Supply chain:** current local govulncheck has zero reachable symbol/package findings but reports module advisory `GO-2026-5932` for `golang.org/x/crypto/openpgp`, no fixed version. This code does not appear to call the affected package; retain and reevaluate it. Signing, publication and provenance verification are absent.
- **Provider integrations:** effect ledger and local HTTP contract tests do not establish a real payment/notification provider, its idempotency semantics, lookup/reconciliation or production credentials.
- **Artifact infrastructure:** MinIO contract/recovery passed locally, but production S3/IAM, KMS, lifecycle, multi-region durability and network policies remain unverified.

## P2 — target-specific assurance

- Repeat the keyset benchmark with representative data, cold/warm cache and target CPU/storage before production index changes.
- Validate rolling deployment, SIGTERM budgets, disruption behavior, resource limits and filesystem/capability restrictions on the selected platform.
- Establish load, contention, chaos and alert-noise thresholds from reviewed SLOs; current local measurements are not SLOs.

Keep `WAVE4_ENGINEERING_PARTIAL` and `PRODUCTION_NOT_APPROVED` until the P0 items close and required product/security owners sign off.
