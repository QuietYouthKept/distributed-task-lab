# Incomplete Evidence Collection

- Run ID: `wave2-20261009T131314543Z-72580b8a7b15`
- Implementation commit: `72580b8a7b15432f24dafd6ab70c3c1bc1bd5191`
- Collection status: `NOT VERIFIED`; the collector ran all configured checks and produced logs/artifacts but did not finalize a manifest or structured exit-code record.
- Failure: one configuration-source fingerprint still called unavailable `Get-FileHash`. The other log/artifact files are preserved as generated; no results have been manually promoted to PASS.
- Chaos Matrix produced a complete summary in the run's copied artifacts; the MinIO contract output shows an authenticated adapter contract PASS. These remain per-check raw observations, not a complete Evidence manifest.
- Temporary MinIO was stopped by wrapper cleanup, used no persistent volume, and its random credentials/temp environment file were removed. Existing services were not altered.
- Follow-up: use the .NET SHA-256 helper for the remaining config hash and collect a new Run ID. Do not alter this directory.
