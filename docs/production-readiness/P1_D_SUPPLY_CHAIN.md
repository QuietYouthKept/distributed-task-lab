# P1-D Local Supply Chain Status

Date: 2026-10-11 (Asia/Shanghai)

## Verified for the local identity fixture image

At source commit `04621d8343b98aed56ab6898cf527783ecd2ddf0`, the following
completed:

- `docker build --pull=false --tag tasklab-oidc-fixture:local-04621 --build-arg SOURCE_COMMIT=04621d8343b98aed56ab6898cf527783ecd2ddf0 --build-arg HTTP_PROXY=http://host.docker.internal:7890 --build-arg HTTPS_PROXY=http://host.docker.internal:7890 -f deployments/Dockerfile.oidc-fixture .` — exit 0.
- `docker image inspect` reported configured user `tasklab` and OCI source/revision/description labels.
- Builder and runtime base references use the Dockerfile's pinned image digests.
- Build context transfer reported 228.07 KiB. No container was started.

Evidence directory:
`<REDACTED_LOCAL_ARTIFACT_ROOT>\production-readiness\evidence\oidc-image-build-20261011T132000+0800`.

The value `sha256:56ef5f7abacddf25ad27822294f575b5f26a0e444a7c142aa87b860907a075a9`
is the local Docker image ID returned by `docker image inspect`; it is **not** a
registry manifest digest and does not establish registry distribution or
reproducibility on a clean host.

## Not yet verified

- No runtime execution/read-only-root/capability-drop test was run.
- No SBOM, vulnerability scan, provenance attestation, or Cosign signature was
  generated or verified for this image.
- No API/Worker/Migration/Gateway image set was rebuilt against one clean source
  SHA in this increment.
- Available RAM after build was 5.96 GiB, below the 6 GiB container E2E gate;
  no Compose service was started.
- A local image signature would still not establish an organizational
  production trust root.

Status: `P1_D_LOCAL_IMPLEMENTED: PARTIAL`; `P1_D_LOCAL_VERIFIED: IMAGE_BUILD_AND_METADATA_ONLY`.
Registry, SBOM, signing, provenance, and runtime hardening remain open.
`PRODUCTION_NOT_APPROVED` remains unchanged.
