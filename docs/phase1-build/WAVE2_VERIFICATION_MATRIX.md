# Wave 2 verification matrix

Environment observed: Windows AMD64, Go 1.26.6, Docker Linux Engine; PostgreSQL 16 container `tasklab-test-postgres` on host port 5433; Docker MinIO health at `127.0.0.1:19000` and Windows MinIO health at `127.0.0.1:9000`. Current process had no MinIO credential environment variables. Historical evidence is not relabeled as current evidence.

| Verification | Result | Command / evidence | Notes |
|---|---|---|---|
| Unit + PostgreSQL integration | PASS | `REQUIRE_TEST_DATABASE=1 go test ./cmd/... ./internal/... ./pkg/... -count=1` | Reran after Store/handler changes; DB tests were required, not skipped. |
| Full Race (earlier worktree rerun) | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./cmd/... ./internal/... ./pkg/... -count=1` | Passed before the later clean Evidence run; not a substitute for that run's failure. |
| PostgreSQL Race repetition | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./internal/storage/postgres -count=5` | Exit 0; 98.960 s. |
| Retry backoff regression | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./internal/storage/postgres -run '^TestRetryBackoff$' -count=20 -v` | 20/20; database clock remains the due-time authority. |
| Store and handler tests | PASS | `go test ./cmd/worker ./internal/artifacts ./internal/worker/handlers -count=1`; matching `-race` run | Local Store only; does not substitute for MinIO. |
| MinIO adapter contract (earlier clean run) | NOT VERIFIED | `wave2-20261009T115734795Z-568c45d5ad66` | That Evidence collector had no credentials; its status remains immutable. |
| MinIO adapter contract (latest clean run) | PASS | `evidence/phase1/runs/wave2-20261009T120751006Z-04d4b231c0cf/manifest.json` | Exit 0 under Race against temporary MinIO from the pinned digest, with random credentials. Covers same/different content, concurrency, missing object, invalid credentials and canceled request. |
| MinIO CI job | CONFIGURED; remote NOT VERIFIED | `.github/workflows/ci.yml` job `minio-contract` | Pinned image digest and generated credentials; remote GitHub Actions has not run because no push was requested. |
| Latest clean Wave 2 collector | PASS | `evidence/phase1/runs/wave2-20261009T120751006Z-04d4b231c0cf/manifest.json` | Implementation `04d4b23`; all eight recorded commands Exit 0, no skips; evidence commit `1f43dc2`. It does not include dual-worker E2E, remote CI, external IdP/provider, OTLP receiver or DR. |
| Two independent workers against MinIO | NOT VERIFIED | No E2E run | No containerized API/worker topology or crash/reclaim run in this continuation. |
| Failure Lab | PASS | `go run ./cmd/failurelab` | 12/12 database-oracle scenarios passed. |
| Chaos Matrix | PASS | `go run ./cmd/chaosmatrix`; ignored local run `benchmarks/chaos/20261009T092028Z` | 6 scenarios × 30 trials; each 30/30. Prior run directories remain untouched. |
| Go vet | PASS | `go vet ./...` | Exit 0 before final doc-only updates. |
| OpenAPI contract | PASS | Included in full unit and Race runs | Existing targeted contract checks; no generated full-schema drift gate claimed. |
| OIDC / RBAC / IDOR | PASS (local tests) | Included in full unit and Race runs | Ephemeral test JWKS and PostgreSQL integration, not a real external IdP deployment. |
| 100k pagination benchmark | NOT VERIFIED | Not run | No new performance figures claimed. |
| OTLP remote trace / backup restore / remote CI | NOT VERIFIED | Not run | No external receiver or recovery drill evidence. |
| `git diff --check` | PASS | Run after code changes | Final check will be repeated before commit. |
| Clean-commit Evidence run | NOT VERIFIED | `evidence/phase1/runs/wave2-20261009T093214275Z-0b73ce734dcd/manifest.json` and 14 raw stdout/stderr logs | Commit `0b73ce7`; 7 executed commands Exit 0; MinIO contract not run due missing env. Evidence Commit `85cf764`. |
| Later clean-commit regression run | FAIL | `evidence/phase1/runs/wave2-20261009T094212634Z-fcd9bf97ea83/manifest.json` | `TestMarkFailureWrongWorker` exposed an instantaneous DB-clock assumption in `claimOne`; the original run and raw logs/artifacts are preserved. Test-only bounded DB-clock fix is `583e019`; PostgreSQL Race ×5, RetryBackoff ×20, full unit and full Race passed after the fix. |
| Latest clean-commit run | FAIL | `evidence/phase1/runs/wave2-20261009T095347192Z-f16c2df5831f/manifest.json` | Full Race hit the 2-second bounded wait in `TestGlobalQueueRateLimitAcrossWorkersAndStableRefresh`; PostgreSQL Race ×5 and RetryBackoff ×20 passed in the same run. Root cause is test timeout under host/DB contention, not a reproduced admission defect. Immutable raw logs remain unchanged. |
| Rate-limit test correction | PASS | `go test -race ./internal/storage/postgres -run '^TestGlobalQueueRateLimitAcrossWorkersAndStableRefresh$' -count=20 -v` | 20/20 after switching the wait to PostgreSQL's calculated token balance and a 10-second hard liveness ceiling; the full required gates passed, with a fresh collector pending. |
| Required PostgreSQL Race after correction | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./internal/storage/postgres -count=5` | Exit 0; 104.001 s. |
| RetryBackoff Race after correction | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./internal/storage/postgres -run '^TestRetryBackoff$' -count=20 -v` | Exit 0; 20/20; 30.241 s. |
| Full required unit/integration after correction | PASS | `REQUIRE_TEST_DATABASE=1 go test ./cmd/... ./internal/... ./pkg/... -count=1` | Exit 0; PostgreSQL integration was required. |
| Full required Race after correction | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./cmd/... ./internal/... ./pkg/... -count=1` | Exit 0; PostgreSQL integration was required. |
| Failure Lab after correction | PASS | `REQUIRE_TEST_DATABASE=1 go run ./cmd/failurelab` | Exit 0; 12/12 database-oracle scenarios. |
| Chaos Matrix after correction | PASS | `REQUIRE_TEST_DATABASE=1 go run ./cmd/chaosmatrix`; `benchmarks/chaos/20261009T115558Z` | Exit 0; 6 scenarios × 30/30 locally. Recovery percentiles are local harness measurements, not production SLOs. |
| Vet/format/whitespace after correction | PASS | `go vet ./...`; `gofmt -l cmd internal pkg`; `git diff --check` | Exit 0; gofmt produced no paths. |
| Latest clean-start Evidence | NOT VERIFIED | `evidence/phase1/runs/wave2-20261009T115734795Z-568c45d5ad66/manifest.json` | Implementation `568c45d`; seven executed commands Exit 0; MinIO contract not run because required variables were absent. Log/artifact hashes verified; Evidence Commit `34e2570`. |

Historical Chaos failures and Phase 0 evidence are immutable and are not replaced by the current run.
