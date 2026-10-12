# Local Production-like Verification Matrix

Current P0 runtime implementation source: `f33c9c0b91c9c46f832adef6e1ab1a402859a6fc` (tree `90be9e2f974da4e20f9e9d3cbe8fe018f274ef0c`). Evidence for this source is external at `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\p0-runtime-static-20261011T135136+0800`. Older rows retain their original implementation SHAs and are not rebound to this source.

`PASS` below means only the verification level named in the row. No row implies
production approval. Older Wave 3/Wave 4 evidence is intentionally not reused
as current-source acceptance.

| ID | Requirement | Current result | Source / test | Evidence or blocker |
|---|---|---|---|---|
| P0-A01 | Core Compose API/Worker/Postgres/MinIO topology | PASS: static config/topology only | `deployments/local-first/compose.core.yml`, `scripts/local-first/test-compose-contract.ps1` | 29 checks pass on `f33c9c0`; no service was built or started; MinIO source digest blocks runtime |
| P0-A02 | API readiness fails without DB or schema 12 | IMPLEMENTED IN CODE; runtime NOT RUN | `cmd/api/main.go`, `internal/storage/postgres/task_repository.go:HealthCheck` | Readiness code is unchanged; no profile container test |
| P0-A03 | Production Worker remains fail-closed | PASS: focused current-source package test | `internal/config/config.go:ValidateWorker`, `internal/domain/taskpayload` | `go test` in sealed P0 static run; production allowlist unchanged |
| P0-A04 | Secret/TLS/health/startup/cleanup topology | PASS: provisioning/static contract; runtime PARTIAL | `scripts/local-first/provision.ps1`, `scripts/local-first/new-test-certs.ps1`, compose core | TLS CA and names 3/3 pass; E: ACLs set; Docker Desktop/Linux secret permissions and cleanup not runtime-verified |
| P0-B01 | Local HTTPS JWKS and fixed test roles | PASS: fixture/unit/TLS only | `cmd/oidc-fixture`; `go test ./cmd/oidc-fixture -count=1 -v` | Commit `a96200653400f6243c295779df8934f25412e7f6`; `evidence/oidc-fixture-20261011T131200+0800` |
| P0-B02 | JWT issuer/audience/signature/expiry and CA trust | PARTIAL PASS | Fixture test with `auth.Verifier`, TLS CA bundle, key rotation | Same evidence; no expired/wrong-issuer/wrong-audience API matrix |
| P0-B03 | Reader/operator/admin API authorization and owner isolation | NOT RUN on local fixture | API routes and auth | Container/API matrix not run |
| P0-B04 | JWKS outage, wrong CA, real OIDC login | NOT VERIFIED | No full IdP flow | Requires Compose/API matrix; hosted IdP is external |
| P0-C01 | Separate migration job and protected schema readiness | IMPLEMENTED_STATIC; PG NOT RUN | `deployments/Dockerfile.migrator`, `cmd/migrate`, Compose core | Migrator job dependency and restricted URL are configured; no migration container run |
| P0-C02 | Runtime accounts cannot DDL; API/Worker privilege split | IMPLEMENTED_STATIC; PG NOT RUN | `postgres/00-create-roles.sh`, `runtime-grants.sql`, `least_privilege_integration_test.go` | 3 non-superuser accounts and explicit DML grants; database denial/success matrix still required |
| P0-C03 | Migration idempotency/lock/checksum/recovery | IMPLEMENTED; NOT RUN on this increment | Embedded migrations and existing tests | Need disposable isolated PostgreSQL and current-SHA run; Migration 012 unchanged |
| P0-D01 | Gateway success, 429, timeout, response loss, HTTP 500, explicit failure | PASS: in-process simulator/handler tests | `cmd/gateway-simulator`, `internal/worker/handlers` | Commit `c5d5a93963ddc3223ebf4f5a8ce75c0cd7395706`; `evidence/gateway-contract-20261011T130500+0800` |
| P0-D02 | Malformed response, invalid usage, cost cap and request conflict | PASS: unit/handler only | Gateway contract tests | Same evidence; no Worker container or DB receipt test |
| P0-D03 | Gateway restart/idempotency and unknown result | PARTIAL / known limitation | Restart test proves in-memory idempotency state is lost | Same evidence; no durable Gateway ledger or cross-restart safe recovery |
| P0-D04 | Connection refusal injection | NOT IMPLEMENTED | No dedicated fixture mode | Add controlled refused-endpoint integration test |
| P0-E01 | OIDC→API→Postgres→Worker→Gateway→MinIO→fenced ACK | NOT RUN | local-first Compose core exists | RESOURCE_LIMITED and MinIO registry digest blocked; no false runtime claim |
| P0-E02 | Worker crash, lease reclaim, stale ACK, artifact reuse | NOT RUN for local-first profile | Existing Wave 4 tests are historical baseline only | Requires isolated current-source multi-process run |
| P0-F01 | Isolated PostgreSQL + MinIO backup/restore | NOT RUN for local-first profile | Existing Wave 4 evidence not rebound | Requires two isolated targets and SHA reconciliation |
| P0-G01 | API/Postgres/Worker/MinIO OTLP span links | NOT RUN for local-first profile | Existing observability code/config | Current-source cross-process receiver trace absent |
| P0-G02 | Normal→Firing→Delivered→Resolved to local receiver | NOT RUN in local-first profile | Existing alert configs only | Need local receiver test run |
| P0-H01 | ≤100-task bounded smoke | NOT RUN | Resource policy | Container gate was below 6 GiB at latest preflight |
| P1-A01 | Shared ingress limiter default-off and failure policy | PASS: router/config unit only | `internal/api/router_test.go`, `internal/config/config_test.go` | Commit `692e3b065479c41c7fd0acec2369457a82b5a256`; `evidence/shared-admission-20261011T131000+0800` |
| P1-A02 | Shared quota across PostgreSQL-backed instances | NOT RUN on current source | `shared_ingress_limiter_integration_test.go` | Explicit isolated DB unavailable under current container gate; see P1-A report |
| P1-B01 | Cursor overlap, cutoff, expiry, tamper, binding, legacy v1 | PASS: unit only | `internal/api/handlers/cursor_keyring_test.go` | Commit `f7d420faffa5374fa6eaa9bc2fefb9525da2a2f4`; `evidence/cursor-rotation-20261011T130700+0800` |
| P1-B02 | Same keyring on separate codec instances | PASS: unit only | Second codec validates active-key cursor | Same evidence; no API process rollout or secret distribution |
| P1-C01 | Retention inventory is read-only and bounded | IMPLEMENTED; current DB test NOT RUN | `internal/storage/postgres/retention_dry_run.go` | Purge remains disabled; result/effect/audit references are conservatively excluded |
| P1-C02 | Archive, restore, deletion audit, resume | NOT IMPLEMENTED | No archive/GC workflow | Requires policy decision and isolated recovery workflow |
| P1-D01 | Identity fixture image build and non-root metadata | PASS: build/inspect only | `deployments/Dockerfile.oidc-fixture` | Commit `04621d8343b98aed56ab6898cf527783ecd2ddf0`; local image ID `sha256:56ef5f7abacddf25ad27822294f575b5f26a0e444a7c142aa87b860907a075a9`; `evidence/oidc-image-build-20261011T132000+0800` |
| P1-D02 | Current-SHA full image set, SBOM, scan, signature/provenance, runtime hardening | NOT RUN | Supply chain tools/pipelines | No SBOM/signature/scan or runtime container was produced |
| P1-E01 | API/auth route roles, anonymous denial, issuer/audience/expiry/algorithm negatives | PASS: package/unit only | `internal/api`, `internal/auth` | Commit `f5a42b68c76a602981ea0fd256d670fe775ec528`; `evidence/security-api-unit-20261011T131800+0800` |
| P1-E02 | Cross-owner DB operations, audit durability, tenant/IDOR/SSRF/logging review | NOT RUN / PARTIAL | Existing DB integration/source still needs review | Requires isolated API+DB test matrix; owner field is not full tenant lifecycle |
| P1-F01 | Startup/shutdown/backup/incident runbook reproduction | PARTIAL | Existing `docs/operations.md` and local-first reports | No local-first runtime reproduction |
| P1-F02 | VPS migration/cost plan | NOT COMPLETE | No current local-first VPS guide | No paid resources were created |

