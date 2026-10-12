# Continuous Engineering V2 Closeout — Local Pre-Cloud Status

**Disposition: `LOCAL_PRE_CLOUD_PARTIAL`; `PRE_CLOUD_ENGINEERING_READY` is not claimed; `PRODUCTION_NOT_APPROVED` remains.** This closes the current local work package, not all external or runtime acceptance.

## Source and Git

- Repository: `QuietYouthKept/distributed-task-lab`.
- Branch: `codex/production-readiness-p0-p1`.
- Implementation commit reviewed: `0a4220cf93cf7bf16034bc37a3f3d5fd29f17176`.
- Implementation tree: `b3884ba497a3665487adad6b984305a55b068704`.
- That commit adds bounded Azure LLM replay tooling, source-bound benchmark evidence metadata/throughput correction, dataset and historical evidence audits, and test-acceptance matrices. The report and a resource-gate note are added in a following local documentation commit. The protected untracked `docs/production-readiness/FINAL_PROJECT_CLOSEOUT.md` is preserved and excluded.
- No push, merge, release, cloud Apply/Destroy, paid resource creation, external database, or other-project container change occurred.

## Implementation and correctness invariants

- `scripts/replay/azure_llm.py` streams the local Azure LLM Code/Conversation CSV schema into bounded deterministic JSONL. It records provenance distinctions and SHA256, rejects invalid/unordered inputs, and publishes without overwriting an existing destination. This creates workload descriptions only; it does not invoke TaskLab or a model/provider.
- `cmd/loadgen` now records Git commit, tree and dirty/unknown state. Throughput is terminal DB tasks divided by elapsed wall time from producer submission start to final terminal state; submission duration and post-submission drain duration are separate. Old benchmark values keep their old denominator and are explicitly historical.
- Supported task writes use DB-time lease/retry predicates and lease-version fencing; Worker execution remains at-least-once. Idempotent provider commit or effect reconciliation is not equivalent to exactly-once external side effects.
- A confirmed security limitation remains: Worker table-level UPDATE privilege allows a holder of the shared DB credential to bypass application-layer fencing by direct SQL. No migration 012 edit or unsafe runtime grant change was made.
- Production Allowlist remains empty; Payment/Notification and formal Purge remain disabled.

## Verification matrix

| Area | Result for this package |
|---|---|
| Python replay unit tests | PASS: 7/7 on Python 3.8.3. Evidence: `docs/verification/evidence/azure-llm-replay-20261012-run02/`. |
| Replay generation | PASS as generation only: 1,000 records / 576,197 bytes, source and output hashed; `GENERATED_NOT_EXECUTED`. 10k/100k generation logs are separate artifacts and likewise do not represent executed tasks. |
| Loadgen integrity Go unit test | PASS on dirty source snapshot with Go 1.26.9: `go test -p 1 ./cmd/loadgen -count=1 -timeout=2m`. Evidence and source file hashes in `docs/verification/evidence/loadgen-integrity-20261012-run02/`. |
| Loadgen vet | PASS: `go vet ./cmd/loadgen`; empty diagnostics and sealed log. |
| Initial loadgen test attempt | FAIL before compiling: local default Go 1.26.6 conflicted with module requirement Go >=1.26.9. Preserved in run01; retry used command-local Go 1.26.9. |
| Current full Go suite | NOT RUN: an immediate pre-run sample was 3.81 GiB available, below the existing 4 GiB compile gate. No test started. Evidence: `docs/verification/evidence/go-unit-current-20261012-run01/`. |
| PostgreSQL integration / Race / current Failure Lab / current Chaos | NOT RUN. Earlier 0a5482d historical runs cannot be relabeled to this commit. |
| Docker/Compose, API E2E, MinIO/S3, real OSS, cloud, backup/restore, OTLP/alert, load/stress/soak | NOT RUN. No containers or cloud resources were started for this package. |
| Formatting and evidence | `gofmt` applied to changed Go files; staged `git diff --check` passed before the implementation commit; evidence SHA256SUMS revalidated. |

## Historical measurements and data

- The retained historical 1/4/8/16 worker matrix ran 1,000 tasks per point against PostgreSQL on an in-process Go pool, using noop/mock execution. It reported 39.8119/80.2218/160.3302/159.0648 tasks/s using a legacy denominator that excludes submission time. It is not current API RPS, real Gateway speed, or a claim of 16 independent processes. Raw-file SHA256 values and full parameters are in `docs/verification/03_HISTORICAL_EVIDENCE_AUDIT.md`.
- Historical Failure Lab 12/12 refers to the exact old evidence commit and its database-oracle scenarios; it does not satisfy all 12 stricter V3 workload scenarios. Current scenario states are in `docs/verification/05_DISTRIBUTED_12_SCENARIOS.md`.
- Azure LLM Code and Conversation audits cover 8,819 and 19,366 records respectively. Azure Functions time units conflict across source descriptions; Alibaba batch-task rows include invalid/missing values and inspected source did not establish a data license. Detailed provenance and unverified archives are in `docs/verification/01_AZURE_LLM_DATA_QUALITY.md` and `02_DATASET_INVENTORY.md`.

## Remaining acceptance work

1. Obtain a trusted isolated PostgreSQL 16 runtime or authorization to push the reviewed branch and run the prepared remote PostgreSQL workflow. Current `POSTGRESQL_LOCAL_INTEGRATION_VERIFIED = NOT CLAIMED`.
2. Close the Worker database privilege boundary with a forward-only, Migration-012-compatible design and a real direct-SQL negative test.
3. Run current-source DB-backed lease/fencing/retry/effect/limiter tests, then API→Worker→Gateway→Artifact→ACK E2E and crash/reclaim.
4. Run independent DB/object restore, two-host recovery, cross-process OTel/alerts, current performance/load and bounded soak only with approved isolated resources.
5. Extend workload replay with checkpoint/resume and additional adapters after data provenance/unit/licensing questions are resolved.

Exact current status and external approvals are in `docs/verification/06_GENERAL_18_CATEGORY_MATRIX.md`, `10_SECURITY_GAPS.md`, `11_CLOUD_ACCEPTANCE.md`, and `13_REMAINING_EXTERNAL_BLOCKERS.md`. Resume-safe claims are in `12_RESUME_METRICS.md`; do not claim cloud production acceptance, exactly-once external effects, current TPS/p95/p99, multi-host recovery, RPO/RTO, or production approval.
