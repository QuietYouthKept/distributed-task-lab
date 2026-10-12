# Alibaba Cloud staging-sim acceptance matrix

All rows begin `NOT RUN`. A test plan, Compose config, mock test, `SKIP`, or local MinIO result is not a cloud PASS. Save source SHA/tree, exact command, UTC start/end, exit code, redacted logs, DB/API assertions, resource/image identity and SHA256SUMS per run. `PRODUCTION_NOT_APPROVED` is invariant.

## Mode 1 — PostgreSQL-only readiness

Local preparation status: `POSTGRESQL_CLOUD_ACCEPTANCE_READY = PARTIAL`; no ECS database has been run. The only permitted PostgreSQL image is `postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea`.

| Check | Preconditions and exact action | PASS criteria | Evidence | Current result |
|---|---|---|---|---|
| TLS bootstrap | `tls-generate.sh --dry-run`; after operator review, explicit `--execute --confirm-run-id` on the Linux ECS | No overwrite; required SANs/filenames; leaf keys mode 0600; CA key outside mounted dirs | Run ID, certificate public metadata only; never private keys | Dry-run PASS locally; actual cert generation NOT RUN |
| Host/run preflight | Clean reviewed checkout, exact digest, private IP assigned to host, owner-only secrets, valid PG TLS, >=6 GiB RAM and >=5 GiB evidence space; run `postgres-only-preflight.sh` | Exits 0 without pulling/starting; refuses existing run project/volume | Source SHA/tree, host resources, sanitized config checks | Static path implemented; ECS preflight NOT RUN |
| Migration and roles | Run `postgres-only-deploy.sh`, then `postgres-acceptance.sh`; only the PostgreSQL-only Compose file | Migration 001–012 and rerun; API/Worker/Migrator distinct non-admin; DDL denied; real integration suite has no unintentional skips | DB identity, schema, roles/grants, redacted Go JSON, exit code, SHA256SUMS | NOT RUN; no ECS or database evidence |
| Scoped cleanup | First run `postgres-only-cleanup.sh` without approval, inspect list, then exact run-ID approval if desired | Only selected Compose project containers/network removed; named data volume kept | Container ownership labels and preserved volume name | Static review only; NOT RUN against Docker |

## Level 1 — single ECS

| Area | Preconditions and exact action | Assertions / PASS criteria | Evidence | Known limitations |
|---|---|---|---|---|
| Image and host preflight | Ubuntu x86_64; >=6 GiB available RAM; >=5 GiB evidence free; digest-pinned linux/amd64 images; `scripts/alicloud/preflight.sh` and `validate.sh` | Both succeed; every image reference is `name@sha256:<64 hex>`; no public app/DB ingress | Resource snapshot, Compose rendered validation exit code, image inspect/digest | Static validation only, not a cloud runtime check |
| Migration/roles | Isolated new PostgreSQL volume; start `postgres`, then `db-migrator`, then `db-runtime-grants` in that order | Schema reaches migration 012; migrator/API/Worker are distinct non-superuser roles; runtime `CREATE TABLE` denied; granted DML works; no skipped assertions | `schema_migrations`, `pg_roles`, SQL privilege probe output, job exit codes, DB version | Single-node PostgreSQL on ECS, no managed DB/HA |
| API → Worker → Gateway → OSS | Fixture OIDC TLS/JWKS; valid role; RAM role; pre-created OSS bucket with versioning disabled; create one eval/gateway task through TLS API | Task completes once; gateway called; artifact `Head`/`Get` SHA256 and content match; database has fenced ACK and expected event; task mode outside staging allowlist rejected | Request IDs, task/events, SQL state, artifact hash metadata and downloaded bytes hash, Worker/API/Gateway logs | Gateway and OIDC are simulations; OSS real contract remains unverified until run |
| Auth negative | No token, expired token, unknown role and invalid signature requests | Unauthorized/forbidden; no task row or side effect created | Redacted HTTP status/body and before/after SQL count | Test signing issuer is not a hosted IdP |
| Lease/Fencing | Use two Workers only after second ECS phase, or a controlled one-host dual-worker test that is explicitly labeled single-host; interrupt worker after claim | Lease expires; one reclaim; version increments; stale ACK fails; exactly one terminal state/effect | DB task/event rows and logs from both worker IDs | Not cross-host unless Worker-B ECS is actually used |
| PostgreSQL backup/restore | Create run-owned dump; restore to independent run-owned target | Task, event, effect and audit counts/IDs and selected rows match; runtime roles remain least privilege | Backup file SHA, restore log, SQL comparison, elapsed times | Local single-region restore only; not RPO/RTO or regional DR |

## Level 2 — multi-ECS

