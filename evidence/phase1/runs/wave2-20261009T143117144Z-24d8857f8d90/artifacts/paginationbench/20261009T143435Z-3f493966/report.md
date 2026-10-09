# PostgreSQL keyset pagination benchmark

- Run: `20261009T143435Z-3f493966`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 4.144 | 7.200 | 7.717 | 8.201 |
| middle_keyset | 40 | 3.433 | 3.974 | 4.082 | 4.347 |
| tail_keyset | 40 | 1.550 | 2.132 | 2.136 | 2.418 |
| filtered_owner | 40 | 3.626 | 5.701 | 5.808 | 6.304 |
| filtered_keyset | 40 | 3.893 | 5.548 | 5.588 | 5.936 |
