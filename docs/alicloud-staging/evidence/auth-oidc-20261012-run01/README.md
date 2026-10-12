# Local source verification — 2026-10-12 run 01

Status: OIDC package and local harness tests PASS; Terraform format/provider-schema validation PASS. PostgreSQL, Docker, Race, full-suite, GitHub Actions, and cloud checks were NOT RUN.

- Source commit: `755af6d6b05e457d298c1e2b9fb0c87f6a226be2`
- Source tree: `3ca2667911d58a1a6de54edae19d755aafa115ce`
- Go: `go1.26.9 windows/amd64` (selected with `GOTOOLCHAIN=auto`; see toolchain note)
- Terraform: `1.14.9 windows_amd64`
- No credentials, database, containers, or cloud APIs were used.

## Results

| Check | Exact command | Result |
|---|---|---|
| Auth package | `go test -p 1 ./internal/auth -count=1 -timeout=2m` | PASS, exit 0 |
| Evidence summarizer tests | `python -B -m unittest scripts.alicloud.test_summarize_go_test -v` | PASS, 5 tests, exit 0 |
| Terraform formatting | `terraform -chdir=infra/alicloud/staging fmt -check -recursive` | PASS, exit 0 |
| Terraform schema validation | `terraform -chdir=infra/alicloud/staging validate -no-color` | PASS, exit 0; locked provider 1.274.0 |

The OIDC change bounds refresh attempts for unknown `kid` values and separates refresh serialization from cached-key reads. Unknown keys continue to fail closed. The 5-second cooldown trades immediate discovery of a newly rotated key for bounded outbound refresh pressure; the cached known key remains verifiable during a refresh.

The CI workflow changes were statically reviewed only. No remote run was triggered because the branch was not pushed. Terraform plan/apply/destroy were not run; this validation is not a cloud deployment or account-specific plan review.

`tested-source-sha256.txt` records the exact content hashes of the auth, harness, and Terraform inputs at test time. The auth package files match the source commit above; the harness and Terraform files were local, uncommitted changes at that point and are separately bound by those content hashes.

The initial Go invocation was blocked by a shell-local `GOTOOLCHAIN=local` selection (Go 1.26.6 while `go.mod` requires 1.26.9); the passing command used Go's automatic toolchain selection. No machine-wide toolchain or environment setting was changed.

`SHA256SUMS` covers the evidence files in this directory except itself.
