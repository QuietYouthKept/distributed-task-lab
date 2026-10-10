# PostgreSQL keyset pagination benchmark

- Run: `20261009T153940Z-dd5603c6`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 4.048 | 5.544 | 5.716 | 7.535 |
| middle_keyset | 40 | 3.738 | 5.964 | 6.362 | 6.564 |
| tail_keyset | 40 | 1.354 | 2.094 | 2.191 | 2.377 |
| filtered_owner | 40 | 4.754 | 5.838 | 5.849 | 6.053 |
| filtered_keyset | 40 | 4.692 | 5.410 | 6.155 | 6.575 |
