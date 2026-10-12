# Artifact boundary and offline adapter tests — 2026-10-12 run 01

Status: selected local-store, key validation, metrics, and OSS fake-client tests PASS. The tests use local files and a fake OSS API only. MinIO integration and the real OSS contract were NOT RUN; this is not S3/OSS service compatibility evidence.

- Source commit: `0167bdc1460a731d94dd97a29debea4323a46a42`
- Source tree: `f247cf819d1fa4e3f8ecd68a2d6912e0a553bd4d`
- Go: `go1.26.9 windows/amd64`
- Exact command is in `test-command.txt`; exit code is 0.
- No Docker, database, cloud credentials, or external service was used.

The exercised unit selection includes traversal/key validation, local store concurrent immutable writes and size/cancellation checks, symlink escape checks, metrics classification, OSS credential/HTTPS validation, and fake-client Put/Get/Head/Delete/error/conditional-write behavior. The fake OSS contract does not prove real OSS semantics, IAM policy sufficiency, or cross-host consistency. `SHA256SUMS` covers the evidence files except itself.
