# Overnight Cloud Engineering Sprint — Closeout

Date: 2026-10-11 (Asia/Shanghai)

## Executive result

Local preparation advanced through bounded, offline work. The highest-value deliverable is now a PostgreSQL-only Linux staging path that does not depend on OSS, ACR, OIDC, Gateway, or a second ECS. OSS fake-client concurrency coverage, an explicit TLS dry-run/bootstrap path, and run-scoped cleanup were also added.

This is not a cloud acceptance. No Alibaba Cloud account, credentials, ECS, OSS bucket, registry, billing API, or paid resource was used. No PostgreSQL container or other container was started. `CLOUD_POSTGRESQL_VERIFIED`, `CLOUD_CORE_E2E_VERIFIED`, and `CLOUD_DISTRIBUTED_RECOVERY_VERIFIED` remain `NOT RUN`; overall state remains `PRODUCTION_NOT_APPROVED`.

## Git checkpoint

- Repository: `QuietYouthKept/distributed-task-lab`
- Branch: `codex/production-readiness-p0-p1`
- Latest implementation commit before this closeout document: `f2ab9ac758b8b457d2b2b44c5c05c199110f356b`
- Tree at that implementation checkpoint: `4a992ba00f50faf0b2f83e09fc65f8c356ae9c45`
- That checkpoint contains 17 tracked-file changes from source base `421fd82a6f360cb9cc51625416e875d659079f6c` across these local commits:
  - `fc2f8fff3c2dbda8d7491095fc0ab9efd706821e` — Terraform safety gates and audit.
  - `c62cd082221305ba3d6df004c3b8c5f88e656932` — PostgreSQL-only Compose/preflight/deploy/acceptance.
  - `239596069e15a05fe7b414143f371a3cbaffe0f2` — PostgreSQL-only runbook and scoped cleanup.
  - `2fff04353f6e6413ec82f30f347f98d9db25dc49` — OSS immutable-write fake-client concurrency tests.
  - `f2ab9ac758b8b457d2b2b44c5c05c199110f356b` — Linux TLS bootstrap and Worker-B Compose hygiene.
- The existing untracked `docs/production-readiness/FINAL_PROJECT_CLOSEOUT.md` was not read into this report, modified, staged, or committed. It remains present and untracked. No D: repository was modified.
- The following closeout/report files are not included in the 17-file count: this file and the updated `OVERNIGHT_ENGINEERING_BOARD.md`.
- No Push, Merge, Release, Terraform plan/apply/destroy, or cloud resource operation occurred.

## Packages A–K

