# PostgreSQL keyset pagination benchmark

- Run: `20261009T153159Z-3f3c1669`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 3.485 | 4.303 | 4.386 | 6.762 |
| middle_keyset | 40 | 4.591 | 5.604 | 5.780 | 6.237 |
| tail_keyset | 40 | 1.937 | 2.527 | 2.660 | 2.716 |
| filtered_owner | 40 | 4.335 | 6.956 | 7.259 | 7.664 |
| filtered_keyset | 40 | 4.392 | 7.980 | 8.673 | 9.797 |
