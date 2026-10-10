# Wave 4 release readiness

Decision: `WAVE4_ENGINEERING_PARTIAL`; `PRODUCTION_NOT_APPROVED`.

## Gate summary

| Area | State | Release decision |
|---|---|---|
| API contract, tracing/effects and local correctness | Locally verified on recorded SHAs | Keep implementation; hosted CI on run head `b3e0020` passed generic unit/integration/Race and MinIO/worker checks. The populated migration-safety E2E remains local-only on `0d3f48d`. |
| Shared ingress admission / cursor rotation | Candidate only, default off / v1 remains default | Do not enable until quota identity, outage mode and key rotation are approved and contention/outage tests are complete. |
| Retention / pagination | Dry-run inventory and local benchmark | No purge or production index decision until archive, deletion, artifact reachability and audit policies are approved. |
| Metrics / local alerts / graceful shutdown | Local checks pass | No production SLO, paging, owner, Alertmanager route or runbook evidence. |
| Database migration rollback | `ROLLBACK_BLOCKED` | Populated schema11→12 data preservation, Wave 4 retry/lease/effect/artifact operations, compatible roll-forward restart, and complete isolated DB+referenced-object restore passed on `0d3f48d`. Wave 3 API/Worker still fail closed on schema12; concurrent old/new operations there and production cutover remain unverified. No direct binary downgrade across migration 012. |
| Artifacts / backup restore | Authenticated MinIO and one isolated DB+object restore pass | No cloud IAM/KMS/region durability or operational backup schedule evidence. |
| Vulnerability scan | Local `govulncheck ./...` pass with residual module advisory | Current source not found to call the affected OpenPGP package; advisory has no fixed version and remains unresolved. |
| Hosted CI / supply-chain checks | PASS on run head SHA `b3e0020f408f26b0d55f97a2b6e49ee6096e1429` | CI `38047488517`, Security `38047488510`, and Supply Chain `38047488545` passed. Security Scorecard skipped by the PR-event condition. Hosted CI runs generic PostgreSQL migration tests but does not exercise the populated schema11→12 safety harness; that E2E is local-only on `0d3f48d`. |
| Production credentials, external providers, deployment | Not exercised | Keep production Worker allowlist closed and all production profiles fail-closed. |

## Required before any production approval

1. Keep direct Wave 3 rollback blocked. Before production, approve and rehearse either a future pre-migration compatibility release or a full target-specific restore/cutover; the local separate-target drill is not production approval.
2. Obtain product decisions for D (identity/scope/quotas/failure mode), E (retention/archive/purge/audit), F (SLOs, alerts, ownership and on-call), and the supported production platform.
3. Validate real identity, TLS/secret distribution, object-store IAM/KMS/policies, provider idempotency/reconciliation, and production backup/restore against the selected environment.
4. Hosted CI, Security, SBOM and image scans passed on run head SHA `b3e0020` (runs above). Review any residual advisory; no failing job may be made optional to obtain green status.
5. Rehearse rollout, rollback boundary, recovery, observability and incident response on the target orchestrator before enabling workers.

No Release, merge, tag, Force Push, production secret, or Worker allowlist change is part of this closeout.
