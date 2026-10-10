# Wave 3 evidence index

| Evidence | Commit | Result | Pointer |
|---|---|---|---|
| CI repair remote verification | `ac46b6bf2411aa68af671897f6ebdb2e0786e323` | CI PASS; Security PASS except expected Scorecard SKIP | [CI Run 37962333979](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37962333979), [Security Run 37962334001](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37962334001). This does not validate later Wave 3 changes. |
| MinIO source builder reproduction | working tree at CI follow-up | PASS for pinned source compile/checks; Docker CLI stubbed locally | `docs/phase1-build/remote-ci-failure-repair.md`; actual Docker build/start and contract run are in CI above. |
| Provider contract targeted Race | `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf` | PASS, count 10 | `go test -race ./internal/effects ./internal/worker/handlers -run '^(TestHTTPProvider|TestSideEffect)' -count=10`; package tests also passed in full suites. |
| Wave 3 clean local Evidence | `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf` | PASS, 14/14 | `evidence/phase1/runs/wave3-20261010T171000Z-9b360f5952bb/manifest.json`, raw logs and hash-verified Chaos/pagination artifacts. PostgreSQL 16.15, Docker 28.3.3, Go 1.26.9, source-built temporary MinIO. |
| Wave 3 Package C clean local Evidence | `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` | PASS, 14/14 | `evidence/phase1/runs/wave3-20261010T171600Z-cedbdc1edb09/manifest.json`, raw logs and hash-verified Chaos/pagination artifacts. Includes ingress lifecycle code, exact commit and clean start/after checks. |
| Wave 3 Package C remote CI | `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` | CI PASS; Security PASS except expected Scorecard SKIP | [CI Run 37965020508](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37965020508), [Security Run 37965020486](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37965020486). |
| Package A cross-container and MinIO response-loss | `9b81e96b498b05d92469efb41c53c6c1fe6acedf` | PASS, two targeted Race gates, clean worktree at test time | Run `tasklab-e2e-980f3ba036b3`; implementation-bound Evidence commit `962ec4e33c2c907a73eae628e9f9d39dd2704acb`. Earlier failed and dirty diagnostic runs remain under `wave3-container-e2e-attempts/` and `wave3-container-e2e/`. |
| Package A strengthened metadata and independent-client checks | `3eef1603293aa0bbc45877bb8263699f09c9e2b5` | PASS, both targeted Race gates, clean worktree at test time | Run `tasklab-e2e-d55fb468070f`; output hash checks, malformed metadata classification and cross-container recovery passed. Evidence commit `67eb27c`. |
| Package A final cross-container closeout | `e74e59f66471c449cd68fba525f57ce2028af5b5` | PASS, four live MinIO/API/Worker E2E gates; clean-bound | Run `tasklab-e2e-10de3fd9335c`; covers response loss in actual Worker PUT, API/two-Worker crash recovery, and same-key writes from two Worker containers. Logs SHA256-recorded. Evidence commit `a1d5873f7fa46b8b421474c22b86c35fd2c2fa07`; prior failed/diagnostic attempts are preserved by `aa11d5f76e587b091306dca407e07b4f49a142b7`. |
| Package D local Prometheus API/Worker scraping | `f5974777a452c2e455a0c9b03bffa2df88ed989f` | PASS, exact-commit Compose runtime; both targets UP | Compose project `tasklab-observability-verify-f597477`; API `api:8080` authenticated by runtime secret and Worker `worker-metrics:9091` scraped over internal network. Temporary containers and volumes removed. Local development only; not production discovery/alerting. |
| Wave 3 14-gate regression after A closeout | `d3c5310910b942ad7232a287a62ba550625cc855` | PASS, 14/14; clean start and clean excluding run artifacts | `evidence/phase1/runs/wave3-d3c5310-full-14-gate/manifest.json`; includes unit/integration, full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab, Chaos, 100K pagination, Vet, public client Race, OTLP receiver, MinIO contract, API/two-Worker recovery, and backup/restore. Evidence is being committed separately from source. |
| Full collector toolchain failure | `962ec4e33c2c907a73eae628e9f9d39dd2704acb` | FAIL, 13/14; all Go commands were rejected before package loading; pagination NOT VERIFIED | `evidence/phase1/runs/wave3-20261010T172000Z-962ec4e/manifest.json`; `GOTOOLCHAIN=local` pinned local Go 1.26.6 against `go 1.26.9`. Immutable failure is preserved; wrapper now switches to `auto` for the next attempt. |
| Full Wave 3 clean regression Evidence | `eafc0a83037d24273eb8cfae881c94a8c7ce3d53` | PASS, 14/14 | Run `wave3-20261010T173000Z-80b7baa`; Go 1.26.9, PostgreSQL 16.15, Docker 28.3.3, source-built MinIO. Separate append-only Evidence commit pending. |
| Full collector with Package A metadata tests | `67eb27c6451f5d85bb3f0f0637d950745340a6a5` | FAIL, 13/14 | `wave3-20261010T174500Z-67eb27c`; only PostgreSQL Race ×5 failed because an existing lease-expiry test used a tiny wall-clock sleep. Fixed the fixture with PostgreSQL `clock_timestamp()`; targeted Race ×20 passed; full rerun pending. |
| Full Wave 3 regression after lease-fixture correction | `22478c8986b8d1c7d6b89d76c6249304f25c8cda` | PASS, 14/14 | Run `wave3-20261010T180000Z-22478c8`; Go 1.26.9, PostgreSQL 16.15, Docker 28.3.3, source-built MinIO HTTP 200, clean start/after. Evidence commit pending. |
| MinIO preflight without proxy | `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf` | NOT VERIFIED, no test gate executed | `evidence/phase1/runs/wave3-minio-proxy-preflight-fail-9b360f5952bb/manifest.json`; Git Bash curl connection reset; retry with local proxy completed the separate 14/14 run. |
| Wave 3 remote CI | `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf` | PASS | [CI Run 37963245838](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37963245838), [Security Run 37963245883](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/37963245883), Scorecard conditionally skipped. |
| Wave 2 latest Evidence | `a292ec7...` implementation / `943b84b` Evidence | 14/14 PASS | `evidence/phase1/runs/wave2-20261009T153618132Z-a292ec7a996b/manifest.json`; historical only. |

