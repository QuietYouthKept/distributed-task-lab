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
