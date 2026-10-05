# Loadgen report: workers-16-queues-4-payload-1k

- Profile: `backlog-drain`; tasks: 1000; workers: 16 x 1; queues: 4; payload: 1024 bytes.
- Throughput: 159.06 tasks/s; processing: 6.287s; submission: 1.039s.
- Claim latency: p50 36.447ms, p95 47.336ms, p99 52.172ms.
- Queue wait: p50 3829.234ms, p95 6283.329ms, p99 6528.588ms.
- End-to-end completion: p50 3838.996ms, p95 6305.841ms, p99 6535.781ms.
- Correctness: success=1000, DLQ=0, retries=0, reclaims=0, duplicate claims=0, submission errors=0.

This is one point in a curve, not a capacity claim. Compare matrix runs only when their manifest parameters and PostgreSQL durability settings match. `raw.csv` contains every claim, queue-wait, and end-to-end sample.
