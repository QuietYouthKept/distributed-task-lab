# Wave 3 release-readiness decision

Snapshot: implementation `0a5482d657ae431751c205abdf01632ebe89cee6`; documentation freeze will follow separately. A green local suite or older hosted run does not imply staging or production acceptance.

| Gate | Implementation complete | Local verified | Hosted CI verified for final snapshot | Staging verified | Production approved |
|---|---|---|---|---|---|
| PostgreSQL queue, leases, retry and fencing | Implemented for the current platform scope | PASS: full integration, Race, PostgreSQL ×5, RetryBackoff ×20, Failure Lab and backup/restore | NOT VERIFIED at `0a5482d`; baseline `49c275c` passed | No | No production workload certification |
| MinIO ArtifactStore and cross-container recovery | Implemented/test harness available | PASS: real MinIO contract and four API/two-Worker container gates | Prior MinIO/host-process CI passed at `49c275c`; revised workflow not rerun | No | No: cloud provider, IAM, TLS/CA, KMS, lifecycle and policy remain open |
| Provider / effect reconciliation | Contract prototype and ledger paths exist | PASS: local HTTP contract Race ×10 and full suite | Generic suite passed at `49c275c`; final workflow pending | No provider sandbox | No: `REAL_PROVIDER_NOT_VERIFIED`; production admission remains disabled |
| API identity, RBAC and audit | Local boundary and tests exist | PASS: final unit/integration/Race; v1 route/auth OpenAPI Race ×20 | CodeQL/unit checks passed at `49c275c`; latest run pending | No external IdP | No issuer/claim/JWKS operations approval |
| OpenAPI | CreateTask DTO schema alignment; route/auth set enumerated | PASS for present tests; full drift is not complete | Prior contract test CI passed at `49c275c`; latest pending | No | No full API contract certification |
| Metrics and tracing | Metrics/trace propagation components exist | PASS for Worker continuation, OTLP receiver, and prior exact-file-set Prometheus scrape | Generic unit/Race at `49c275c`; no full trace workflow | No real collector chain | No alert/SLO/on-call evidence |
| Backup and recovery | Local drill exists | PASS for disposable PostgreSQL restore + MinIO object checksum | Backup/restore job passed at `49c275c`; latest run pending | No cloud restore | No measured production RPO/RTO |
| CI / security / supply chain | Workflows pinned and statically validated locally | PASS: actionlint and script syntax checks | `NOT VERIFIED`: push failed; prior three workflows passed at `49c275c` | No | No signing, registry publication or release approval |
| Production Worker admission | Deliberately fail-closed | Empty production allowlist validated | CI result is not an allowlist grant | No | No task type approved |

## Decision

Wave 3 local engineering closeout is **PARTIAL** until the final implementation/workflow snapshot is pushed and the three required hosted workflows complete successfully. No local P0 correctness issue was found in the executed tests. `PRODUCTION_NOT_APPROVED` remains the production decision.

Do not merge PR #1, create a milestone/release tag, publish production images, enable a Worker allowlist or begin Wave 4 as part of this closeout. A future stage requires current hosted CI, external IdP/provider/cloud decisions, staging deployment and rollback, backup/RPO/RTO evidence, alert/on-call ownership and explicit release approval.
