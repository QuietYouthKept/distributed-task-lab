# Wave 3 engineering closeout

Date: 2026-10-10 (Asia/Shanghai)

## Outcome

`WAVE3_ENGINEERING_CLOSEOUT_COMPLETE` **for the bounded engineering verification and delivery milestone**. The final implementation snapshot `0a5482d` passed the clean 14/14 local regression and four live cross-container gates; its documentation-only descendant `ad52365b` was pushed successfully and CI, Security, and Container Supply Chain all completed SUCCESS on the first attempt. No local P0 correctness failure was reproduced by these checks. Packages B–E retain explicitly documented capability/production gaps; this status does **not** mean full Wave 3 feature or production acceptance.

`PRODUCTION_NOT_APPROVED`. Do not enable production task types, real external effects, production storage, or production identities from this result. Wave 4 is not started.

## Frozen source and Git state

- Starting HEAD: `49c275c21ff2c35a80e83b3f57a93916af9cf5bd` (`ee400bc39a4008694abb74c4c6d87a3356d129de` tree), clean.
- Final implementation snapshot: `0a5482d657ae431751c205abdf01632ebe89cee6`.
- Implementation tree: `91940cda4428026b4ab360b3aa00bcaab491dda7`.
- Branch: `codex/phase1-production-hardening`.
- Changes in the implementation commits are limited to CI runner/action pins and the bounded MinIO evidence-runner preflight/health-probe handling. No production Go code or database schema was changed.
- The final full regression and cross-container test both ran from clean E: worktrees at the implementation SHA. Test output and E: evidence-index changes are artifacts, not source edits.
- The Phase 0 tag `upgrade-p0-complete-20261008` remains at `e66cf722c51b740243ae6d6126eea7fe361ada04`; it was not moved.
- No merge or release was performed. The earlier Schannel TLS handshake failures are preserved as historical logs. A later normal (non-force) push through proxy `127.0.0.1:7892` advanced the remote branch to `ad52365b`. This documentation-only descendant adds no production-code or schema changes after the locally tested `0a5482d` implementation.

## Engineering package closeout

| Package | Delivered and verified locally | Remaining limitation | Status |
|---|---|---|---|
| A — cross-container recovery | Four live gates passed at the frozen snapshot: MinIO response loss, API plus two Worker-container crash recovery, Worker PUT response loss/reclaim, and two-Worker same-key PUT barrier. PostgreSQL fencing, attempt/generation, immutable object SHA and final references were asserted. | Local Docker/MinIO only; the cross-container topology has no hosted CI run. This is not an exactly-once guarantee for arbitrary external effects. | LOCAL PASS; HOSTED NOT VERIFIED |
| B — Effect / Provider | Provider HTTP contract and side-effect handler Race tests passed ×10; full unit/integration and full Race also passed. Tests cover success, permanent/retryable rejection, timeout/response loss as unknown, duplicate and conflicting idempotency keys, and reconciliation states. | The adapter is a contract prototype, not wired to production admission. No selected provider, sandbox, transaction or real reconciliation authority. Keep `REAL_PROVIDER_NOT_VERIFIED`. | PARTIAL |
| C — API / Security | Stable v1 route/auth/role contract tests passed; OpenAPI Race ×20 passed. CreateTask request, payload and response schemas are checked against DTO JSON fields. Existing owner isolation, audit, authentication and fail-closed tests passed in the final suites. | Only CreateTask has systematic DTO/schema parity. Thirteen of fifteen documented v1 operations lack a response-body schema; full API drift, external IdP operations, cursor key rotation and cross-instance rate limits remain open. | PARTIAL |
| D — Observability / Operations | Worker trace continuation and OTLP receiver tests passed in final full Race. The local Prometheus API/Worker scrape previously passed at `f597477`; its observability source files are unchanged in this snapshot. | No API→DB→queue→separate Worker→MinIO/fenced ACK→real Collector end-to-end trace, production alert firing, Alertmanager/on-call, cloud DR or production SLO evidence. | PARTIAL |
| E — CI / Supply Chain | Workflows pin Ubuntu 24.04 and immutable action commit SHAs; local `actionlint` and PowerShell parser checks passed. | Updated CI, Security, and Supply Chain workflows all passed in hosted GitHub Actions at `ad52365b`; publishing/signing, cloud staging, and release/rollback approval remain open. | HOSTED PASS for current workflow; PRODUCTION PARTIAL |

## Final local verification

The clean-worktree full run `wave3-final-0a5482d-20261010` passed 14/14 with exit code 0, bound to the exact implementation SHA. It includes unit/integration, full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab, Chaos Matrix, 100K PostgreSQL keyset benchmark, vet, external client Race, OTLP receiver Race, authenticated MinIO contract Race, API/two-Worker process recovery, and PostgreSQL backup/restore. Manifest and all command-log hashes were checked; Chaos and pagination artifact hashes matched.

The separate clean-worktree cross-container run `tasklab-e2e-50ff56931d69` passed 4/4. It used a source-built MinIO image from upstream commit `9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a`, PostgreSQL 16, one API container and two independent Worker containers. The two workers reached the same-key PUT barrier before MinIO accepted either write; both results referenced the same verified object SHA. Logs and manifests report no credentials.

