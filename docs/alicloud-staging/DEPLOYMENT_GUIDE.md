# Alibaba Cloud staging-sim deployment preparation

Status: code-prepared, not cloud-verified. No ECS, VPC, OSS bucket, RAM role, or paid service has been created. `PRODUCTION_NOT_APPROVED` remains in force.

This guide provisions one Ubuntu 24.04 x86_64 ECS first. A second ECS is a separate, short-lived phase for cross-host Worker recovery. PostgreSQL remains on the core ECS private network; API is reached through an SSH tunnel; the Gateway Simulator is reachable only from the run-owned VPC. This is a bounded staging simulation, not a production deployment or a managed HA architecture.

## Source and safety gates

Keep the Worktree on the approved E: location. Before each run record `git rev-parse HEAD`, `git rev-parse 'HEAD^{tree}'`, and `git status --porcelain`. Build images from that exact source tree, label them with the source revision, push them to a private ACR Personal Edition namespace only after a separate manual approval, resolve each image to an immutable `repository@sha256:...` reference, and put only those references in the run-scoped Compose environment file. Never deploy `latest`.

Production `ValidateWorker` remains fail-closed. The cloud flag is accepted only with `APP_PROFILE=staging-sim`; only the existing `eval/gateway` simulation policy is intended. The fixture OIDC issuer, Gateway Simulator, and performance numbers are simulated. No real provider, hosted identity provider, or production SLO is verified.

Run `scripts/alicloud/preflight.sh`, then `scripts/alicloud/validate.sh`. These perform read-only host/resource and Compose checks. The first requires at least 6 GiB available RAM and 5 GiB free evidence space on the Linux host. This is a Linux-host check, not a reuse of the Windows RAM snapshot. It intentionally does not pull or start containers.

Terraform uses two independent false-by-default gates: `enable_resources` and `budget_approved`. First run `terraform fmt -check`, `terraform init -backend=false`, `terraform validate`, and `terraform plan -out=review.tfplan` from `infra/alicloud/staging`. Terraform plan does not calculate a trustworthy account bill: separately use the live regional pricing calculator and record ECS, disk, public IPv4/bandwidth, OSS, ACR, snapshots, transfer and tax before approval. Plan is a review only; do not apply until account-specific cost, region, instance type, public access, and resource ownership have been reviewed and separately approved. Never put credentials in `.tfvars` or Terraform state. The sample state is local; remote state is not part of this low-cost candidate.

## Manual Alibaba Cloud preparation

The operator must select a region and zone with an available x86_64 4-vCPU/16-GiB instance, select the exact Ubuntu 24.04 x86_64 image ID, import an SSH public key, and set SSH ingress to their current public IPv4 `/32`. Keep public API, PostgreSQL, and Gateway ingress absent. Confirm disk, public IPv4/bandwidth, image egress, snapshots, OSS storage/requests/egress, ACR, taxes, and billing minimums in the account console before approving a budget.

After the VPC/ECS plan is approved and applied by the operator in a later phase, create a narrowly scoped RAM role for the Worker ECS. Its OSS policy must name only the pre-created bucket and the run prefix; permit only the operations used by the adapter: bucket metadata/versioning read, object `PutObject`, `GetObject`, `HeadObject`, and `DeleteObject` on the run prefix. The role must not create buckets or manage lifecycle/versioning. Attach the role to each Worker ECS that needs artifact access. The adapter refuses a bucket with versioning enabled because OSS may ignore `x-oss-forbid-overwrite` in that mode. No AccessKey is required in containers.

Create the bucket separately with public access blocked, TLS-only transport, versioning disabled for the immutable-write contract, and server-side encryption selected per account policy. Retention/lifecycle deletion stays disabled for this acceptance. Record region, bucket owner, policy revision, and bucket settings without recording credentials.

## Mode 1 — PostgreSQL-only acceptance

This path is independent of OSS, MinIO, ACR, hosted OIDC, Gateway, API/Worker services, and ECS-B. It uses the reviewed `postgres:16-alpine` digest, a run-owned PostgreSQL Compose project, a local Migrator image build, and the existing PostgreSQL integration package. It still requires a Linux x86_64 ECS with Docker Compose v2, Go 1.26.9 or newer installed from an operator-approved official source, at least 6 GiB available RAM and 5 GiB free under `/var/lib`, a private IPv4 assigned to the host, and a clean committed source checkout. Verify the installed Go toolchain and its official checksum before the run; the acceptance script uses `GOTOOLCHAIN=local` and will not download or silently replace it. It is not a cloud PASS until the test actually runs against that ECS database.

Generate the Linux-host test certificates without overwriting an existing run directory. The first command is informational only; `--execute` is an explicit host mutation and should be run only after the source, run ID, and private address have been reviewed:

