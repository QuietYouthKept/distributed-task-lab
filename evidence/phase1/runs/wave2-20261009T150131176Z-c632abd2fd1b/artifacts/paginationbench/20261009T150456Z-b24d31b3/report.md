# PostgreSQL keyset pagination benchmark

- Run: `20261009T150456Z-b24d31b3`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 3.268 | 3.867 | 3.889 | 4.338 |
| middle_keyset | 40 | 3.362 | 4.834 | 6.230 | 6.656 |
| tail_keyset | 40 | 1.680 | 2.155 | 2.282 | 3.754 |
| filtered_owner | 40 | 3.899 | 4.837 | 5.232 | 5.424 |
| filtered_keyset | 40 | 3.996 | 4.535 | 4.623 | 5.796 |
