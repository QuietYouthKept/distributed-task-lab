# TL-SHUT-002 — Real Docker graceful drain

- Source model: controlled Jepsen client teardown.
- Setup: 3 s cooperative handler, 2 s renewable lease, 10 s shutdown deadline.
- Fault: `docker stop --time 10` (SIGTERM) after claim.
- Oracle: task reaches `success` at attempt 1 before the container exits.
- Result: **TEST VERIFIED** in `results/...-rerun1/docker-faults-rerun4/TL-SHUT-002`.
- Boundary: uncooperative handlers and SIGKILL use lease recovery instead.
