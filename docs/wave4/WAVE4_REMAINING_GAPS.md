# Wave 4 remaining gaps

## P0 — release safety

- **Old binary rollback:** migration 012 raises schema to 12; Wave 3 API and Worker reject it before serving. No compatible bridge or production restore rehearsal exists. This is an observed compatibility failure, not a theoretical concern. The process matrix covers startup/readiness, not all API/task/effect operations per version/schema cell or a populated-data upgrade/drain workflow.
- **Hosted final-SHA validation:** no Wave 4 CI, CodeQL, secret scan, SBOM or container scan has completed on the final source. Historic Wave 3 checks are not transferable.
- **Operational recovery:** one isolated backup/restore passed, but no scheduled backup, offsite retention, restore operator, RPO/RTO, or production cutover evidence exists.

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
