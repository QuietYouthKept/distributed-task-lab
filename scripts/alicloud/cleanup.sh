#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == --confirm-run-id && "${2:-}" == "${TASKLAB_RUN_ID:-}" ]] || { echo 'Expected --confirm-run-id <same TASKLAB_RUN_ID>' >&2; exit 2; }
root=$(git rev-parse --show-toplevel)
project="tasklab-alicloud-${TASKLAB_RUN_ID}"
compose=(docker compose --project-name "$project" --env-file "$TASKLAB_COMPOSE_ENV" -f "$root/deployments/alicloud/compose.single.yml")
mapfile -t ids < <("${compose[@]}" ps -a -q)
(( ${#ids[@]} > 0 )) || { echo 'FAIL: no containers found for the selected Compose project; nothing removed.' >&2; exit 1; }
for id in "${ids[@]}"; do
  owner=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$id")
  [[ "$owner" == "$project" ]] || { echo 'FAIL: a container does not carry the expected Compose ownership label; nothing removed.' >&2; exit 1; }
done
actual=$(printf '%s\n' "${ids[@]}")
printf 'The following run-owned container IDs will be stopped/removed; named PostgreSQL data volume is preserved:\n%s\n' "$actual"
[[ "${TASKLAB_CLEANUP_APPROVED:-}" == "$TASKLAB_RUN_ID" ]] || { echo 'DRY RUN: set TASKLAB_CLEANUP_APPROVED to this exact run ID to apply. No cloud resource is ever deleted by this script.'; exit 0; }
"${compose[@]}" down --remove-orphans
printf 'PASS: run-owned containers/network removed; data volume preserved. Remove ECS/VPC/OSS only through a separately reviewed cloud inventory and explicit operator action.\n'
