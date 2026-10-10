# Wave 3 progress

Updated: 2026-10-10 (Asia/Shanghai)

## Recovery point

- Branch: `codex/phase1-production-hardening`.
- Latest pushed implementation commit: `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` (safe ingress bucket reclamation and ADR-0006); its separate Evidence commit `8cd730018d01bfb16512b1a9480771f6f4583584` is also pushed.
- New local commits not yet pushed: implementation `9b81e96b498b05d92469efb41c53c6c1fe6acedf` and diagnostic/attempt Evidence `4129483ce5228c00070911bf3c41d747bdd6d62a`.
- Evidence for `9b360f5`: `0f12b94dfa33cd6fb0ed8ea1a30692100541ddd5` (pushed).
- PR #1 remains OPEN and Draft.
- CI Run `37962333979`: PASS, including actual source-built MinIO image, S3 contract, API/two-Worker recovery, PostgreSQL backup/restore, PostgreSQL integration, full Race, and public Go Client module.
- Security Run `37962334001`: govulncheck, CodeQL, and Secret Scan PASS; Scorecard was conditionally SKIPPED.
- CI Run `37963245838` at `9b360f5`: all five jobs PASS. Security Run `37963245883`: govulncheck, CodeQL, Secret Scan PASS; Scorecard was conditionally SKIPPED.
- CI Run `37965020508` at `cedbdc1`: all five jobs PASS, including real MinIO image build/start, S3 contract, API/two-Worker process recovery, backup/restore, PostgreSQL Integration, full Race and public client. Security Run `37965020486`: govulncheck, CodeQL and Secret Scan PASS; Scorecard was conditionally SKIPPED.
- Prior failed runs `37956337334`, `37956336466`, and `37960805439` remain unchanged.
- Wave 2 Evidence `wave2-20261009T153618132Z-a292ec7a996b` stays bound to implementation `a292ec7a996bc39214ab413c420eacd9268a7298`; it does not validate new code.

## Completed in this continuation

- CI follow-up fix was pushed as `ac46b6b`; remote CI and Security results above are bound only to that SHA.
- The MinIO JSON checksum parser now accepts the trailing comma emitted by Go 1.26.9; failures report non-secret diagnostics. Module and license hashes are verified before image construction.
- `test/external-client/go.mod` now targets the root module's Go 1.26.9 toolchain.
- Began Wave 3 package B: added provider-independent HTTP Provider port and contract client with outcomes that distinguish not-sent, confirmed rejection/success, explicit retryable rejection, and unknown. Added HTTP contract/fault tests and made the side-effect handler surface errors when persisting Unknown to the Effect Ledger fails.
- Production Worker allowlist remains empty/fail-closed. The new HTTP contract is not wired into production handler selection.
- Clean local Evidence `wave3-20261010T171000Z-9b360f5952bb`: 14/14 PASS, exact implementation `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf`, clean start and clean after excluding the run artifacts. PostgreSQL 16.15, Docker 28.3.3, Go 1.26.9, temporary source-built MinIO image; all command-log hashes and benchmark/chaos artifact hashes are in the manifest.
- A first Evidence-wrapper preflight did not start tests because Git Bash `curl` lacked the local proxy and the license fetch was reset. The `NOT VERIFIED` preflight and observed log are preserved separately; retry through `127.0.0.1:7892` completed 14/14.

## Package C completed locally; Evidence commit pending

- Inspection found the 10,000-entry process-local ingress limiter never reclaimed inactive buckets; after enough distinct principals, new identities could remain rejected until process restart.
- Commit `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` reclaims only buckets whose normal refill calculation would already restore the full burst; no cluster-wide rate-limit semantics were introduced.
- Regression test checks just-before and exact full refill. Targeted Race ×100 and vet passed; full clean Evidence run `wave3-20261010T171600Z-cedbdc1edb09` passed 14/14 against the exact commit, with clean start and clean after excluding run artifacts. Manifest, raw logs, and hashed Chaos/pagination artifacts are preserved in that run directory.
- CI Run `37965020508` and Security Run `37965020486` both completed successfully for `cedbdc1`; Scorecard was conditionally skipped by its PR-event condition.
- ADR-0006 proposes, without enabling, shared-quota and cursor-key-rotation decision requirements. No cursor format/config changed.

## Wave 3 package A — cross-container E2E in progress

