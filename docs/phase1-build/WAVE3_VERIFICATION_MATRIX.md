# Wave 3 verification matrix

| Area | Verification | Result | Evidence boundary |
|---|---|---|---|
| CI follow-up | GitHub CI Run `37962333979` at `ac46b6bf2411aa68af671897f6ebdb2e0786e323` | PASS, all 5 jobs | Includes source-built MinIO, S3 contract, process recovery, backup/restore, Postgres integration, full Race, client module. Does not validate subsequent changes. |
| Security CI | Run `37962334001` at `ac46b6b` | PASS: govulncheck, CodeQL, Secret Scan; Scorecard SKIP by event condition | No finding is represented as production security certification. |
| MinIO builder | Linux Go 1.26.9 container; full upstream MinIO source compile; fixed module/go.mod/license checksums | PASS | Docker CLI build/inspect were stubbed for this local script execution; actual image build/start was separately verified by CI `37962333979`. Binary SHA256 recorded in `remote-ci-failure-repair.md`. |
| Public client | `go -C test/external-client test -race ./... -count=1 -v` | PASS | Separate consumer module; targeted local run. CI also passed at `ac46b6b`. |
| Provider HTTP contract | `go test -race ./internal/effects -count=10` | PASS | Local `httptest` HTTP listener; test contract only, no external Provider. |
| Provider + side-effect handlers | Targeted Race ×10 and full suites in clean Evidence | PASS | Exact implementation `9b360f5`; Provider is a controlled local HTTP contract, not a real external service. |
| Clean Wave 3 Evidence | `scripts/evidence/run-wave2-with-ephemeral-minio.ps1 -RunID wave3-20261010T171000Z-9b360f5952bb` | PASS, 14/14, all Exit 0 | Exact commit `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf`, clean start/after excluding run artifacts. Manifest records logs, SHA256, configuration fingerprint, PostgreSQL 16.15, MinIO image ID/health and test artifacts. |
| Full unit/integration and Race | Included in the clean Evidence collector | PASS | `REQUIRE_TEST_DATABASE=1`; actual PostgreSQL 16.15. |
| PostgreSQL Race ×5 and RetryBackoff Race ×20 | Included in the clean Evidence collector | PASS | Exact raw logs and hashes in the Evidence run. |
| Failure Lab / Chaos / pagination benchmark | Included in the clean Evidence collector | PASS | Failure Lab 12/12; Chaos six scenarios ×30/30; pagination 100,000 rows, 201 pages, zero duplicates. Measured timings are local only. |
| MinIO S3 / host-process recovery / backup-restore | Included in the clean Evidence collector | PASS | Source-built MinIO, HTTP 200 health; two independent Worker processes (not containers); PostgreSQL 16.15 dump/restore and artifact hash check. |
| Provider + side-effect full suite | Included in full unit/integration and full Race at `9b360f5` | PASS | Local targeted Race ×10 also passed before commit. Contract only, no real provider. |
| Wave 3 remote CI | GitHub CI Run `37963245838` at `9b360f5` | PASS, all 5 jobs | Includes MinIO real image/test path; this is remote CI, not staging. |
| Wave 3 security CI | GitHub Security Run `37963245883` at `9b360f5` | PASS: govulncheck, CodeQL, Secret Scan; Scorecard SKIP | Scorecard skip is an expected PR-event condition. |
| Cross-container ArtifactStore crash/recovery and Put response loss | Compose topology and fault injection | NOT VERIFIED | Existing remote CI recovery is independent host processes, not API/Worker containers or response-loss proxy. |
| Real Provider/Sandbox | Provider-specific contract and reconciliation | NOT VERIFIED | No selected provider, credentials, sandbox, or reconciliation authority. |
| Cross-instance rate limit / cursor key rotation | shared state and rotation policy | NOT VERIFIED | Must not change production semantics without ADR/config approval. |
| Full distributed trace / alert firing | Collector and operational alert | NOT VERIFIED | Existing local OTLP receiver does not verify API-to-worker continuity or production alert closure. |
| SBOM / image vulnerability scan / release smoke | supply-chain and deployment evidence | NOT VERIFIED | Existing CI CodeQL/govulncheck do not substitute for container image scanning/SBOM. |

No Wave 2 Evidence is relabeled to cover Wave 3 code.
