# Loadgen report: workers-8-queues-4-payload-1k

- Profile: `backlog-drain`; tasks: 1000; workers: 8 x 1; queues: 4; payload: 1024 bytes.
- Throughput: 160.33 tasks/s; processing: 6.237s; submission: 1.025s.
- Claim latency: p50 17.453ms, p95 33.842ms, p99 35.404ms.
- Queue wait: p50 3718.107ms, p95 6214.985ms, p99 6410.500ms.
- End-to-end completion: p50 3727.224ms, p95 6224.983ms, p99 6421.637ms.
- Correctness: success=1000, DLQ=0, retries=0, reclaims=0, duplicate claims=0, submission errors=0.

This is one point in a curve, not a capacity claim. Compare matrix runs only when their manifest parameters and PostgreSQL durability settings match. `raw.csv` contains every claim, queue-wait, and end-to-end sample.
