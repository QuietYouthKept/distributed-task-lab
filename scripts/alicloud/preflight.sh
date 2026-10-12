#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "required command missing: $1"; }

[[ "${TASKLAB_RUN_ID:-}" =~ ^[A-Za-z0-9][A-Za-z0-9-]{7,39}$ ]] || fail 'TASKLAB_RUN_ID must be a unique 8-40 character run id'
[[ "${TASKLAB_SECRET_DIR:-}" == /etc/tasklab/secrets/* ]] || fail 'TASKLAB_SECRET_DIR must be an isolated /etc/tasklab/secrets/<run-id> directory'
[[ "${TASKLAB_EVIDENCE_DIR:-}" == /var/lib/tasklab/evidence/* ]] || fail 'TASKLAB_EVIDENCE_DIR must be an isolated /var/lib/tasklab/evidence/<run-id> directory'
[[ "${TASKLAB_PG_TLS_DIR:-}" == /etc/tasklab/tls/* ]] || fail 'TASKLAB_PG_TLS_DIR must be an isolated /etc/tasklab/tls/<run-id> directory'
[[ "${TASKLAB_IDENTITY_TLS_DIR:-}" == /etc/tasklab/tls/* ]] || fail 'TASKLAB_IDENTITY_TLS_DIR must be an isolated /etc/tasklab/tls/<run-id> directory'
[[ "${TASKLAB_API_TLS_DIR:-}" == /etc/tasklab/tls/* ]] || fail 'TASKLAB_API_TLS_DIR must be an isolated /etc/tasklab/tls/<run-id> directory'
[[ "${TASKLAB_COMPOSE_ENV:-}" == "$TASKLAB_SECRET_DIR/compose.env" && -s "$TASKLAB_COMPOSE_ENV" ]] || fail 'TASKLAB_COMPOSE_ENV must be the run-scoped mode-0600 compose.env file'
need docker
docker compose version >/dev/null
docker info >/dev/null 2>&1 || fail 'Docker Engine is unavailable to the current user'

mem_kib=$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)
(( mem_kib >= 6291456 )) || fail 'at least 6 GiB available RAM is required before container acceptance'
free_kib=$(df -Pk /var/lib 2>/dev/null | awk 'NR==2 {print $4}')
[[ -n "$free_kib" ]] || fail 'evidence filesystem is unavailable'
(( free_kib >= 5242880 )) || fail 'at least 5 GiB free space is required for images, database and evidence'

for name in postgres-superuser-password tasklab-migrator-password tasklab-api-password tasklab-worker-a-password \
  migrator-database-url api-database-url worker-a-database-url gateway-api-key api-cursor-signing-key; do
  [[ -s "$TASKLAB_SECRET_DIR/$name" ]] || fail "required run-scoped secret file missing: $name"
done
[[ -s "${TASKLAB_PG_TLS_DIR:-}/ca.crt" && -s "${TASKLAB_PG_TLS_DIR:-}/server.crt" && -s "${TASKLAB_PG_TLS_DIR:-}/server.key" ]] || fail 'PostgreSQL TLS CA/server certificate/key files are required'
[[ -s "${TASKLAB_IDENTITY_TLS_DIR:-}/identity.crt" && -s "${TASKLAB_IDENTITY_TLS_DIR:-}/identity.key" ]] || fail 'identity fixture TLS certificate/key files are required'
[[ -s "${TASKLAB_API_TLS_DIR:-}/api.crt" && -s "${TASKLAB_API_TLS_DIR:-}/api.key" && -s "${TASKLAB_API_TLS_DIR:-}/test-ca.crt" ]] || fail 'API TLS certificate/key and test CA files are required'

printf 'PASS: run=%s host_arch=%s mem_available_gib=%s evidence_free_gib=%s docker=%s\n' \
  "$TASKLAB_RUN_ID" "$(uname -m)" "$((mem_kib / 1048576))" "$((free_kib / 1048576))" "$(docker version --format '{{.Server.Version}}')"
printf 'NOTICE: This preflight is read-only; it does not pull images or start/stop containers.\n'
