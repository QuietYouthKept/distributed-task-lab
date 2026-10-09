# Wave 3 release-readiness gate

Statuses are deliberately independent; a local test does not imply remote, staging, or production acceptance.

| Gate | Code Implemented | Local Verified | Remote CI Verified | Staging Verified | Production Approved |
|---|---|---|---|---|---|
| PostgreSQL queue, lease and fencing | Yes (prior work) | Yes (Wave 2 evidence) | Yes for required CI checks at `ac46b6b` | No | No production workload certification |
| Shared ArtifactStore | Yes (S3 adapter) | Yes against MinIO (Wave 2) | Yes against source-built MinIO at `ac46b6b` | No | No: provider, TLS/CA, IAM, KMS, policy and retention not approved |
| Worker execution allowlist | Production admission remains empty | Fail-closed validation tested | Not a permission grant | No | No task type approved |
| Identity and RBAC | OIDC verifier and local controls exist | Local issuer/JWKS and PostgreSQL tests | CodeQL and unit/integration jobs pass at `ac46b6b` | No external IdP exercise | No: issuer, audience, JWKS/claim policy operational approval absent |
| External side-effect Provider | HTTP contract prototype only; not wired | Targeted local HTTP Race tests on current work | No for Wave 3 code yet | No | No: `REAL_PROVIDER_NOT_VERIFIED` |
| API cursor rotation / global rate limit | No approved cluster semantics | No | No | No | No decision |
| Full async trace / production alerts | Partial local OTLP and API metrics | Prior local receiver only | Not full path | No Collector/Alertmanager exercise | No SLO/on-call ownership |
| Backup and recovery | Local automation/rehearsal exists | PostgreSQL + MinIO restore verified in Wave 2 | Backup/restore CI passed at `ac46b6b` | No cloud DR exercise | No production RPO/RTO |
| Build, SBOM and release | CI source-pinned MinIO test dependency; application release work partial | MinIO build script verified | CI and security jobs pass at `ac46b6b` | No deployment | No release approval or signed production image |

## Blockers before any production release

- Production IdP/JWKS choice, claim mapping, key rotation and operator ownership.
- Approved production Worker task types and complete execution/reconciliation acceptance.
- Real Provider selection and sandbox evidence; no real payment/notification transaction was sent.
- Production object-store vendor, TLS/CA, workload identity, KMS, lifecycle and retention policy.
- Cloud backup encryption/offsite retention and approved, measured RPO/RTO.
- Actionable SLO/alert thresholds, Alertmanager routing, on-call owner and incident runbooks.
- Staging deployment, migration compatibility and rollback rehearsal.
- Release owner approval; no tag, merge, production deployment, or production permission change occurred.
