# PostgreSQL keyset pagination benchmark

- Run: `20261010T011923Z-9cd86df6`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 5.754 | 8.648 | 8.716 | 8.757 |
| middle_keyset | 40 | 5.742 | 7.573 | 8.167 | 8.879 |
| tail_keyset | 40 | 1.678 | 2.244 | 2.275 | 3.153 |
| filtered_owner | 40 | 5.385 | 8.344 | 8.422 | 8.678 |
| filtered_keyset | 40 | 5.226 | 6.867 | 7.489 | 8.264 |
