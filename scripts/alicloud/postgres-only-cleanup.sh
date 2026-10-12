#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ "${TASKLAB_RUN_ID:-}" =~ ^[A-Za-z0-9][A-Za-z0-9-]{7,39}$ ]] || fail 'TASKLAB_RUN_ID must be a unique 8-40 character run ID'
[[ "${1:-}" == --confirm-run-id && "${2:-}" == "$TASKLAB_RUN_ID" ]] || fail 'Expected --confirm-run-id <same TASKLAB_RUN_ID>'
[[ "${TASKLAB_COMPOSE_ENV:-}" == "/etc/tasklab/secrets/$TASKLAB_RUN_ID/postgres-only.env" && -s "$TASKLAB_COMPOSE_ENV" ]] || fail 'run-scoped PostgreSQL Compose env file is required'

root=$(git rev-parse --show-toplevel)
project="tasklab-postgres-$TASKLAB_RUN_ID"
compose=(docker compose --project-name "$project" --env-file "$TASKLAB_COMPOSE_ENV" -f "$root/deployments/alicloud/compose.postgres-only.yml")
mapfile -t ids < <("${compose[@]}" ps -a -q)
if ((${#ids[@]} == 0)); then
  printf 'No containers found for run %s; no resources changed.\n' "$TASKLAB_RUN_ID"
  exit 0
fi
for id in "${ids[@]}"; do
  owner=$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$id")
  [[ "$owner" == "$project" ]] || fail 'a selected container lacks the expected Compose ownership label; nothing removed'
done

printf 'Run-owned containers to stop/remove (named PostgreSQL volume is preserved):\n'
printf '%s\n' "${ids[@]}"
volume="tasklab-postgres-${TASKLAB_RUN_ID}-data"
if docker volume inspect "$volume" >/dev/null 2>&1; then
  printf 'Preserved data volume: %s\n' "$volume"
fi
[[ "${TASKLAB_CLEANUP_APPROVED:-}" == "$TASKLAB_RUN_ID" ]] || {
  printf 'DRY RUN: set TASKLAB_CLEANUP_APPROVED to this exact run ID to remove only these Compose containers/network.\n'
  exit 0
}
"${compose[@]}" down --remove-orphans
printf 'PASS: run-owned PostgreSQL containers/network removed; database volume preserved.\n'
