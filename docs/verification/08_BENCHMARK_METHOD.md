# Benchmark Method

`cmd/loadgen` runs a bounded in-process Go worker pool against the PostgreSQL queue/repository. It does not run the HTTP API. The default `noop` task uses `handlers.NewMockHandler`; non-noop modes can use deterministic execution or a configured Gateway. Therefore every report must state task type and handler mode, and Gateway/provider figures must not be compared with noop scheduler measurements.

Current source computes end-to-end throughput as terminal DB tasks divided by wall time from just before producer submission until all queued tasks are terminal. `SubmitSeconds` and post-submit `ProcessingSeconds` remain separate. Failure and submission error counts remain visible. E2E latency is read from DB `created_at` to `finished_at`; it is not request HTTP latency.

Each newly generated manifest records `git_sha`, `git_tree_sha`, and `git_dirty`; unknown metadata is represented as unknown/null instead of silently clean. Raw samples are emitted before report creation. Do not compare runs with different durability, workload profile, DB version, handler mode or hardware.

The existing four-worker historical matrix uses a legacy denominator and is documented separately in `03_HISTORICAL_EVIDENCE_AUDIT.md`. The corrected code has focused unit tests but was not run against PostgreSQL in this turn.
