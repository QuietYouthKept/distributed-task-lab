# distributed-task-lab — Continuous Engineering Closeout

Date: 2026-10-12 (Asia/Shanghai)

## Executive disposition

This continuation performed the available local, low-resource safety work and refreshed the live task board. The project is **not fully accepted**. PostgreSQL-only integration remains unverified because available RAM stayed below the enforced 6-GiB container gate; cloud, OSS, multi-host, restore, observability, and supply-chain runtime checks also remain unverified. Keep `PRODUCTION_NOT_APPROVED`.

No database/container was started, no existing container was stopped or changed, and no cloud resource or paid service was created. The exact PostgreSQL digest is cached locally and matches `linux/amd64`, but cache presence is not a database test.

## Source and Git state

- Repository: `QuietYouthKept/distributed-task-lab`
- Branch: `codex/production-readiness-p0-p1`
- Implementation source validated by the shell checks: commit `ebd237d0af40376bd746051bde4e435d9006e3d1`, tree `301c445cdb6290eb1dafae14eb6081a61ff3ad2a`.
- Documentation checkpoint before this report: commit `9310ac53df3ee62df49b2c1c417965e54b6bc359`, tree `fe0efa5ce6df7669144344bc35a064c55041160b`.
- Targeted Go tests, Docker digest pull, and Terraform initialization ran at commit `f6a3c6e146f34a031f7da5e50d005638867d46cd`, tree `953e0839bfe4def7d2142dba08a82b8032b61fe8`; intervening tracked changes were documentation/evidence and the generated provider lock only.
- The only pre-existing untracked file, `docs/production-readiness/FINAL_PROJECT_CLOSEOUT.md`, was not read, edited, staged, hashed, or committed.
- No D: repository was changed. No Push, Merge, Release, Terraform plan/apply/destroy, or cloud account operation occurred.

## Continuous acceptance matrix

| Area | Current state | What is actually supported by evidence | Not established / next gate |
|---|---|---|---|
| PostgreSQL local integration (P0) | `RESOURCE_LIMITED`; `POSTGRESQL_LOCAL_INTEGRATION_VERIFIED = NOT CLAIMED` | Exact pinned image was freshly pulled/verified: `postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`, `linux/amd64`. Run-scoped Compose, migration, role/grant, test and cleanup scripts exist. | No PostgreSQL process/container, Migration 001–012 execution, SQL assertions, or integration tests ran. Latest RAM is 2.98 GiB < 6 GiB. After operator-managed resource recovery, recheck and run isolated local acceptance; do not use another project's database. |
| Terraform (P1) | `PARTIAL` | Via host proxy `127.0.0.1:7890`, official Registry returned HTTP 200; pinned `aliyun/alicloud 1.274.0` installed with Terraform signature verification (partner key ID `47422B4AA9FA381B`). New `.terraform.lock.hcl` records the exact version and archive hashes. `terraform init -backend=false`, `terraform fmt -check -recursive`, and `terraform validate -no-color` passed. Validate reported a non-blocking deprecation: `alicloud_security_group.name` should become `security_group_name`. | No plan/apply/destroy. Current available RAM is below the 4-GiB high-cost build/Race threshold; address the field warning and revalidate only when the host has adequate headroom. |
| Core API → Worker → Gateway → Artifact → Fenced ACK (P0) | `NOT RUN` | Staging Compose, TLS/JWT fixture, Gateway simulator and OSS adapter code are present. | No real PostgreSQL/object store or independent processes ran end-to-end. No task, artifact SHA, receipt, or fenced ACK result exists for this source checkpoint. |
| OSS contract | `NOT RUN` | OSS adapter's historical fake-client tests cover immutable/concurrent write semantics and ambiguous write outcomes. | Fake-client tests are not OSS verification. No bucket, constrained RAM role, SigV4, metadata isolation, or live Put/Get/Head test. Requires separately approved OSS setup and real contract run. |
| Worker recovery / multi-instance | `NOT RUN` | Worker-B Compose and recovery design are prepared. | No Worker A crash, lease expiry/reclaim, version increment, stale ACK rejection, dual API, or cross-host run. ECS-B requires separate explicit cost/resource approval. |
| Backup / restore | `NOT RUN` | Existing local backup artifacts remain in the repository. | No independent PostgreSQL plus object-store restore, integrity reconciliation, or RTO/RPO result. Requires two isolated live targets. |
| OTLP / alerts | `NOT RUN` | Existing collector/alert configuration remains. | No cross-process trace export or Normal→Firing→Delivered→Resolved runtime test. No paid ARMS. |
| Supply chain | `NOT RUN` | Digest-oriented deployment configuration exists. | No new image build, SBOM, vulnerability scan, Cosign signature/provenance, or rollback test. `syft`, `grype`, `trivy`, and `cosign` were unavailable in PATH. |
| Production posture | `PRODUCTION_NOT_APPROVED` | Production Worker fail-closed; production Allowlist remains empty; Migration 012 unchanged from the source base; Payment/Notification remain disabled; purge/retention deletion remains disabled. | No production identity provider, real commercial provider, formal approval, HA, operations SLO, or production recovery verification. |

