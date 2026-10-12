# PostgreSQL keyset pagination benchmark

- Run: `20261010T024108Z-bdce1201`
- PostgreSQL: `16.15`
- Rows: 100000
- Generator: `paginationbench-v1`; seed: `distributed-task-lab-pagination-v1-seed-20261009`
- Page size: 500
- Keyset walk: 100000 rows across 201 pages; duplicates: 0
- Concurrent write check: 100000 initial rows; 50 new rows; duplicate count: 0

Latency values are local measurements for this host/container and are not production SLOs. Full `EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON)` plans and indexes are in `manifest.json`.

| Query | Samples | P50 ms | P95 ms | P99 ms | Max ms |
|---|---:|---:|---:|---:|---:|
| head | 40 | 4.817 | 5.497 | 5.510 | 5.672 |
| middle_keyset | 40 | 4.073 | 5.764 | 5.790 | 6.722 |
| tail_keyset | 40 | 1.722 | 2.353 | 2.634 | 2.938 |
| filtered_owner | 40 | 5.690 | 6.432 | 6.692 | 6.798 |
| filtered_keyset | 40 | 5.611 | 6.578 | 7.034 | 7.126 |
