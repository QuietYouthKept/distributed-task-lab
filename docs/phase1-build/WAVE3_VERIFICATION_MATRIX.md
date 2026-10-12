# Wave 3 final verification matrix

Final implementation snapshot: `0a5482d657ae431751c205abdf01632ebe89cee6` (tree `91940cda4428026b4ab360b3aa00bcaab491dda7`). This matrix binds local source Evidence to `0a5482d` and hosted CI/Security/Supply Chain success to its documentation-only descendant `ad52365b` (no production Go/schema changes between them). The two boundaries remain separate. All new raw run outputs are on E:; historical Wave 2/Wave 3 evidence remains append-only.

| Requirement | Test / command | Source SHA | Local status and evidence | Hosted CI status | Limitations |
|---|---|---|---|---|---|
| Unit + PostgreSQL integration | `go test ./cmd/... ./internal/... ./pkg/... -count=1`, `REQUIRE_TEST_DATABASE=1` | `0a5482d` | PASS in 14-gate run `wave3-final-0a5482d-20261010`; PostgreSQL 16.15; log hash in manifest | PASS, CI `38029233771` at `ad52365b` (format/unit/build and required PostgreSQL integration) | Local Windows host and isolated test DB |
| Full Race | `go test -race ./cmd/... ./internal/... ./pkg/... -count=1` | `0a5482d` | PASS in the same clean-start 14-gate run | PASS, CI `38029233771` at `ad52365b` (PostgreSQL Race job) | No production load claim |
| PostgreSQL Race ×5 | `go test -race ./internal/storage/postgres -count=5` | `0a5482d` | PASS; exit 0, log SHA256 in manifest | No separately hosted ×5 gate; standard PostgreSQL Race passed in CI `38029233771` | Uses local PostgreSQL 16.15 |
| RetryBackoff Race ×20 | `go test -race ./internal/storage/postgres -run '^TestRetryBackoff$' -count=20 -v` | `0a5482d` | PASS; exit 0, log SHA256 in manifest | No separately hosted ×20 gate; local repeated Race Evidence retained | None beyond local DB/toolchain |
| Failure Lab | `go run ./cmd/failurelab` | `0a5482d` | PASS, 12/12 database-oracle scenarios | NOT RUN as a dedicated hosted gate; local 12/12 | Deterministic/local scenarios |
| Chaos Matrix | `go run ./cmd/chaosmatrix` | `0a5482d` | PASS, six scenarios ×30/30 (180 total); summary/events/report hashes in manifest | NOT RUN as a dedicated hosted gate; local 180/180 | Local simulation, not production chaos |
| 100K keyset pagination | `go run ./cmd/paginationbench` | `0a5482d` | PASS: 100,000 rows, 201 pages, 0 duplicates; concurrent 50-row insert check also 0 duplicates. E: report and EXPLAIN artifact hash-verified. | NOT RUN as a hosted benchmark; local 100K evidence retained | Local timings only; filtered-owner plans still merit representative production-shaped analysis |
| PostgreSQL backup/restore | `go test -race ./internal/storage/postgres -run '^TestDatabaseBackupRestoreDrill$' -count=1 -v` | `0a5482d` | PASS in final run; restored table counts and referenced MinIO object checksum verified | PASS in required MinIO/worker-recovery CI job `38029233771` | Local drill only; no cloud RPO/RTO |
| MinIO S3 contract | `go test -race ./internal/artifacts -run '^TestMinIOStoreContractAndConcurrentConditionalWrite$' -count=1 -v` | `0a5482d` | PASS against source-built MinIO; injected lost PUT/HEAD responses, retry reused verified SHA; artifacts in final run | PASS authenticated S3 contract in CI `38029233771` | MinIO is not proof of AWS/other S3 implementations |
| Cross-container recovery / artifact race | `scripts/e2e/run-cross-container.ps1`; four live Race gates | `0a5482d` tested snapshot | PASS 4/4 at `tasklab-e2e-50ff56931d69`; API, PostgreSQL, MinIO, two separate Worker containers; crash-after-Put, lease reclaim/fencing, Worker PUT response loss and same-key barrier. Manifest/log hashes verified. | NOT RUN in hosted two-Worker container topology; local 4/4 is separate | Local Docker topology; no exactly-once claim for arbitrary external effects |
| Provider / side-effect contract | `go test -race ./internal/effects ./internal/worker/handlers -count=10` | `0a5482d` | PASS, exact SHA, stdout/stderr hashes under E: final logs | Generic Go unit/Race PASS in CI `38029233771`; real external Provider NOT VERIFIED | Local HTTP contract only; `REAL_PROVIDER_NOT_VERIFIED` |
| Public Go client | `go test -race ./... -count=1 -v` from `test/external-client` | `0a5482d` | PASS in final 14-gate run | PASS in independent module CI job `38029233771` | Separate consumer module; hosted result predates workflow pins |
| OpenAPI route/auth contract | `go test -race ./internal/api -run '^TestOpenAPI' -count=20` | `0a5482d` | PASS ×20; logs/hash under E: final logs | PASS current stable v1 OpenAPI contract in CI `38029233771`; full DTO/schema drift NOT VERIFIED | 15 v1 operations/routes are enumerated. Only CreateTask request/payload/response has systematic DTO JSON-field parity; 13 operations lack response-body schemas. |
| OTLP receiver | `go test -race ./internal/observability -run '^TestSetupTracingExportsSpanToOTLPHTTPReceiver$' -count=1 -v` | `0a5482d` | PASS in final run | General unit/Race pipeline PASS at `ad52365b`; no dedicated hosted collector-chain gate | Local receiver, not a multi-process application trace |
| Worker trace continuation | `TestWorkerExecutionContinuesPersistedTraceContext` in full unit/Race | `0a5482d` | PASS in final full Race | General unit/Race pipeline PASS at `ad52365b`; full distributed trace E2E NOT VERIFIED | Full API→DB→queue→Worker→MinIO→ACK→Collector chain is NOT VERIFIED |
| API/Worker metrics scrape | Exact-commit local Compose scrape at `f597477` | `f597477` | PASS; both targets UP with API scrape auth. Metric/overlay source files are unchanged through `0a5482d`. | No hosted Prometheus scrape job; local exact-file-set Compose result only | Development overlay only; alert firing and production discovery are NOT VERIFIED |
| Vulnerability scan | `govulncheck ./...` | Code/dependencies unchanged from `49c275c`; workflow updated in `0a5482d` | Current direct attempt BLOCKED by TLS EOF fetching `vuln.go.dev`; GO-2026-5932 triage is documented separately | PASS govulncheck at `ad52365b`, Security `38029233767`; local EOF remains BLOCKED, Worker UNKNOWN advisory retained | Prior pass is not relabeled to current workflow SHA; advisory remains UNKNOWN at module level |
| CodeQL / secret scan | Existing Security workflow | `49c275c` baseline only | Local focused diff review; no new full secret/code scan tool run | PASS CodeQL and Secret Scan at `ad52365b`, Security `38029233767` | Does not certify uninspected terminal logs or external credential reuse |
| SBOM / image vulnerability scan | CycloneDX + full-severity Trivy at `75bce21` | `75bce21` exact scan source | PASS historically; API 0 findings, Worker 1 UNKNOWN GO-2026-5932, HIGH/CRITICAL 0. Artifacts preserved in `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave3-artifacts\package-e-75bce21`. | PASS container supply-chain job at `ad52365b`, Run `38029233773`; Worker module-level UNKNOWN triage retained | No signing/publication/staging; current source's hosted scan awaits push |
| Workflow static validation | `actionlint` on CI, Security, Supply Chain workflows; PowerShell parser on evidence runner | `0a5482d` | PASS; raw logs/hashes in E: final logs | Updated pinned workflows executed successfully at `ad52365b`; independent local actionlint PASS | Static checks do not replace GitHub Actions execution |
| Hosted CI final pushed snapshot | CI/Security/Supply Chain runs at `49c275c` | `49c275c` | N/A | FINAL hosted SHA `ad52365b`: CI `38029233771`, Security `38029233767`, Supply Chain `38029233773` all SUCCESS (attempt 1); prior `49c275c` successes remain historical | Exact old SHA only. New action pins and runner changes are NOT VERIFIED remotely because push failed. |

