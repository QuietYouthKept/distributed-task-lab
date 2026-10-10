# PostgreSQL keyset pagination benchmark

- Run: `20261010T005543Z-453bf99f`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 32.796 | 40.353 | 45.088 | 69.149 |
| middle_keyset | 40 | 30.618 | 39.225 | 42.699 | 52.743 |
| tail_keyset | 40 | 7.682 | 13.385 | 15.372 | 25.483 |
| filtered_owner | 40 | 33.197 | 51.926 | 54.016 | 61.410 |
| filtered_keyset | 40 | 32.868 | 41.106 | 42.031 | 48.708 |
