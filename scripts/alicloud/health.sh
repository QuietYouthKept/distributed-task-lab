#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == --confirm-run-id && "${2:-}" == "${TASKLAB_RUN_ID:-}" ]] || { echo 'Expected --confirm-run-id <same TASKLAB_RUN_ID>' >&2; exit 2; }
root=$(git rev-parse --show-toplevel)
compose=(docker compose --project-name "tasklab-alicloud-${TASKLAB_RUN_ID}" --env-file "$TASKLAB_COMPOSE_ENV" -f "$root/deployments/alicloud/compose.single.yml")
"${compose[@]}" ps
for service in postgres gateway-simulator identity api worker; do
  cid=$("${compose[@]}" ps -q "$service")
  [[ -n "$cid" ]] || { printf 'FAIL: service not created: %s\n' "$service" >&2; exit 1; }
  state=$(docker inspect --format '{{.State.Status}}' "$cid")
  health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$cid")
  [[ "$state" == running && "$health" == healthy ]] || { printf 'FAIL: %s state=%s health=%s\n' "$service" "$state" "$health" >&2; exit 1; }
done
printf 'PASS: required long-running services are healthy for run %s.\n' "$TASKLAB_RUN_ID"
