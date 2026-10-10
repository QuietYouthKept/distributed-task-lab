# Wave 4 release readiness

Decision: `WAVE4_ENGINEERING_PARTIAL`; `PRODUCTION_NOT_APPROVED`.

## Gate summary

| Area | State | Release decision |
|---|---|---|
| API contract, tracing/effects and local correctness | Locally verified on recorded SHAs | Keep implementation; hosted checks still required for final Wave 4 SHA. |
| Shared ingress admission / cursor rotation | Candidate only, default off / v1 remains default | Do not enable until quota identity, outage mode and key rotation are approved and contention/outage tests are complete. |
| Retention / pagination | Dry-run inventory and local benchmark | No purge or production index decision until archive, deletion, artifact reachability and audit policies are approved. |
| Metrics / local alerts / graceful shutdown | Local checks pass | No production SLO, paging, owner, Alertmanager route or runbook evidence. |
| Database migration rollback | `ROLLBACK_BLOCKED` | Wave 3 API/Worker fail closed on schema12. Runtime matrix proves startup/readiness only; task/effect mixed-version data semantics and a populated-data forward/rollback drill remain unverified. No binary downgrade across migration 012. |
| Artifacts / backup restore | Authenticated MinIO and one isolated DB+object restore pass | No cloud IAM/KMS/region durability or operational backup schedule evidence. |
| Vulnerability scan | Local `govulncheck ./...` pass with residual module advisory | Current source not found to call the affected OpenPGP package; advisory has no fixed version and remains unresolved. |
| Hosted CI / supply-chain checks | PASS on head `00c04793119b8ab1e435dc7ea0b88fb9f9afd6b5`, attempt 1 | Required CI, Security, and Supply Chain jobs passed; Scorecard skipped as expected for PR event. A docs-only closeout update will trigger a follow-up run. |
| Production credentials, external providers, deployment | Not exercised | Keep production Worker allowlist closed and all production profiles fail-closed. |

## Required before any production approval

1. Resolve the migration boundary with a tested pre-migration compatibility release, or an approved complete restore/cutover procedure.
2. Obtain product decisions for D (identity/scope/quotas/failure mode), E (retention/archive/purge/audit), F (SLOs, alerts, ownership and on-call), and the supported production platform.
3. Validate real identity, TLS/secret distribution, object-store IAM/KMS/policies, provider idempotency/reconciliation, and production backup/restore against the selected environment.
4. Run Wave 4 hosted CI, security, SBOM and image scans on the exact pushed SHA. Review any residual advisory; no failing job may be made optional to obtain green status.
5. Rehearse rollout, rollback boundary, recovery, observability and incident response on the target orchestrator before enabling workers.

No Release, merge, tag, Force Push, production secret, or Worker allowlist change is part of this closeout.