The Evidence directory and `evidence/phase1/index.json` are append-only. Do not edit the historical Wave 2 run or index entries.

## Package E local supply-chain artifacts

| Exact source commit | Result | Artifacts and scope |
|---|---|---|
| `75bce219fb329d1960bc66e25fcda37b3898f099` | PASS locally; remote GitHub Actions NOT VERIFIED | API/Worker images built with matching OCI revision labels; CycloneDX SBOMs (251/262 components) and full Trivy JSON scans under `E:\distributed-task-lab-temp\wave3-artifacts\package-e-75bce21`. API: 0 vulnerabilities. Worker: one preserved UNKNOWN `GO-2026-5932` module-level advisory, not apparently called according to govulncheck; HIGH/CRITICAL: 0. Build actionlint passed. No signing, publication, staging or release/rollback claim. |

The external E: artifacts are immutable run outputs; earlier failed and intermediate image scan reports remain in their original sibling directories.

## Final closeout snapshot — 2026-10-10

| Evidence | Exact snapshot | Result | Pointer |
|---|---|---|---|
| Final 14-gate clean-worktree regression | `0a5482d657ae431751c205abdf01632ebe89cee6` | PASS, 14/14, all exit codes 0 | Run `wave3-final-0a5482d-20261010`; manifest and hash-verified logs/artifacts in `E:\distributed-task-lab-temp\wave3-artifacts\final-closeout-20261010-0a5482d\evidence\wave3-final-0a5482d-20261010`. Source tree SHA `91940cda4428026b4ab360b3aa00bcaab491dda7`. |
| Final API/two-Worker-container E2E | Tested snapshot `0a5482d657ae431751c205abdf01632ebe89cee6`; the manifest's `implementation_commit` field identifies the latest commit touching its E2E source files (`e74e59f`) | PASS, 4/4; `worktree_dirty_at_test=false` | Compose project `tasklab-e2e-50ff56931d69`; copied manifest and SHA-verified logs in `E:\distributed-task-lab-temp\wave3-artifacts\final-closeout-20261010-0a5482d\evidence\cross-container\tasklab-e2e-50ff56931d69`. Credentials are not recorded. |
| Provider contract repeated Race | `0a5482d657ae431751c205abdf01632ebe89cee6` | PASS ×10 | E: `logs/provider-and-side-effect-race-x10.stdout.log`; HTTP test server only, no real external provider. |
| OpenAPI contract repeated Race | `0a5482d657ae431751c205abdf01632ebe89cee6` | PASS ×20 | E: `logs/openapi-contract-race-x20.stdout.log`; only CreateTask DTO/schema parity is complete. |
| Final workflow static checks | `0a5482d657ae431751c205abdf01632ebe89cee6` | PASS | E: actionlint stdout/stderr logs; updated workflows are still hosted-unverified. |
| Worker advisory package analysis | `0a5482d657ae431751c205abdf01632ebe89cee6` | No `openpgp` package/symbol found in Linux Worker build; module-level UNKNOWN retained | E: Linux/amd64 Worker binary SHA256 `B38DE39013A4EB3504FEEDFB132453A25C58E5784B7EC6A07CE07EBF779CD72B`; blocked local govulncheck diagnostic preserved. See `WAVE3_VULNERABILITY_TRIAGE.md`. |
| Final local MinIO credential rotation | Local-only `tasklab-wave2-minio` test instance | PASS: real S3 contract passed; container stopped after test; named data volume preserved | ACL-restricted raw/review logs and hashes in `E:\distributed-task-lab-temp\wave3-artifacts\final-closeout-20261010-0a5482d\security\logs`; random replacement credentials are outside Git in `E:\distributed-task-lab-temp\local-secrets\tasklab-wave2-minio-20261010`. |
| Implementation push | `0a5482d657ae431751c205abdf01632ebe89cee6` | BLOCKED, no remote update confirmed | Normal push via `127.0.0.1:7897` failed with Schannel TLS handshake EOF; raw failure preserved under E: `logs/push-implementation-attempt-01.log`. No force-push or alternate remote write. |

