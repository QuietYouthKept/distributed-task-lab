# Overnight engineering board

Updated: 2026-10-12. Worktree: `E:\\distributed-task-lab-temp\\production-readiness\\worktree`; branch: `codex/production-readiness-p0-p1`.

## Current execution checkpoint

- Implementation HEAD under validation: `ebd237d0af40376bd746051bde4e435d9006e3d1`; Tree: `301c445cdb6290eb1dafae14eb6081a61ff3ad2a`. Targeted Go tests, Docker pull, Terraform init, and Terraform validate ran at `f6a3c6e146f34a031f7da5e50d005638867d46cd`, tree `953e0839bfe4def7d2142dba08a82b8032b61fe8`.
- Current changes include report/evidence updates and the new Terraform lock file. The pre-existing untracked `docs/production-readiness/FINAL_PROJECT_CLOSEOUT.md` remains untouched and unstaged.
- Latest resource sample on 2026-10-12: 2.98 GiB free RAM. No container was started, stopped, or modified. Docker is available; exact PostgreSQL digest `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` was pulled and verified as linux/amd64.
- The 6-GiB container gate is not met (and RAM is below the 4-GiB high-cost build/Race gate). PostgreSQL has not been started or tested. Do not connect to other projects' databases or stop their containers.
- Latest implementation commit `ebd237d` pins Go toolchain use in the acceptance path and validates TLS SAN/key mode. `bash -n scripts/alicloud/*.sh` and `git diff --check` PASS at that HEAD. Targeted Go packages also passed at documentation checkpoint `f6a3c6e`/tree `953e083`; raw logs are in the current Evidence bundle. Full suite/Race/Vet remain unverified at this checkpoint.
- Host/Desktop upstream proxy is now configured by the user for port 7890. Host Registry request and exact digest pull passed; Docker Engine still reports the Desktop-internal proxy endpoint `http.docker.internal:3128`, which is compatible with the successful pull. Terraform init, fmt, and validate PASS; validate emitted a provider deprecation warning (`name` → `security_group_name`). No plan, apply, or destroy was performed.

## Implementation checkpoint before documentation closeout

- HEAD: `f2ab9ac758b8b457d2b2b44c5c05c199110f356b`
- Tree: `4a992ba00f50faf0b2f83e09fc65f8c356ae9c45`
- Source base: `421fd82a6f360cb9cc51625416e875d659079f6c`; 17 tracked files differ across five local commits.
- Worktree contains only the pre-existing untracked `docs/production-readiness/FINAL_PROJECT_CLOSEOUT.md`; it is intentionally not edited or staged.
- Latest snapshot: 4.73 GiB available RAM, 47.41 GiB free on E:, 255 TIME_WAIT, 23 running containers. No containers were started/stopped, no D: files changed, and no cloud account/resources were touched.
- Safety invariants remain: `PRODUCTION_NOT_APPROVED`; Production Worker fail-closed; production Allowlist empty; Migration 012 unchanged; Payment/Notification and purge disabled; no Push/Merge/Release.

## Packages

The table below is the current live status (the historical package narrative below records prior work and is not a newer status snapshot).

| ID / Priority | Dependencies | Status | Changed files / evidence | Last failure | Next action |
|---|---|---|---|---|---|
| A / P1 Terraform | Terraform CLI and pinned official provider | PARTIAL | `infra/alicloud/staging/.terraform.lock.hcl` generated for signed `aliyun/alicloud 1.274.0`; init, fmt, and validate pass. Validate warns that `alicloud_security_group.name` is deprecated in favor of `security_group_name`. No plan/apply/destroy. | Non-blocking provider deprecation warning; current RAM is 2.98 GiB, below high-cost build/Race gate. | In a resource-safe follow-up, rename the deprecated field and re-run fmt/validate; do not run plan/apply. |
| B / P0 PostgreSQL | Exact digest, clean source, >=6 GiB RAM | RESOURCE_LIMITED | Compose/preflight/deploy/acceptance at `c62cd08`, `2395960`, `ebd237d`; exact digest freshly pulled and verified linux/amd64. No DB Evidence. | Latest recorded RAM 2.98 GiB; 6-GiB gate prevents startup. | After operator-managed RAM recovery, recheck and run isolated local PostgreSQL acceptance; ECS is not required. `POSTGRESQL_LOCAL_INTEGRATION_VERIFIED = NOT CLAIMED`. |
| C / P1 OSS contract | Live OSS bucket and constrained role | NOT_VERIFIED | Fake atomic-write tests in `internal/artifacts`; earlier unit/race/vet results are historical. | No live OSS bucket/credentials or contract run. | Execute real contract only after separate OSS setup approval. |
| D / P1 Linux Compose/TLS | Linux host and run-scoped TLS | NOT_VERIFIED | `deployments/alicloud/*`, `scripts/alicloud/tls-generate.sh`; latest TLS dry-run PASS and public-IP rejection. | No Linux certificate issuance or service startup. | Run on a reviewed Linux staging host and verify SAN/mode/readiness. |
| E / P1 Multi-ECS | Level 1 PASS and approved Worker-B ECS | AWAITING_APPROVAL | Existing Worker-B Compose; no second host. | No ECS-B authorization/resource. | After Level 1 and explicit cost approval, run controlled cross-host recovery. |
| F / P1 Backup/restore | Isolated PostgreSQL and object-store recovery target | NOT_VERIFIED | Existing local backup assets retained; no new restore Evidence. | No independent live DB + OSS target. | Run actual independent restore when targets exist. |
| G / P1 Observability | Collector/receiver runtime and service path | NOT_VERIFIED | Existing OTel/alert configs retained. | No cross-process Collector/alert run. | Run local Compose verification after RAM gate; cloud only if separately approved. |
| H / P1 Supply chain | Images and trusted scanner/signing tools | NOT_VERIFIED | Digest-oriented deployment assets retained. | Scanner/signing tools unavailable in PATH; no fresh SBOM/signature. | Use an approved tool source and retain actual outputs. |
| I / P1 Evidence | Successful real test run | NOT_VERIFIED | Static evidence manifest plus targeted Go logs, with exact SHA256. | No PostgreSQL run; prior broad Go run lacks sealed source/tree. | Add DB/runtime evidence only after those tests execute. |
| J / P1 Cost/reclaim | Cloud account and explicit approval | AWAITING_APPROVAL | Cost checklist and exact-Run-ID cleanup retained; mismatched confirmation rejected before Docker operations. | No live account quote/resource inventory. | Review live quotes before any paid-resource action. |
| K / P2 Failure-path dry-run | Local safe static environment | NOT_VERIFIED | Shell syntax and TLS/cleanup guard checks PASS; Compose/Terraform formatting passed earlier. | Full cloud failure-path executor is absent. | Avoid low-value harness work; prioritize real acceptance. |

