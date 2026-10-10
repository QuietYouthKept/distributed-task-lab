# PostgreSQL keyset pagination benchmark

- Run: `20261009T170709Z-8ffb8b06`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 3.885 | 4.926 | 5.068 | 5.447 |
| middle_keyset | 40 | 4.123 | 4.411 | 4.499 | 6.657 |
| tail_keyset | 40 | 1.667 | 2.221 | 2.304 | 2.669 |
| filtered_owner | 40 | 5.572 | 7.141 | 7.448 | 7.601 |
| filtered_keyset | 40 | 5.448 | 7.231 | 7.341 | 7.758 |
