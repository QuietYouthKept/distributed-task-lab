# Terraform security review

Status: `TERRAFORM_STATIC_VERIFIED = PASS`; `ALICLOUD_RESOURCES_NOT_CREATED = TRUE`. This is a source/configuration review only. No plan, apply, destroy, credential lookup, or cloud API request was performed.

## Findings and changes

| Area | Finding | Disposition |
|---|---|---|
| Resource gates | All billable VPC, vSwitch, security-group, and ECS resources have `count = 0` unless both `enable_resources` and `budget_approved` are true. | Confirmed by source inspection. A plan was not run. |
| SSH ingress | A non-wildcard CIDR could still cover half the IPv4 address space (for example `/1`). | Fixed: only an empty CIDR or a single IPv4 `/32` is accepted. Empty keeps public bandwidth at zero and creates no SSH rule. |
| API/PostgreSQL ingress | No public API or PostgreSQL rule exists. PostgreSQL and Gateway ingress use the run-owned Security Group as source and `intranet` NIC type. | Confirmed by source inspection. API is loopback-bound in Compose and intended to be reached by SSH tunnel. |
| Network ranges | VPC/vSwitch CIDRs had no validation. | Fixed: IPv4 syntax is required and vSwitch must resolve to a subnet contained in the VPC; subnet split is bounded to 8 bits. Terraform minimum raised to 1.9 for cross-variable validation. |
| ECS-B cost | ECS-B inherited the Core instance type. | Fixed: ECS-B now requires an explicit, different `worker_b_instance_type`; it remains disabled by default. Choose a small type in the target account only after checking Worker memory/CPU and live price. |
| Public connectivity | Public bandwidth defaults to zero; setting an SSH `/32` enables 1 Mbps outbound bandwidth and the public SSH rule. No NAT Gateway or load balancer is defined. | Kept explicit. With no public connectivity, image/module/artifact egress must be supplied through a separately reviewed private path or preloaded images; do not silently add paid NAT. |
| Disk and residual cost | Core and optional Worker each receive a 40-GiB ESSD system disk by default. Stopping ECS does not release disks. | Documented as a recurring/retained-cost item requiring account-console review and inventory-based cleanup. |
| State/secrets | Provider credentials are not declared in variables or files; sample tfvars contains placeholders only. | Confirmed by source inspection and credential-pattern scan. The local state/plan can still contain resource metadata; keep both private and outside Git. |
| Provider compatibility | Provider source/version are pinned to `aliyun/alicloud` 1.274.0; the deprecated security-group `name` argument was replaced with `security_group_name`. | Fixed and validated against the locked provider. This is a new, false-by-default template with no existing managed resource state in scope. No plan was run, so no resource replacement analysis is claimed. |

## Tool verification

- Terraform 1.14.9 Windows AMD64 archive was downloaded from the official HashiCorp release site into the E: temporary tools directory. Its SHA-256 matched the official checksum manifest: `5518b04ea610115b17b427b57d8ec1d50a42a2ad63ab2d7a453b788d6c804d14`.
- The provider lock pins `aliyun/alicloud` 1.274.0 and records its package checksums. The previously installed provider was signature-verified by Terraform (HashiCorp partner key ID `47422B4AA9FA381B`).
- `terraform fmt -check -recursive infra/alicloud/staging` — PASS.
- `terraform validate -no-color` with the existing E: `TF_DATA_DIR` and plugin cache — PASS, no warnings after the argument change.
- The provider documentation marks `security_group_name` available since 1.239.0 and `name` deprecated since 1.239.0. The pinned 1.274.0 schema validation also passed with the new argument.
- No `terraform plan`, `apply`, or `destroy` was run. No Alibaba Cloud credential or account was accessed.

Official sources: [Terraform 1.14.9 release](https://releases.hashicorp.com/terraform/1.14.9/), [official checksum manifest](https://releases.hashicorp.com/terraform/1.14.9/terraform_1.14.9_SHA256SUMS), and [Aliyun provider 1.274.0 security group resource](https://registry.terraform.io/providers/aliyun/alicloud/1.274.0/docs/resources/security_group).

## Readiness

Terraform format and provider-schema validation are complete for the staging template. This does not establish account-specific plan safety, resource creation, cost approval, or cloud deployment readiness. Resource creation remains disabled by default and requires both explicit gates plus separate user authorization.
