# Alibaba Cloud OSS compatibility assessment

Status: separate OSS-native adapter implemented but not compiled or live-tested in this resource-constrained turn. `CLOUD_OSS_CONTRACT_VERIFIED` is **NOT CLAIMED**. Existing MinIO configuration and S3 adapter remain unchanged.

The current `internal/artifacts/s3.go` path uses MinIO Go SDK v7, static V4 credentials, bucket existence/create checks, Put/Get/Head/Delete, SHA256 user metadata and conditional immutable writes. The current application does not use ETag as SHA256 and does not rely on multipart, bucket lifecycle, tags, ACLs, or MinIO-only admin operations for artifact I/O.

Alibaba's S3 compatibility documents virtual-hosted addressing and an operation-specific compatibility boundary; OSS `PutObject` does not implement S3 conditional `If-None-Match` semantics. The OSS-native SDK supports Signature V4 and ECS RAM-role credentials. The adapter therefore uses the official Alibaba Go SDK v2, regional HTTPS endpoint, RAM role provider, `GetBucketInfo`, `GetBucketVersioning`, `PutObject`, `GetObject`, `HeadObject`, and `DeleteObject`. It sends OSS `x-oss-forbid-overwrite: true`, MD5 for transfer integrity, and an application SHA256 metadata value; downloads are SHA256-verified. It never interprets ETag as SHA256 and never creates a bucket.

The adapter now fails startup if bucket versioning is `Enabled`. OSS documents that forbid-overwrite may be ignored for versioned buckets; `Suspended` or never-enabled versioning is therefore a precondition for the application's immutable key contract. This must also be confirmed in the live account. The adapter's `GetBucketInfo` and `GetBucketVersioning` checks require read permissions in the Worker RAM role.

## Operation and permission inventory

| Operation | OSS API | Intended permission | Behavior |
|---|---|---|---|
| Startup bucket check | GetBucketInfo | Bucket metadata read | Does not create bucket |
| Immutable put | PutObject | Put object in the run prefix | Sends SHA256 metadata, MD5, forbid-overwrite; HEAD verifies result |
| Read | HeadObject + GetObject | Read object metadata/body | Requires SHA metadata, checks length and recomputed SHA256 |
| Inspect | HeadObject | Read object metadata | Reports SHA from application metadata, never ETag |
| Delete | DeleteObject | Delete object in run prefix | Used only by explicit retention/application cleanup; no automatic purge |
| Startup write-policy check | GetBucketVersioning | Bucket versioning status read | Fails if Enabled |

Policy must scope object operations to `acs:oss:*:*:<bucket>/tasklab/<run-id>/*` (confirm exact OSS policy resource syntax in the account console). No bucket create/delete, policy update, lifecycle, versioning, ACL, or account-wide list rights are required. This is a policy draft only; do not attach it without manual review.

## Contract tests and remaining checks

The live contract test requires an explicit opt-in, run ID, HTTPS regional endpoint, bucket, region, and ECS RAM role. It writes only unique objects under the run prefix and removes only those exact objects. It must verify Put/Get/Head, metadata and content SHA256, missing object, duplicate key semantics, concurrent conflicting writers, timeout/unknown outcome recovery, and required IAM denial for operations outside the prefix. It must be run from ECS with the attached role, not with long-lived keys. The current worktree has only fake-client tests; no live OSS test has run.

Account-level contract gates before use:

- Endpoint matches the selected region and TLS certificate validates.
- SDK signing mode is V4 and server accepts the SDK's canonical request.
- Forbid-overwrite works under simultaneous writes and versioning is not Enabled.
- Duplicate/unknown-outcome HEAD verification sees the committed object as expected.
- SHA metadata survives Put/Head/Get exactly; body hash matches independently.
- `NoSuchKey`, `AccessDenied`, timeout, throttling and service errors are classified correctly.
- Read-after-write consistency and any regional endpoint behavior are demonstrated in the target account.
- Restore to a different prefix/bucket and recompute content hashes.
- MinIO contract tests still run separately; OSS cannot substitute for MinIO evidence.

References (official Alibaba Cloud): [S3 compatibility](https://www.alibabacloud.com/help/en/oss/developer-reference/compatibility-with-amazon-s3), [PutObject](https://www.alibabacloud.com/help/en/oss/developer-reference/putobject), [prevent object overwrite](https://www.alibabacloud.com/help/en/oss/user-guide/prevent-file-overwrite), and [Go SDK v2](https://www.alibabacloud.com/help/en/oss/developer-reference/manual-for-go-sdk-v2/).
