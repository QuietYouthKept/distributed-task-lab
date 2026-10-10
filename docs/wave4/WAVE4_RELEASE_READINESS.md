# Wave 4 release readiness

Decision: `WAVE4_ENGINEERING_PARTIAL`; `PRODUCTION_NOT_APPROVED`.

## Gate summary

| Area | State | Release decision |
|---|---|---|
| API contract, tracing/effects and local correctness | Locally verified on recorded SHAs | Keep implementation; the latest hosted runs on `8ec4fbd` passed, but the migration-harness commits `778e01c` and `0d3f48d` require their own exact-SHA hosted runs after push. |
| Shared ingress admission / cursor rotation | Candidate only, default off / v1 remains default | Do not enable until quota identity, outage mode and key rotation are approved and contention/outage tests are complete. |
| Retention / pagination | Dry-run inventory and local benchmark | No purge or production index decision until archive, deletion, artifact reachability and audit policies are approved. |
| Metrics / local alerts / graceful shutdown | Local checks pass | No production SLO, paging, owner, Alertmanager route or runbook evidence. |
| Database migration rollback | `ROLLBACK_BLOCKED` | Populated schema11→12 data preservation, Wave 4 retry/lease/effect/artifact operations, compatible roll-forward restart, and complete isolated DB+referenced-object restore passed on `0d3f48d`. Wave 3 API/Worker still fail closed on schema12; concurrent old/new operations there and production cutover remain unverified. No direct binary downgrade across migration 012. |
| Artifacts / backup restore | Authenticated MinIO and one isolated DB+object restore pass | No cloud IAM/KMS/region durability or operational backup schedule evidence. |
| Vulnerability scan | Local `govulncheck ./...` pass with residual module advisory | Current source not found to call the affected OpenPGP package; advisory has no fixed version and remains unresolved. |
| Hosted CI / supply-chain checks | PASS on `00c0479`, `fb412eb`, and `8ec4fbd`; new source commits pending | Required CI, Security, and Supply Chain jobs passed on prior exact SHAs; Scorecard skipped as expected for PR event. Do not transfer that status to `0d3f48d` or its docs descendants. |
| Production credentials, external providers, deployment | Not exercised | Keep production Worker allowlist closed and all production profiles fail-closed. |

## Required before any production approval

1. Keep direct Wave 3 rollback blocked. Before production, approve and rehearse either a future pre-migration compatibility release or a full target-specific restore/cutover; the local separate-target drill is not production approval.
2. Obtain product decisions for D (identity/scope/quotas/failure mode), E (retention/archive/purge/audit), F (SLOs, alerts, ownership and on-call), and the supported production platform.
3. Validate real identity, TLS/secret distribution, object-store IAM/KMS/policies, provider idempotency/reconciliation, and production backup/restore against the selected environment.
4. Run Wave 4 hosted CI, security, SBOM and image scans on the exact pushed `0d3f48d` source, then on the final documentation SHA if it changes. Review any residual advisory; no failing job may be made optional to obtain green status.
5. Rehearse rollout, rollback boundary, recovery, observability and incident response on the target orchestrator before enabling workers.

No Release, merge, tag, Force Push, production secret, or Worker allowlist change is part of this closeout.