| Package | Status | Work / commits | Verification | Remaining blocker / next action |
|---|---|---|---|---|
| A — Terraform audit | PARTIAL | Two false-by-default resource/budget gates retained; SSH restricted to empty or `/32`; IPv4 VPC/subnet containment validation; distinct optional Worker-B type. `fc2f8ff` | Terraform 1.14.9 version and `fmt -check -recursive` PASS; no plan/apply/destroy | Pinned Aliyun provider download stalled; `init` incomplete and `validate` NOT RUN. Do not claim provider schema validation. |
| B — PostgreSQL-only | IMPLEMENTED_NOT_VERIFIED | Isolated Compose, exact reviewed PostgreSQL digest (cached locally, linux/amd64), Linux preflight, local Migrator build, migration/grant orchestration, DB identity/role/DDL probes, hashed evidence, scoped cleanup preserving DB volume. `c62cd08`, `2395960`, `ebd237d` | Compose static configs and shell syntax checks previously PASS; no database/container test run. Current 3.57 GiB available RAM is below 6-GiB gate. | Wait for operator-managed RAM recovery to >=6 GiB without stopping other projects, then recheck gate and run the isolated PostgreSQL-only path. A Linux ECS is not required for this local acceptance. `POSTGRESQL_LOCAL_INTEGRATION_VERIFIED = NOT CLAIMED`. |
| C — OSS adapter | PARTIAL | Added atomic fake-client concurrent same-key/same-content and different-content cases; lost response reconciliation and unknown outcome test. `2fff043` | `go test -p 1 ./internal/artifacts ./internal/config -count=1` PASS; targeted `go test -race -p 1 ./internal/artifacts -run 'TestOSSStore(ConcurrentPutIfAbsentNeverOverwrites|ClassifiesLostPutResponseAndUnverifiableOutcome)$' -count=1` PASS; `go vet -p 1 ./internal/artifacts ./internal/config` PASS | Fake models the conditional-write contract; live OSS, versioning, RAM metadata isolation and cross-host consistency NOT RUN. |
| D — Linux Compose/TLS | PARTIAL | Added non-overwriting, explicit Linux test TLS bootstrap with CA key outside mount dirs; corrected Worker-B network name and log rotation. `f2ab9ac` | TLS dry-run PASS; PostgreSQL-only, single-ECS and Worker-B Compose `config --quiet` PASS; shell syntax PASS | Certificate issuance, Linux filesystem permissions, Docker startup/readiness and failure-path checks NOT RUN on ECS. |
| E — Multi-ECS | PARTIAL | Existing Worker-B Compose reused and made less misleading; no Worker-B ECS provisioned. | Compose parse only | Controlled cross-host crash/reclaim, stale ACK and dual-API Shared Limiter/Cursor Rotation NOT RUN; second ECS requires separate user approval. |
| F — Backup/restore | PARTIAL | Existing local backup/restore artifacts retained; no new cloud restore tool. | Prior repository evidence only; no new database run | No isolated cloud PostgreSQL + OSS object restore, hash reconciliation, RTO/RPO or independent target. |
| G — Observability | PARTIAL | Existing local OTel/alert configs and E2E retained. | No container E2E this run because host RAM was below 6 GiB. | Cross-process cloud traces and delivered alert NOT RUN; no paid ARMS. |
| H — Supply chain | PARTIAL | Existing digest-oriented deploy preparation retained; no scanner added. | Terraform format only; scanner binaries not available in PATH. | No fresh image build, SBOM, vuln scan, signature/provenance or registry rollback. |
| I — Evidence | PARTIAL | PostgreSQL acceptance now records source/tree, DB/system/schema/image identity, command, times, exit code, redacted test JSON and SHA256SUMS. | Script syntax and Compose static checks PASS; no acceptance Evidence generated. | Actual DB/ECS/resource/cost evidence cannot exist until a real run; prior evidence was not modified. |
| J — Cost/reclaim | PARTIAL | Existing cost/security checklist retained; PostgreSQL cleanup is exact-Run-ID dry-run and preserves its named volume. | Cleanup script shell syntax PASS; no cleanup executed. | Actual region/account quotes and residual resource/billing inventory require user account and approval. |
| K — Dry-run | PARTIAL | Terraform formatting, TLS dry-run, and three Compose render/validation paths executed. | PASS as specified above; no services or cloud resources started. | The 20 cloud workflow failure paths are not covered by a dedicated fake executor; do not claim complete end-to-end dry-run. |

