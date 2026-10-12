# P0/P1 Runtime Unblocking — 2026-10-11

## Checkpoint identity

- Branch: `codex/production-readiness-p0-p1`
- Verified input HEAD: `2a89e02ebd8ccf94e9d1ed3afbb3d1883e7b487f`
- Verified input tree: `35158bca5b0cd0768986da5f123e45af689d11ad`
- Worktree was clean before this report was created.
- Scope: read-only resource, Docker image-cache, historic evidence, and public
  source investigation. No D: changes, Compose mutation, container start/stop,
  database connection, test credential issuance, or historical Evidence change.

## Runtime resource gate

Read-only sample from `check-resources.ps1 -Workload container -ReportOnly`:

| Measure | Result | Gate |
|---|---:|---|
| Total physical RAM | 31.22 GiB | informational |
| Guard-reported available RAM | 3.27 GiB | **BLOCKED**; container requires 6 GiB |
| Windows free physical memory (CIM snapshot) | 4.44 GiB | informational; guard value is more conservative |
| TIME_WAIT | 509 | allowed (limit 5,000) |
| E: free | 49.5 GiB | allowed (limit 20 GiB) |
| CPU load | 36% | informational |
| Docker Engine | 28.3.3, available | available but not sufficient to override RAM gate |
| Docker Desktop VM | 15.23 GiB reported, 24 running containers listed | other project resources only; left untouched |
| WSL | `docker-desktop` running; `Ubuntu-24.04` stopped | no WSL state changed |

The largest host process was `vmmemWSL` (8.43 GiB working set, 15.60 GiB
private bytes), followed by ChatGPT (1.01 GiB), Docker backend (0.99 GiB), and
several Chrome processes (0.66 GiB, 0.57 GiB, 0.48 GiB, 0.42 GiB, and 0.41
GiB). These are observations only; no process was terminated. Because the
guard reports 3.27 GiB, neither PostgreSQL container acceptance nor Go/Race
commands were started. No repeated polling was performed.

## MinIO source and image investigation

### Existing local artifact

The Docker cache contains:

- Tag: `minio/minio:RELEASE.2024-12-18T13-15-44Z`
- Local image ID: `sha256:1dce27c494a16bae114774f1cec295493f3613142713130c2d22dd5696be6ad3`
- Local `RepoDigests` metadata: `minio/minio@sha256:1dce27c494a16bae114774f1cec295493f3613142713130c2d22dd5696be6ad3`
- OS/architecture: `linux/amd64`
- Created: `2024-12-19T01:16:42Z`
- Image labels include release `RELEASE.2024-12-18T13-15-44Z` and
  `vcs-ref=10988923ed65297b3e5a5c9cf1c59f779fd76274`.

This is local-cache metadata, not a successful current registry verification.
The digest and Image ID happen to be identical, which does not make an Image ID
an OCI manifest digest. The official signed release page identifies upstream
MinIO commit `16f8cf1c52f0a77eeb8f7565aaf7f7df12454583`; that differs from the
image's `vcs-ref` label and requires explanation before claiming a reproducible
source-to-image chain. Prior Wave 4 static manifests record no MinIO image
source/build result; they record only `BLOCKED` and local configuration checks.

The unrelated local tag `tasklab-minio-ci:latest` is not present. No image was
started or changed.

### Registry revalidation

Proxy reachability check to `127.0.0.1:7890` succeeded. A read-only OCI
manifest request for the exact local `RepoDigest` used the registry-provided
Bearer challenge:

| Step | Observation |
|---|---|
| Initial manifest request | HTTP 401 with `repository:minio/minio:pull` Bearer challenge |
| Docker Hub token endpoint | HTTP 200; token was held in process memory only and never printed or saved |
| Authenticated manifest request | HTTP 401; no manifest body or `Docker-Content-Digest` was obtained |
| `docker manifest inspect --verbose` | Timed out while connecting to Docker Hub registry; no image pull occurred |

An earlier captured attempt records the authenticated response as
`401 insufficient_scope`. The current direct retry independently confirms the
manifest remains inaccessible from this environment. No index or platform
manifest was obtained. Status remains `MINIO_IMAGE_SOURCE_BLOCKED`.

