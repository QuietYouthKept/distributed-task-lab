# Loadgen Evidence Integrity — Run 01

- Source commit/tree: `d6fd35ca80fc692eb40bf99ac23196bd6200b089` / `9d9610e2706c6545047303ca74ee227cf966faf0`.
- Working tree: dirty; source under test was uncommitted. File hashes are in the successful run02 manifest README and top-level audit.
- Command: `go test -p 1 ./cmd/loadgen -count=1 -timeout=2m`.
- Environment: Windows amd64; default `go` was 1.26.6; `GOTOOLCHAIN=local`.
- Result: FAIL before compilation/tests because `go.mod` requires Go >=1.26.9. This is a toolchain mismatch, not a test assertion failure.
- Exit code: 1. Raw output is preserved in `go-test.log`.
- No container, database, external service, or cloud resource was used.
