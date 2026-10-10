# Phase 1 build decisions

This file records implementation assumptions for the authorized Phase 1 build. It is not a claim of production readiness. Update links to ADRs as they are created.

## Defaults accepted from the user

- Deployment audience: single-tenant internal task platform; no billing or cross-organization model in this phase.
- Principals: human and service identities; roles `reader`, `operator`, `admin` are explicit issuer-claim mappings, not names inferred from token text.
- Identity: configurable OIDC issuer/audience/JWKS; tests use local ephemeral signing keys and a controlled JWKS server. No production credentials.
- Storage: MinIO is the real local/CI S3-compatible target; LocalStore is development/test only. Never claim AWS S3 conformance solely from MinIO tests.
- External effects: no real provider sandbox credentials are assumed. Provider contract and controlled fake are allowed; TASK-016 remains incomplete until a real sandbox E2E is supplied.
- Production: fail closed. A missing dependency/configuration must not downgrade to static auth, local storage, mock handlers, or deterministic evaluation.
- Git/evidence (historical build assumption): local package commits are allowed; no release tag. This final closeout explicitly authorized a normal push of the current branch; the attempt failed at proxy TLS. No force-push, merge, release tag, or historical evidence rewrite is allowed.

## Implementation choices to preserve

- Authenticated principal is created only by verifier middleware. Actor/owner/role values from payload, query, or caller headers are untrusted and ignored/rejected.
- Default task resource scope is creator principal; admin can access all. Legacy tasks with no creator are not visible to ordinary callers by default; admin-only access until a deliberate backfill policy is approved.
- Security audit writes must be transactionally coupled to privileged DB mutations where possible. Audit failure must fail the mutation closed.
- S3 references are server-configured bucket + prefix; task payload cannot select endpoint, bucket, credentials, or arbitrary URL.
- ObjectInfo Key is a logical key and never embeds the private prefix, so an `artifact://` reference can be passed back through the same Store configuration. Stable Handler receipts use immutable per-case or content-addressed keys; do not model a shared Store as a cross-worker append-only file.
- Object integrity uses explicit SHA256 metadata or actual content hashing, never ETag as universal SHA256. Conditional put semantics must be proven against MinIO and the chosen Go SDK before production enablement.
- Task cancellation remains database-authoritative; workers poll ownership/cancel state on a bounded interval if no push channel is implemented. A cancelled or stale generation cannot successfully ack.
- Queue rate limiting is not described as API ingress rate limiting; API limiter is process-local unless a shared backend is implemented.
- Cursor is versioned, signed, bound to normalized filters and page size; stable order is `(created_at,id)`.

## Product/platform decisions still open (not blockers to local work)

- Production OIDC issuer/JWKS availability and operational rotation SLO.
- Production S3 vendor/region, encryption/key management, lifecycle policy, max object size and workload identity mechanism.
- Real payment/notification sandbox and resolution authority; until provided, no real provider E2E claim.
- Production deployment platform, approved secret distribution, on-call owner, SLO, RPO/RTO and registry signing policy.
- Historical collector runs without MinIO variables remain NOT VERIFIED and are not overwritten by later passes. The authenticated S3 adapter and independent API/two-Worker container E2E have since passed on recorded local runs; they remain local MinIO/Docker evidence, not other-cloud or production proof. Never retrieve secrets from Docker metadata/logs or infer defaults.
- Whether/when to execute optional AI Track (real vector database/model, dataset rights, privacy, budget and evaluation review).

## Final closeout decisions — 2026-10-10

- Keep the source repository on D:; keep new run logs, binaries, build caches, secrets and final independent Evidence on E:. Do not move the repository.
- Treat `0a5482d657ae431751c205abdf01632ebe89cee6` as the implementation snapshot. Any docs-only commit after it must say tests are bound to that SHA and must not imply a new source test run.
- Pin GitHub-hosted runners to `ubuntu-24.04` and Node 24 action implementations by signed full commit SHA. This removes observed deprecation/runner drift warnings, but a hosted run remains required.
- Keep the local Worker credential rotated and stored only in restricted E: files. Preserve original logs; publish only reviewed redacted copies. If the credential was reused outside this inspected local instance, rotate those copies separately.
- Treat GO-2026-5932 as a retained module-level UNKNOWN with no OpenPGP reachability found in the final Worker binary; do not suppress it or claim a fixed version.
- Keep `REAL_PROVIDER_NOT_VERIFIED`, `EXTERNAL_IDP_NOT_VERIFIED`, cloud DR NOT VERIFIED and the production Worker allowlist empty until actual external systems and owners are approved.
- Because the implementation push did not complete, final remote CI is pending. Do not merge or release based on the prior `49c275c` runs.

## ADR links

- Identity, RBAC and principal/resource scope: [ADR-0001](../adr/ADR-0001-identity-resource-scope-and-audit.md).
- ArtifactStore and object consistency: [ADR-0002](../adr/ADR-0002-artifact-store-and-production-gate.md).
- Effect reconciliation: [ADR-0004](../adr/ADR-0004-effect-reconciliation.md); worker cancellation fencing remains pending.
- Cursor pagination and ingress admission: [ADR-0003](../adr/ADR-0003-signed-pagination-and-ingress-limits.md).
- Release/rollback: staging smoke and rollback evidence remain NOT VERIFIED; production release approval is external.