## Work completed in this continuation

1. Re-checked branch, Git history, actual working tree, Docker Engine, image metadata, resource limits, and running container inventory. No other-project container was touched.
2. Confirmed the cached PostgreSQL image's ID, OS/architecture, and matching RepoDigest. This was an image metadata inspection only.
3. Reviewed the PostgreSQL-only preflight/deploy/acceptance/cleanup scripts, Linux Compose and TLS bootstrap. No production code defect was confirmed that justified a change.
4. Ran all AliCloud shell scripts through `bash -n`; ran `git diff --check` successfully.
5. Ran TLS dry-run with a private IPv4 and verified it created no files; verified a public IPv4 is rejected. Verified cleanup rejects a mismatched run confirmation before any Docker call.
6. Confirmed Migration 012 is unchanged from source base `421fd82a6f360cb9cc51625416e875d659079f6c`.
7. Updated `docs/alicloud-staging/OVERNIGHT_ENGINEERING_BOARD.md` with current status, priorities, dependencies, failures, and next actions; committed it separately.
8. Added a new local static-check Evidence bundle with a SHA256 manifest. Historical evidence was not rewritten.
9. After the user confirmed proxy port 7890, confirmed a host-side Registry request succeeds and performed one exact-digest Docker pull (exit 0; image already up to date). Docker Engine reports its Desktop proxy endpoint as `http.docker.internal:3128`; this internal address did not prevent the pull.
10. Completed Terraform init after discovering the first read-only-lock failure was due to a missing local lock file, not a download failure. The provider was signature-verified and `.terraform.lock.hcl` was generated for the already pinned 1.274.0. `terraform validate -no-color` then exited 0, with the documented field deprecation warning.

## Test ledger for this continuation

| Check | Result | Scope and limitation |
|---|---|---|
| `bash -n scripts/alicloud/*.sh` | PASS | Shell syntax only. |
| `git diff --check` | PASS | Whitespace check only. |
| TLS generator, private test IP, `--dry-run` | PASS | No certificates or files created. |
| TLS generator, public IP `8.8.8.8` | PASS negative test | Rejected with non-zero exit as expected. |
| PostgreSQL cleanup, mismatched confirmation | PASS negative test | Rejected before Docker operations. |
| Docker image inspect for the exact pinned PostgreSQL reference | PASS metadata check | `linux/amd64` and matching RepoDigest; no container start. |
| Docker exact-digest pull | PASS, exit 0 | Fixed digest reported and image up to date; no container started. |
| Migration 012 source comparison | PASS | No change from stated source base. |
| Targeted Go unit tests | PASS | At HEAD `f6a3c6e146f34a031f7da5e50d005638867d46cd`, tree `953e0839bfe4def7d2142dba08a82b8032b61fe8`, cached Go 1.26.9, `GOMAXPROCS=1`, `-p 1`: `./internal/artifacts ./internal/config` and `./internal/auth ./internal/api/... ./internal/domain/taskpayload`, both exit 0. Raw logs and SHA256 are in the Evidence bundle. |
| Terraform initialization | PASS | `terraform init -backend=false` via proxy `127.0.0.1:7890`; signed provider 1.274.0 installed and exact lock file created. Earlier `-lockfile=readonly` attempt exited 1 because the lock file did not yet exist; full logs are preserved. |
| Terraform format | PASS | Terraform 1.14.9 `fmt -check -recursive` exit 0. |
| Go Race / full suite / Vet at this checkpoint | NOT RUN | These were not run after resource review; earlier results remain historical and are not rebound to this source/tree. |
| PostgreSQL / Compose runtime / E2E / OSS / recovery / restore / OTLP | NOT RUN | RAM gate and/or external service/account requirements not met. No Skip is counted as PASS. |
| Terraform `validate` | PASS WITH WARNING | Exit 0; provider schema validation succeeded. Warning: rename deprecated security-group `name` field to `security_group_name`. No cloud operations were run. |