### Candidate assessment (no service change made)

| Candidate | Provenance / maintenance | License and risk | Reproducibility / compatibility | Decision |
|---|---|---|---|---|
| A — official registry | Current Docker Hub Bearer flow cannot retrieve manifest/digest. Official GitHub release `RELEASE.2024-12-18T13-15-44Z` is signed and points to a verified source commit; upstream repository was archived 2026-04-25. | MinIO source is AGPLv3; archived upstream means no continuing upstream fixes. Any redistribution or service use decision needs review. No vulnerability scan was run. | A signed source tag is available, but no complete verified build recipe, pinned toolchain/base digest, or matching image build provenance was found in the preserved Evidence. | Remains blocked; do not trust cache as substitute. |
| B — build official source locally | Exact release source tag/commit is identifiable from the official release page. The local image's `vcs-ref` is a different hash. | Same AGPLv3 and archived-source risk as A; building does not restore maintenance. No CVE scan performed. | Potentially buildable after an approved pinned source/toolchain/base-image recipe, source hash, SBOM, and platform contract test. A reproducible build was not attempted under current RAM gate. | Viable only after license/security approval and a controlled build plan; not implemented. |
| C — SeaweedFS S3-compatible test backend | Official project publishes an S3 gateway and compatibility test materials; repository activity is visible, but this is not a MinIO vendor guarantee. | Official repository license file is Apache-2.0. No vulnerability scan or independent audit was performed. | Not MinIO-equivalent. Must verify SigV4, bucket policy, multipart, checksums, consistency, path style, and the exact S3 operations used by this project. | Research candidate only. Requires user approval before Compose or contract changes. |

