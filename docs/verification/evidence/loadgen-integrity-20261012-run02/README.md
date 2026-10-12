# Loadgen Evidence Integrity — Run 02

- Run ID: `loadgen-integrity-20261012-run02`.
- Source commit/tree: `d6fd35ca80fc692eb40bf99ac23196bd6200b089` / `9d9610e2706c6545047303ca74ee227cf966faf0`.
- Working tree: dirty; this verifies the changed source snapshot, not a committed build.
- Source SHA256: `cmd/loadgen/artifacts.go` `06ca47d48a1e4890c04b77b36a5afd8508763d4a775d658594bd73cf2208547f`; `cmd/loadgen/main.go` `5e5d9ec1d33a85c6d17b5dd938a10a01debbe8a024c839c0a63b35381414af3c`; `cmd/loadgen/artifacts_test.go` `202140263eae0ee89a33d83e0543fc91b6585adcb76c477b132260f2975b4ef5`.
- Runtime: `go version go1.26.9 windows/amd64` selected with the command-local `GOTOOLCHAIN=go1.26.9` setting. The downloaded toolchain was Go's versioned toolchain distribution; no repository configuration was changed.
- Unit command: `go test -p 1 ./cmd/loadgen -count=1 -timeout=2m`; result PASS, one package, test output `ok ... 0.968s`, exit 0.
- Vet command: `go vet ./cmd/loadgen`; PASS, exit 0, empty diagnostics.
- The unit test checks clean/dirty/unknown Git metadata and the terminal-task / full-run throughput denominator, including zero-duration and zero-task bounds.
- Resource gate immediately before compile: 5.78 GiB available RAM. No containers or database were used.
- No load benchmark or PostgreSQL integration was run.

`go-test.log`, `go-vet.log`, and this README are sealed in `SHA256SUMS`.
