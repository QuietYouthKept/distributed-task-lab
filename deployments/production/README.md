# Production configuration boundary

The Compose file in the parent directory is a **development-only** setup. It
uses an in-container PostgreSQL instance, development credentials, disabled
database TLS, and local artifact volumes. It must not be promoted to a public
or multi-user environment.

Production startup requires `APP_PROFILE=production`; the application rejects:

- empty or identical API/admin bearer tokens;
- `sslmode=disable`, weak PostgreSQL passwords, and local database hosts;
- local artifact directories in place of a shared `s3://` or `minio://` URI.
- deterministic evaluation or a gateway without an HTTPS endpoint, profile,
  and API key.

Inject secrets through the target platform's secret mechanism. Do not commit
`.env` files, private database URLs, provider keys, or OIDC credentials. A
future deployment manifest must use a least-privilege runtime database user,
TLS verification appropriate to its provider, and a shared artifact adapter.

The API production profile can validate these boundaries today, but the worker
deliberately refuses to start in production: its artifact handlers still use
filesystem APIs and no MinIO/S3 adapter is wired in yet. This fail-closed guard
prevents an `s3://` or `minio://` value from being interpreted as a local path.
