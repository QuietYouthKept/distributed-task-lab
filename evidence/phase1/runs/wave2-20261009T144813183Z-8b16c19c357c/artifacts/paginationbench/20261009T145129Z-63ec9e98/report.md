# PostgreSQL keyset pagination benchmark

- Run: `20261009T145129Z-63ec9e98`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 4.033 | 6.348 | 6.363 | 6.884 |
| middle_keyset | 40 | 5.033 | 6.530 | 6.640 | 6.720 |
| tail_keyset | 40 | 1.792 | 2.117 | 2.226 | 2.292 |
| filtered_owner | 40 | 4.426 | 5.000 | 5.152 | 5.521 |
| filtered_keyset | 40 | 6.287 | 8.958 | 9.081 | 11.200 |
