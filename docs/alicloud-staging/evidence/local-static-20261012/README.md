# Local static and safety-check evidence

Run date: 2026-10-12 (Asia/Shanghai)

## Source identity

- Code source tested: commit `ebd237d0af40376bd746051bde4e435d9006e3d1`, tree `301c445cdb6290eb1dafae14eb6081a61ff3ad2a`.
- Targeted Go tests, Docker pull, Terraform init, and Terraform validation ran at commit `f6a3c6e146f34a031f7da5e50d005638867d46cd`, tree `953e0839bfe4def7d2142dba08a82b8032b61fe8`; intervening tracked changes were documentation/evidence and the generated provider lock only.
- The first board/evidence checkpoint was commit `9310ac53df3ee62df49b2c1c417965e54b6bc359`, tree `fe0efa5ce6df7669144344bc35a064c55041160b`.
- At execution time only the engineering board was modified; the scripts under test matched the committed code source. The pre-existing untracked `docs/production-readiness/FINAL_PROJECT_CLOSEOUT.md` was not read, modified, staged, or hashed.
- This includes targeted local package tests, static/safety checks, image pull, and Terraform initialization. It does not establish database, container-runtime, OSS, cloud, or production acceptance.

## Commands and observed results

1. `bash -n scripts/alicloud/*.sh` — exit 0; all AliCloud shell scripts parsed.
2. `git diff --check` — exit 0; no whitespace errors.
3. `TASKLAB_RUN_ID=trial-20261012 scripts/alicloud/tls-generate.sh --dry-run --api-private-ip 10.20.0.12` — exit 0. Output confirmed dry-run mode and intended SANs; no files/certificates were created.
4. Same TLS dry-run with `--api-private-ip 8.8.8.8` — exit 1; public IP rejected by the private-address validation.
5. `TASKLAB_RUN_ID=trial-20261012 scripts/alicloud/postgres-only-cleanup.sh --confirm-run-id wrong-run` — exit 1 at the confirmation guard, before invoking Docker.
6. `docker image inspect postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea --format '{{.Id}} {{.Os}}/{{.Architecture}} {{json .RepoDigests}}'` — returned image ID `sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`, platform `linux/amd64`, and matching repository digest. This inspected the cache only; it did not start a container.
7. Migration 012 comparison against source base `421fd82a6f360cb9cc51625416e875d659079f6c` — unchanged.
8. `curl.exe --proxy http://127.0.0.1:7890 --silent --show-error --head --location --max-time 15 https://registry.terraform.io/` — exit 0; CONNECT established and Registry returned HTTP 200. Docker Engine reports its Desktop-managed internal proxy endpoint as `http.docker.internal:3128`; the exact pinned pull below succeeded after the user changed the host/Desktop upstream proxy to port 7890.
9. `docker pull --platform linux/amd64 postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea` — exit 0; digest matched and image was up to date.
10. `terraform init -backend=false -lockfile=readonly` — exit 1 after signature-verified provider install because no lock file existed and write was explicitly disallowed.
11. `terraform init -backend=false` — exit 0; generated `.terraform.lock.hcl` for `aliyun/alicloud` version `1.274.0`, signed by HashiCorp partner key ID `47422B4AA9FA381B`.
12. Terraform 1.14.9 `fmt -check -recursive` — exit 0.
13. `terraform validate -no-color` — exit 0; provider schema validation succeeded with a deprecation warning that `alicloud_security_group.name` should migrate to `security_group_name`. No cloud plan/apply/destroy was run. The warning is recorded as a follow-up, not suppressed.

Relevant tested-file SHA256 values (hexadecimal, lowercase):

- `scripts/alicloud/postgres-only-preflight.sh`: `cc67cb6c09139ecee0df618764a8fd090d2c409b92883abba6d82ee85ea084d7`
- `scripts/alicloud/postgres-acceptance.sh`: `b5ee64790715987553740709a2bc2ebbe7203431b5cd248e8c81a621721a59ca`
- `scripts/alicloud/tls-generate.sh`: `216561267b5c1ab372fa25cdd03e6f7e6e430005350bb754a0fc6712cb957db9`
- `go-test-artifacts-config.log`: `ea5ea1161d36b0b8bc238e62195e7c5a5158f263c119c7b6ee6ab8713e51d700`
- `go-test-auth-api-payload.log`: `81b5d7e1c29cd6efe33af1b3a794769d6c62282f66751d0af496d22c0c45543b`
- `postgres-digest-pull.log`, `terraform-init-readonly.log`, `terraform-init-success.log`, `terraform-validate.log`, and `infra/alicloud/staging/.terraform.lock.hcl` are also covered by `SHA256SUMS`.

## Targeted Go package tests

At HEAD `f6a3c6e146f34a031f7da5e50d005638867d46cd`, tree `953e0839bfe4def7d2142dba08a82b8032b61fe8`, the cached Go 1.26.9 toolchain was selected with `GOTOOLCHAIN=auto`, `GOMAXPROCS=1`, and package parallelism 1:

- `go test -p 1 ./internal/artifacts ./internal/config -count=1` — exit 0; `artifacts` 1.045s, `config` 0.306s.
- `go test -p 1 ./internal/auth ./internal/api/... ./internal/domain/taskpayload -count=1` — exit 0; `auth` 1.043s, `api` 0.896s, `api/contract` 0.328s, `api/handlers` 0.747s, `taskpayload` 0.406s.

These are targeted package tests only. The standard output does not enumerate test-level skips; no PostgreSQL, Race, or full-suite acceptance is claimed.

## Resource gate and disposition

- First snapshot: 3.57 GiB available RAM, E: free 47.16 GiB, TIME_WAIT 334, 24 running Docker containers.
- Later snapshot: 5.16 GiB available RAM, E: free 47.16 GiB, TIME_WAIT 234, 28 running Docker containers.
- After targeted Go tests: 4.63 GiB available RAM, E: free 47.16 GiB, TIME_WAIT 369, 25 running containers. After Terraform init: 3.63 GiB available RAM, E: free 46.59 GiB, 26 running containers. Latest read-only sample on 2026-10-12: 2.98 GiB available RAM; the 6-GiB database gate remains closed. No other-project container was touched.
- PostgreSQL container gate: at least 6 GiB available RAM. The gate was not met at either snapshot; no database container was started and no other-project container was touched.
- The two short-lived negative-test logs were created under the Git Bash `/tmp` mapping on C: and removed immediately after verifying their exact paths. No temporary test file remains from those checks.

## Evidence limitations

- No PostgreSQL SQL/test logs, DB assertions, or test-count manifest exist because PostgreSQL was not started.
- Provider installation succeeded through the configured proxy and init generated the lock file. Terraform schema validation ran and passed with the deprecation warning above. The current resource snapshot is below the 4-GiB high-cost build/Race threshold and the 6-GiB container gate.
- The two targeted Go runs above are bound to their exact HEAD/tree and raw output logs. Older broad Go runs remain historical and are not represented as current PASS evidence.
