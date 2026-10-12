#!/usr/bin/env bash
set -euo pipefail

[[ "${1:-}" == --confirm-run-id && "${2:-}" == "${TASKLAB_RUN_ID:-}" ]] || {
  echo 'Usage: TASKLAB_RUN_ID=<id> scripts/alicloud/postgres-only-deploy.sh --confirm-run-id <same-id>' >&2
  exit 2
}
root=$(git rev-parse --show-toplevel)
"$root/scripts/alicloud/postgres-only-preflight.sh" --confirm-run-id "$TASKLAB_RUN_ID"
project="tasklab-postgres-$TASKLAB_RUN_ID"
compose=(docker compose --project-name "$project" --env-file "$TASKLAB_COMPOSE_ENV" -f "$root/deployments/alicloud/compose.postgres-only.yml")

printf 'Pulling only the reviewed PostgreSQL digest for run %s.\n' "$TASKLAB_RUN_ID"
"${compose[@]}" pull postgres
printf 'Building the Migrator locally from source %s; no ACR is used.\n' "$TASKLAB_SOURCE_SHA"
"${compose[@]}" build db-migrator
"${compose[@]}" up -d postgres

deadline=$((SECONDS + 120))
container=$("${compose[@]}" ps -q postgres)
[[ -n "$container" ]] || { echo 'FAIL: PostgreSQL container was not created.' >&2; exit 1; }
while (( SECONDS < deadline )); do
  state=$(docker inspect --format '{{.State.Status}}' "$container")
  health=$(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}' "$container")
  if [[ "$state" == running && "$health" == healthy ]]; then break; fi
  if [[ "$state" == exited || "$state" == dead ]]; then
    docker logs "$container" >&2
    echo "FAIL: PostgreSQL exited before readiness (state=$state)." >&2
    exit 1
  fi
  sleep 2
done
[[ "$(docker inspect --format '{{.State.Health.Status}}' "$container")" == healthy ]] || {
  docker logs "$container" >&2
  echo 'FAIL: PostgreSQL did not become healthy within 120 seconds.' >&2
  exit 1
}

"${compose[@]}" run --rm --no-deps db-migrator
"${compose[@]}" run --rm --no-deps db-runtime-grants

printf 'PASS: PostgreSQL-only database initialized for run %s. No OSS, API, Worker, OIDC, Gateway, or ACR services were started.\n' "$TASKLAB_RUN_ID"
printf 'Next: scripts/alicloud/postgres-acceptance.sh --confirm-run-id %s\n' "$TASKLAB_RUN_ID"
