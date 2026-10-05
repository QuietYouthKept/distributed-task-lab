# Loadgen report: workers-4-queues-4-payload-1k

- Profile: `backlog-drain`; tasks: 1000; workers: 4 x 1; queues: 4; payload: 1024 bytes.
- Throughput: 80.22 tasks/s; processing: 12.465s; submission: 1.038s.
- Claim latency: p50 13.403ms, p95 15.348ms, p99 20.088ms.
- Queue wait: p50 7197.833ms, p95 11105.074ms, p99 11591.388ms.
- End-to-end completion: p50 7206.213ms, p95 11115.698ms, p99 11601.435ms.
- Correctness: success=1000, DLQ=0, retries=0, reclaims=0, duplicate claims=0, submission errors=0.

This is one point in a curve, not a capacity claim. Compare matrix runs only when their manifest parameters and PostgreSQL durability settings match. `raw.csv` contains every claim, queue-wait, and end-to-end sample.
