# P0/P1 Local-First Deep Engineering Closeout

## Continuation checkpoint — 2026-10-11

This section supersedes time-sensitive status values in the historical
checkpoint below. The implementation is committed at
`366c320cbc60392de4cbe32f94b98d02c5cd9edd`, tree
`ce6b4a5feb602e27bc71114b8fe7269970810420`. A docs/evidence-index-only
follow-up is being recorded separately. No D: files, other projects'
containers, or historical Evidence were changed.

- Extended work completed: evidence inventory now includes staged files and
  seals all closed run files; the offline audit reconciles the matrix and
  checks source commit/tree, counters, exact command, inventory hashes, and
  SHA256SUMS. The audit currently reports 8 historical metadata gaps only
  (5 early dirty runs without inventories; 3 blocked runs without exact
  commands). No historical record was rewritten.
- Post-commit offline acceptance: PowerShell static `PASS` (0 functional
  tests); evidence-integrity 15/15; Compose cleanup/recovery-scope 16/16;
  database-safety 9/9; backup-manifest 12/12; supply-chain-static 4/4;
  retention-fixture 12/12. The manifest-backed matrix contains 88 runs.
  Exact run IDs are in `STATE.md`; detailed counters and hashes are in
  `VERIFICATION_MATRIX.json` and `EVIDENCE_INTEGRITY_ACCEPTANCE.md`.
- Recovery harness now requires the shared exact Run-ID/project/environment
  validation and an explicit matching `-ConfirmRunId` before any fault action.
  Operations named network-loss, request-failure, and short-disconnect are
  explicitly documented as service-stop approximations, not packet- or
  request-level injection. No fault action was executed.
- Resource snapshot at 2026-10-11T11:43:14Z: available RAM 4.02 GiB,
  TIME_WAIT 390, E: free 49.5 GiB, CPU 18%. The compile guard allowed a
  bounded compile workload, but the container gate remains 6 GiB and did not
  pass. No container workload was started. Existing unrelated containers
  were only inventoried.
- PostgreSQL/MinIO, full E2E, API identity authorization, backup/restore,
  cross-process OTLP/alerts, vulnerability scanners, SBOM, signing, and
  provenance remain unverified. MinIO remains `MINIO_IMAGE_SOURCE_BLOCKED`.
  Status remains `PRODUCTION_NOT_APPROVED`.

The implementation commit and docs/evidence-index follow-up are recorded in
`STATE.md`. Post-commit evidence has an empty dirty inventory and is bound to
the implementation tree; local fixture/static results are not cloud or
production acceptance. The current whole-archive audit remains FAIL on eight
historical metadata gaps; all 88 current manifest rows are in the generated
matrix, and the latest seven test bundles pass their recorded checks.

Date: 2026-10-11 (Asia/Shanghai). Scope: offline/local engineering only. This
report distinguishes source implementation from static, unit, race, database,
container, and production acceptance.

## Source and evidence identity

- Worktree: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\worktree`
- Branch: `codex/production-readiness-p0-p1`
- Base implementation HEAD: `8cf7da3a62f724bf9f9ccbea5537a1b74dc8860a`
- Base tree: `8fce4357ed52084629b6a9a23b007f98b6b3858b`
- Local implementation checkpoint: `b75c221009a903bc8ada31c4017c26ba094383a1`.
- PostgreSQL backup-harness follow-up: `b9faa53989ba1ca6ec1d581401f9f76383842575`.
- Offline retention-manifest follow-up: `581ca0fd74d69250b0209cc7952b343c528ee2eb`.
- At test time, new implementation files were uncommitted. Every generated Go
  test manifest records the base commit/tree, `dirty_at_start: true`, and a
  SHA-256 inventory of all changed/untracked source files. The per-run evidence
  is therefore bound to the exact tested working tree, not misrepresented as
  the clean base commit. No push is authorized.
- Evidence root: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\deep-engineering`.
  Historical Wave 3/Wave 4 evidence and original failures were not edited.
- The verification runner writes and closes raw logs before hashing `raw.log`,
  manifest, and source inventory into `SHA256SUMS`. The backup harness separately
  hashes the completed dump, its closed stderr log, and manifest.

## Final status

- `P0_IMPLEMENTED: PARTIAL`
- `P0_STATIC_VERIFIED: PARTIAL`
- `P0_LOCAL_E2E_VERIFIED: NOT VERIFIED`
- `P1_IMPLEMENTED: PARTIAL`
- `P1_STATIC_VERIFIED: PARTIAL`
- `P1_LOCAL_E2E_VERIFIED: NOT VERIFIED`
- `RESOURCE_LIMITED: YES`
- `DEPENDENCY_BLOCKED: YES`
- `EXTERNAL_NOT_VERIFIED: YES`
- `PRODUCTION_NOT_APPROVED`

