# Benchmark methodology

Repository microbenchmarks and system load tests answer different questions.

- `BenchmarkEnqueueSequential/Parallel`: repository enqueue cost/contention.
- idempotency conflict: unique-key conflict path.
- same/multiple queue claim: queue-row serialization versus parallel queues.
- lifecycle: enqueue, claim, fenced terminal write.
- loadgen: end-to-end worker throughput and latency under a declared profile.

The current loadgen reports `Throughput` as terminal tasks divided by elapsed
wall time from submission start through the last terminal state. It records
`SubmitSeconds`, `ProcessingSeconds` (the post-submission drain window), and
`EndToEndSeconds` separately; failed enqueue attempts are not counted as
terminal work. Older manifests predate this corrected denominator and their
throughput uses the configured task count divided by only the post-submission
drain window (so it can overstate full-run throughput and include failed
enqueue attempts); the historical scope is documented in
`docs/verification/03_HISTORICAL_EVIDENCE_AUDIT.md`.

The recorded historical worker matrix uses the in-process Go worker pool and
PostgreSQL repository/queue directly, with the default mock handler for `noop`;
it does not traverse HTTP API, OIDC, Gateway, or ArtifactStore. Its figures are
scheduler-kernel backlog-drain measurements, not end-to-end service capacity
or provider performance.

Always report PostgreSQL/Go version, CPU/storage, pool settings, task count,
queue configuration, payload bytes, producer/worker concurrency, seed, warm-up,
duration, and whether durability settings changed. Never compare numbers from
different correctness settings. Report all runs, including errors/reclaims/DLQ;
do not select the best run.

## Evidence bundle and fixed matrix

`make loadgen` writes `benchmarks/<run_id>/manifest.json`, `raw.csv`,
`postgres-stats.json`, `summary.json`, and `report.md`. The run directory is
created once and never overwritten. `raw.csv` includes claim, queue-wait, and
end-to-end samples; the manifest records the Git SHA, Go/PostgreSQL versions,
machine OS/architecture/CPU count, and workload parameters.

Use `make benchmark-matrix` for the initial worker scaling curve: 1, 4, 8, and
16 workers; four queues; 1 KiB payload; 1,000 backlog-drain tasks. Commit the
generated evidence directories only after running on the stated benchmark
machine. The harness intentionally does not invent a throughput result when a
PostgreSQL/Docker environment is absent.

## AI workload matrix

`LOADGEN_TASK_TYPE=noop|llm_eval|strategy_replay` separates scheduler-kernel
results from AI-handler results. Set `LOADGEN_HANDLER_MODE=deterministic` for
artifact and fingerprint overhead, or `gateway` only with a controlled real
gateway. `LOADGEN_CASES_PER_TASK` bounds shard size and
`LOADGEN_PROVIDER_CONCURRENCY` maps to the in-process provider gate.

The report records queue wait, handler latency, fenced-ack latency,
end-to-end latency, provider semaphore wait, provider duration, and estimated
artifact-write time. Do not compare Gateway tasks/s to scheduler noop tasks/s.
