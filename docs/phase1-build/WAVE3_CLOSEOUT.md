# Wave 3 engineering closeout

Date: 2026-10-10 (Asia/Shanghai)

## Outcome

`WAVE3_ENGINEERING_CLOSEOUT_PARTIAL` — the final local source snapshot passed the complete 14-gate regression and the four live cross-container gates. The required push could not complete because the configured proxy closed the GitHub TLS handshake. Therefore the revised CI workflows, current security scan, and container supply-chain workflow have not yet run remotely. No unresolved local P0 correctness failure was found in these checks.

`PRODUCTION_NOT_APPROVED`. Do not enable production task types, real external effects, production storage, or production identities from this result. Wave 4 is not started.

## Frozen source and Git state

- Starting HEAD: `49c275c21ff2c35a80e83b3f57a93916af9cf5bd` (`ee400bc39a4008694abb74c4c6d87a3356d129de` tree), clean.
- Final implementation snapshot: `0a5482d657ae431751c205abdf01632ebe89cee6`.
- Implementation tree: `91940cda4428026b4ab360b3aa00bcaab491dda7`.
- Branch: `codex/phase1-production-hardening`.
- Changes in the implementation commits are limited to CI runner/action pins and the bounded MinIO evidence-runner preflight/health-probe handling. No production Go code or database schema was changed.
- The final full regression and cross-container test both ran from clean E: worktrees at the implementation SHA. Test output and E: evidence-index changes are artifacts, not source edits.
- The Phase 0 tag `upgrade-p0-complete-20261008` remains at `e66cf722c51b740243ae6d6126eea7fe361ada04`; it was not moved.
- No merge or release was performed. The implementation push failed with a Schannel TLS handshake error; the local branch remains ahead of its origin tracking ref. The failure is preserved under the E: closeout logs.

## Engineering package closeout

| Package | Delivered and verified locally | Remaining limitation | Status |
|---|---|---|---|
| A — cross-container recovery | Four live gates passed at the frozen snapshot: MinIO response loss, API plus two Worker-container crash recovery, Worker PUT response loss/reclaim, and two-Worker same-key PUT barrier. PostgreSQL fencing, attempt/generation, immutable object SHA and final references were asserted. | Local Docker/MinIO only; the cross-container topology has no hosted CI run. This is not an exactly-once guarantee for arbitrary external effects. | LOCAL PASS; HOSTED NOT VERIFIED |
| B — Effect / Provider | Provider HTTP contract and side-effect handler Race tests passed ×10; full unit/integration and full Race also passed. Tests cover success, permanent/retryable rejection, timeout/response loss as unknown, duplicate and conflicting idempotency keys, and reconciliation states. | The adapter is a contract prototype, not wired to production admission. No selected provider, sandbox, transaction or real reconciliation authority. Keep `REAL_PROVIDER_NOT_VERIFIED`. | PARTIAL |
| C — API / Security | Stable v1 route/auth/role contract tests passed; OpenAPI Race ×20 passed. CreateTask request, payload and response schemas are checked against DTO JSON fields. Existing owner isolation, audit, authentication and fail-closed tests passed in the final suites. | Only CreateTask has systematic DTO/schema parity. Thirteen of fifteen documented v1 operations lack a response-body schema; full API drift, external IdP operations, cursor key rotation and cross-instance rate limits remain open. | PARTIAL |
| D — Observability / Operations | Worker trace continuation and OTLP receiver tests passed in final full Race. The local Prometheus API/Worker scrape previously passed at `f597477`; its observability source files are unchanged in this snapshot. | No API→DB→queue→separate Worker→MinIO/fenced ACK→real Collector end-to-end trace, production alert firing, Alertmanager/on-call, cloud DR or production SLO evidence. | PARTIAL |
| E — CI / Supply Chain | Workflows now pin Ubuntu 24.04 and signed full commit SHAs for checkout v6.1.0, setup-go v6.5.0 and upload-artifact v6.0.0. Local `actionlint` and PowerShell parser checks passed. | The new workflow versions are not hosted-verified. Prior CI/Security/Supply Chain runs passed at `49c275c`, before these workflow edits. Latest govulncheck and image scan runs are pending push. | PARTIAL |

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
- A direct local govulncheck retry could not fetch `vuln.go.dev` because the configured proxy terminated the TLS handshake. The previous Security workflow at `49c275c` passed govulncheck, CodeQL and Secret Scan, with Scorecard skipped by its PR condition; that does not verify the new workflow commit.

## Hosted CI and handoff

The last verified hosted baseline is SHA `49c275c`:

- [CI Run 38024299670](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38024299670) — SUCCESS.
- [Security Run 38024299679](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38024299679) — SUCCESS; Scorecard was an expected PR-event skip.
- [Container Supply Chain Run 38024299777](https://github.com/QuietYouthKept/distributed-task-lab/actions/runs/38024299777) — SUCCESS.

All three were bound to `49c275c`; none is represented as verification of `0a5482d`. The next action is to retry a normal push of `codex/phase1-production-hardening`, then confirm all three workflows against the pushed source/workflow snapshot and preserve their Run IDs, attempt numbers, job conclusions and head SHA. Do not force-push. PR #1 was last known as open/draft on the earlier snapshot; its live state was not retrievable during this closeout.

No further feature implementation is part of this handoff. After remote CI, remaining external-dependency decisions are listed in [WAVE3_REMAINING_GAPS.md](WAVE3_REMAINING_GAPS.md). Production approval, merge and Wave 4 require a separate explicit scope.
