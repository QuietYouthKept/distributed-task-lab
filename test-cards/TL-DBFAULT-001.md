# TL-DBFAULT-001 — PostgreSQL pause and recovery

- Source model: Jepsen pause nemesis and heal/final-read phase.
- Setup: API + worker + PostgreSQL Docker stack; DB operation timeout 1 s; lease 2 s.
- Fault: `docker pause` PostgreSQL for 3 s, then `docker unpause`.
- Oracle: `/ready` fails closed during pause and the abandoned task later reaches success at attempt >=2.
- Result: **TEST VERIFIED** in `results/...-rerun1/docker-faults-rerun4/TL-DBFAULT-001`.
- Boundary: no disk corruption, data loss, or replicated failover was injected.