## Work package closeout

| Package | Actual change / verification | Remaining boundary |
|---|---|---|
| A — MinIO source | Two bounded official Docker Hub OCI attempts: initial 401 challenge, token endpoint 200, authorized manifest 401 `insufficient_scope`; token value not logged. Added source-chain report and object-store ADR. | No media type, registry digest, index, child platform, or trusted pull. Status `MINIO_IMAGE_SOURCE_BLOCKED`; no mirror/cache substitution. |
| B — DB privileges | Reviewed roles/grants and repository paths; lexical scan of 31 Go/SQL files; documented table-level UPDATE/fencing residual risk. Added loopback-only DB bootstrap and privilege scripts with `REQUIRE_TEST_DATABASE=1`, E: logs and checksums. | Neither script ran because no explicitly provisioned disposable maintenance DB exists. No PostgreSQL PASS. Exact role SQL still needs DB-level coverage for renew/retry/DLQ/migrator denial. |
| C — Compose | Compose network and two volume names derive from each run ID. Added JSON preflight, ownership-scoped recovery harness, and staged startup/readiness runner with closed-file SHA sealing and dirty-source inventory. | Static Compose contract 29/29 PASS and `docker compose config --quiet` exit 0. PowerShell AST static run passed. Valid-project runner preflight captured resource, MinIO digest, and local image blockers and wrote `executed=false`; no Compose mutation. RAM was 0.45 GiB; runner does not yet execute cross-process acceptance or clean up containers/networks/secrets. |
| D — Identity | Added JWKS cache-expiry/outage fail-closed and JWKS HTTP-timeout tests; updated identity/API matrix. | Part of 115 unit/race tests. Wrong CA/expired/SAN negative matrix and HTTPS API+PostgreSQL authorization E2E remain incomplete. No hosted IdP. |
| E — Gateway | Added eight concurrent identical requests test; unknown-outcome/retry and fault matrices. | In-process fixture only. Same-process dedupe works; simulator restart loses its in-memory ledger. No durable/provider guarantee or cross-process test. |
| F — Backup/restore | Safe source/destination preflight, explicit-opt-in PostgreSQL-only `pg_dump` from an exactly owned Compose service into a fresh ACL-restricted E: directory, PGDMP/hash verifier, and snapshot-consistency design. | No dump was executed. Object inventory/copy and independent restore are still absent. `LOCAL_RESTORE_VERIFIED` is not claimed. |
| G — limiter/cursor | Current unit tests are included in the 115 test run; added detailed acceptance boundaries. | Two-instance PostgreSQL contention, DB failure/recovery, live two-API cursor rotation and secret distribution were not run. |
| H — retention | Preserved read-only DB dry-run; added offline bounded archive-manifest compiler, external checksum verifier, snapshot/cutoff-bound resume cursor and 12 fixture assertions, including invalid duplicate-ID, future-cutoff, and missing-history rejection. | Not DB/S3 integrated; artifact hashes are format-checked, not fetched; token is not signed; DELETE/Purge remains disabled. Existing PG retention tests were not run. |
| I — observability | Observability tests are included in the targeted 115 unit/race suite; documented finite label and sensitive-data rules plus acceptance sequence. | No in-process OTLP receiver, cross-process traces, Prometheus/Alertmanager or test receiver was started. |
| J — supply chain | Documented image pins/provenance and checked available tools without download. | `govulncheck`, `syft`, `trivy`, `cosign`, and `grype` are absent. No current source/image scan, SBOM, image build, signing, or provenance. |
| K — evidence | Added runner with `PASS`, `FAIL`, `NOT RUN`, `RESOURCE_LIMITED`, `DEPENDENCY_BLOCKED`, `IMPLEMENTED_NOT_VERIFIED`; counts executed/pass/fail/skip; hashes dirty-source inventory; generates JSON/Markdown matrix from manifests and preserves older matrix content. | Audit/verification of all legacy Evidence remains a separate task. Initial Go version failure remains untouched; generated matrix correctly interprets zero executed tests and its raw toolchain error as `DEPENDENCY_BLOCKED`. |
| L — full system | None started. | No PostgreSQL-only run, Core happy path, crash recovery, restore, cross-process OTLP, or local Alert receiver test. Resource and MinIO dependency gates prohibit claiming them. |

## Actual tests and assertions

1. PowerShell static runner: PASS on latest script set. This parses all
   `scripts/local-first/*.ps1` files and runs `git diff --check` on tracked
   changes at that run. It does not prove script runtime behavior for every
   branch.
