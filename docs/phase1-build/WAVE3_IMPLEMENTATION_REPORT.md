# Wave 3 implementation report

Date: 2026-10-10 (Asia/Shanghai). This report separates code, local verification, remote CI, staging, and production claims.

## Stage 0 — CI follow-up closure

Implementation commit: `ac46b6bf2411aa68af671897f6ebdb2e0786e323`.

- Fixed `GoModSum` parsing for Go's `go mod download -json` output, whose field contains a trailing comma. The prior silent `test` failure is now diagnosed explicitly.
- Pinned and SHA-256 verified the upstream MinIO license fetched from the same source commit as the Go module; source module checksums remain fixed.
- Updated the separate consumer module's Go directive to 1.26.9.
- Remote CI Run `37962333979` passed all five jobs, including real MinIO image construction/start and required S3, recovery and backup/restore tests. Security Run `37962334001` passed CodeQL, govulncheck and Secret Scan; Scorecard was skipped by its PR-event condition.
- No production API, database schema, or worker admission policy changed in this commit. The original failed Runs remain immutable.

## Package B — provider-independent HTTP contract (implemented; provider integration remains open)

New source: `internal/effects/http_provider.go`; new tests: `internal/effects/http_provider_test.go`; decision: `docs/adr/ADR-0005-provider-independent-http-contract.md`.

The port carries operation/idempotency/request/task identity, lease generation and attempt. It treats a transport timeout or reset after dispatch, 5xx, and malformed success as Unknown. A pre-dispatch cancellation is Not Sent. The local contract tests cover success, permanent and retryable rejection, 5xx, malformed response, redirect, timeout, connection reset, duplicate key replay, same-key/different-payload rejection, and reconciliation states. Provider response bodies are not copied into returned errors.

`internal/worker/handlers/side_effect.go` now returns Effect Ledger persistence failures together with the original uncertain outcome instead of silently discarding the `MarkUnknown` error.

This is a real local HTTP contract/fault-injection test, not a real payment/notification Provider integration. The production handler is not wired to this adapter. `REAL_PROVIDER_NOT_VERIFIED` remains true.

Implementation commit: `9b360f5952bb745fcbd25dfcb32c7c9bbe20f1bf`. Clean Evidence `wave3-20261010T171000Z-9b360f5952bb` passed 14/14 against this commit. Its new provider and side-effect tests passed within full unit and full Race suites, plus the targeted Race ×10 run preceding the commit.

The same commit passed remote CI Run `37963245838` and Security Run `37963245883`. Remote CI includes PostgreSQL Integration, full Race, the actual source-built MinIO image, S3 Contract, host-process API/two-Worker recovery, PostgreSQL backup/restore, and external client. The local clean Evidence additionally recorded Failure Lab, Chaos 6×30, 100k pagination, vet, OTLP receiver, and all hashes. These are development/local validations, not staging or production evidence.

The full local Evidence environment was Windows AMD64, Go 1.26.9, Docker Engine 28.3.3, PostgreSQL 16.15, and an ephemeral source-built MinIO image with local image ID `sha256:7c340841a5a8e0f7d0013b6e5e8e03bde8beb9cb009608e37de6f79c9a19e2cc` (not a Registry Digest). The MinIO health endpoint returned HTTP 200. Backup/restore measured about 358 ms for dump and 420 ms for restore in this isolated fixture. Those timings are not an RPO/RTO commitment.

The 100,000-row keyset traversal completed 201 pages with zero duplicates; local middle-cursor P50/P95 was 4.123/4.411 ms and owner-filtered P50/P95 was 5.572/7.141 ms on the recorded fixture. The six Chaos scenarios passed 30/30 each. Recorded recovery percentiles are local harness measurements and not production SLOs. See the manifest and copied artifacts for query plans and exact configuration.

## Package C — local rate-limit bucket lifecycle fix and policy proposal

Review confirmed that `internal/api/router.go` kept a hard-bounded map of 10,000 verified Principal buckets but never evicted any. Once filled, even inactive identities remained unable to obtain a bucket until process restart. Commit `cedbdc1edb09a5abf4225ecdbc288ab7e623389d` reclaims only a bucket whose available tokens would already have reached the configured burst under normal refill. `TestIngressLimiterReclaimsOnlyFullyRefilledIdleBuckets` checks just-before-refill denial and exact-refill reclamation; targeted Race ×100 and vet passed.

Clean Evidence `wave3-20261010T171600Z-cedbdc1edb09` passed 14/14 against the exact implementation commit, including full tests, full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab 12/12, Chaos, pagination, MinIO contract, process-recovery and backup/restore. CI Run `37965020508` and Security Run `37965020486` also passed for this commit (Scorecard was conditionally skipped).

This is only process-local bucket lifecycle management. It does not implement cluster-wide quotas. ADR-0006 records the decisions needed for shared quotas and cursor key rotation; no cursor format or production setting changed.

## Package A — isolated container recovery and S3 response loss (local implementation verified; broader package remains partial)

Implementation commit: `9b81e96b498b05d92469efb41c53c6c1fe6acedf`; diagnostic/attempt Evidence commit: `4129483ce5228c00070911bf3c41d747bdd6d62a`; clean targeted Evidence commit: `962ec4e33c2c907a73eae628e9f9d39dd2704acb`.

