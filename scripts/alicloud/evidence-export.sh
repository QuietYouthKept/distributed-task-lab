#!/usr/bin/env bash
set -euo pipefail
[[ "${1:-}" == --confirm-run-id && "${2:-}" == "${TASKLAB_RUN_ID:-}" ]] || { echo 'Expected --confirm-run-id <same TASKLAB_RUN_ID>' >&2; exit 2; }
root=$(git rev-parse --show-toplevel)
out=${TASKLAB_EVIDENCE_DIR:?Set run-scoped evidence directory}
mkdir -p "$out"
chmod 0700 "$out"
git -C "$root" rev-parse HEAD > "$out/source-commit.txt"
git -C "$root" rev-parse 'HEAD^{tree}' > "$out/source-tree.txt"
git -C "$root" status --porcelain=v1 > "$out/source-status.txt"
docker version --format 'server={{.Server.Version}} os={{.Server.Os}} arch={{.Server.Arch}}' > "$out/docker-runtime.txt"
docker image ls --digests --format '{{.Repository}}:{{.Tag}}@{{.Digest}} {{.ID}}' > "$out/local-images.txt"
docker compose --project-name "tasklab-alicloud-${TASKLAB_RUN_ID}" --env-file "$TASKLAB_COMPOSE_ENV" \
  -f "$root/deployments/alicloud/compose.single.yml" ps --format json > "$out/compose-status.json"
for service in postgres db-migrator db-runtime-grants gateway-simulator identity api worker; do
  cid=$(docker compose --project-name "tasklab-alicloud-${TASKLAB_RUN_ID}" --env-file "$TASKLAB_COMPOSE_ENV" \
    -f "$root/deployments/alicloud/compose.single.yml" ps -a -q "$service")
  [[ -z "$cid" ]] || docker logs "$cid" 2>&1 | python3 "$root/scripts/alicloud/redact.py" --secret-dir "$TASKLAB_SECRET_DIR" > "$out/${service}.log"
done
find "$out" -type f ! -name SHA256SUMS -print0 | sort -z | xargs -0 sha256sum > "$out/SHA256SUMS"
printf 'Evidence exported to %s. Review logs for secrets before external sharing; do not include secret directory contents.\n' "$out"
