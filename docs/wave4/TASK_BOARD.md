# Wave 4 requirement / test / evidence board

Board initiated at the verified Wave 3 baseline `a8656a7b72d792986775d94fe007acc7771c072b`; entries below are the current states after the first Package A implementation and audit. A status is only accepted when it includes corresponding source and test evidence.

| ID | Priority | Owner | Implementation status | Test status | Evidence | Blocking dependency / next action |
|---|---|---|---|---|---|---|
| A-01 | P0 | Codex | IMPLEMENTED | PASS: Full Unit, Full Race, PostgreSQL Integration | `WAVE4_PACKAGE_A_API_CONTRACT.md`; E:\distributed-task-lab-temp\wave4-artifacts\package-a-final-e3d4964-20261010 | Hosted CI remains unverified for Wave 4 SHA. |
| A-02 | P0 | Codex | IMPLEMENTED | PASS: schema/DTO drift; full regression | Same as A-01 | Hosted CI remains unverified for Wave 4 SHA. |
| A-03 | P0 | Codex | IMPLEMENTED | PASS: router/auth contract, owner-isolation PG test, full regression | Same as A-01 | Hosted CI remains unverified for Wave 4 SHA. |
| B-01 | P0 | Codex | IMPLEMENTED | PASS: full unit, full Race, vet, required PostgreSQL integration | `WAVE4_PACKAGE_B_DISTRIBUTED_TRACING.md`; E:\distributed-task-lab-temp\wave4-artifacts\package-b-final-8099c68-20261010 | Hosted CI remains unverified; production trace backend/retention is outside this package. |
| B-02 | P0 | Codex | IMPLEMENTED | PASS: clean-commit isolated Collector E2E plus two-worker crash/reclaim | Same as B-01; clean E2E manifest and logs under E:\distributed-task-lab-temp\wave4-artifacts\tracing-e2e-runs\evidence\tasklab-e2e-4df8581dda83 | Hosted CI remains unverified; no production readiness claim. |
| C-01 | P0 | Codex | IMPLEMENTED | PASS: full unit/race, strict PostgreSQL, immutable identity/receipt regressions | `WAVE4_PACKAGE_C_EFFECT_RECONCILIATION.md`; E:\distributed-task-lab-temp\wave4-artifacts\package-c-final-8e76d2f-20261010 | Real vendor adapter is intentionally not enabled; future provider payload fields must enter the canonical fingerprint. |
| C-02 | P0 | Codex | IMPLEMENTED | PASS: local HTTP lost-response/reconcile with PostgreSQL; receipt/manual-resolution Race | Same as C-01 | Vendor-specific semantics, production provider credentials, and hosted CI remain unverified. |
| D-01 | P1 | Codex | PARTIAL (candidate implemented; disabled) | PASS: isolated PostgreSQL Race; two-container shared-burst E2E; API/config Race and vet | `WAVE4_PACKAGE_D_GLOBAL_ADMISSION.md`; `E:\distributed-task-lab-temp\wave4-artifacts\shared-admission-e2e-runs\admission-daf1761e5397`; `E:\distributed-task-lab-temp\wave4-artifacts\package-d-postgres-race-clean-5258c5a-20261010` | Product quota identity/scope/rate and outage policy approval; backend outage and contention testing remain. |
| D-02 | P1 | Codex | PARTIAL (opt-in v2 implemented; v1 default retained) | PASS: overlap/cutoff, expiry, binding, tamper, revocation, config, Race | `WAVE4_PACKAGE_D_GLOBAL_ADMISSION.md`; API Race log under `E:\distributed-task-lab-temp\wave4-artifacts\package-d-api-race-vet-5258c5a-20261010` | Owner decision for TTL/overlap and secret distribution; rolling mixed-version deployment is not verified. |
| E-01 | P1 | Codex | AUDIT_IN_PROGRESS | NOT VERIFIED | `docs/storage-lifecycle.md`, migration/source audit summarized in STATE | Define dry-run candidates only; purge requires retention and effect-history policy. |
| E-02 | P1 | Codex | TODO | NOT VERIFIED | None | Implement bounded dry-run lifecycle candidate report, resume semantics and isolated DB tests; purge remains off absent approval. |
| E-03 | P1 | Codex | AUDIT_IN_PROGRESS | NOT VERIFIED | Wave 3 100K local benchmark; run details in Wave 3 docs | Extend with skew/selectivity/deep cursor and read/write/index-cost comparison using a fresh E: dataset. |
| F-01 | P1 | Codex | AUDIT_IN_PROGRESS | NOT VERIFIED | Existing metrics source audit summarized in STATE | Inspect scrape/alert stack; add and exercise dev rules only if isolated evaluator is available. |
| F-02 | P1 | Codex | AUDIT_PENDING | NOT VERIFIED | None | Inspect startup/readiness/drain/migration compatibility and build isolated release/rollback smoke. |
| F-03 | P1 | Codex | AUDIT_PENDING | NOT VERIFIED | Wave 3 local supply-chain artifacts | Recheck workflow, base pins, SBOM, vulnerability findings, runtime hardening and signing readiness. |
| G-01 | P1 | Codex | TODO | NOT VERIFIED | None | Produce code-grounded architecture diagrams and source links. |
| G-02 | P1 | Codex | TODO | NOT VERIFIED | None | Document real call chains from entry point through repository/worker and tests. |
| G-03 | P1 | Codex | TODO | NOT VERIFIED | None | Write interview materials with every quantitative claim linked to evidence; no production claims. |
| W4-FINAL | P0 | Codex | TODO | NOT VERIFIED | None | Freeze source SHA/tree, clean E: worktree, run complete applicable matrix, hash raw evidence, remote workflows if push succeeds. |

Allowed result vocabulary for gates: `PASS`, `FAIL`, `SKIPPED`, `BLOCKED`, `NOT VERIFIED`. Historic evidence stays bound to its original SHA and is never relabeled.