Older rows above remain immutable historical evidence; do not relabel them as results for this final snapshot.

## Package C OpenAPI schema alignment

| Implementation | Result | Scope |
|---|---|---|
| `93f7bee8ba77e6bb65f2a71f68f6e71da8b839d4` | PASS locally | Targeted OpenAPI tests (5), full `internal/api` package, and OpenAPI Race ×10 passed. `POST /v1/tasks` success references `CreateTaskResponse`; request/payload/response property sets are checked against Go DTO JSON tags. This is not all-operation schema drift evidence. |

## Package D Worker trace continuation

| Implementation | Result | Scope |
|---|---|---|
| `17c1fba31a7e35a76622c34bd0adafdb1b299857` | PASS locally | Worker trace continuation test ×20, full `internal/worker` package, and test Race ×10 passed. Validates extracted parent context and propagation-field filtering only; it does not close the distributed persistence/Collector path. |

## Combined local regression after Package C/D follow-ups

| Exact source commit | Result | Checks and boundary |
|---|---|---|
| `aa0cff3a7cfc340f1f17225dadab0a16e5095101` | PASS locally, clean worktree | `go test ./cmd/... ./internal/... ./pkg/... -count=1`; `go test -race ./cmd/... ./internal/... ./pkg/... -count=1`; `go vet ./cmd/... ./internal/... ./pkg/...`; `git diff --check`. This is Go regression evidence only; no Docker E2E, Failure Lab, remote CI, staging, or production claim. |

Artifact SHA256 (uppercase hex):

- `api-image.tar`: `E7179E346D9A9E0CD909E9913678875CF9A1DAA30CBD4BB077836939EBDDD46C`
- `worker-image.tar`: `776D782AA749B7E00CA09E1A3DE5E905A77AB0CCC8CECD184B872F31584E20AA`
- `api-image.cdx.json`: `1E2F044DF6B931804F938F90D9A92E5853BEB26B541EADAF2FC0E8784E697231`
- `worker-image.cdx.json`: `AFCC633A33C8C7BE37125761B7E528D3541CC94CA8F27AD5C40CFEF186C090F4`
- `api-image-trivy.json`: `9D909780809A5A3FB6AD183B8F90BF33E214635919CF7D1FE1BFF91AB8A9C2C2`
- `worker-image-trivy.json`: `F8B2AAECBAEE9C7962EB4E0D4CE4BAF20B22195B4264FCFDF6D8074DE45F7B4E`

## Final hosted verification receipt — append-only update (2026-10-10)

This is a newer observation than historical push-failed / hosted-pending entries above. Original failed attempts, prior run results and archived E: manifests are not modified.

| Workflow | Hosted SHA | Run / attempt | Final status and coverage |
|---|---|---|---|
| CI | `ad52365b7dae2ddb3e05ec32f310d626e0583611` | [38029233771](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233771), attempt 1 | SUCCESS, 5/5 jobs: format/vet/unit/build; PostgreSQL integration and Race; source-built authenticated MinIO, independent Worker **process** recovery, backup/restore; external Go client |
| Security | `ad52365b7dae2ddb3e05ec32f310d626e0583611` | [38029233767](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233767), attempt 1 | SUCCESS: govulncheck, CodeQL, Secret Scan PASS; Scorecard expected SKIPPED |
| Container Supply Chain | `ad52365b7dae2ddb3e05ec32f310d626e0583611` | [38029233773](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233773), attempt 1 | SUCCESS: API+Worker image build, SBOM and vulnerability scanning; residual `GO-2026-5932` module-level UNKNOWN retained |

Implementation `0a5482d657ae431751c205abdf01632ebe89cee6` (tree `91940cda4428026b4ab360b3aa00bcaab491dda7`) was separately tested with clean 14/14 local regression and 4/4 live cross-container gates; the hosted SHA is its **docs-only** descendant. Neither hosted CI nor Supply Chain runs should be described as a hosted two-Worker **container** fault-injection test or real Provider/S3-cloud/production validation. The subsequent archive commit is not the SHA against which these workflows were run.
