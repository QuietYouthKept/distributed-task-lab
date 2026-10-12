# P0/P1 Deep Engineering Task Board

Checkpoint source: `codex/production-readiness-p0-p1`, latest implementation
commit `e2714f36b357146249b0fbc21d226f99125d6139` (tree
`2498cab7a64ef38d0605ba6ab6bf5802ac6116f6`). Evidence and reports remain on
E:. No local full-stack E2E or production approval is claimed.

| Package | State | Delivered in this checkpoint | Remaining acceptance / gate |
|---|---|---|---|
| A — MinIO image | `MINIO_IMAGE_SOURCE_BLOCKED` | Official Docker Hub Bearer flow attempted twice, auth result captured; source/ADR reports | Need authorized official pull or approved source/license plan; no digest/manifest |
| B — PostgreSQL privileges | `OFFLINE_HARNESS_HARDENED; LIVE_DB_NOT_RUN` | SQL lexical inventory and table-level UPDATE residual-risk audit; explicit loopback+port+maintenance role+dedicated DB gate; redacted logs; local Go toolchain floor selection with no toolchain download | Real isolated DB tests; retry/renewal/DLQ exact grant coverage; narrow column permissions decision |
| C — Compose runtime | `IMPLEMENTED_STATIC; SCOPED_CLEANUP_OFFLINE_VERIFIED; E2E_NOT_VERIFIED` | Per-run topology, staged startup, exact Run-ID cleanup with fake-executor checks, bounded HTTPS/API/Worker/Gateway/MinIO task acceptance runner | Preflight remains `RESOURCE_LIMITED`/`MINIO_IMAGE_SOURCE_BLOCKED`; no API request, token, service start, Docker cleanup, or cross-process acceptance run |
| D — Identity/API security | `UNIT_VERIFIED_PARTIAL` | JWKS expiry-outage and timeout fail-closed tests; security matrix | Full API+PG owner/RBAC/audit matrix; wrong CA/expired/SAN negative cases; container OIDC |
| E — Gateway/retry | `UNIT_VERIFIED_PARTIAL` | 8-way same-ID concurrent dedupe test; unknown-outcome/fault matrices | Connection refused/reconciliation and cross-process provider guarantees |
| F — Backup/restore | `OFFLINE_MANIFEST_VERIFIED; LIVE_BACKUP_RESTORE_NOT_RUN` | Safe PostgreSQL dump preflight/verifier plus object inventory/path/hash/snapshot/restore-target validator (12/12 offline assertions) | No actual dump or object transfer; independent PostgreSQL/MinIO target and restore consistency remain |
| G — limiter/cursor | `UNIT_VERIFIED_PARTIAL` | Unit matrix for current limiter/router and rotating cursors; exact limits documented | Real shared PostgreSQL contention, outage/recovery, two API replicas/key distribution |
| H — retention | `OFFLINE_MANIFEST_FIXTURE_VERIFIED` | Read-only candidate inventory; bounded manifest compiler, external hash verification, snapshot-bound resume cursor; 12 fixture assertions including duplicate-ID, future-cutoff, and missing-history rejection | Not DB/S3 integrated; artifact bytes not verified; no archive deletion or purge |
| I — observability | `UNIT_VERIFIED_PARTIAL` | Selected observability package tests in 115 unit/race cases; cardinality/privacy boundary report | In-process OTLP receiver, cross-process collector, actual alert delivery/resolve |
| J — supply chain | `STATIC_SOURCE_AUDIT_VERIFIED; SCANNERS_NOT_RUN` | Pinned Dockerfile bases, non-root/revision label checks, secret/evidence context exclusion, scanner/tool inventory; E2E proxy base digest pinned | Mutable action tag review, Compose SOURCE_COMMIT propagation, read-only filesystem feasibility, vulnerability scan, SBOM, image build/signature/provenance |
| K — evidence automation | `SELFTEST_18_OF_18; AUDIT_HAS_10_PRESERVED_GAPS` | Guarded Unit/Race/Vet runners use compile-resource gates and `-p 1`; source inventory filters Git stderr; blocked Race now records its resource command; current matrix has 117 manifests | Five earliest dirty runs lack source inventories; three old blocked runs lack exact command; two preserved current resource-guard runs predate their exact-command fixes; all remain preserved |
| L — true system test | `RESOURCE_LIMITED + DEPENDENCY_BLOCKED` | Clean-tree Unit passed 227/227 on `ec4d0bb`; latest Race code passed 227/227 with hash-inventoried runner changes before commit `e2714f3`; latest Race retry was resource-limited; Vet remains resource-limited on clean commit; PostgreSQL container gate remains below 6 GiB | PostgreSQL-only when separately provisioned; full chain after RAM and verified object-image gates |

