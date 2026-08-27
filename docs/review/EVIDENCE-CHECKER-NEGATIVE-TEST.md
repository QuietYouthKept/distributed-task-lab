# Evidence Checker Negative Test

Run the checker only against a disposable copy, because it creates `INDEPENDENT-CHECKER.json` with `CreateNew` semantics:

```powershell
Copy-Item -Recurse $env:ASSET_ROOT\results\<run> $env:TEMP\tasklab-evidence-negative
Remove-Item $env:TEMP\tasklab-evidence-negative\INDEPENDENT-CHECKER.json
# Delete one `owned` event from TL-CLAIM-002/operations.jsonl.
& .\scripts\check_asset_evidence.ps1 -ResultRoot $env:TEMP\tasklab-evidence-negative
```

Expected outcome: non-zero exit status; `TL-CLAIM-002.independent_oracle` is false because the checker independently requires 1,000 unique ownership events, matching acknowledgements, and 1,000 successful database rows. The canonical evidence directory is never changed by this test.

Audit run on 2026-08-26: changing only `TL-CLAIM-002/result.json.passed` to `false` left checker v2 passing; deleting one `owned` history event caused a non-zero exit and `independent_oracle=false`.

Use `-CoreOnly` for a newly generated Asset Lab directory before the separate migration and Docker fault bundles have been collected; omit it for the complete production-hardening evidence root.
