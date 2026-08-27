# PR: Stabilize PostgreSQL task execution and release evidence

## Summary

This PR makes the queue's PostgreSQL correctness protocol reviewable and
reproducible: queue-authority admission, lease fencing/renewal, rate governance,
migration integrity, API boundaries, fault labs, and evidence cards.

## Review focus

- Queue authority is locked before task ownership; `SKIP LOCKED` keeps an
  unavailable queue from blocking unrelated queues.
- Capacity and shared token admission are rechecked under the queue lock using
  a fresh `READ COMMITTED` statement; no transaction sleeps for rate tokens.
- Owner-only success, failure, and renewal writes are fenced by task ID,
  worker ID, and lease version.
- Pool completion cancellation is not interpreted as distributed lease loss.
- Failure Lab provisions an isolated database per invocation; Asset Lab pauses
  completed capacity rounds so global claim selection cannot contaminate the
  next round's oracle.
- Checker v2 recomputes key invariants from history and DB snapshots instead of
  trusting `result.json`.

## Validation

- `go test ./cmd/... ./internal/... ./pkg/... -count=1`
- `REQUIRE_TEST_DATABASE=1 go test ./internal/storage/postgres -count=1 -v`
- `go test -race ./cmd/... ./internal/... ./pkg/... -count=1`
- Failure Lab: 12/12 PASS; Asset Lab core: 11/11 PASS; Docker faults: 3/3 PASS.
- `govulncheck`: zero reachable vulnerabilities (one imported-package and one
  required-module advisory are not reached by this code).

## Boundaries

The project remains at-least-once. It does not provide exactly-once external
effects, PostgreSQL HA/failover guarantees, or a replicated-log protocol.
