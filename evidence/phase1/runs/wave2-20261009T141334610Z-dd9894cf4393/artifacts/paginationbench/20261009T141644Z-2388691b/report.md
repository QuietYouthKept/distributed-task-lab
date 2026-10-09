# PostgreSQL keyset pagination benchmark

- Run: `20261009T141644Z-2388691b`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 3.318 | 3.651 | 4.016 | 4.197 |
| middle_keyset | 40 | 3.477 | 4.213 | 4.284 | 4.329 |
| tail_keyset | 40 | 1.517 | 2.168 | 2.187 | 2.252 |
| filtered_owner | 40 | 3.730 | 5.321 | 5.364 | 5.900 |
| filtered_keyset | 40 | 3.946 | 5.361 | 5.402 | 6.963 |