| Package | Result | What changed / evidence | Open gap |
|---|---|---|---|
| A — Terraform audit | PARTIAL | Kept both resource gates false by default; SSH ingress restricted to empty or IPv4 `/32`; validates IPv4 VPC/subnet containment with bounded subnet enumeration; optional Worker-B now has a separate explicit instance type and remains disabled by default. Review: `TERRAFORM_SECURITY_REVIEW.md`. Terraform 1.14.9 ran `version` and `fmt -check -recursive` successfully. | Provider `aliyun/alicloud` 1.274.0 installation stalled. `terraform validate` could not run without that provider. No plan or cloud API call. `TERRAFORM_STATIC_VERIFIED = PARTIAL`. |
| B — PostgreSQL-only | IMPLEMENTED_NOT_VERIFIED | Added `compose.postgres-only.yml`, exact PostgreSQL 16 Alpine digest checks, Linux resource/TLS/secret/run/project preflight, local Migrator image build, migration and grants steps, DB identity/schema/role/DML/DDL assertions, bounded database test invocation, redacted test JSON summary and SHA256SUMS, and run-ID-scoped cleanup that preserves the named volume. Added direct run instructions to the deployment guide and acceptance matrix. | No Linux ECS or PostgreSQL was available/started. `POSTGRESQL_CLOUD_ACCEPTANCE_READY = PARTIAL`; `CLOUD_POSTGRESQL_VERIFIED = NOT RUN`. |
| C — OSS | PARTIAL | Existing adapter retained; fake atomic object client now covers concurrent identical writes, conflicting concurrent payloads, write-committed/response-lost reconciliation, and an unobservable write result. | Fake behavior is not OSS evidence. Live versioning, real concurrent consistency, RAM metadata isolation, and cloud contract remain NOT RUN. `CLOUD_OSS_CONTRACT_VERIFIED = NOT RUN`. |
| D — Linux Compose/TLS | PARTIAL | Added a default-dry-run TLS generator with explicit Run ID confirmation, short-lived service certificates with expected file names/SAN intent, no-overwrite checks, restrictive key modes, and CA key outside mounted TLS paths. Worker-B bridge was renamed from misleading `host`, with log rotation. | Only dry-run/static parsing was executed; no certificate issuance, Linux ownership/mode verification, or Compose service startup. |
| E — Multi-ECS | PARTIAL | Existing Worker-B design retained; Compose network naming/log rotation improved. | No second ECS, controlled worker barrier, DB-clock expiry observation, reclaim, stale ACK, duplicate effect, or dual API run. Requires separate ECS purchase/approval. |
| F — Backup/restore | PARTIAL | Existing local PostgreSQL backup/restore and manifests were preserved. | This sprint did not build or execute an independent cloud PostgreSQL+OSS restore. No RTO/RPO result. |
| G — Observability | PARTIAL | Existing local OTLP and alert configurations were preserved. | No Collector/Receiver container was started; no cross-process cloud trace or delivered alert. Paid ARMS remains disabled. |
| H — Supply chain | PARTIAL | Existing digest-oriented deployment model retained. Terraform CLI came from a fixed official release and its archive hash had already been verified in the Terraform audit. | No new image build, SBOM, vulnerability scan, signature/provenance, or ACR rollback. `syft`, `grype`, `trivy`, and `cosign` were not available on PATH. |
| I — Evidence | PARTIAL | PostgreSQL acceptance script now records source/tree, image/database/schema identity, test command, timestamps, exit code, redacted JSON output, and hashes only after log close. | No real PostgreSQL/ECS evidence directory or manifest was generated. Historical Evidence was not changed. |
| J — Cost/reclaim | PARTIAL | PostgreSQL Compose cleanup defaults to dry-run, checks exact Compose project ownership, and preserves the data volume. Existing cost/security checklist remains the operator inventory basis. | No current account quote or billing export; stopped disks/snapshots/VPC/OSS/registry residual costs remain operator checks. No cleanup action was executed. |
| K — dry-run | PARTIAL | Ran TLS dry-run, Terraform format check, and three Compose config renders without service startup. | No 20-path fake cloud executor or full deploy/restore/cleanup failure simulation. Static render is not deployment acceptance. |

## Test and validation record

All results below are local and bound to the worktree code state during this sprint. They do not imply cloud execution.

| Category | Result | Exact command / scope |
|---|---|---|
| Go unit | PASS | `GOTOOLCHAIN=auto go test -p 1 ./internal/artifacts ./internal/config -count=1` |
| Go Race | PASS | `GOTOOLCHAIN=auto go test -race -p 1 ./internal/artifacts -run 'TestOSSStore(ConcurrentPutIfAbsentNeverOverwrites|ClassifiesLostPutResponseAndUnverifiableOutcome)$' -count=1` |
| Go vet | PASS | `GOTOOLCHAIN=auto go vet -p 1 ./internal/artifacts ./internal/config` |
| Shell syntax | PASS | `bash -n` for `scripts/alicloud/*.sh`, including PostgreSQL-only cleanup and TLS bootstrap |
| TLS behavior | PASS, dry-run only | `TASKLAB_RUN_ID=dryrun-20261011 scripts/alicloud/tls-generate.sh --dry-run --api-private-ip 10.20.0.12`; no files or keys were generated |
| Compose | PASS, static only | `docker compose ... config --quiet` for `compose.postgres-only.yml`, `compose.single.yml`, and `compose.worker-b.yml`; these validate/render and did not start services |
| Terraform | PARTIAL | `terraform version` = 1.14.9; `terraform fmt -check -recursive` PASS; `terraform validate` unavailable because the pinned provider package was not installed |
| Full Go/Race and PostgreSQL | NOT RUN | No full suite or database integration executed during this sprint |
| Docker E2E / MinIO / OSS / Cloud | NOT RUN | RAM gate requires >=6 GiB before container acceptance; no cloud resources were touched |
| Scanner/SBOM/signature | NOT RUN | Tools unavailable or not installed; no unverified tool download attempted |

