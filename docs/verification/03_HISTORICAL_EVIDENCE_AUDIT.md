# Historical Evidence Audit

审计时间：2026-10-12。本文区分历史基线与当前工作树结果；历史证据不因当前源代码是其后代而自动适用于当前 SHA。

## Source lineage

- Current branch: `codex/production-readiness-p0-p1`.
- Current committed HEAD: `d6fd35ca80fc692eb40bf99ac23196bd6200b089`.
- Current committed tree: `9d9610e2706c6545047303ca74ee227cf966faf0`.
- The evidence implementation SHA `0a5482d657ae431751c205abdf01632ebe89cee6` is an ancestor of HEAD, but `git diff --name-status 0a5482d..HEAD` shows changes to CI, API/auth/config/task payload, PostgreSQL/effects/worker/artifact code and Migration 012. Consequently none of its PASS results is a current-HEAD acceptance.
- Current uncommitted worktree additions (dataset audit/replay and verification docs) are not included in the committed tree above. Their own test evidence is bound by the per-run manifests and hashes in `evidence/azure-llm-replay-20261012*`.

## Verifiable historical PostgreSQL/Wave 3 run

External evidence directory: `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave3-artifacts\final-closeout-20261010-0a5482d\evidence\wave3-final-0a5482d-20261010`.

Its `manifest.json` binds 14 checks to implementation commit `0a5482d657ae431751c205abdf01632ebe89cee6`, records Windows 11, Go 1.26.9, PostgreSQL 16.15, and a local PostgreSQL container. The manifest reports all 14 checks PASS with exit code 0. The listed 26 stdout/stderr log SHA256 values were independently recomputed and matched. Six separately listed chaos/pagination artifacts were also hash-checked. The run is real historical evidence for that exact source/configuration snapshot only.

| Historical check | Exact command / result | Scope limitation |
|---|---|---|
| Unit + integration | `go test ./cmd/... ./internal/... ./pkg/... -count=1`, PASS | PostgreSQL-backed tests used that run's local DB; not current HEAD. |
| Race | `go test -race ./cmd/... ./internal/... ./pkg/... -count=1`, PASS | Exact 0a snapshot only. |
| PostgreSQL race repeat | `go test -race ./internal/storage/postgres -count=5`, PASS | Exact 0a snapshot only. |
| Retry backoff race | `go test -race ./internal/storage/postgres -run ^TestRetryBackoff$ -count=20 -v`, PASS | Exact 0a snapshot only. |
| Failure Lab | `go run ./cmd/failurelab`, PASS; 12 DB-oracle scenarios | Deterministic correctness demo, not 12 independent OS processes or production-scale load. |
| Chaos Matrix | `go run ./cmd/chaosmatrix`, PASS | Historical run parameters and artifacts are in the manifest; no current rerun. |
| Other gates | 14/14 manifest checks PASS | Read exact manifest/logs before reusing any narrower claim. |

Separate historical cross-container evidence at `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave3-artifacts\final-closeout-20261010-0a5482d\evidence\cross-container\manifest.json` records four passing cases (MinIO response loss; API/two-Worker shared-artifact recovery; Worker PUT response-loss recovery; two-worker same-artifact race). The tested source snapshot is documented as 0a; a runner-source commit field is not the same as the tested implementation SHA. This evidence does not establish current-worktree E2E.

The repository also contains an earlier 14-gate run under `evidence/phase1/runs/wave3-d3c5310-full-14-gate/manifest.json`, bound to `d3c5310910b942ad7232a287a62ba550625cc855`. It is a distinct historical run, not an additional validation of 0a or current HEAD.

Hosted CI documentation refers to a later docs-only descendant and historical GitHub runs. Since current workflow files have changed, those remote green runs do not prove current workflow behavior.

## Historical load benchmark matrix

Four existing result sets under `benchmarks/workers-{1,4,8,16}-queues-4-payload-1k` bind to source `cd3696e7141527b01b2b0361f2674729855203d1`, Go 1.26.6, Windows/amd64, 24 logical CPUs, PostgreSQL 16.15. Each ran 1,000 1-KiB tasks, four queues, four producers, backlog-drain; worker concurrency was one. All reported 1,000 success, zero DLQ/submission errors/retries/reclaims/duplicate claims.

| Workers | Historical throughput (tasks/s) | Processing window (s) | Submit (s) | E2E p95 (ms) | raw.csv SHA256 |
|---:|---:|---:|---:|---:|---|
| 1 | 39.8119 | 25.1181 | 1.0439 | 21866.708 | `10bdadef4d25c6c0ca6df617f27228487870419a097e404bd34f41d9d5ecb586` |
| 4 | 80.2218 | 12.4654 | 1.0382 | 11115.698 | `859b902e61f325d5c387b277dfa83b80a31885bdd71aad08b7d13c87ebeb8dc5` |
| 8 | 160.3302 | 6.2371 | 1.0251 | 6224.983 | `781f5e765ad93219f7e50c7f02af618a7ec76f01f771edade134a5d7dfafad66` |
| 16 | 159.0648 | 6.2867 | 1.0392 | 6305.841 | `82b37a409cb120b82e85ae0cb1b3be5f64629ddb8894f0af597a54a702b5ffb1` |

These are retained historical numbers. They do not establish current code performance, a stable scaling curve, API RPS, real Gateway/provider speed, or 100 independent Workers. `ProcessingSeconds` excludes enqueue; the manifest and raw CSVs should be consulted before comparison. Raw SHA256 values above were recomputed during this audit.

An older README once stated “593 tasks/s” for an in-process run (500 successful tasks, four worker pools × concurrency four). The code/docs later removed the number; no exact raw samples/timing evidence were found for it. Treat that statement as `NOT_VERIFIED`, not as a resume metric.

## Current-worktree verification from this audit

The Azure LLM replay converter was tested in the dirty worktree, not committed source: 7/7 Python tests passed on Python 3.8.3, and a 1,000-record deterministic replay was generated outside Git. The corresponding run02 manifest binds source hashes and output hash. Generated tasks have status `GENERATED_NOT_EXECUTED`; no API, Worker, Gateway, S3, or PostgreSQL task execution occurred in this turn.

No PostgreSQL/Docker integration, Go full suite, Race, Failure Lab, Chaos Matrix, E2E, or load benchmark was run against current HEAD in this audit. The latest resource snapshot found 4.26 GiB available RAM and 29 running containers, none owned by this TaskLab worktree; no containers were started or stopped.