Follow-up implementation `3eef1603293aa0bbc45877bb8263699f09c9e2b5` strengthens the real MinIO contract: the injected object with invalid `sha256` metadata must surface both `ErrUnknownOutcome` and `ErrCorrupt`, and same-key concurrent writes use distinct S3 client instances. The cross-container runner now temporarily selects the module-required Go toolchain and restores the caller setting. Exact clean run `tasklab-e2e-d55fb468070f` passed the MinIO contract and API/two-worker recovery at `3eef160`; separate Evidence commit and full-suite rerun remain pending.

The new Compose topology starts a disposable PostgreSQL, source-built pinned MinIO, API, and two separately built Worker containers with independent filesystems and a unique project network. The runner generates one-run credentials and random host ports, checks the MinIO live endpoint, saves redacted diagnostics, and removes only its own Compose project and temporary MinIO image. It accepts proxy configuration through the caller's environment and does not embed a user-specific proxy port.

The exact clean implementation run `tasklab-e2e-980f3ba036b3` passed both targeted Race gates. `TestMinIOStoreContractAndConcurrentConditionalWrite` uses a local fault proxy in front of real MinIO to drop successful PUT and verification HEAD responses after persistence; the adapter reports `ErrUnknownOutcome`, a direct S3 read verifies stored bytes/hash, and a same-content retry reconciles successfully. Existing real-MinIO assertions cover same-key/different-content conflict and concurrent conditional writes.

`TestAPIWorkerContainersRecoverSharedArtifact` runs API and two separate Worker containers. A real PostgreSQL row lock blocks ACK after the first worker writes its artifact; the exact owner container is killed; the second worker reclaims the lease at attempt/generation 2 and reports reuse. The test independently reads the output from MinIO, checks the DB result hash/reference, verifies stale-generation `MarkSuccess` is rejected, and checks exactly one succeeded event. The run manifest records implementation SHA, MinIO source commit/image ID, PASS statuses, and per-log SHA256/byte counts; `worktree_dirty_at_test` is false and generated credentials are not recorded. Evidence is indexed under `wave3-container-e2e-tasklab-e2e-980f3ba036b3`.

Earlier failed fixture runs and one pre-commit diagnostic pass remain preserved under their original paths and statuses; none is edited to PASS. Remaining package-A work: inject the response-loss fault inside the Worker container execution path (current real S3 response-loss contract is a separate test), verify malformed object metadata recovery, and run same-case simultaneous writes from two Worker containers. This remains at-least-once processing and does not imply end-to-end exactly-once execution.

The first full Wave 3 Evidence collector attempt at `962ec4e` failed before all Go commands could load packages because the host set `GOTOOLCHAIN=local` with Go 1.26.6 while the module requires Go 1.26.9. The 13 matching failures are an environment/toolchain gate, not package-test outcomes; the benchmark was not run. The immutable run and logs are retained. The wrapper was corrected to temporarily select `GOTOOLCHAIN=auto`, honor the module directive, and restore the prior environment; the subsequent full rerun is recorded below.

That toolchain fix was verified by full clean Evidence run `wave3-20261010T173000Z-80b7baa`: 14/14 PASS, all exit codes 0, clean at start and after excluding run artifacts, exact recorded snapshot `eafc0a83037d24273eb8cfae881c94a8c7ce3d53`. The collector used Go 1.26.9, PostgreSQL 16.15, Docker 28.3.3 and source-built MinIO (HTTP 200). It exercised unit/integration, full Race, PostgreSQL Race ×5, RetryBackoff Race ×20, Failure Lab, Chaos Matrix, 100K pagination, vet, external client, OTLP receiver, MinIO S3 contract, existing API/two-process recovery and PostgreSQL backup/restore. This is local development evidence, not remote CI, staging or production acceptance. The cross-container test is a separate targeted run, not one of the 14 collector gates. The full run's separate Evidence commit is pending.

The subsequent full run `wave3-20261010T174500Z-67eb27c` is preserved as FAIL (13/14); the sole failure was `TestExpiredRunningReclaimIgnoresCapacity` under PostgreSQL Race ×5. The fixture used a 2ms lease and a 15ms wall-clock sleep; it now uses a PostgreSQL-clock update to make expiry deterministic and asserts one row was updated. The stale-generation renewal fixture was hardened the same way and now checks initial claim and task-read errors. Targeted Race ×20 for both tests passed. This is a test-fixture correction, not a production scheduler change. The clean full rerun `wave3-20261010T180000Z-22478c8` then passed 14/14 on exact snapshot `22478c8986b8d1c7d6b89d76c6249304f25c8cda`; all command and artifact hashes are in its manifest. Its separate Evidence commit is pending.

## Other packages

Package A has local clean cross-container Evidence for the stated gates but remains partial against the entire Wave 3 acceptance list. Package D's local API/Worker scrape is verified; full asynchronous trace continuity, actionable alert firing, retention/GC safety and production monitoring remain open. Package E now has locally verified source-linked image SBOMs and vulnerability scans at `75bce21`; remote Actions, image signing/publication and release/rollback smoke remain unverified. Package B remains a provider-independent HTTP contract only; no real payment/notification Provider or reconciliation authority was supplied. Package C still has no shared limiter or cursor-key rotation. No production/staging claims are inferred from local results; see `WAVE3_REMAINING_GAPS.md`.
