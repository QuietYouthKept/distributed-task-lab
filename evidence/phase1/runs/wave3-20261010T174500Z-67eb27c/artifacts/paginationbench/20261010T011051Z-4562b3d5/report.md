# PostgreSQL keyset pagination benchmark

- Run: `20261010T011051Z-4562b3d5`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 3.750 | 5.344 | 5.394 | 11.224 |
| middle_keyset | 40 | 3.782 | 4.831 | 5.067 | 5.125 |
| tail_keyset | 40 | 2.103 | 3.654 | 6.048 | 7.298 |
| filtered_owner | 40 | 5.966 | 7.548 | 8.544 | 14.068 |
| filtered_keyset | 40 | 4.843 | 6.962 | 7.228 | 15.233 |
