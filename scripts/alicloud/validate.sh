#!/usr/bin/env bash
set -euo pipefail

root=$(git rev-parse --show-toplevel)
cd "$root"
"$root/scripts/alicloud/preflight.sh"

for name in TASKLAB_POSTGRES_IMAGE TASKLAB_IMAGE_MIGRATOR TASKLAB_IMAGE_GATEWAY TASKLAB_IMAGE_IDENTITY TASKLAB_IMAGE_API TASKLAB_IMAGE_WORKER; do
  value=${!name:-}
  [[ "$value" =~ @sha256:[0-9a-f]{64}$ ]] || { printf 'FAIL: %s must be pinned by OCI digest\n' "$name" >&2; exit 1; }
done
expected_postgres_image='postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
[[ "${TASKLAB_POSTGRES_IMAGE}" == "$expected_postgres_image" ]] || { echo 'FAIL: PostgreSQL must use the reviewed official PostgreSQL 16 Alpine digest.' >&2; exit 1; }

docker compose --project-name "tasklab-alicloud-${TASKLAB_RUN_ID}" --env-file "$TASKLAB_COMPOSE_ENV" \
  -f deployments/alicloud/compose.single.yml config --quiet
docker compose --project-name "tasklab-alicloud-worker-${TASKLAB_RUN_ID}" --env-file "$TASKLAB_COMPOSE_ENV" \
  -f deployments/alicloud/compose.worker-b.yml config --quiet
printf 'PASS: compose interpolation and OCI digest pins validated; no services started.\n'