Targeted follow-ups also passed on the same implementation snapshot:

- `go test -race ./internal/effects ./internal/worker/handlers -count=10`.
- `go test -race ./internal/api -run '^TestOpenAPI' -count=20`.
- `actionlint` on all three workflows and PowerShell syntax parsing of the evidence runner.
- A Linux/amd64 `CGO_ENABLED=0` Worker build matching the Dockerfile toolchain; `openpgp` package/import and symbol checks were empty.

Full manifests, logs, copied redacted/original MinIO logs and test binaries are under `E:\distributed-task-lab-temp\wave3-artifacts\final-closeout-20261010-0a5482d` (the clean source worktrees and build caches remain in the adjacent initial closeout workspace whose directory name includes `49c275c`). The full run manifest itself records each command, timestamps, exit codes and hashes.

## Security closeout

- The stopped local `tasklab-wave2-minio` instance had its test credential rotated. Only that container was replaced; the existing `tasklab-wave2-minio-data` volume was preserved. The replacement is stopped, matching its pre-task stopped state. The new randomly generated credentials are in user/SYSTEM-only files under `E:\distributed-task-lab-temp\local-secrets\tasklab-wave2-minio-20261010`, mounted read-only when the container is started; they are not in container environment values, repo files, or test output. A real MinIO S3 contract run passed with the rotated credential.
- Docker logs for the prior container and rotation runs remain ACL-restricted. Each has an original and review copy; the pairs have identical SHA256 because the inspected log content contained no credential values. The user's separately saved terminal logs were not located, so their contents and any external credential reuse cannot be certified. Rotate any reused credential in other environments; no past chat or terminal disclosure can be retracted.
- `GO-2026-5932` applies to `golang.org/x/crypto/openpgp` and its subpackages, all versions, with no known fixed version. The final Worker module graph contains `golang.org/x/crypto v0.57.0` through MinIO encryption/Argon2, but the Linux Worker package graph and binary contain no `openpgp` package/symbol. The finding remains as a module-level `UNKNOWN`; see [WAVE3_VULNERABILITY_TRIAGE.md](WAVE3_VULNERABILITY_TRIAGE.md).
- The final local standalone govulncheck attempt remains BLOCKED by an EOF fetching `vuln.go.dev`; its failed log is preserved. Separately, the updated hosted Security workflow at documentation-only descendant `ad52365b` passed govulncheck, CodeQL, and Secret Scan (Scorecard was expected SKIPPED). This closes the *hosted verification* gap, not the residual Trivy module-level `GO-2026-5932` UNKNOWN finding.

## Final hosted CI verification and handoff

The frozen implementation was `0a5482d657ae431751c205abdf01632ebe89cee6` (tree `91940cda4428026b4ab360b3aa00bcaab491dda7`). Commit comparison confirmed that `ad52365b7dae2ddb3e05ec32f310d626e0583611` is its **documentation-only** descendant; no production source or database schema changed between the two revisions.

The following hosted runs all completed **SUCCESS**, first attempt, against the **same pushed SHA `ad52365b7dae2ddb3e05ec32f310d626e0583611`**:

| Workflow | Run | Verified results |
|---|---|---|
| CI | [38029233771](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233771) | Five jobs PASS: format/vet/unit/build; required PostgreSQL integration; PostgreSQL Race; authenticated MinIO S3 contract, two Worker **process** recovery and backup/restore; independent public Go client |
| Security | [38029233767](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233767) | govulncheck, CodeQL and Secret Scan PASS; Scorecard expected SKIPPED for PR event |
| Container Supply Chain | [38029233773](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38029233773) | API and Worker image build, SBOM and image vulnerability scan job PASS; the separately triaged module-level advisory remains documented |

All three were `completed/success` with `run_attempt=1`. This verifies the updated Ubuntu/action-pinned workflows at `ad52365b`; it does **not** independently run the local two-Worker **container** topology, 14-gate Chaos/performance matrix, real Provider sandbox, full async trace, or staging. Those retain their own local or NOT VERIFIED classifications. The previous baseline successes at `49c275c` remain historical, and the original failed pushes and local govulncheck EOF logs are not overwritten.

PR [#1](https://github.com/QuietYouthKept/distributed-task-lab/pull/1) is open and **Draft**, targeting `codex/p0-verified-evidence-20261008`. No merge, tag, release, image publication, worker allowlist change, or production action was performed.

This remote-verification archival is documentation-only. A later archive commit must not retroactively rebind these runs to itself. The stable, resume-ready baseline is implementation `0a5482d` plus hosted verification at its documentation-only descendant `ad52365b`; any future source/workflow change requires its own gate checks.

**Disposition:** scoped engineering closeout **COMPLETE**; remaining functionality and staging/production gates **PARTIAL / NOT VERIFIED**; `PRODUCTION_NOT_APPROVED`. Wave 4 is not started. See [WAVE3_REMAINING_GAPS.md](WAVE3_REMAINING_GAPS.md) before any new engineering scope.
