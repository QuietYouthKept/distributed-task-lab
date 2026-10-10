# Wave 3 release-readiness decision

Snapshot: locally tested implementation `0a5482d657ae431751c205abdf01632ebe89cee6`, documentation-only pushed/hosted-verified descendant `ad52365b7dae2ddb3e05ec32f310d626e0583611`. All three GitHub Actions workflows succeeded on that pushed descendant. Neither local nor hosted success implies staging or production approval.

| Gate | Implementation complete | Local verified | Hosted CI verified for final snapshot | Staging verified | Production approved |
|---|---|---|---|---|---|
| PostgreSQL queue, leases, retry and fencing | Implemented for the current platform scope | PASS: full integration, Race, PostgreSQL ×5, RetryBackoff ×20, Failure Lab and backup/restore | PASS for the current pinned-workflow scope at `ad52365b`: required PostgreSQL integration and Race in CI `38029233771`; not production certification | No | No production workload certification |
| MinIO ArtifactStore and cross-container recovery | Implemented/test harness available | PASS: real MinIO contract and four API/two-Worker container gates | PASS hosted authenticated MinIO and two-Worker **process** recovery in CI `38029233771`; two-Worker **container** 4/4 remains local-only | No | No: cloud provider, IAM, TLS/CA, KMS, lifecycle and policy remain open |
| Provider / effect reconciliation | Contract prototype and ledger paths exist | PASS: local HTTP contract Race ×10 and full suite | General unit/Race CI PASS at `ad52365b`; no real Provider/Sandbox or real reconciliation authority | No provider sandbox | No: `REAL_PROVIDER_NOT_VERIFIED`; production admission remains disabled |
| API identity, RBAC and audit | Local boundary and tests exist | PASS: final unit/integration/Race; v1 route/auth OpenAPI Race ×20 | Current unit/integration/Race and Security workflows PASS at `ad52365b`; no external IdP integration gate | No external IdP | No issuer/claim/JWKS operations approval |
| OpenAPI | CreateTask DTO schema alignment; route/auth set enumerated | PASS for present tests; full drift is not complete | Stable v1 route/auth contract CI PASS at `ad52365b`; 13 response-body schemas still missing | No | No full API contract certification |
| Metrics and tracing | Metrics/trace propagation components exist | PASS for Worker continuation, OTLP receiver, and prior exact-file-set Prometheus scrape | Generic unit/Race CI PASS at `ad52365b`; no full Collector-chain/production alert gate | No real collector chain | No alert/SLO/on-call evidence |
| Backup and recovery | Local drill exists | PASS for disposable PostgreSQL restore + MinIO object checksum | Isolated PostgreSQL backup/restore drill PASS in CI `38029233771`; cloud DR/RPO/RTO absent | No cloud restore | No measured production RPO/RTO |
| CI / security / supply chain | Workflows pinned and statically validated locally | PASS: actionlint and script syntax checks | PASS at `ad52365b`: CI `38029233771`, Security `38029233767`, Supply Chain `38029233773`; Scorecard expected SKIP; Worker advisory UNKNOWN triaged | No | No signing, registry publication or release approval |
| Production Worker admission | Deliberately fail-closed | Empty production allowlist validated | Fail-closed remains intact; CI pass does not grant execution approval | No | No task type approved |

## Decision

Wave 3 **bounded engineering delivery and evidence closeout is COMPLETE**: final clean local gates passed at `0a5482d`, and all three applicable hosted workflows passed at its docs-only descendant `ad52365b` on first attempt. This is **not full Wave 3 feature acceptance**: external Provider, production IdP, cloud storage/DR, complete distributed trace, cursor rotation/global ingress limits, and full API drift remain PARTIAL/NOT VERIFIED. `PRODUCTION_NOT_APPROVED` is unchanged.

Do not merge PR #1, create a milestone/release tag, publish production images, enable a Worker allowlist or begin Wave 4 as part of this closeout. A future stage requires current hosted CI, external IdP/provider/cloud decisions, staging deployment and rollback, backup/RPO/RTO evidence, alert/on-call ownership and explicit release approval.