Sources: [MinIO official repository and archive status](https://github.com/minio/minio),
[MinIO signed release/tag](https://github.com/minio/minio/releases/tag/RELEASE.2024-12-18T13-15-44Z),
[MinIO release commit](https://github.com/minio/minio/commit/16f8cf1c52f0a77eeb8f7565aaf7f7df12454583),
[SeaweedFS official S3 project documentation](https://github.com/seaweedfs/seaweedfs),
[SeaweedFS license](https://github.com/seaweedfs/seaweedfs/blob/master/LICENSE).

## PostgreSQL and current-source tests

- PostgreSQL 16 has an immutable digest in the Compose defaults, but the
  required container resource budget is not met. No new PostgreSQL container
  was started.
- No dedicated loopback `tasklab_local_test_admin` maintenance database was
  provisioned for this run. Existing unrelated project databases were not
  queried or modified.
- Current implementation HEAD `2a89e02...` Go Unit/Race/Vet/Security/Evidence
  suites were not run in this continuation because the resource guard reports
  3.27 GiB, below the 4 GiB compile/test threshold. Older unit/race evidence
  remains scoped to its older recorded commit/tree.
- Core E2E, Worker crash/reclaim, Backup/Restore, OTLP/Alert, and P1 multi-
  instance acceptance remain not run.

## Historical Evidence exceptions

The 8 known historical metadata gaps are listed individually in
`HISTORICAL_EVIDENCE_EXCEPTIONS.md`. The old manifests, raw logs, and checksums
remain unchanged. This continuation also exposed one new current-run metadata
defect: resource guard run `20261011T120008856Z` omitted the guard command from
its sealed manifest. It is preserved and listed separately in the exception
file. The runner now records the resource-check command before returning
`RESOURCE_LIMITED`; regression check `20261011T120124069Z` passed 16/16. The
fixed runner's resource-only run is `20261011T120126088Z` (`RESOURCE_LIMITED`,
0 task tests executed). The regenerated matrix has 92 runs; the whole-archive
audit remains `FAIL` with 9 issues (8 historical gaps plus this preserved
runner-metadata issue). No E2E request or service mutation occurred.

## Result and next recovery point

- `POSTGRESQL_LOCAL_INTEGRATION_VERIFIED`: **NOT CLAIMED**.
- `P0_CORE_E2E_LOCAL_VERIFIED`: **NOT CLAIMED**.
- `MINIO_IMAGE_SOURCE_BLOCKED`: **YES**.
- `RESOURCE_LIMITED`: **YES** for PostgreSQL containers, Go compilation/Race,
  and Core E2E.
- `PRODUCTION_NOT_APPROVED`.

Next safe action: wait for available RAM to exceed 6 GiB, verify the approved
MinIO image source (or obtain an explicit decision for a controlled historical
source build/alternative backend), and provision only a disposable loopback
PostgreSQL maintenance database. Then run PostgreSQL-only acceptance before
the one-task Core E2E. Do not stop unrelated workloads to manufacture headroom.

## Follow-up checkpoint — bounded Go verification and inventory integrity

The current implementation branch advanced after the initial report above:

- `3ee8135fd13fdd676e10aef28996978876eea6f3` added compile-resource gates for
  Go Unit/Vet, one-package-at-a-time execution, and an explicit bounded Vet
  suite. It also expanded the selected Unit/Race package set to cover config,
  payload registry, Gateway, API, observability, worker handlers, and public
  client behavior.
- `7e386ae78ec6d54314aa5720b42a2d3d9b80d63a` fixed source inventory capture
  so Git diagnostic text on stderr cannot be mistaken for a source pathname.
  The old manifests containing the spurious warning entry are retained exactly
  as generated; subsequent clean-tree runs are the validation for this fix.
- `e2714f36b357146249b0fbc21d226f99125d6139` records the exact Race guard
  command on future blocked runs. Regression evidence: self-test 18/18 and
  Race 227/227 (`20261011T121729277Z`, `20261011T121731548Z`).
- Current implementation tree after `e2714f3`:
  `2498cab7a64ef38d0605ba6ab6bf5802ac6116f6`.

Before the inventory fix, bounded checks completed Unit `227/227`, Race
`227/227`, and Vet PASS. After `3ee8135`, Unit and Race again passed `227/227`
on the same Go source. The source inventory fix is PowerShell-only, so Go
package bytes were unchanged. On the clean docs checkpoint `ec4d0bb` (tree
`82bed902a347cd4196e6d902c30af24a44444106`), Unit passed `227/227` (run
`20261011T121456412Z`). Race was blocked by the per-run 4 GiB guard at 3.79 GiB
(run `20261011T121525531Z`); Vet was also blocked at 0 tests (run
`20261011T121542624Z`). The current clean-commit Race is therefore not newly
verified; the 227/227 pass remains on `3ee8135` with docs-only changes in its
source inventory.

On `ec4d0bb`, clean-tree Unit passed 227/227 (`20261011T121456412Z`); its Race
attempt and Vet were resource-limited at zero tests. Static validation and
Evidence self-tests passed (`20261011T121540263Z`, `20261011T121540764Z`).
After the Race metadata fix, the code-under-test passed 227/227 Race and the
18/18 self-test (`20261011T121731548Z`, `20261011T121729277Z`). That manifest
records the pre-commit `ec4d0bb` plus an 8-path changed-file inventory. The two
runner source files' hashes match `e2714f3`; six documentation files were later
updated while producing this final report, so their current hashes differ from
the test-time inventory. Evidence bundle SHA256 validation passes. Treat this
as dirty-source evidence, not a clean-tree run.

The refreshed full matrix has 117 manifests; archive audit is `FAIL` with 10
preserved issues: the original eight, the earlier E2E guard command omission,
and the Race guard command omission from run `20261011T121525531Z`. The Race
runner now records the resource command for future blocked runs; the historical
manifest remains untouched.

Latest sampled resources were 4.27 GiB available, TIME_WAIT 296, E: free
49.48 GiB, CPU 54%; the 6 GiB container gate remains blocked by RAM. No new containers,
database, or service were started. PostgreSQL-only and Core E2E remain
unverified; MinIO remains `MINIO_IMAGE_SOURCE_BLOCKED`.

The latest implementation commit is `e2714f36b357146249b0fbc21d226f99125d6139`;
`ec4d0bb` is the clean docs/evidence checkpoint used for Unit and the blocked
Race/Vet records. A final docs-only update will publish the 117-run matrix and
10-issue audit.
