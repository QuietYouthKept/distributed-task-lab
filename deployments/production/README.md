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
deliberately refuses to start in production: no task execution type has passed
the complete provider, shared-artifact, fencing, and recovery acceptance gate.
An explicit S3 Store can be used by development/test workers, but that does not
approve production execution. Do not replace this fail-closed guard until the
end-to-end gates are satisfied.