`git diff --check` passed before each implementation commit. Test output was not promoted to a new Evidence manifest because this local session did not produce a sealed run package. No prior evidence manifest was overwritten.

## Security and resource invariants

- `PRODUCTION_NOT_APPROVED` remains unchanged; production Worker fail-closed and production task Allowlist remains empty.
- Only the staging simulation profile is intended for `eval/gateway`; local OIDC/JWKS and Gateway remain simulated.
- Migration 012 was not modified. Payment/Notification providers were not enabled. Purge/retention deletion remains disabled.
- No AccessKey, password, JWT, private key, or cloud credential was added to the repository or logs.
- No cloud resources were created and no paid service was used.
- No D: repository was edited. No other project container was stopped, removed, or changed. Docker Compose was used only for config rendering; it did not start containers.
- Start/end snapshots varied with host activity. The final recorded snapshot before Race was approximately 4.73 GiB available RAM, 47.41 GiB E: free, TIME_WAIT 255, and 23 running Docker containers. The container acceptance gate is still 6 GiB; it was not lowered.

## Cloud readiness statuses

| Status | Value | Basis |
|---|---|---|
| `TERRAFORM_STATIC_VERIFIED` | PARTIAL | Formatting/source gates complete; provider install/schema validation incomplete |
| `CLOUD_POSTGRESQL_ACCEPTANCE_READY` | PARTIAL | Run-scoped isolated entry exists; no live database run |
| `CLOUD_OSS_CONTRACT_READY` | PARTIAL | Adapter/fake tests exist; live service and IAM isolation not proven |
| `CLOUD_CORE_E2E_READY` | PARTIAL | Existing single ECS Compose path remains dependent on OSS, credentials, certificates and role-boundary proof |
| `CLOUD_MULTI_ECS_READY` | PARTIAL | Worker-B Compose exists; no second-host execution or cross-host recovery evidence |
| `CLOUD_BACKUP_RESTORE_READY` | PARTIAL | Local backup components exist; cloud restore workflow and independent target remain unverified |
| `CLOUD_OBSERVABILITY_READY` | PARTIAL | Local assets exist; cloud telemetry/alert delivery not run |
| `CLOUD_POSTGRESQL_VERIFIED` | NOT RUN | No ECS/PostgreSQL execution |
| `CLOUD_CORE_E2E_VERIFIED` | NOT RUN | No API-to-Worker-to-Gateway-to-OSS execution |
| `CLOUD_DISTRIBUTED_RECOVERY_VERIFIED` | NOT RUN | No two-ECS execution |
| `PRODUCTION_NOT_APPROVED` | TRUE | No production approval was sought or granted |

## Cost scenarios and safe operating sequence

Exact prices are intentionally not quoted: region, zone, instance stock, system disk type/size, public IPv4/bandwidth, image egress, snapshots, OSS storage/requests/egress, taxes, and billing minimums must be checked in the live account calculator immediately before each stage. Stopping an ECS may leave disks, snapshots, IPs, VPC/security resources, buckets and registry images billable.

