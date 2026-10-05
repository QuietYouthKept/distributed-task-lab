# Database capacity and query analysis

Run these against a non-production copy at each intended task cardinality
(100k and 1m are the initial checkpoints). Keep the matching loadgen evidence
bundle beside the captured plans.

```sql
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM tasks
WHERE queue = 'llm-eval' AND status = 'pending' AND next_run_at <= clock_timestamp()
ORDER BY priority DESC, created_at ASC LIMIT 20 FOR UPDATE SKIP LOCKED;

EXPLAIN (ANALYZE, BUFFERS) UPDATE tasks SET lease_expires_at = clock_timestamp() + interval '90 seconds'
WHERE id = 'example' AND status = 'running' AND leased_by = 'worker' AND lease_version = 1;
```

Inspect `pg_stat_activity.wait_event`, `pg_locks`, `pg_stat_database`, pool wait
metrics, WAL growth, and plans for claim, heartbeat, acknowledgement, retry,
and reclaim. The queue row is intentionally a serialization point for strict
concurrency and rate admission: its lock wait, database connections, handler
duration, and rate gate are distinct saturation causes.

Do not publish a universal worker knee point. Plot the committed matrix's
throughput and p95/p99 claim latency against workers, then identify the first
plateau from the accompanying database evidence. Backlog queue-wait growth is
shown by `tasklab_oldest_pending_age_seconds`; retain all error/reclaim counts.
