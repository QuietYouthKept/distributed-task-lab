# Benchmark methodology

Repository microbenchmarks and system load tests answer different questions.

- `BenchmarkEnqueueSequential/Parallel`: repository enqueue cost/contention.
- idempotency conflict: unique-key conflict path.
- same/multiple queue claim: queue-row serialization versus parallel queues.
- lifecycle: enqueue, claim, fenced terminal write.
- loadgen: end-to-end worker throughput and latency under a declared profile.

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
