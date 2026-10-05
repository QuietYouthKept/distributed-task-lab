# Loadgen report: workers-1-queues-4-payload-1k

- Profile: `backlog-drain`; tasks: 1000; workers: 1 x 1; queues: 4; payload: 1024 bytes.
- Throughput: 39.81 tasks/s; processing: 25.118s; submission: 1.044s.
- Claim latency: p50 11.612ms, p95 13.389ms, p99 14.151ms.
- Queue wait: p50 13089.712ms, p95 21857.898ms, p99 22812.583ms.
- End-to-end completion: p50 13097.974ms, p95 21866.708ms, p99 22820.355ms.
- Correctness: success=1000, DLQ=0, retries=0, reclaims=0, duplicate claims=0, submission errors=0.

This is one point in a curve, not a capacity claim. Compare matrix runs only when their manifest parameters and PostgreSQL durability settings match. `raw.csv` contains every claim, queue-wait, and end-to-end sample.
