# Wave 3 evidence index

| Evidence | Commit | Result | Pointer |
|---|---|---|---|
| CI repair remote verification | `ac46b6bf2411aa68af671897f6ebdb2e0786e323` | CI PASS; Security PASS except expected Scorecard SKIP | [CI Run 37962333979](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37962333979), [Security Run 37962334001](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37962334001). This does not validate later Wave 3 changes. |
| MinIO source builder reproduction | working tree at CI follow-up | PASS for pinned source compile/checks; Docker CLI stubbed locally | `docs/phase1-build/remote-ci-failure-repair.md`; actual Docker build/start and contract run are in CI above. |
| Provider contract targeted Race | Wave 3 source currently uncommitted | PASS, count 10 | `go test -race ./internal/effects ./internal/worker/handlers -run '^(TestHTTPProvider|TestSideEffect)' -count=10`; not yet a clean Implementation/Evidence commit. |
| Wave 2 latest Evidence | `a292ec7...` implementation / `943b84b` Evidence | 14/14 PASS | `evidence/phase1/runs/wave2-20261009T153618132Z-a292ec7a996b/manifest.json`; historical only. |

New clean Evidence for the Wave 3 Implementation Commit has not yet been collected. Append a new row after the full suite and hash-verifying collector completes; do not edit the historical Wave 2 run or index entries.
