# Historical Evidence Metadata Exceptions

Snapshot: 2026-10-11. This is a read-only adjudication aid; it does not alter,
replace, or re-seal any run or log. Eight entries are pre-existing historical
gaps; two additional missing-command records were emitted by resource-blocked
runs and are preserved as-is. The overall audit remains `FAIL` until all
metadata gaps are resolved by evidence or accepted under an explicit policy.

| Historical Run ID | Missing field | Recoverability | Available corroboration | Impact on claims | Exception status |
|---|---|---|---|---|---|
| `20261011T060427988Z` | Source inventory for `dirty_at_start=true` | Not reliably reconstructable | Manifest records source commit/tree; sealed raw log and hashes remain valid | Cannot bind the PASS to exact uncommitted files; treat as historical, non-current evidence | OPEN / do not reconstruct |
| `20261011T060428323Z` | Source inventory for dirty tree | Not reliably reconstructable | Manifest, raw failure log, and hash list remain intact | Test command failed before producing test results; still cannot identify exact dirty source | OPEN / do not reconstruct |
| `20261011T060614491Z` | Source inventory for dirty tree | Not reliably reconstructable | Manifest records 28/28; raw log and checksums remain intact | The 28-test result cannot be tied to exact uncommitted source; later 115-test evidence is separate | OPEN / do not reconstruct |
| `20261011T060725858Z` | Source inventory for dirty tree | Not reliably reconstructable | Manifest source commit/tree and sealed log remain intact | Static PASS cannot be associated with exact dirty file set | OPEN / do not reconstruct |
| `20261011T060726345Z` | Source inventory for dirty tree | Not reliably reconstructable | Manifest records 115/115; raw log and checksums remain intact | The result cannot be bound to exact uncommitted source; do not use as current-commit validation | OPEN / do not reconstruct |
| `20261011T100455342Z` | Exact invoking command | Invocation string unrecoverable; execution intent corroborated | Raw log explicitly says no integration command executed; manifest says `DEPENDENCY_BLOCKED`, 0 tests; runner source at recorded commit is available | No database acceptance can be claimed; missing command has little positive-claim impact but remains an audit defect | OPEN / intent corroborated only |
| `20261011T100455719Z` | Exact invoking command | Invocation string unrecoverable; execution intent corroborated | Raw log explicitly says no E2E/failure command executed; manifest says `RESOURCE_LIMITED`, 0 tests; runner source at recorded commit is available | No E2E or failure behavior can be claimed; missing command remains an audit defect | OPEN / intent corroborated only |
| `20261011T100501556Z` | Exact invoking command | Invocation string unrecoverable; execution intent corroborated | Raw log explicitly says no scanner command executed; manifest says `DEPENDENCY_BLOCKED`, 0 tests; runner source at recorded commit is available | No security scan can be claimed; missing command remains an audit defect | OPEN / intent corroborated only |
| `20261011T120008856Z` | Exact guard command | Recoverable from the invoking session, but not from the sealed manifest | Raw log says no E2E/failure command executed; manifest says `RESOURCE_LIMITED`, 0 tests; current script source identifies the guard operation | No E2E ran and none is claimed. This is a runner metadata defect, not test evidence loss | OPEN / preserve as emitted |
| `20261011T121525531Z` | Exact resource guard command | Recoverable from the invoking session, but not from the sealed manifest | Raw log says no Race command executed; manifest records `RESOURCE_LIMITED`, 0 tests and a 3.79 GiB reading against the 4 GiB gate; current runner fix records the exact guard command | No Race test ran and none is claimed. The failure is limited to audit metadata; the run remains immutable | OPEN / preserve as emitted |

## Handling rule

- Original manifests, logs, and `SHA256SUMS` remain immutable.
- Do not infer or backfill an old source inventory from a later run, current
  checkout, Git commit, Docker cache, or neighboring run.
- Do not insert reconstructed command text into old evidence. The raw logs
  already establish that the three blocked suites launched no test/scanner
  command; preserve that as corroboration, not as an exact command.
- Current-source verification is reported separately in the manifest-backed
  `VERIFICATION_MATRIX.json` and `VERIFICATION_MATRIX.md`.
- The audit remains failed while these eight original rows and two preserved
  guard-run metadata gaps remain unresolved. A future
  policy may classify non-reconstructable records as historical exceptions,
  but that policy decision is not made here.