1. **PostgreSQL-only (one ECS):** choose the smallest account-available x86_64 Linux ECS that meets the script's 6-GiB available-RAM gate. Use a short bounded runtime; exit after migration/role/integration evidence. This path needs no OSS, ACR, Gateway or second ECS.
2. **Single-ECS Core E2E:** only after PostgreSQL acceptance and separate budget approval. Add pre-created private OSS bucket and run-prefix-only Worker RAM role; prove non-Worker containers cannot retrieve metadata credentials, or keep OSS acceptance blocked/move Worker to ECS-B. Then deploy digest-pinned API/Worker/Gateway/identity simulator and execute one `eval/gateway` task.
3. **Multi-ECS recovery:** separately approve and create the lower-cost Worker-B type only after Level 1 passes. Run one bounded cross-host Worker crash/reclaim/stale-ACK test, collect evidence, and stop the test quickly.
4. **Optional cloud operations:** backup/restore, OTLP/alert, scanning, ACR and rollback are individually costed and approved; do not enable RDS, ACK, SLB, NAT or ARMS by default.

### Operator's shortest next path after acquiring one ECS

1. In the Alibaba Cloud console, choose a region/zone and x86_64 Ubuntu image, import an SSH public key, configure SSH ingress only from the operator's current public IP `/32`, and keep PostgreSQL/API/Gateway absent from public ingress. Review live pricing and disk/IP/snapshot charges.
2. Separately review and approve VPC/ECS creation. No approval in this sprint authorizes an Apply or cloud change.
3. SSH to the ECS private/admin endpoint and install Docker Engine/Compose from an operator-approved package source. Clone the exact reviewed branch commit; check `git status` is clean and verify `git rev-parse HEAD`.
4. Create a unique Run ID and owner-only `/etc/tasklab/secrets/<run-id>` files; stage the PostgreSQL certificate materials using `tls-generate.sh --dry-run`, review the paths/SANs, then explicitly execute it on the Linux ECS. Keep the CA key outside mounts and Evidence.
5. Populate `postgres-only.env` with only non-secret run configuration and the exact pinned PostgreSQL digest. Put passwords and role-specific database URLs in separate mode-0600 secret files. Verify the acceptance DSN targets only this ECS private IP and uses `verify-full` with the run CA.
6. Run `postgres-only-preflight.sh`, then `postgres-only-deploy.sh`, then `postgres-acceptance.sh`, all with the same `TASKLAB_RUN_ID` and `--confirm-run-id`. Inspect `SHA256SUMS` and the redacted run Evidence before proceeding.
7. Configure OSS only after the PostgreSQL checkpoint: manually create a private bucket with versioning disabled and TLS; attach a least-privilege prefix-scoped RAM role to Worker only; prove metadata credential isolation. Then separately prepare/pin Gateway, API, Worker, and identity simulator images and test authentication-negative and illegal-mode rejection before one permitted task.
8. Export/review Evidence, capture actual usage/billing, and run a residual-resource inventory. First use PostgreSQL-only cleanup in dry-run; if appropriate, authorize the exact Run ID to remove only its containers/network (the database volume remains). Release ECS, disks, public IP, VPC/Security Group, snapshots, OSS and registry resources only through a separately reviewed, explicit account-console/Terraform plan and operator approval. Reconcile the billing page afterward.

Manual approval is required for every cloud resource creation, OSS bucket/RAM change, image push, ECS-B, deletion/release, Terraform Apply/Destroy, or paid observability/registry operation. `terraform validate`, Compose config, local fake clients and this guide do not perform those actions.

## Final disposition

The project is better prepared for an inexpensive, staged acceptance, with PostgreSQL-only as the next concrete milestone. The blocking items are now explicit: install the pinned Terraform provider safely, use a real qualifying Linux ECS and obtain PostgreSQL evidence, prove the ECS metadata/OSS role boundary before Core E2E, then request separate approval for any additional paid stage. Do not call the current state production-ready.