## Final hosted gate receipt (append-only)

| Field | Recorded outcome |
|---|---|
| Pushed docs HEAD | `ad52365b7dae2ddb3e05ec32f310d626e0583611` |
| Frozen tested implementation | `0a5482d657ae431751c205abdf01632ebe89cee6` (docs-only ancestor relation verified) |
| CI | [38029233771](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233771) — completed/success; 5/5 jobs |
| Security | [38029233767](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233767) — completed/success; CodeQL/govulncheck/secret-scan PASS; Scorecard expected SKIP |
| Container Supply Chain | [38029233773](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233773) — completed/success; 1/1 job |
| Run attempts | 1 for each |
| Not covered by these hosted runs | Local two-Worker **container** 4-gate test, Failure Lab 12/12, Chaos 180/180, 100K pagination benchmark, real Provider, full Collector E2E, staging and production |
| Residual risk | Trivy Worker `GO-2026-5932` UNKNOWN remains triaged, not suppressed; real Provider/IdP/cloud/production approval absent |

The remote receipt binds to `ad52365b`, not to a later documentation archive commit. No prior raw log, manifest, or historical FAIL/BLOCKED status is rewritten.

## Exact-snapshot evidence paths

- Full 14-gate manifest and command logs: `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave3-artifacts\final-closeout-20261010-0a5482d\evidence\wave3-final-0a5482d-20261010`.
- Four-gate cross-container manifest/log bundle: `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave3-artifacts\final-closeout-20261010-0a5482d\evidence\cross-container\tasklab-e2e-50ff56931d69`.
- Provider/OpenAPI logs, Worker binary and security triage raw diagnostics: same final-closeout package under `logs/` and `security/`.
- The working source worktrees and Go caches remain under the adjacent initial closeout workspace `<REDACTED_LOCAL_ARTIFACT_ROOT>\wave3-artifacts\final-closeout-20261010-49c275c`; no repository checkout was moved from D:.

Status vocabulary: `PASS`, `FAIL`, `SKIPPED`, `BLOCKED`, `NOT VERIFIED`. Historical pass is never substituted for a newer source/workflow SHA.
