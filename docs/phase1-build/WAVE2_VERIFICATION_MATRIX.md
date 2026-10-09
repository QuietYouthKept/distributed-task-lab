# Wave 2 verification matrix

Environment observed: Windows AMD64, Go 1.26.6, Docker Linux Engine; PostgreSQL 16 container `tasklab-test-postgres` on host port 5433; Docker MinIO health at `127.0.0.1:19000` and Windows MinIO health at `127.0.0.1:9000`. Current process had no MinIO credential environment variables. Historical evidence is not relabeled as current evidence.

| Verification | Result | Command / evidence | Notes |
|---|---|---|---|
| Unit + PostgreSQL integration | PASS | `REQUIRE_TEST_DATABASE=1 go test ./cmd/... ./internal/... ./pkg/... -count=1` | Reran after Store/handler changes; DB tests were required, not skipped. |
| Full Race | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./cmd/... ./internal/... ./pkg/... -count=1` | Reran after Store/handler changes. |
| PostgreSQL Race repetition | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./internal/storage/postgres -count=5` | Exit 0; 98.960 s. |
| Retry backoff regression | PASS | `REQUIRE_TEST_DATABASE=1 go test -race ./internal/storage/postgres -run '^TestRetryBackoff$' -count=20 -v` | 20/20; database clock remains the due-time authority. |
| Store and handler tests | PASS | `go test ./cmd/worker ./internal/artifacts ./internal/worker/handlers -count=1`; matching `-race` run | Local Store only; does not substitute for MinIO. |
| MinIO adapter contract | NOT VERIFIED | `TestMinIOStoreContractAndConcurrentConditionalWrite` not run | Health is 200, but no credential variables were available in this execution context. |
| Two independent workers against MinIO | NOT VERIFIED | No E2E run | No containerized API/worker topology or crash/reclaim run in this continuation. |
| Failure Lab | PASS | `go run ./cmd/failurelab` | 12/12 database-oracle scenarios passed. |
| Chaos Matrix | PASS | `go run ./cmd/chaosmatrix`; ignored local run `benchmarks/chaos/20261009T092028Z` | 6 scenarios × 30 trials; each 30/30. Prior run directories remain untouched. |
| Go vet | PASS | `go vet ./...` | Exit 0 before final doc-only updates. |
| OpenAPI contract | PASS | Included in full unit and Race runs | Existing targeted contract checks; no generated full-schema drift gate claimed. |
| OIDC / RBAC / IDOR | PASS (local tests) | Included in full unit and Race runs | Ephemeral test JWKS and PostgreSQL integration, not a real external IdP deployment. |
| 100k pagination benchmark | NOT VERIFIED | Not run | No new performance figures claimed. |
| OTLP remote trace / backup restore / remote CI | NOT VERIFIED | Not run | No external receiver or recovery drill evidence. |
| `git diff --check` | PASS | Run after code changes | Final check will be repeated before commit. |

Historical Chaos failures and Phase 0 evidence are immutable and are not replaced by the current run.
