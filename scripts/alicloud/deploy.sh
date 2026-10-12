#!/usr/bin/env bash
set -euo pipefail

[[ "${1:-}" == --confirm-run-id && "${2:-}" == "${TASKLAB_RUN_ID:-}" ]] || {
  echo 'Usage: TASKLAB_RUN_ID=<id> scripts/alicloud/deploy.sh --confirm-run-id <same-id>' >&2; exit 2;
}
root=$(git rev-parse --show-toplevel)
"$root/scripts/alicloud/validate.sh"
cd "$root"
project="tasklab-alicloud-${TASKLAB_RUN_ID}"
compose=(docker compose --project-name "$project" --env-file "$TASKLAB_COMPOSE_ENV" -f deployments/alicloud/compose.single.yml)

"${compose[@]}" pull
"${compose[@]}" up -d postgres identity gateway-simulator
"${compose[@]}" up --no-deps db-migrator
"${compose[@]}" up --no-deps db-runtime-grants
"${compose[@]}" up -d api worker
"$root/scripts/alicloud/health.sh" --confirm-run-id "$TASKLAB_RUN_ID"
