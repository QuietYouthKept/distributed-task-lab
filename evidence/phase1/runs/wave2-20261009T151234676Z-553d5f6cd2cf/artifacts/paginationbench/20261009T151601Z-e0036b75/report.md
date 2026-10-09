# PostgreSQL keyset pagination benchmark

- Run: `20261009T151601Z-e0036b75`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 4.029 | 5.291 | 6.103 | 6.461 |
| middle_keyset | 40 | 6.880 | 10.291 | 10.589 | 12.448 |
| tail_keyset | 40 | 2.143 | 2.658 | 2.658 | 2.925 |
| filtered_owner | 40 | 6.335 | 7.885 | 8.847 | 9.633 |
| filtered_keyset | 40 | 5.597 | 6.745 | 7.105 | 9.183 |
