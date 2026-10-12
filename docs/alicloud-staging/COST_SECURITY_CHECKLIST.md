# Alibaba Cloud staging-sim cost and security checklist

No resources have been created and no account-specific price has been verified. The earlier ¥100–150 budget is a planning ceiling only, not a quote or cost guarantee. Confirm all values in the actual region/account immediately before each stage.

## Cost approval

- [ ] One ECS only for Level 1; candidate 4 vCPU / 16 GiB x86_64, pay-as-you-go, with an explicitly sized system disk.
- [ ] Record ECS hourly/secondly rate and minimum billable interval, public IPv4/bandwidth, disk, snapshots, data transfer and tax.
- [ ] Include OSS capacity, requests, outbound traffic, ACR storage/transfer, and any CloudMonitor/OTel components.
- [ ] Do not create RDS, ACK, paid SLB, NAT Gateway, KMS, or paid observability by default.
- [ ] Add separate approval before optional ECS-B; remove it after cross-host test only through reviewed run-owned resource cleanup.
- [ ] Use ResourceTTL tags as reminders only; set a calendar deletion review and reconcile residual resources in console.
- [ ] Export billing/resource inventory and compare actual cost to the approved ceiling after every run.

Alibaba ECS billing can vary by region, instance family, disk, network mode and minimum billing interval; never estimate a precise total from a generic instance label. Check the account's live pricing calculator and billing terms before plan/apply. See [ECS pricing and billing docs](https://www.alibabacloud.com/help/en/ecs/instance-types) and [pay-as-you-go billing](https://help.aliyun.com/en/ecs/pay-as-you-go-1).

## Identity, network and data

- [ ] SSH key pair is imported by the operator; private key stays off the ECS and out of the repo.
- [ ] SSH ingress, if needed, is a current single operator `/32`; no API, PostgreSQL or Gateway public ingress.
- [ ] API is reached over SSH tunnel to loopback; PostgreSQL and Gateway private rules are limited to the run-owned security group.
- [ ] Worker ECS uses a RAM role with only required OSS read/write/delete and bucket metadata/versioning reads, scoped to a run prefix. No AccessKey in Compose, Terraform, shell history or logs.
- [ ] OSS bucket blocks public access, requires TLS, has a reviewed encryption policy, versioning disabled for immutable-put semantics, and no lifecycle purge during acceptance.
- [ ] PostgreSQL uses CA-verified TLS; distinct migrator/API/Worker accounts; runtime roles cannot own schema or execute DDL.
- [ ] OIDC fixture and test certificates have short expiry, limited roles and are identified as test-only; no real user identity data.
- [ ] Compose images run non-root with resource limits; use immutable digests and source revision labels.
- [ ] Run-scoped logs and evidence are access-restricted and redacted before export.
- [ ] Backups are encrypted, hash-verified, restored only to a separate run-owned target, and not retained past the approved window.

## Supply chain and cleanup

- [ ] Build from a clean/recorded source SHA and tree; record dirty state if not clean.
- [ ] Generate SBOM and scan all application images and dependencies; preserve tool versions and unmodified output.
- [ ] Verify image signature/provenance before deploy when an approved signer is configured; this repository has no live ACR/signing setup yet.
- [ ] Deploy and roll back by digest only; never use `latest`.
- [ ] Before stopping or deleting anything, inventory ECS IDs, disks, snapshots, VPC/vSwitch/SG, OSS bucket/prefix and ACR images against RunID tags.
- [ ] `scripts/alicloud/cleanup.sh` only removes Compose containers/network and preserves named PostgreSQL data volume; cloud cleanup remains a separate explicit console/Terraform action after approval.
- [ ] Verify no residual resources and no ongoing billing after cleanup; archive Evidence independently and never delete prior Evidence to save space.

Official security reference: [ECS security best practices](https://www.alibabacloud.com/help/en/ecs/user-guide/best-security-practices), [attaching an instance RAM role](https://www.alibabacloud.com/help/en/ecs/user-guide/attach-an-instance-ram-role-to-an-ecs-instance), and [security group practices](https://www.alibabacloud.com/help/en/ecs/user-guide/start-using-security-groups).