- Added isolated Compose services for PostgreSQL, source-built MinIO, API, and two separately built Worker containers. The Compose project, network, ports, credentials, and artifact prefix are unique per run; the two workers have no shared local filesystem.
- `TestAPIWorkerContainersRecoverSharedArtifact` has passed against the live containers: a worker wrote a MinIO artifact while ACK was blocked by a real PostgreSQL row lock; the owning container was killed; the other worker reclaimed at attempt/generation 2, reused the object, and completed. The test independently re-reads object bytes from MinIO, checks SHA256 against the task result reference, rejects stale generation ACK, and confirms exactly one succeeded event.
- The real MinIO contract drops successful PUT responses after persistence and verification HEAD responses, expects `ErrUnknownOutcome`, reads the persisted object through a direct S3 client, then retries the same content and verifies hash reuse. It also seeds an existing object with invalid SHA256 metadata and requires both `ErrUnknownOutcome` and `ErrCorrupt`, plus concurrent conditional writes from independent S3 client instances (same-key/different-content and same-key/same-content).
- Local run `tasklab-e2e-e71c60f64c06` passed both targeted Race tests using proxy `127.0.0.1:7897`, but its manifest correctly marks `worktree_dirty_at_test: true` and binds the then-current `8cd7300` HEAD; treat it as diagnostic, not final commit-bound Evidence. The earlier API-readiness and proxy-fault-fixture failures are preserved under `evidence/phase1/runs/wave3-container-e2e-attempts/` and must not be rewritten.
- The exact clean rerun `tasklab-e2e-980f3ba036b3` is bound to implementation `9b81e96b498b05d92469efb41c53c6c1fe6acedf`, with `worktree_dirty_at_test: false`; its append-only Evidence is committed separately as `962ec4e33c2c907a73eae628e9f9d39dd2704acb`.
- Full Evidence run `wave3-20261010T172000Z-962ec4e` is preserved as FAIL (13 FAIL, 1 NOT VERIFIED). Root cause is environmental/toolchain selection, not 13 independent code failures: the collector inherited `GOTOOLCHAIN=local` and invoked installed Go 1.26.6 while `go.mod` requires Go 1.26.9; all Go commands exited before package loading with the same message. The benchmark consequently did not run. The wrapper now temporarily sets `GOTOOLCHAIN=auto` and restores the caller's setting; rerun the full collector against this committed fix before making suite claims.
- After fixing toolchain selection, clean full Evidence run `wave3-20261010T173000Z-80b7baa` passed 14/14 on collector/tooling snapshot `eafc0a83037d24273eb8cfae881c94a8c7ce3d53`, clean at start and after excluding run artifacts. Environment: Go 1.26.9, Windows AMD64, Docker 28.3.3, PostgreSQL 16.15, source-built MinIO HTTP 200. This covers the established 14-gate suite, including full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab, Chaos, 100k pagination, MinIO, process-based recovery and backup/restore; the new cross-container harness is recorded separately. Its append-only Evidence commit remains to be created.
- Remaining package-A gaps: inject PUT response loss in the actual Worker container path (the current response-loss contract exercises the real S3 adapter through a fault proxy separately from the container crash test) and simultaneous same-case writes from two Worker containers. No exactly-once claim is made.
- Package A was strengthened in local implementation commit `3eef1603293aa0bbc45877bb8263699f09c9e2b5`: the MinIO contract now seeds an existing object with invalid SHA256 metadata and requires both `ErrUnknownOutcome` and `ErrCorrupt`; competing same-key writes now originate from independent S3 client instances. `run-cross-container.ps1` also temporarily selects `GOTOOLCHAIN=auto`, preserving/restoring the caller value. Full regression on the latest fixture-corrected source passed in run `wave3-20261010T180000Z-22478c8`.
- Exact clean run `tasklab-e2e-d55fb468070f` passed both targeted Race gates at implementation `3eef160`, with `worktree_dirty_at_test: false`. Its MinIO response-loss + metadata contract and API/Worker A/Worker B crash recovery logs are SHA256-recorded, with random credentials omitted; the run and index entry are committed. Full 14-gate run `wave3-20261010T180000Z-22478c8` then passed after the database-clock lease fixture correction.
- Two additional attempts are preserved: `tasklab-e2e-51d7f822ec1c` stopped before package loading under the old `GOTOOLCHAIN=local`; `tasklab-e2e-e78ee25a94b4` exposed an overly strict test expectation (the safe actual classification includes both unknown outcome and corrupt metadata). Neither represents a production scheduler/storage regression.
- The full collector rerun `wave3-20261010T174500Z-67eb27c` is preserved as FAIL (13/14). The single failure was `TestExpiredRunningReclaimIgnoresCapacity`: a 2ms lease plus `time.Sleep(15ms)` in the test was nondeterministic under Race ×5; there is no production-code change indicated. Its fixture now starts with a normal lease and explicitly expires the row with PostgreSQL `clock_timestamp()`, checking the affected-row count; the same treatment was applied to the stale-renew fixture. Targeted Race ×20 of both tests passed, and the subsequent clean full run passed 14/14 as recorded below.
- After that fixture correction, full collector run `wave3-20261010T180000Z-22478c8` passed 14/14 (all exit 0) on exact snapshot `22478c8986b8d1c7d6b89d76c6249304f25c8cda`, clean start/after. It includes unit/integration, full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab 12/12, Chaos Matrix, 100K pagination, vet, external client, OTLP, MinIO, process recovery and backup/restore; Go 1.26.9, PostgreSQL 16.15, source-built MinIO HTTP 200. Command logs and benchmark/chaos artifacts are hashed in the manifest. Separate Evidence commit is pending.

## Remaining work and next commands

1. Commit the full 14/14 Evidence for `eafc0a8` separately, preserving the preceding toolchain-blocked run; then continue with Package A's remaining Worker-path fault injection and independent D/E gaps.
2. Continue independent packages: OpenAPI schema coverage; full trace/alerts/backup operations; image/SBOM/release readiness.
3. Preserve Production Fail-Closed. External IdP, real Provider/Sandbox, cloud S3/DR, production RPO/RTO, and SLO/on-call remain unverified.

Latest resumable command after checking status: `git status -sb; git rev-parse HEAD; go test -race ./internal/api -run '^TestIngress' -count=100`.
