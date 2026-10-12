# General 18-Category Test Coverage Matrix

Scope: current source/test inventory plus dated historical evidence. Status legend follows the request: `PASS` means current-source evidence exists; `FAIL` means an executed test failed; `SKIP` means an executed test was explicitly skipped; `NOT_RUN` means no current run; `BLOCKED` means a required environment/authority is absent; `NOT_APPLICABLE` only where the category does not fit. Historical PASS is called out explicitly and is not promoted to current PASS.

| # | Category | Existing source/test entry points | Current status | Acceptance gap / next action |
|---:|---|---|---|---|
| 1 | Unit | Go `*_test.go`; `scripts/replay/test_azure_llm.py`; `cmd/loadgen/artifacts_test.go` | NOT_RUN for Go against current HEAD; replay 7/7 passed on the dirty worktree and loadgen targeted unit test passed on dirty source | Run Go unit suite against an exact committed source when resource gate allows; record skips. |
| 2 | Integration | PostgreSQL integration package and `TEST_DATABASE_URL` harness | BLOCKED / NOT_RUN current | Run isolated PostgreSQL migrations, role grants, repository, effects and limiter tests; historical 0a PASS only. |
| 3 | API | `internal/api` router/handler tests, OpenAPI contract tests | NOT_RUN current; historical tests exist | Current DB-backed HTTP acceptance and auth/error contract needed. |
| 4 | E2E | Compose/deploy/evidence tooling; historical cross-container tests | NOT_RUN current | Current API→DB→Worker→Gateway→Artifact→fenced ACK is not executed. |
| 5 | Regression | named regressions including retry, stale generation, cursor and crash tests | NOT_RUN current | Re-run full targeted regressions against current source; retain old failing evidence unchanged. |
| 6 | Contract | OpenAPI contract tests; S3/MinIO contract tests | NOT_RUN current | Current MinIO/S3 provider contract and public client compatibility unverified. |
| 7 | Database | `internal/storage/postgres/*_integration_test.go` | BLOCKED / NOT_RUN current | Need trusted isolated PostgreSQL runtime; static inventory is not a database PASS. |
| 8 | Concurrency | concurrent claim, queue capacity, idempotency, limiter tests | NOT_RUN current; historical DB tests exist | Current-source race/integration, multi-instance and 100-logical-worker limits needed. |
| 9 | Performance | `cmd/loadgen`, `benchmarks/workers-*` | Historical PASS only, source `cd3696e` | Keep historical numbers labeled; run current source only after controlled DB/resource gate. |
| 10 | Load | loadgen constant/ramp/spike/backlog-drain profiles | NOT_RUN current | No current task execution; submission window and handler mode must remain reported separately. |
| 11 | Stress | no dedicated current stress runner; loadgen is bounded | NOT_RUN | No high-scale stress executed; define resource guard and DB assertions first. |
| 12 | Soak | no dedicated runner found | NOT_RUN | Implement/execute bounded 1h smoke only when isolated host/resources approved; longer durations remain external. |
| 13 | Security | JWT/JWKS, role, owner isolation, SQL grants, endpoint safety tests | NOT_RUN current | Current static review indicates worker table-level UPDATE/fencing boundary needs direct SQL negative proof; no security scan rerun here. |
| 14 | Chaos | `cmd/chaosmatrix`, historical cross-container crash matrix | NOT_RUN current; historical PASS exists | Current-source controlled Docker/PostgreSQL/MinIO chaos required; preserve fail logs and run ID. |
| 15 | Recovery | backup/restore integration and lease reclaim tests | NOT_RUN current | Independent PostgreSQL+object restore with SHA/reference validation not run on current source. |
| 16 | Deployment/Configuration | Compose, Dockerfiles, Terraform, cloud scripts | STATIC INVENTORY ONLY; runtime NOT_RUN | Current Compose/Terraform/Linux deployment not validated by this turn; actual cloud not authorized. |
| 17 | Observability | OTel, metrics, alert rules/receiver code | NOT_RUN current | Cross-process trace propagation and Normal→Firing→Delivered→Resolved not executed. |
| 18 | Release/Rollback | migrations, image digests, compatibility tests | NOT_RUN current | Historical migration evidence does not verify current SHA; Migration 012 compatibility and app rollback remain gated. |

Current in-turn evidence is limited to the replay generator suite and bounded replay file generation, described in [03_HISTORICAL_EVIDENCE_AUDIT.md](03_HISTORICAL_EVIDENCE_AUDIT.md). Historical PostgreSQL and benchmark evidence retains its original source binding and cannot be described as current acceptance.
