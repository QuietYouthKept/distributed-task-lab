# P1-D — Supply-chain closeout status

Source Dockerfiles are multi-stage for API/Worker/Migrator/Gateway/identity
fixtures and use non-root runtime users; this is source configuration, not a
built-image scan. A static audit now covers base-reference pinning, non-root
directives, OCI revision-label declarations, `.dockerignore` secret/evidence
exclusions, workflow action mutability, and scanner availability. The E2E S3
proxy's Alpine runtime was pinned to the repository's existing Alpine digest
and given OCI labels; local build context now excludes `evidence/`.

| Control | Status in this checkpoint |
|---|---|
| Go unit/security scan | `govulncheck` was not run; toolchain floor mismatch blocks Go commands |
| Secret scan | Existing prior baseline evidence is historical, not a scan of this tree |
| SBOM | Not generated for current source/image |
| Image build / image scan | Not run; resource budget and MinIO source block |
| OCI registry digest | PostgreSQL digest pinned from prior source evidence; MinIO `MINIO_IMAGE_SOURCE_BLOCKED` |
| Cosign signing/verification | Not run; no built image artifact |
| Provenance attestation | Candidate only; no trusted build provenance |
| Non-root image config | Dockerfile source review only |
| Multi-architecture output | Not built or verified |
| Static source audit | Offline fixture; latest result is in `VERIFICATION_MATRIX.json` |
| OCI revision argument at Compose build | GAP: Compose does not consistently pass `SOURCE_COMMIT`; labels may say `unknown` |
| Read-only root filesystem | FEASIBILITY_NOT_VERIFIED: writable paths/tmpfs not tested |
| Mutable GitHub Action tags | REVIEW_REQUIRED; do not pin to an unverified commit hash |

The static audit is not a vulnerability scan. `govulncheck`, Trivy, Syft, and
Cosign availability are reported without installing anything. No current-source
vulnerability scan, SBOM, image build, signature, provenance, read-only
runtime, or multi-architecture build was performed. Next safe step: install or
use approved bounded scanners after reviewing their source and run them against
the current module; build images only after resource and immutable image-source
gates pass. No unbounded tool download or registry credential was used here.