| Area | Preconditions and exact action | Assertions / PASS criteria | Evidence | Known limitations |
|---|---|---|---|---|
| Cross-host claim | ECS-B attached to same VPC/SG, Worker-B URL verifies PostgreSQL CA; run two Workers against shared queue | Concurrent unique claims; no duplicate active ownership; DB lease/fence values authoritative | Host IDs, task/event SQL, connection TLS evidence | Only two temporary ECS nodes |
| Crash/reclaim/stale ACK | Submit one slow `eval/gateway`; stop only Worker-A service at the claimed-task barrier; wait for DB-clock lease expiry; allow B to reclaim; resume A | B claim increments lease version; stale A ACK rejected; final result committed once; artifact SHA valid | Exact stop/restart events, task history, DB rows, hashes, logs | Timing and capacity bounded to a single scenario; simulated provider |
| Shared limiter | Two separately configured API instances with the same PostgreSQL limiter and hash key; issue a bounded burst from each | Combined allowance follows one shared bucket; fail-closed on DB outage; no plaintext principal key stored | SQL bucket state, HTTP 429/Retry-After, API instance IDs | Existing Compose does not yet wire a second API/OIDC routing on ECS-B; implementation/precondition gap, do not mark PASS |
| Cursor rotation | API instances share active/previous key ring; issue with old then new key across instances | During overlap old cursor accepted, after accept-until rejected; new cursor valid across instances | Config checksum, requests and statuses | Compose currently has only single API cursor secret; rotation rollout not wired |

## Level 3 — Cloud operations

| Area | Preconditions and exact action | Assertions / PASS criteria | Evidence | Known limitations |
|---|---|---|---|---|
| OSS contract | Worker RAM role has bucket/prefix-only permissions; versioning disabled; run `go test ./internal/artifacts -run '^TestOSSCloudContract$' -count=1` with explicit contract env on ECS | Put/Get/Head/Delete; SHA-256 recomputed; same-key same-content idempotent; conflicting and concurrent different-content writes cannot overwrite; missing object maps correctly | SDK operation results, object hashes, role ARN/name, bucket settings, test log | Must run live in selected account/region; test harness status pending |
| OSS recovery | Separate pre-created recovery bucket/prefix; copy only run prefix with reviewed OSS tool; read back and hash | All listed objects restore and task references resolve; hashes match; no unapproved deletion | Source/target manifests and SHA256, copy logs, API references | Automated script not implemented/verified yet |
| ECS RAM credential boundary | Single-ECS role attached; test metadata access from each Compose container; compare with Worker | Only Worker can obtain role credentials; other containers denied | Redacted per-container probe output and host firewall rules | Compose network isolation alone is not proof; if not enforceable, move Worker to ECS-B before OSS use |
| OTel/alert | Collector and receiver endpoints reachable from private network; use current instrumentation and alert config | Trace links API→DB/Worker→Gateway; Normal→Firing→Delivered→Resolved | OTLP collector and receiver logs plus trace IDs | Cloud observability Compose/profile not yet added |
| Supply chain/rollback | Private registry; build exact source SHA, SBOM and vulnerability results; pin current and previous image digest | Runtime uses digest; previous digest can be redeployed; no `latest`; scan results preserved | SBOM, scanner logs, inspect digest, rollback run | ACR account, credentials and registry policy not configured here |
| Cost/cleanup | Account cost review and full tag/resource inventory; stop/destroy only after explicit approval | All resources are RunID-tagged; residual ECS/disk/snapshot/VPC/OSS/ACR costs reconciled | Console inventory and billing export | TTL tag does not expire resources; no auto destroy |

## Status snapshot

- Local config and OSS adapter unit tests: PASS on Windows with Go toolchain selected from the module directive (`GOTOOLCHAIN=auto`): `go test ./internal/config ./internal/artifacts -count=1` (both packages passed). API/Worker wiring compile smoke: PASS with `go test ./cmd/worker ./cmd/api -run '^$' -count=1` (no package tests exist in those command packages). The opt-in live OSS contract harness remains `IMPLEMENTED_NOT_VERIFIED`; live cloud test is `NOT RUN`.
- Compose static parse/interpolation: PASS with synthetic digest-pinned values; no services started.
- Linux shell syntax: PASS with Git Bash `bash -n`.
- Terraform validate: NOT RUN; Terraform CLI is not installed on this host.
- Full Go, Race, integration, and cloud tests: NOT RUN. The package-level unit tests above were run at 4.60 GiB available RAM; container gate remains 6 GiB, so no containers were started.
- PostgreSQL, OSS, API E2E, multi-ECS, backup/restore, OTel and cost: `NOT RUN`.
- Cloud acceptance status: `NOT CLAIMED`.
