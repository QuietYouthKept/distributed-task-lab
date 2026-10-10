# Wave 4 closeout

Date: 2026-10-10. Branch: `codex/wave4-engineering-hardening`. Stable base: `a8656a7b72d792986775d94fe007acc7771c072b`. Current implementation/release status: `WAVE4_ENGINEERING_PARTIAL`; production status: `PRODUCTION_NOT_APPROVED`.

## Verified during this closeout

- Migration 012 was inspected and left unchanged. Its DDL is additive, but a real process matrix showed Wave 3 API and Worker refuse schema12 while Wave 4 API/Worker accept schema12 and automatically upgrade schema11. Old-binary rollback is therefore blocked; see `WAVE4_MIGRATION_ROLLBACK_COMPATIBILITY.md`.
- The source-pinned MinIO build and authenticated S3 contract passed. The process-level API/two-Worker crash recovery passed against the shared artifact store. The PostgreSQL 16.15 custom-format backup/restore drill passed and verified the restored task plus its referenced MinIO object. E: evidence includes redacted logs and SHA256SUMS.
- `govulncheck` v1.8.0 scanned the complete `./...` module on Go 1.26.9 through the user's proxy at `127.0.0.1:7892`: zero affected imported packages or reachable symbols; one unresolved module-level `GO-2026-5932` (`x/crypto/openpgp`, no fixed version). The prior EOF under proxy 7897 remains historical and untouched.
- Verification scripts added in commit `3fca9369672bc7a5953b1a23b4c2447cf2d7d0a2` use fresh names, random credentials, dynamic ports and scoped cleanup. They do not depend on Windows image cache or touch other Docker projects. The final-source full unit/integration, full Race/integration, RetryBackoff Race count 20, PostgreSQL Race count 5 and Failure Lab 12/12 all passed on that commit; logs and checksums are at `E:\distributed-task-lab-temp\wave4-artifacts\final-verification-3fca936`.
- The earlier Failure Lab database-connectivity failure due to an unavailable `127.0.0.1:5433` remains unchanged at `E:\distributed-task-lab-temp\wave4-artifacts\final-matrix-de5567f-20261010`; its corrected retry and this turn's isolated-database 12/12 pass are separate evidence, not a rewrite of the first attempt.

## Gates and limits

The prior exact implementation commit `de5567f2f9e84a8bc8adfe5deb551db8c71f6eea` passed full unit/PostgreSQL and Race/PostgreSQL suites, Failure Lab 12/12, vet, alert lifecycle E2E and Worker graceful-drain smoke. Those results remain bound to that SHA. The verification-harness commit `3fca9369672bc7a5953b1a23b4c2447cf2d7d0a2` then passed full unit/integration, full Race/integration, RetryBackoff Race count 20, PostgreSQL Race count 5 and Failure Lab 12/12. Hosted Wave 4 CI, security and supply-chain checks remain unverified until push and completion of the Draft PR workflows.

No production migrations, real provider credentials, external effects, release tags or Wave 3/Phase 0 changes were made. No historical evidence was deleted or rewritten. The Wave 3 PR and Phase 0 tag remain unchanged.

## Delivery state

Normal branch push and a Draft PR against `codex/phase1-production-hardening` are authorized by the task instruction, subject to a fast-forward-only push and passing local gates. Do not merge or publish a release. After opening the Draft PR, record each hosted workflow's final state and source SHA. If any required check is non-green or pending, keep the result partial and report the exact recovery point.