```sh
TASKLAB_RUN_ID=<run-id> scripts/alicloud/tls-generate.sh --dry-run --api-private-ip <ecs-private-ip>
sudo env TASKLAB_RUN_ID=<run-id> scripts/alicloud/tls-generate.sh --execute --confirm-run-id <run-id> --api-private-ip <ecs-private-ip>
```

The CA private key is outside all mounted TLS directories. The generator emits API (`api.crt`/`api.key`), Identity (`identity.crt`/`identity.key`), and PostgreSQL (`server.crt`/`server.key`) leaves and only mounts CA certificates to consumers. Keep `/etc/tasklab/private/<run-id>` off Evidence and do not copy it into a container.

Provision mode-specific one-time passwords and URL files under `/etc/tasklab/secrets/<run-id>` with owner-only permissions. The `postgres-acceptance-dsn` must use the exact private ECS IPv4, the disposable superuser, database `/postgres`, `sslmode=verify-full`, and that run's `/etc/tasklab/tls/<run-id>/postgres/ca.crt`. Create `postgres-only.env` at the exact same run secret directory with mode `0600`; set `TASKLAB_RUN_ID`, `TASKLAB_SOURCE_SHA`, `TASKLAB_SECRET_DIR`, `TASKLAB_PG_PRIVATE_IP`, `TASKLAB_PG_TLS_DIR`, `TASKLAB_EVIDENCE_DIR`, and the exact reviewed `TASKLAB_POSTGRES_IMAGE` digest. Never put secrets in shell arguments or Evidence.

Run the bounded path in order:

```sh
TASKLAB_RUN_ID=<run-id> scripts/alicloud/postgres-only-preflight.sh --confirm-run-id <run-id>
TASKLAB_RUN_ID=<run-id> scripts/alicloud/postgres-only-deploy.sh --confirm-run-id <run-id>
TASKLAB_RUN_ID=<run-id> scripts/alicloud/postgres-acceptance.sh --confirm-run-id <run-id>
```

The preflight is read-only and checks clean source identity, host resources, certificate SANs, private database target, the exact PostgreSQL digest, and that the run's Compose project/volume do not already exist. Deploy pulls only that digest, builds Migrator locally, starts only PostgreSQL, then runs migrations and runtime grants. Acceptance checks schema version 12, image and database identity, three distinct restricted roles, expected DML grants, and API/Worker DDL denial, then runs the PostgreSQL integration package with `REQUIRE_TEST_DATABASE=1`. The MinIO-dependent `TestDatabaseBackupRestoreDrill` is excluded; any other skipped integration test fails the run. The evidence path is `/var/lib/tasklab/evidence/<run-id>/postgres-only` and includes redacted test JSON, SQL/version metadata, exit status, exact command, source/tree SHAs, and SHA256SUMS.

Container cleanup is a dry-run by default and preserves the named database volume. Review the exact run-owned container IDs before setting the approval variable:

```sh
TASKLAB_RUN_ID=<run-id> scripts/alicloud/postgres-only-cleanup.sh --confirm-run-id <run-id>
TASKLAB_RUN_ID=<run-id> TASKLAB_CLEANUP_APPROVED=<run-id> scripts/alicloud/postgres-only-cleanup.sh --confirm-run-id <run-id>
```

The cleanup helper never deletes the PostgreSQL data volume or any ECS, disk, VPC, OSS, or registry resource. Those require separate inventory and explicit operator action.

## Bootstrap and TLS

Create unique `/etc/tasklab/secrets/<run-id>`, `/etc/tasklab/tls/<run-id>`, and `/var/lib/tasklab/evidence/<run-id>` directories with owner-only permissions. Provision the exact files named in `compose.single.yml` and `compose.worker-b.yml`; never print values. Generate a short-lived private test CA and certificates on the ECS host with SANs for `identity`, `localhost`, `127.0.0.1`, `postgres`, and the core ECS private IP as applicable. PostgreSQL clients use `sslmode=verify-full&sslrootcert=/run/secrets/pg_root_ca`; the Worker-B DSN must use the PostgreSQL private IP/DNS name present in the server certificate. The API certificate must include `DNS:localhost` so local SSH-tunnel clients can validate it. The identity certificate must include `DNS:identity`. These are staging test certificates, not public-Internet server credentials.

The database URL files must be distinct role-specific URLs (`tasklab_migrator`, `tasklab_api`, `tasklab_worker`) and point at the private PostgreSQL endpoint. The migration job alone receives the migrator URL. Runtime API and Worker accounts must not be superusers or table owners. The setup script reads passwords from mounted files and does not expose them in shell arguments.

Cross-platform audit: the existing `deployments/local-first/*.ps1` scripts and hard-coded E: defaults remain Windows-local tooling; the new Compose mounts use repository-relative paths and are intended for Linux after cloning. The existing local test certificates contain only local SANs and must not be reused as cloud test certificates. The cloud Compose expects Linux paths and generated run-scoped TLS material. Docker Desktop's Windows RAM/CPU snapshot is not evidence for the ECS host gate.

