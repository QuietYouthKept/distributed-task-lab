# Worker and handler unit regression — 2026-10-12 run 01

Status: both `internal/worker` and `internal/worker/handlers` PASS, exit 0, with no skipped tests in the package output. These are process-local unit tests with fake queues/stores and local HTTP servers, not PostgreSQL or cross-process recovery evidence.

- Source commit: `a02194368a1f7ed41d27692d5795ff4ddf3bbd1f`
- Source tree: `66caf4bf5780d267ed92b7f31280de0a371145fa`
- Go: `go1.26.9 windows/amd64`
- Exact command: `go test -p 1 ./internal/worker/... -count=1 -timeout=2m`
- Exit: 0.
- Race, PostgreSQL, Docker, multi-process crash/reclaim, and live providers: NOT RUN.

Coverage includes bounded local claims, graceful shutdown, lease renewal during long handlers, ownership-loss cancellation and no-ACK, local renewal cancellation, effect intent/unknown-outcome reconciliation, and the Gateway redirect/response-limit behavior from the preceding commit. `SHA256SUMS` covers evidence files except itself.