## Test ledger

- PASS: `go test -p 1 ./internal/artifacts ./internal/config -count=1` (Go toolchain selected from module requirement with `GOTOOLCHAIN=auto`).
- PASS: targeted OSS race test command recorded in Package C.
- PASS: `go vet -p 1 ./internal/artifacts ./internal/config`.
- PASS: `bash -n` for all `scripts/alicloud/*.sh`; TLS dry-run printed only non-secret paths/metadata.
- PASS: `docker compose ... config --quiet` for PostgreSQL-only, single ECS and Worker-B configs; these commands did not start services.
- PASS: Terraform 1.14.9 `fmt -check -recursive` and `terraform validate -no-color`; validate succeeded with the documented provider deprecation warning. Exact logs are in the local Evidence bundle.
- PASS at HEAD `ebd237d`: `bash -n scripts/alicloud/*.sh`; `git diff --check`; exact cached PostgreSQL image metadata inspection (linux/amd64 and matching RepoDigest); verified production migration 012 unchanged from source base. These static/read-only checks do not constitute database acceptance.
- PASS at HEAD `ebd237d`: TLS dry-run with `TASKLAB_RUN_ID=trial-20261012` and private IP `10.20.0.12` via bundled Python (no files created); public IP `8.8.8.8` rejected; cleanup with mismatched confirmation rejected before Docker operations.
- PASS at HEAD `f6a3c6e146f34a031f7da5e50d005638867d46cd`, tree `953e0839bfe4def7d2142dba08a82b8032b61fe8`, cached Go 1.26.9, `GOMAXPROCS=1`, `-p 1`: `go test ./internal/artifacts ./internal/config -count=1` and `go test ./internal/auth ./internal/api/... ./internal/domain/taskpayload -count=1`; both exit 0, raw logs sealed under `evidence/local-static-20261012/`.
- NOT RUN: Terraform plan/apply/destroy, full Go/Race suite, Docker/container runtime tests, PostgreSQL, MinIO/OSS, E2E, multi-ECS, backup/restore, cloud, scanner/SBOM/signature.
- An earlier non-PostgreSQL package-suite run returned exit 0 with environment-gated tests skipped; exact skip count and source SHA/tree were not sealed. It remains historical and is not a full-suite PASS claim.
- The new local Evidence manifest covers only targeted package and shell checks; no historical Evidence was rewritten.

## Ordered implementation commits before documentation closeout

1. `fc2f8fff3c2dbda8d7491095fc0ab9efd706821e` — `fix(terraform): tighten cloud staging safety gates`
2. `c62cd082221305ba3d6df004c3b8c5f88e656932` — `feat(cloud): add isolated postgres-only acceptance`
3. `239596069e15a05fe7b414143f371a3cbaffe0f2` — `docs(cloud): document postgres-only acceptance and cleanup`
4. `2fff04353f6e6413ec82f30f347f98d9db25dc49` — `test(oss): cover concurrent immutable writes`
5. `f2ab9ac758b8b457d2b2b44c5c05c199110f356b` — `feat(cloud): add guarded linux staging tls bootstrap`
6. `8337b764506029cf1bd2a972436208e6bc92783a` — `docs(cloud): close out overnight engineering sprint`
7. `e1d186c805aa3317c9437680840615a6d2203f37` — `docs(cloud): finalize overnight status board`
8. `ebd237d0af40376bd746051bde4e435d9006e3d1` — `fix(cloud): pin postgres acceptance toolchain`

Next safe action: preserve the 6-GiB gate and run local PostgreSQL-only acceptance after available RAM reaches the threshold. Resume Terraform validation only when the pinned provider is obtainable through a trusted path. The historical `FINAL_PROJECT_CLOSEOUT.md` remains untouched and unstaged.
