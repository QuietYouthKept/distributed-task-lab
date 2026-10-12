# Evidence

This directory contains reproducible, reviewable evidence for claims made by
this repository. It deliberately separates **measured** facts from targets,
plans, and historical artifacts.

## Baseline

Run the baseline collector from the repository root:

```powershell
pwsh -NoProfile -File scripts/evidence/collect-baseline.ps1
```

It creates an immutable directory under
`evidence/baseline/runs/<baseline-id>/` containing `manifest.json` and command
logs, then regenerates `evidence/index.json`. The
manifest records the commit, dirty state, non-secret configuration fingerprint,
tool versions, known migrations/routes, benchmark inventory, and the actual
exit status of each verification command.

The collector intentionally exits non-zero when a required verification command
cannot run or fails. Its manifest is still written in that case, so a missing
Docker daemon or PostgreSQL test database is evidence of an unverified baseline,
not a passing test.

Each manifest is a point-in-time record. The collector refuses to overwrite an
existing run ID. Do not edit test statuses or metric values by hand; rerun the
collector to create a new baseline record.

An index record is `measured_valid` only when every required check ran with
exit code zero and the repository was clean. Skipped checks, failed checks, or
a dirty working tree remain `unverified`, because the recorded commit alone
cannot reconstruct uncommitted code.

## Evidence rules

- Keep raw logs and artifacts alongside the manifest that references them.
- Store only non-secret configuration values. Database URLs are redacted before
  they are written.
- A benchmark file is an inventory item until its environment and raw data are
  independently verified.
- Resume claims must eventually reference an evidence ID and a manifest or run
  artifact in this directory.
