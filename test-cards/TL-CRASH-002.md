# TL-CRASH-002 — Real Docker hard-crash recovery

- Source model: Jepsen process crash plus NATS/Redpanda final recovery scan.
- Setup: one PostgreSQL, one API, worker A; 2 s lease; 10 s handler. Start worker B after A claims.
- Fault: `docker kill` worker A.
- Oracle: final task is `success`, `attempt >= 2`, `lease_version >= 2`; claim events identify distinct owners.
- Result: **TEST VERIFIED** in `results/...-rerun1/docker-faults-rerun4/TL-CRASH-002`.
- Boundary: PostgreSQL itself was not crashed in this scenario; at-least-once execution remains possible.