Evidence bundle: [`evidence/local-static-20261012/README.md`](evidence/local-static-20261012/README.md) and [`evidence/local-static-20261012/SHA256SUMS`](evidence/local-static-20261012/SHA256SUMS). The manifest verifies the README, test/pull/init logs, acceptance scripts, and Terraform lock file; hashes are recorded in the manifest.

## Resource and external gates

- First snapshot: 3.57 GiB available RAM, E: 47.16 GiB free, 334 TIME_WAIT, 24 unrelated running containers.
- Later snapshot: 5.16 GiB available RAM, E: 47.16 GiB free, 234 TIME_WAIT, 28 running containers.
- After targeted Go tests: 4.63 GiB available RAM, E: 47.16 GiB free, TIME_WAIT 369, 25 running containers. After Terraform initialization: 3.63 GiB available RAM, E: 46.59 GiB free, 26 running containers. Latest read-only sample: 2.98 GiB available RAM.
- Latest snapshot is below the 6-GiB container gate and 4-GiB high-cost build/Race gate. No global Windows network setting changed; no unrelated service was stopped.
- Host proxy port 7890 accepted the Registry request and exact Docker pull exited 0. Terraform provider installation and schema validation succeeded through the configured path; no Terraform plan/apply/destroy was run.
- No Alibaba Cloud account credentials, ECS, OSS bucket, ACR, paid observability, or billing data were used. Actual cost is therefore **¥0 spent by this continuation**; this does not estimate future account charges.

## Local commit history relevant to this work

1. `fc2f8fff3c2dbda8d7491095fc0ab9efd706821e` — `fix(terraform): tighten cloud staging safety gates`
2. `c62cd082221305ba3d6df004c3b8c5f88e656932` — `feat(cloud): add isolated postgres-only acceptance`
3. `239596069e15a05fe7b414143f371a3cbaffe0f2` — `docs(cloud): document postgres-only acceptance and cleanup`
4. `2fff04353f6e6413ec82f30f347f98d9db25dc49` — `test(oss): cover concurrent immutable writes`
5. `f2ab9ac758b8b457d2b2b44c5c05c199110f356b` — `feat(cloud): add guarded linux staging tls bootstrap`
6. `8337b764506029cf1bd2a972436208e6bc92783a` — `docs(cloud): close out overnight engineering sprint`
7. `e1d186c805aa3317c9437680840615a6d2203f37` — `docs(cloud): finalize overnight status board`
8. `ebd237d0af40376bd746051bde4e435d9006e3d1` — `fix(cloud): pin postgres acceptance toolchain`
9. `9310ac53df3ee62df49b2c1c417965e54b6bc359` — `docs(cloud): refresh continuous engineering status`

This closeout itself is a documentation/evidence commit; the final post-commit HEAD is available from Git history rather than embedded recursively in this file.

## Minimum next actions

1. Have the operator safely reduce unrelated host load by their own choice; do not stop services on their behalf. Once available RAM is at least 6 GiB, repeat the resource guard and run the already prepared isolated local PostgreSQL-only acceptance against only its unique Compose project and volume.
2. After RAM is safely above the low-resource threshold, replace the deprecated `name` field with `security_group_name`, then run `terraform fmt -check -recursive` and `terraform validate` with the existing E-drive provider data/cache and exact lock file; no new provider source/version or cloud call is needed.
3. Only after PostgreSQL passes, request explicit approval for OSS and any paid ECS/account resources needed for the next level. Keep the first cloud phase single ECS and short-lived.

Until those gates pass, report `LOCAL_PARTIAL`, `POSTGRESQL_LOCAL_INTEGRATION_VERIFIED = NOT CLAIMED`, cloud items `NOT RUN`, and `PRODUCTION_NOT_APPROVED`.
