# Current Go Suite Resource Gate — Run 01

- Run ID: `go-unit-current-20261012-run01`.
- Source commit/tree at gate: `0a4220cf93cf7bf16034bc37a3f3d5fd29f17176` / `b3884ba497a3665487adad6b984305a55b068704`.
- Available RAM immediately before the planned test: 3.81 GiB.
- Existing compile gate: 4 GiB minimum. The guarded command exited before invoking Go tests.
- Planned command: `go test -json -p 1 ./cmd/... ./internal/... ./pkg/... -count=1 -timeout=10m`.
- Test status: `NOT_RUN`; no tests were counted and no Skip is claimed.
- Safety: no container was started. Both `TEST_DATABASE_URL` and `DATABASE_URL` were scoped to `127.0.0.1:1`; `REQUIRE_TEST_DATABASE`, backup/restore and MinIO opt-ins were unset, so the attempt could not mutate a user database or invoke the restore drill.
- This is a resource-gate record, not a test log or PASS.