## Safe stop conditions

- No container start while available RAM is below 6 GiB.
- No Race or large build below 4 GiB; Unit/Race/Vet runners enforce the compile gate and limit Go package parallelism to one.
- No default-port test that may create/drop a database on an unknown service. DB scripts require an explicit loopback `tasklab_local_test_admin` URL.
- Do not stop or clean any other project's containers, network, or volumes.
- No production credentials, external provider calls, purge, cloud provisioning, push, merge, or release.
- Preserve all existing Wave 3/Wave 4/Benchmark evidence and all original failure logs.
- Startup runner preflight-only evidence is stored at `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\compose-runs\valid-project-smoke-postredaction\run-20261011t140857p0800`; it records all blockers and `executed=false`.

## V4 focused execution update (2026-10-12)

Implementation source: commit `8945bfddd2d82fc7b8882863c8530054f35fb7d5`,
tree `a7bd25e8653c84992d72c2668c265c03cb760584`.

| Area | State | Current result | Not yet verified |
|---|---|---|---|
| Replay soak (T11) | `CREATED; UNIT_TESTED` | `scripts/replay/soak.py` runs the existing authenticated API Replay Runner in uniquely identified iterations, supports 1h/4h/8h, checks Linux available memory/RSS/evidence limits, preserves interrupted output, and seals run artifacts. Six soak-specific unit tests are included in the 27-test Python suite. | No real API/Worker/Gateway/ArtifactStore run; no 1h/4h/8h soak or capacity claim. |
| PostgreSQL acceptance gate | `STATIC_VALIDATED; UNIT_TESTED` | CI and Alibaba Cloud PostgreSQL acceptance require the same 17 critical test names; missing tests, zero tests, any skip, or failed package status prevent a pass claim. Python 3.8 annotation compatibility is covered by five summarizer tests. | No current-source Go/PostgreSQL execution; remote workflow has not been triggered from this local-only increment. |

Evidence for implementation commit `8945bfddd2d82fc7b8882863c8530054f35fb7d5`
is external at
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\replay-soak-runner-20261012-8945bfd`;
its `SHA256SUMS` file has SHA256
`BF484373EAE28F67E227A303A5577F8E458DDA9A0AF206A0E2C87E0BCB36EF89`.
Verified: replay/soak Python suite 27/27, PostgreSQL test-summary gate 5/5,
Python compile, CLI help, CI YAML parse, PostgreSQL shell syntax, and
`git diff --check`. Go compilation, PostgreSQL integration, container E2E,
and long-duration soak remain `NOT_RUN` under the current resource/dependency
gates. `PRE_CLOUD_ENGINEERING_READY` is not claimed; `PRODUCTION_NOT_APPROVED`
remains unchanged.

CI branch-trigger follow-up: commit `49e136055695ed37e32b0fb15b0ef8bbc98c8379`,
tree `d2315c0004db3c4680771e583341cfd2bf1b56e7`, adds the hardening branch to
the push filters for CI, Security, and Container Supply Chain. All three
workflow YAML files parse and include the intended branch filter. No Push was
performed and no remote run was triggered. Static evidence is external at
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\workflow-trigger-20261012-49e1360`;
its `SHA256SUMS` file hash is
`f2b13e3237d89073f567b00049df77af31c9dc5cdc9fb0d9169d6f0f9dae0c48`.