The ECS-attached RAM role is host-scoped. Before enabling it on a single ECS that runs API and Worker, verify from inside the API, identity, PostgreSQL and Gateway containers that the ECS metadata endpoint cannot be reached; only the Worker should be able to acquire the constrained role credentials. If the OS/container network cannot enforce and prove this boundary, do not run OSS acceptance on that shared host—use ECS-B for the Worker or add an explicitly reviewed credential-isolation mechanism. Compose network isolation alone is not proof of per-container role isolation.

## Build and run on ECS-A

Install a supported Docker Engine/Compose v2 on Ubuntu using the operator-approved package source. Clone the exact reviewed source revision into `/opt/tasklab/src/<run-id>`. Build the API, Worker, Migrator, Gateway Simulator, and identity fixture for `linux/amd64`; create an SBOM and run the approved vulnerability scanner before publishing. Publish to the approved private registry and resolve digests. ACR setup and image push are manual approval gates; this preparation does not authenticate or push.

Create a mode-0600 `TASKLAB_COMPOSE_ENV` with run ID, private bind address, run-scoped secret/TLS directories, pre-provisioned OSS endpoint/region/bucket/RAM role, and digest-pinned images. Keep environment files out of Git and Evidence. Then run:

```sh
TASKLAB_RUN_ID=<run-id> scripts/alicloud/deploy.sh --confirm-run-id <run-id>
TASKLAB_RUN_ID=<run-id> scripts/alicloud/health.sh --confirm-run-id <run-id>
```

Deployment pulls only the explicitly pinned images; starts PostgreSQL, identity and Gateway; waits for the one-shot migration and runtime-grant jobs; then starts API and Worker. It never calls Terraform or creates Alibaba Cloud resources. Inspect `docker compose ps` and health before testing.

On the operator workstation, create an SSH tunnel to ECS-A that forwards local `18443` to the ECS loopback-bound API TLS port. Call only `https://localhost:18443`; provide the run-scoped test CA and a short-lived fixture JWT. Test API no-token and forbidden-role cases before creating the one legal `eval/gateway` task. Do not save JWTs in shell history or Evidence. Confirm task state, event sequence, attempt, lease version and artifact SHA256 via the API and a read-only SQL query. The local OIDC fixture remains test-only.

## ECS-B and recovery phase

Only after Level 1 passes and a separate cost approval, add the optional second ECS in the same private VPC/security group and run `compose.worker-b.yml` with its own secret directory, same run ID, the private PostgreSQL/Gateway endpoints, Worker-B database URL with CA verification, and the same run prefix. Confirm both Worker IDs in the database. Perform one controlled Worker-A interruption using an explicitly approved Compose stop; observe lease expiry, Worker-B reclaim, incremented fencing version, rejection of stale completion, and final single committed task result. Do not use host-wide process kills or stop unrelated services.

## Backup, restore, evidence and cleanup

PostgreSQL backup must use an explicit destination under the run evidence root, be created with a bounded timeout, and be restored only into a separate run-owned database/host. OSS objects must be copied to a separate pre-created recovery bucket/prefix using a reviewed official OSS transfer tool and the same constrained RAM role; record object keys, sizes and SHA256, then read the copied objects back and recompute SHA256. This repository does not yet contain a tested OSS backup/restore automation, so do not claim restore acceptance until that procedure is implemented and actually run.

Use `scripts/alicloud/evidence-export.sh --confirm-run-id <run-id>` only after tests. It records source revision/tree, dirty state, runtime metadata, Compose state, logs and a SHA256 manifest. Inspect and redact logs before external sharing; secret directory contents, tokens, database URLs, AccessKeys and private keys must never enter Evidence.

`scripts/alicloud/cleanup.sh --confirm-run-id <run-id>` defaults to a dry-run. Setting `TASKLAB_CLEANUP_APPROVED=<run-id>` permits removal of this Compose project's containers/networks only; it preserves the PostgreSQL named volume. ECS, disks, snapshots, VPC, vSwitch, security groups, OSS buckets and registry images require a separate inventory and explicit console/operator cleanup. TTL tags are reminders, not automatic expiry.

Exact next order after an ECS purchase: (1) freeze source SHA/tree; (2) confirm actual account pricing and region; (3) import SSH key and prepare RAM role/bucket manually; (4) review Terraform plan; (5) receive separate approval and create only ECS-A/VPC; (6) bootstrap TLS/secrets; (7) build, scan, publish and record image digests; (8) run read-only preflight/Compose validation; (9) run migration/role/PostgreSQL acceptance; (10) run one API-to-Worker-to-Gateway-to-OSS task; (11) export and review Evidence; (12) separately approve ECS-B and recovery tests; (13) backup/restore and observability acceptance; (14) inventory residual resources and explicitly clean up.
