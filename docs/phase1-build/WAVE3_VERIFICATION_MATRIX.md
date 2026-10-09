# Wave 3 verification matrix

| Area | Verification | Result | Evidence boundary |
|---|---|---|---|
| CI follow-up | GitHub CI Run `37962333979` at `ac46b6bf2411aa68af671897f6ebdb2e0786e323` | PASS, all 5 jobs | Includes source-built MinIO, S3 contract, process recovery, backup/restore, Postgres integration, full Race, client module. Does not validate subsequent changes. |
| Security CI | Run `37962334001` at `ac46b6b` | PASS: govulncheck, CodeQL, Secret Scan; Scorecard SKIP by event condition | No finding is represented as production security certification. |
| MinIO builder | Linux Go 1.26.9 container; full upstream MinIO source compile; fixed module/go.mod/license checksums | PASS | Docker CLI build/inspect were stubbed for this local script execution; actual image build/start was separately verified by CI `37962333979`. Binary SHA256 recorded in `remote-ci-failure-repair.md`. |
| Public client | `go -C test/external-client test -race ./... -count=1 -v` | PASS | Separate consumer module; targeted local run. CI also passed at `ac46b6b`. |
| Provider HTTP contract | `go test -race ./internal/effects -count=10` | PASS | Local `httptest` HTTP listener; test contract only, no external Provider. |
| Provider + side-effect handlers | `go test -race ./internal/effects ./internal/worker/handlers -run '^(TestHTTPProvider|TestSideEffect)' -count=10` | PASS | Repeated local checks on uncommitted Wave 3 work; clean-commit re-run pending. |
| PostgreSQL / full repository | Wave 3 clean-commit required suite | NOT VERIFIED | Run after implementation commit. |
| Cross-container ArtifactStore crash/recovery and Put response loss | Compose topology and fault injection | NOT VERIFIED | Existing remote CI recovery is independent host processes, not API/Worker containers or response-loss proxy. |
| Real Provider/Sandbox | Provider-specific contract and reconciliation | NOT VERIFIED | No selected provider, credentials, sandbox, or reconciliation authority. |
| Cross-instance rate limit / cursor key rotation | shared state and rotation policy | NOT VERIFIED | Must not change production semantics without ADR/config approval. |
| Full distributed trace / alert firing | Collector and operational alert | NOT VERIFIED | Existing local OTLP receiver does not verify API-to-worker continuity or production alert closure. |
| SBOM / image vulnerability scan / release smoke | supply-chain and deployment evidence | NOT VERIFIED | Existing CI CodeQL/govulncheck do not substitute for container image scanning/SBOM. |

No Wave 2 Evidence is relabeled to cover Wave 3 code.
