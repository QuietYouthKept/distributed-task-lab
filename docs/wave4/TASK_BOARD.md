# Wave 4 requirement / test / evidence board

Board initiated at the verified Wave 3 baseline `a8656a7b72d792986775d94fe007acc7771c072b`; entries below are the current states after the first Package A implementation and audit. A status is only accepted when it includes corresponding source and test evidence.

| ID | Priority | Owner | Implementation status | Test status | Evidence | Blocking dependency / next action |
|---|---|---|---|---|---|---|
| A-01 | P0 | Codex | IMPLEMENTED | PASS: Full Unit, Full Race, PostgreSQL Integration | `WAVE4_PACKAGE_A_API_CONTRACT.md`; E:\distributed-task-lab-temp\wave4-artifacts\package-a-final-e3d4964-20261010 | Hosted CI remains unverified for Wave 4 SHA. |
| A-02 | P0 | Codex | IMPLEMENTED | PASS: schema/DTO drift; full regression | Same as A-01 | Hosted CI remains unverified for Wave 4 SHA. |
| A-03 | P0 | Codex | IMPLEMENTED | PASS: router/auth contract, owner-isolation PG test, full regression | Same as A-01 | Hosted CI remains unverified for Wave 4 SHA. |
| B-01 | P0 | Codex | AUDIT_IN_PROGRESS | NOT VERIFIED | Wave 3 docs and source audit summarized in STATE | Finish mapping API/Worker/DB/artifact spans and collector harness; then implement async-link semantics and span gaps. |
| B-02 | P0 | Codex | TODO | NOT VERIFIED | None | Build isolated API/PostgreSQL/MinIO/two-Worker/OTLP Collector trace E2E; prove fresh-run collection and redaction. |
| C-01 | P0 | Codex | AUDIT_IN_PROGRESS | NOT VERIFIED | Source audit summarized in STATE | Verify integration cases, then prevent fingerprint/receipt mutation under stable idempotency identity. |
| C-02 | P0 | Codex | TODO | NOT VERIFIED | None | Expand real PostgreSQL + isolated HTTP provider fault matrix and race/manual-resolution tests. |
| D-01 | P1 | Codex | AUDIT_IN_PROGRESS | NOT VERIFIED | ADR-0006 and source audit summarized in STATE | Policy decision is not approved; an optional disabled candidate may proceed without changing defaults. |
| D-02 | P1 | Codex | AUDIT_IN_PROGRESS | NOT VERIFIED | Cursor v1 source audit summarized in STATE | Existing cursor has no issued-at/TTL/key-id; rotation compatibility requires an explicit legacy acceptance boundary. |
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
