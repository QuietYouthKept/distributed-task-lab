# Verification Baseline

Captured: 2026-10-12 10:59:05 +08:00
Repository: `<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\worktree`
Branch: `codex/production-readiness-p0-p1`
HEAD: `d6fd35ca80fc692eb40bf99ac23196bd6200b089`
Tree: `9d9610e2706c6545047303ca74ee227cf966faf0`

## Git state

The only untracked path at capture time was the protected
`docs/production-readiness/FINAL_PROJECT_CLOSEOUT.md`. It is intentionally not
read, staged, edited, moved, or included in this baseline. No other worktree
changes were present.

The historical evidence source `0a5482d657ae431751c205abdf01632ebe89cee6`
exists and is an ancestor of this HEAD. That establishes source lineage only;
it does not make its test results current-version results.

## Host and tools

| Item | Observed value |
|---|---|
| Windows | Windows 11 Home, build `10.0.26200` |
| CPU | AMD Ryzen 9 7845HX |
| Logical processors | 24 |
| Visible physical memory | 31.22 GiB |
| Available memory at capture | 4.26 GiB |
| E: free space | 43.82 GiB |
| Go | `go1.26.6 windows/amd64` |
| Docker client / Engine | `28.3.3 / 28.3.3` |
| Docker Compose | `v2.39.2-desktop.1` |
| PostgreSQL client (`psql`) | Not installed/on PATH |
| Terraform | Not installed/on PATH |
| Python | `3.8.3` |
| jq | Not installed/on PATH |
| Docker images | 225 unique image IDs locally; local presence is not registry provenance |
| Docker containers | 29 running at capture; none belonged to this TaskLab worktree |

No container was started, stopped, or otherwise modified. Because available
memory was below the existing 6 GiB container gate, container-level acceptance
was not run. Docker image cache contents are not treated as authenticated
registry evidence.

## Dataset inventory snapshot

`E:\distributed-task-lab-datasets` contained 42 files totaling 783,327,186
bytes across `.jsonl`, `.csv`, `.json`, `.tsv`, `.txt`, `.zip`, `.gz`, and
`.rar`. This is a path-level snapshot, not a claim that all datasets have been
semantically audited. Additional large files exist directly under E:, including
multi-gigabyte Parquet and DocLayNet archives; they were not decoded or
extracted.

The Azure LLM Code and Conversation raw files are present in both `raw` and
`downloads`; each pair has matching byte length and matching published local
manifest hashes. Their full streaming quality/statistical audit is recorded
separately in `01_AZURE_LLM_DATA_QUALITY.md`.

## Execution boundary

This baseline is tied to the HEAD/Tree above. Later code or evidence must record
its own tested source SHA and Tree SHA. Historical results, cached images, and
static checks must not be promoted to current integration, race, E2E, cloud, or
performance PASS claims.
