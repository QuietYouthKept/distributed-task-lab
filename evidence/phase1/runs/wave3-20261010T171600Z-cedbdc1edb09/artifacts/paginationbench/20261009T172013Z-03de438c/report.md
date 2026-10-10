# PostgreSQL keyset pagination benchmark

- Run: `20261009T172013Z-03de438c`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 4.693 | 5.580 | 5.969 | 5.984 |
| middle_keyset | 40 | 4.744 | 6.953 | 7.081 | 7.455 |
| tail_keyset | 40 | 2.053 | 2.784 | 2.803 | 2.934 |
| filtered_owner | 40 | 5.704 | 7.139 | 7.443 | 8.747 |
| filtered_keyset | 40 | 5.375 | 7.190 | 7.447 | 7.732 |
