# PostgreSQL keyset pagination benchmark

- Run: `20261009T152304Z-b963a609`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 3.262 | 4.529 | 4.895 | 5.294 |
| middle_keyset | 40 | 3.778 | 5.912 | 5.945 | 6.140 |
| tail_keyset | 40 | 1.536 | 2.046 | 2.051 | 2.281 |
| filtered_owner | 40 | 3.704 | 4.581 | 4.592 | 5.017 |
| filtered_keyset | 40 | 3.561 | 4.976 | 5.373 | 5.561 |
