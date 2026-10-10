# Wave 3 release-readiness gate

Statuses are deliberately independent; a local test does not imply remote, staging, or production acceptance.

| Gate | Code Implemented | Local Verified | Remote CI Verified | Staging Verified | Production Approved |
|---|---|---|---|---|---|
| PostgreSQL queue, lease and fencing | Yes (prior work) | Yes (Wave 2 and Wave 3 Evidence) | Yes for required CI checks at `9b360f5` | No | No production workload certification |
| Shared ArtifactStore | Yes (S3 adapter) | Yes against MinIO in Wave 3 Evidence | Yes against source-built MinIO at `9b360f5` | No | No: provider, TLS/CA, IAM, KMS, policy and retention not approved |
| Cross-container crash recovery and response loss | Yes, disposable Compose harness at `9b81e96` | Targeted Race PASS in clean run `tasklab-e2e-980f3ba036b3`; Evidence commit `962ec4e` | No remote run for this topology | No | Partial package acceptance: worker-path response loss, malformed metadata and dual-writer same-case tests remain |
| Worker execution allowlist | Production admission remains empty | Fail-closed validation tested | Not a permission grant | No | No task type approved |
| Identity and RBAC | OIDC verifier and local controls exist | Local issuer/JWKS and PostgreSQL tests | CodeQL and unit/integration jobs pass at `9b360f5` | No external IdP exercise | No: issuer, audience, JWKS/claim policy operational approval absent |
| External side-effect Provider | HTTP contract prototype only; not wired | Full and targeted HTTP Race tests in Evidence `wave3-20261010T171000Z-9b360f5952bb` | CI at `9b360f5` passed unit/full Race; no real external service | No | No: `REAL_PROVIDER_NOT_VERIFIED` |
| API cursor rotation / global rate limit | ADR-0006 proposal; local limiter full-refill bucket reclamation implemented at `cedbdc1` | Clean Evidence 14/14, targeted Race ×100 | CI `37965020508` and Security `37965020486` pass for `cedbdc1`; no shared/global limiter or key ring | No | No decision on quota outage policy, cursor TTL/overlap |
| Full async trace / production alerts | Partial local OTLP and API metrics | Prior local receiver only | Not full path | No Collector/Alertmanager exercise | No SLO/on-call ownership |
| Backup and recovery | Local automation/rehearsal exists | PostgreSQL + MinIO restore verified in Wave 3 Evidence | Backup/restore CI passed at `9b360f5` | No cloud DR exercise | No production RPO/RTO |
| Build, SBOM and release | CI source-pinned MinIO test dependency; application release work partial | MinIO build and clean Evidence verified | CI and security jobs pass at `9b360f5` | No deployment | No release approval or signed production image |

## Blockers before any production release

- Production IdP/JWKS choice, claim mapping, key rotation and operator ownership.
- Approved production Worker task types and complete execution/reconciliation acceptance.
- Real Provider selection and sandbox evidence; no real payment/notification transaction was sent.
- Production object-store vendor, TLS/CA, workload identity, KMS, lifecycle and retention policy.
- Cloud backup encryption/offsite retention and approved, measured RPO/RTO.
- Actionable SLO/alert thresholds, Alertmanager routing, on-call owner and incident runbooks.
- Staging deployment, migration compatibility and rollback rehearsal.
- Release owner approval; no tag, merge, production deployment, or production permission change occurred.
