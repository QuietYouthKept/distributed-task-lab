# TL-MIG-002 — Migration fault matrix

- Setup: fresh dedicated PostgreSQL database per case.
- Faults: concurrent startup, held advisory lock, tampered checksum, missing history version, and invalid statement appended mid-file.
- Oracle: concurrency serializes; held lock times out then recovers; checksum/gap fail closed; failed file leaves no partial DDL.
- Result: **TEST VERIFIED** by seven migration integration cases on PostgreSQL 16.15.
- Boundary: forward-only migration is tested; automated downgrade is not provided.