2. Go targeted unit: `go test ./internal/auth ./cmd/gateway-simulator
   ./internal/api/... ./internal/observability` via `-json`: 115 executed, 115
   pass, 0 fail, 0 skip. It includes JWT issuer/audience/signature/role
   boundaries, JWKS outage/timeout, rotating cursors, limiter router logic,
   gateway malformed/429/timeout/response-loss/cost/idempotency, and selected
   observability tests. It does not connect PostgreSQL or MinIO.
3. Go targeted race: same four package groups with `-race`: 115 executed, 115
   pass, 0 fail, 0 skip. This is not a full repository Race suite.
4. Compose contract: 29/29 static assertions pass. Compose config renders with
   run-scoped identifiers. No image pull or `docker compose up` occurred.
5. Preflight negative case: nonzero exit with structured reasons and no
   password/DSN pattern in output. It did not start, stop, or delete containers.
6. PostgreSQL integration: NOT RUN; no `TEST_DATABASE_URL` for a dedicated
   loopback `tasklab_local_test_admin` database. Full `go test ./...` was not
   run because repository integration helpers can create/drop databases on
   `localhost:5433`; no unknown project database was touched.
7. MinIO/S3, cross-process, backup/restore, OTLP and alert delivery: NOT RUN.
8. Retention archive fixture: latest run `retention-fixture-20261011T103317845Z`,
   12/12 assertions PASS; two eligible records across
   bounded pages, five unsafe/boundary records excluded, event/audit retention,
   deterministic file hash, resume continuation and snapshot mismatch rejection.
   It consumed only a local fixture; no DB, S3, or delete operation occurred.
9. Compose runner preflight smoke: `valid-project-smoke-postredaction/run-20261011t140857p0800` —
   `RESOURCE_LIMITED`, no Compose mutation; manifest, preflight, and changed-source
   inventory have SHA-256 entries. This is blocker evidence, not an E2E PASS.
   Secret-value scan found no generated credential in evidence. Source inventory
   SHA-256: `0f62674d33ad112eb29a096c70fe87a73370413458a95d0f1177c688f850291b`.

## Resource and dependency record

The authorized targeted Race run sampled RAM 4.63 GiB, TIME_WAIT 200, E: free
52.84 GiB, CPU 27%, Docker available; it passed its 4 GiB race gate. Later
container preflight sampled RAM 2.64 GiB, TIME_WAIT 375, E: free 52.62 GiB,
CPU 38%, so the 6 GiB container gate failed and no container workload ran.
Other projects' Docker resources were only inventoried; none were changed.

Go 1.26.9 was absent initially while `go.mod` requires >=1.26.9. The Go
toolchain's official verified module distribution installed that fixed version
into the E: cache; no module minimum was weakened. First test attempt ran zero
tests and was preserved. Latest unit and race suites ran on Go 1.26.9.

## Evidence and next recovery point

- Unit manifest: `unit-20261011T060845426Z`; source inventory SHA-256
  `1a40c297f7af179a0a81f15f2134f2dcbc577f1d30aa2ba19373058ccd03a858`.
- Race manifest: `race-20261011T100305783Z`; source inventory SHA-256
  `7d396c9ebbf0467daf354a1436acacd47fce024525d97cbede84eb5c3c70c424`.
- Container gate manifest: `e2e-20261011T100455719Z`; RAM 2.64 GiB, status
  `RESOURCE_LIMITED`.
- Retention fixture runner manifest: `retention-fixture-20261011T103229088Z`;
  its source inventory SHA is recorded in the manifest. The latest nested fixture
   result reports 12 assertions, eligible 2/excluded 5; both runner and fixture
   SHA256SUMS were independently verified. Runner records PowerShell 7.6.5,
   the tool actually used for this fixture-only suite; source inventory SHA-256:
   `a0c3d81fb8df10e7bf243f448cf980455a263833d3ffec75a7c7c2760ea17cd4`.
- Current generated run index: [VERIFICATION_MATRIX.json](VERIFICATION_MATRIX.json)
  and [VERIFICATION_MATRIX.md](VERIFICATION_MATRIX.md). Every PASS is scoped to
  the exact source inventory and tests in its manifest.
- Useful next step when safe: authorize/provide an official MinIO pull
  capability (or decide on a reviewed source/license alternative), then wait
  for RAM ≥6 GiB. First run the isolated PostgreSQL bootstrap/privilege scripts;
  only after that, start one uniquely named Compose profile and collect new
  current-source E2E evidence.

## Approval still required / not authorized

No alternate object-store implementation/license posture was chosen. No hosted
IdP, commercial provider, paid cloud service, production secrets, formal purge,
remote push, merge, or release was used or approved. Local production-like
evidence is not cloud production acceptance.