## Resource and external boundary

P0 static evidence at `p0-runtime-static-20261011T135136+0800` measured 4.82 GiB
available RAM, TIME_WAIT 266, E: free 53.71 GiB, and CPU 65%; the 6 GiB container
gate failed on RAM and no container E2E started. This evidence is bound to
`f33c9c0` and all manifest SHA256 entries were verified.
Existing Docker containers belong to other work and were left untouched.

External IdP, paid provider, cloud storage, cross-region DR, production SLO,
production on-call, and production deployment remain `NOT VERIFIED`.
`PRODUCTION_NOT_APPROVED` is unchanged.

<!-- CURRENT RUNS BEGIN: generated from evidence manifests -->
### Current deep-engineering runs (generated at UTC 2026-10-11T12:18:00.6304186Z)

| Run ID | Suite | Status | Source commit | Tests | Pass | Fail | Skip | Evidence path |
|---|---|---|---|---:|---:|---:|---:|---|
| 20261011T060427988Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `static-20261011T060427988Z` |
| 20261011T060428323Z | unit | DEPENDENCY_BLOCKED | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `unit-20261011T060428323Z` |
| 20261011T060614491Z | unit | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 28 | 28 | 0 | 0 | `unit-20261011T060614491Z` |
| 20261011T060725858Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `static-20261011T060725858Z` |
| 20261011T060726345Z | unit | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 115 | 115 | 0 | 0 | `unit-20261011T060726345Z` |
| 20261011T060844810Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `static-20261011T060844810Z` |
| 20261011T060845426Z | unit | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 115 | 115 | 0 | 0 | `unit-20261011T060845426Z` |
| 20261011T100149186Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `static-20261011T100149186Z` |
| 20261011T100305783Z | race | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 115 | 115 | 0 | 0 | `race-20261011T100305783Z` |
| 20261011T100336340Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `static-20261011T100336340Z` |
| 20261011T100455342Z | integration | DEPENDENCY_BLOCKED | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `integration-20261011T100455342Z` |
| 20261011T100455719Z | e2e | RESOURCE_LIMITED | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `e2e-20261011T100455719Z` |
| 20261011T100501556Z | security | DEPENDENCY_BLOCKED | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `security-20261011T100501556Z` |
| 20261011T100602438Z | static | PASS | 8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a | 0 | 0 | 0 | 0 | `static-20261011T100602438Z` |
| 20261011T101242969Z | static | PASS | 47974b3c7e11447e60eec6afda9ab40513e65a51 | 0 | 0 | 0 | 0 | `static-20261011T101242969Z` |
| 20261011T101607464Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 0 | 0 | 0 | 0 | `static-20261011T101607464Z` |
| 20261011T101738309Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 0 | 0 | 0 | 0 | `static-20261011T101738309Z` |
| 20261011T101808311Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 0 | 0 | 0 | 0 | `static-20261011T101808311Z` |
| 20261011T101919330Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 0 | 0 | 0 | 0 | `static-20261011T101919330Z` |
| 20261011T101919757Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 9 | 9 | 0 | 0 | `retention-fixture-20261011T101919757Z` |
| 20261011T102011958Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 0 | 0 | 0 | 0 | `static-20261011T102011958Z` |
| 20261011T102012348Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 9 | 9 | 0 | 0 | `retention-fixture-20261011T102012348Z` |
| 20261011T102114274Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 0 | 0 | 0 | 0 | `static-20261011T102114274Z` |
| 20261011T102114664Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 9 | 9 | 0 | 0 | `retention-fixture-20261011T102114664Z` |
| 20261011T102154311Z | static | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 0 | 0 | 0 | 0 | `static-20261011T102154311Z` |
| 20261011T102154716Z | retention-fixture | PASS | 6713b8d1cad358285ec0f7d0491418c0496741f4 | 9 | 9 | 0 | 0 | `retention-fixture-20261011T102154716Z` |
| 20261011T102427928Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 0 | 0 | 0 | 0 | `static-20261011T102427928Z` |
| 20261011T102540809Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 0 | 0 | 0 | 0 | `static-20261011T102540809Z` |
| 20261011T102622442Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 0 | 0 | 0 | 0 | `static-20261011T102622442Z` |
| 20261011T102917751Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 0 | 0 | 0 | 0 | `static-20261011T102917751Z` |
| 20261011T103019956Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 0 | 0 | 0 | 0 | `static-20261011T103019956Z` |
| 20261011T103054400Z | static | PASS | 779bb9ac2c4045e9785b52702c6e1faf71a4e483 | 0 | 0 | 0 | 0 | `static-20261011T103054400Z` |
| 20261011T103202147Z | retention-fixture | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | 12 | 12 | 0 | 0 | `retention-fixture-20261011T103202147Z` |
| 20261011T103229088Z | retention-fixture | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | 12 | 12 | 0 | 0 | `retention-fixture-20261011T103229088Z` |
| 20261011T103233171Z | static | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | 0 | 0 | 0 | 0 | `static-20261011T103233171Z` |
| 20261011T103317845Z | retention-fixture | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | 12 | 12 | 0 | 0 | `retention-fixture-20261011T103317845Z` |
| 20261011T103322262Z | static | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | 0 | 0 | 0 | 0 | `static-20261011T103322262Z` |
| 20261011T103347962Z | static | PASS | f7b26896aac9f46a0145d53a590ae587008ccfe2 | 0 | 0 | 0 | 0 | `static-20261011T103347962Z` |
| 20261011T111357873Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T111357873Z` |
| 20261011T111400325Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T111400325Z` |
| 20261011T111554553Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T111554553Z` |
| 20261011T111557353Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T111557353Z` |
| 20261011T112052387Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 10 | 10 | 0 | 0 | `compose-cleanup-20261011T112052387Z` |
| 20261011T112054430Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T112054430Z` |
| 20261011T112212586Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 7 | 7 | 0 | 0 | `database-safety-20261011T112212586Z` |
| 20261011T112214771Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 10 | 10 | 0 | 0 | `compose-cleanup-20261011T112214771Z` |
| 20261011T112217192Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T112217192Z` |
| 20261011T112219594Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T112219594Z` |
| 20261011T112537861Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 12 | 12 | 0 | 0 | `backup-manifest-20261011T112537861Z` |
| 20261011T112540456Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 7 | 7 | 0 | 0 | `database-safety-20261011T112540456Z` |
| 20261011T112542631Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 10 | 10 | 0 | 0 | `compose-cleanup-20261011T112542631Z` |
| 20261011T112545221Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T112545221Z` |
| 20261011T112548753Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T112548753Z` |
| 20261011T112558343Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T112558343Z` |
| 20261011T112739419Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 8 | 8 | 0 | 0 | `database-safety-20261011T112739419Z` |
| 20261011T112741846Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 12 | 12 | 0 | 0 | `backup-manifest-20261011T112741846Z` |
| 20261011T112744592Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 10 | 10 | 0 | 0 | `compose-cleanup-20261011T112744592Z` |
| 20261011T112747665Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T112747665Z` |
| 20261011T112750931Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T112750931Z` |
| 20261011T112803914Z | evidence-audit | FAIL | 91ca876da73644b47bed76bbc14f50f01a11a54e | 59 | 51 | 8 | 0 | `evidence-audit-20261011T112803914Z` |
| 20261011T113028538Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 13 | 13 | 0 | 0 | `compose-cleanup-20261011T113028538Z` |
| 20261011T113034364Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T113034364Z` |
| 20261011T113228300Z | supply-chain-static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 4 | 4 | 0 | 0 | `supply-chain-static-20261011T113228300Z` |
| 20261011T113539985Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 9 | 9 | 0 | 0 | `database-safety-20261011T113539985Z` |
| 20261011T113542764Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 12 | 12 | 0 | 0 | `backup-manifest-20261011T113542764Z` |
| 20261011T113545207Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 13 | 13 | 0 | 0 | `compose-cleanup-20261011T113545207Z` |
| 20261011T113547912Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T113547912Z` |
| 20261011T113550721Z | supply-chain-static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 4 | 4 | 0 | 0 | `supply-chain-static-20261011T113550721Z` |
| 20261011T113553245Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T113553245Z` |
| 20261011T113954997Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T113954997Z` |
| 20261011T114044981Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T114044981Z` |
| 20261011T114331778Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T114331778Z` |
| 20261011T114332176Z | evidence-integrity | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T114332176Z` |
| 20261011T114333732Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 13 | 13 | 0 | 0 | `compose-cleanup-20261011T114333732Z` |
| 20261011T114334891Z | database-safety | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 9 | 9 | 0 | 0 | `database-safety-20261011T114334891Z` |
| 20261011T114336119Z | backup-manifest | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 12 | 12 | 0 | 0 | `backup-manifest-20261011T114336119Z` |
| 20261011T114337682Z | supply-chain-static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 4 | 4 | 0 | 0 | `supply-chain-static-20261011T114337682Z` |
| 20261011T114339053Z | retention-fixture | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 12 | 12 | 0 | 0 | `retention-fixture-20261011T114339053Z` |
| 20261011T114340591Z | evidence-audit | FAIL | 91ca876da73644b47bed76bbc14f50f01a11a54e | 78 | 63 | 15 | 0 | `evidence-audit-20261011T114340591Z` |
| 20261011T114523729Z | compose-cleanup | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 16 | 16 | 0 | 0 | `compose-cleanup-20261011T114523729Z` |
| 20261011T114525500Z | static | PASS | 91ca876da73644b47bed76bbc14f50f01a11a54e | 0 | 0 | 0 | 0 | `static-20261011T114525500Z` |
| 20261011T114644279Z | static | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | 0 | 0 | 0 | 0 | `static-20261011T114644279Z` |
| 20261011T114644762Z | evidence-integrity | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | 15 | 15 | 0 | 0 | `evidence-integrity-20261011T114644762Z` |
| 20261011T114646483Z | compose-cleanup | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | 16 | 16 | 0 | 0 | `compose-cleanup-20261011T114646483Z` |
| 20261011T114648096Z | database-safety | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | 9 | 9 | 0 | 0 | `database-safety-20261011T114648096Z` |
| 20261011T114649283Z | backup-manifest | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | 12 | 12 | 0 | 0 | `backup-manifest-20261011T114649283Z` |
| 20261011T114650665Z | supply-chain-static | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | 4 | 4 | 0 | 0 | `supply-chain-static-20261011T114650665Z` |
| 20261011T114652003Z | retention-fixture | PASS | 366c320cbc60392de4cbe32f94b98d02c5cd9edd | 12 | 12 | 0 | 0 | `retention-fixture-20261011T114652003Z` |
| 20261011T120008856Z | e2e | RESOURCE_LIMITED | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 0 | 0 | 0 | 0 | `e2e-20261011T120008856Z` |
| 20261011T120124069Z | evidence-integrity | PASS | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 16 | 16 | 0 | 0 | `evidence-integrity-20261011T120124069Z` |
| 20261011T120125787Z | static | PASS | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 0 | 0 | 0 | 0 | `static-20261011T120125787Z` |
| 20261011T120126088Z | e2e | RESOURCE_LIMITED | 2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f | 0 | 0 | 0 | 0 | `e2e-20261011T120126088Z` |
| 20261011T120239543Z | evidence-integrity | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 16 | 16 | 0 | 0 | `evidence-integrity-20261011T120239543Z` |
| 20261011T120241222Z | static | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 0 | 0 | 0 | 0 | `static-20261011T120241222Z` |
| 20261011T120241494Z | e2e | RESOURCE_LIMITED | 4c05943d66064e9afc63b5303a4b182e985acc89 | 0 | 0 | 0 | 0 | `e2e-20261011T120241494Z` |
| 20261011T120525107Z | static | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 0 | 0 | 0 | 0 | `static-20261011T120525107Z` |
| 20261011T120525622Z | evidence-integrity | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 17 | 17 | 0 | 0 | `evidence-integrity-20261011T120525622Z` |
| 20261011T120527345Z | unit | RESOURCE_LIMITED | 4c05943d66064e9afc63b5303a4b182e985acc89 | 0 | 0 | 0 | 0 | `unit-20261011T120527345Z` |
| 20261011T120533380Z | vet | RESOURCE_LIMITED | 4c05943d66064e9afc63b5303a4b182e985acc89 | 0 | 0 | 0 | 0 | `vet-20261011T120533380Z` |
| 20261011T120623118Z | static | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 0 | 0 | 0 | 0 | `static-20261011T120623118Z` |
| 20261011T120623638Z | evidence-integrity | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 17 | 17 | 0 | 0 | `evidence-integrity-20261011T120623638Z` |
| 20261011T120625397Z | unit | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 227 | 227 | 0 | 0 | `unit-20261011T120625397Z` |
| 20261011T120746728Z | vet | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 0 | 0 | 0 | 0 | `vet-20261011T120746728Z` |
| 20261011T120821725Z | race | PASS | 4c05943d66064e9afc63b5303a4b182e985acc89 | 227 | 227 | 0 | 0 | `race-20261011T120821725Z` |
| 20261011T121021874Z | unit | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 227 | 227 | 0 | 0 | `unit-20261011T121021874Z` |
| 20261011T121044229Z | vet | RESOURCE_LIMITED | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 0 | 0 | 0 | 0 | `vet-20261011T121044229Z` |
| 20261011T121050008Z | race | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 227 | 227 | 0 | 0 | `race-20261011T121050008Z` |
| 20261011T121231709Z | static | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 0 | 0 | 0 | 0 | `static-20261011T121231709Z` |
| 20261011T121232271Z | evidence-integrity | PASS | 3ee8135fd13fdd676e10aef28996978876eea6f3 | 18 | 18 | 0 | 0 | `evidence-integrity-20261011T121232271Z` |
| 20261011T121456412Z | unit | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 227 | 227 | 0 | 0 | `unit-20261011T121456412Z` |
| 20261011T121525531Z | race | RESOURCE_LIMITED | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 0 | 0 | 0 | 0 | `race-20261011T121525531Z` |
| 20261011T121540263Z | static | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 0 | 0 | 0 | 0 | `static-20261011T121540263Z` |
| 20261011T121540764Z | evidence-integrity | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 18 | 18 | 0 | 0 | `evidence-integrity-20261011T121540764Z` |
| 20261011T121542624Z | vet | RESOURCE_LIMITED | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 0 | 0 | 0 | 0 | `vet-20261011T121542624Z` |
| 20261011T121728700Z | static | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 0 | 0 | 0 | 0 | `static-20261011T121728700Z` |
| 20261011T121729277Z | evidence-integrity | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 18 | 18 | 0 | 0 | `evidence-integrity-20261011T121729277Z` |
| 20261011T121731548Z | race | PASS | ec4d0bb782b3751a311dedf5e1a34bf207f895b2 | 227 | 227 | 0 | 0 | `race-20261011T121731548Z` |

A PASS applies only to the exact tests and source SHA recorded by that evidence manifest; it does not imply whole-system or production acceptance.
<!-- CURRENT RUNS END -->
