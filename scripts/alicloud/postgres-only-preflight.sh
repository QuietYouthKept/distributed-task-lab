#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "required command missing: $1"; }

[[ "${TASKLAB_RUN_ID:-}" =~ ^[A-Za-z0-9][A-Za-z0-9-]{7,39}$ ]] || fail 'TASKLAB_RUN_ID must be a unique 8-40 character run ID'
[[ "${1:-}" == --confirm-run-id && "${2:-}" == "$TASKLAB_RUN_ID" ]] || fail 'Expected --confirm-run-id <same TASKLAB_RUN_ID>'
[[ "${TASKLAB_SOURCE_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'TASKLAB_SOURCE_SHA must be the reviewed 40-character commit SHA'
[[ "${TASKLAB_SECRET_DIR:-}" == "/etc/tasklab/secrets/$TASKLAB_RUN_ID" ]] || fail 'TASKLAB_SECRET_DIR must exactly match this run ID'
[[ "${TASKLAB_COMPOSE_ENV:-}" == "$TASKLAB_SECRET_DIR/postgres-only.env" && -s "$TASKLAB_COMPOSE_ENV" ]] || fail 'TASKLAB_COMPOSE_ENV must be the private run-scoped postgres-only.env'
[[ "${TASKLAB_PG_TLS_DIR:-}" == "/etc/tasklab/tls/$TASKLAB_RUN_ID/postgres" ]] || fail 'TASKLAB_PG_TLS_DIR must exactly match this run ID'
[[ "${TASKLAB_EVIDENCE_DIR:-}" == "/var/lib/tasklab/evidence/$TASKLAB_RUN_ID/postgres-only" ]] || fail 'TASKLAB_EVIDENCE_DIR must exactly match this run ID'
expected_postgres_image='postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
[[ "${TASKLAB_POSTGRES_IMAGE:-}" == "$expected_postgres_image" ]] || fail 'TASKLAB_POSTGRES_IMAGE must equal the reviewed official PostgreSQL 16 Alpine digest'
[[ "${TASKLAB_PG_PRIVATE_IP:-}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || fail 'TASKLAB_PG_PRIVATE_IP must be an IPv4 private address'
[[ "${TASKLAB_SOURCE_SHA}" == "$(git rev-parse HEAD)" ]] || fail 'checkout HEAD does not match TASKLAB_SOURCE_SHA'
[[ -z "$(git status --porcelain=v1)" ]] || fail 'source checkout must be clean before acceptance'

need docker; need python3; need openssl; need ip; need awk; need df; need stat
need go; need timeout; need sha256sum; need grep
docker compose version >/dev/null 2>&1 || fail 'Docker Compose v2 is required'
docker info >/dev/null 2>&1 || fail 'Docker Engine is unavailable to the current user'
go_version=$(GOTOOLCHAIN=local go version)
go_platform="$(GOTOOLCHAIN=local go env GOOS)/$(GOTOOLCHAIN=local go env GOARCH)"
export TASKLAB_GO_VERSION="$go_version" TASKLAB_GO_PLATFORM="$go_platform"
python3 - <<'PY'
import os, re
match = re.search(r'\bgo(\d+)\.(\d+)\.(\d+)\b', os.environ["TASKLAB_GO_VERSION"])
if not match or tuple(map(int, match.groups())) < (1, 26, 9):
    raise SystemExit("FAIL: local Go toolchain must be Go 1.26.9 or newer; auto-download is disabled")
if os.environ["TASKLAB_GO_PLATFORM"] != "linux/amd64":
    raise SystemExit("FAIL: PostgreSQL acceptance requires Go linux/amd64 to match the pinned image platform")
PY
[[ -f /proc/meminfo ]] || fail '/proc/meminfo is unavailable'
mem_kib=$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)
(( mem_kib >= 6291456 )) || fail 'at least 6 GiB available RAM required; no pull or service was started'
evidence_kib=$(df -Pk /var/lib 2>/dev/null | awk 'NR==2 {print $4}')
[[ -n "$evidence_kib" ]] && (( evidence_kib >= 5242880 )) || fail 'at least 5 GiB evidence space required'

python3 - <<'PY'
import ipaddress, os
address = ipaddress.ip_address(os.environ["TASKLAB_PG_PRIVATE_IP"])
if address.version != 4 or not address.is_private or address.is_loopback or address.is_link_local:
    raise SystemExit("FAIL: PostgreSQL endpoint must be an assigned private IPv4 address")
PY
ip -o -4 addr show | awk '{split($4,a,"/"); print a[1]}' | grep -Fxq "$TASKLAB_PG_PRIVATE_IP" || fail 'TASKLAB_PG_PRIVATE_IP is not assigned to this ECS host'

check_private_dir() {
  local path=$1 mode
  [[ -d "$path" ]] || fail "required directory missing: $path"
  mode=$(stat -c '%a' "$path")
  (( (8#$mode & 077) == 0 )) || fail "directory permissions must not grant group/other access: $path"
}
check_private_file() {
  local path=$1 mode
  [[ -s "$path" ]] || fail "required non-empty file missing: $path"
  mode=$(stat -c '%a' "$path")
  (( (8#$mode & 077) == 0 )) || fail "file permissions must not grant group/other access: $path"
}
check_private_dir "$TASKLAB_SECRET_DIR"
check_private_file "$TASKLAB_COMPOSE_ENV"
for name in postgres-superuser-password tasklab-migrator-password tasklab-api-password tasklab-worker-a-password \
  migrator-database-url postgres-acceptance-dsn; do
  check_private_file "$TASKLAB_SECRET_DIR/$name"
done
check_private_dir "$TASKLAB_PG_TLS_DIR"
for name in ca.crt server.crt server.key; do check_private_file "$TASKLAB_PG_TLS_DIR/$name"; done
[[ ! -e "$TASKLAB_PG_TLS_DIR/ca.key" ]] || fail 'CA private key must not be mounted with PostgreSQL TLS materials'
openssl verify -CAfile "$TASKLAB_PG_TLS_DIR/ca.crt" "$TASKLAB_PG_TLS_DIR/server.crt" >/dev/null 2>&1 || fail 'PostgreSQL server certificate does not validate under the run CA'
openssl x509 -in "$TASKLAB_PG_TLS_DIR/server.crt" -noout -checkip "$TASKLAB_PG_PRIVATE_IP" -checkhost postgres >/dev/null 2>&1 || fail 'PostgreSQL certificate must contain the private IP and postgres DNS SANs'

export TASKLAB_DSN_FILE="$TASKLAB_SECRET_DIR/postgres-acceptance-dsn"
python3 - <<'PY'
import ipaddress, os, pathlib, urllib.parse
u = urllib.parse.urlparse(pathlib.Path(os.environ["TASKLAB_DSN_FILE"]).read_text().strip())
if u.scheme not in ("postgres", "postgresql") or u.path != "/postgres":
    raise SystemExit("FAIL: acceptance DSN must target the postgres maintenance database")
if ipaddress.ip_address(u.hostname or "") != ipaddress.ip_address(os.environ["TASKLAB_PG_PRIVATE_IP"]):
    raise SystemExit("FAIL: acceptance DSN host must equal the selected ECS private IPv4")
query = urllib.parse.parse_qs(u.query)
if query.get("sslmode", [""])[0] != "verify-full" or query.get("sslrootcert", [""])[0] != os.environ["TASKLAB_PG_TLS_DIR"] + "/ca.crt":
    raise SystemExit("FAIL: acceptance DSN must use verify-full and this run's CA")
if not u.username or not u.password:
    raise SystemExit("FAIL: isolated disposable PostgreSQL admin credentials are missing")
PY

root=$(git rev-parse --show-toplevel)
project="tasklab-postgres-$TASKLAB_RUN_ID"
compose=(docker compose --project-name "$project" --env-file "$TASKLAB_COMPOSE_ENV" -f "$root/deployments/alicloud/compose.postgres-only.yml")
"${compose[@]}" config --quiet || fail 'postgres-only Compose configuration is invalid'
existing=$(docker ps -aq --filter "label=com.docker.compose.project=$project")
[[ -z "$existing" ]] || fail 'run-scoped Compose project already has containers; refusing to reuse it'
volume="tasklab-postgres-${TASKLAB_RUN_ID}-data"
if docker volume inspect "$volume" >/dev/null 2>&1; then fail 'run-scoped PostgreSQL volume already exists; refusing to reuse it'; fi
printf 'PASS: PostgreSQL-only preflight; run=%s source=%s RAM_GiB=%s disk_GiB=%s go=%s platform=%s. Read-only; no image pull or service start.\n' \
  "$TASKLAB_RUN_ID" "$TASKLAB_SOURCE_SHA" "$((mem_kib / 1048576))" "$((evidence_kib / 1048576))" "$go_version" "$go_platform"
