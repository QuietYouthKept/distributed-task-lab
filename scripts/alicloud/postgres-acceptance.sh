#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ "${1:-}" == --confirm-run-id && "${2:-}" == "${TASKLAB_RUN_ID:-}" ]] || fail 'Expected --confirm-run-id <same TASKLAB_RUN_ID>'
[[ "${TASKLAB_SOURCE_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'TASKLAB_SOURCE_SHA must be the reviewed 40-character commit SHA'
root=$(git rev-parse --show-toplevel)
[[ "$(git -C "$root" rev-parse HEAD)" == "$TASKLAB_SOURCE_SHA" ]] || fail 'checkout HEAD does not match TASKLAB_SOURCE_SHA'
[[ -z "$(git -C "$root" status --porcelain)" ]] || fail 'source checkout must be clean before acceptance'
[[ "${TASKLAB_SECRET_DIR:-}" == "/etc/tasklab/secrets/$TASKLAB_RUN_ID" ]] || fail 'secret directory must exactly match the run ID'
[[ "${TASKLAB_COMPOSE_ENV:-}" == "$TASKLAB_SECRET_DIR/postgres-only.env" && -s "$TASKLAB_COMPOSE_ENV" ]] || fail 'run-scoped PostgreSQL Compose env file is required'
[[ "${TASKLAB_PG_PRIVATE_IP:-}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || fail 'TASKLAB_PG_PRIVATE_IP must be the run-owned ECS private IPv4'
expected_postgres_image='postgres:16-alpine@sha256:721873c34ceb9f8d8fc265984940dc982404c105f19ad51be9fdc5970a6080ea'
[[ "${TASKLAB_POSTGRES_IMAGE:-}" == "$expected_postgres_image" ]] || fail 'TASKLAB_POSTGRES_IMAGE must equal the reviewed official PostgreSQL 16 Alpine digest'
dsn_file="$TASKLAB_SECRET_DIR/postgres-acceptance-dsn"
[[ -s "$dsn_file" ]] || fail 'isolated PostgreSQL superuser DSN file missing'
[[ -f /proc/meminfo ]] || fail '/proc/meminfo is unavailable'
mem_kib=$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)
(( mem_kib >= 6291456 )) || fail 'at least 6 GiB available RAM required; no tests launched'
command -v go >/dev/null 2>&1 || fail 'Go toolchain is required'
command -v timeout >/dev/null 2>&1 || fail 'GNU timeout is required'
command -v python3 >/dev/null 2>&1 || fail 'Python 3 is required for private-IP and test-event checks'

export TASKLAB_DSN_FILE="$dsn_file"
export TASKLAB_EXPECTED_PRIVATE_IP=${TASKLAB_PG_PRIVATE_IP:?Set the run-owned ECS private IPv4 address}
export TASKLAB_PG_TLS_DIR=${TASKLAB_PG_TLS_DIR:?Set this run PostgreSQL TLS directory}
export TASKLAB_RUN_ID
python3 - <<'PY'
import ipaddress, os, pathlib, urllib.parse
raw = pathlib.Path(os.environ["TASKLAB_DSN_FILE"]).read_text().strip()
u = urllib.parse.urlparse(raw)
expected = ipaddress.ip_address(os.environ["TASKLAB_EXPECTED_PRIVATE_IP"])
host = ipaddress.ip_address(u.hostname or "")
q = urllib.parse.parse_qs(u.query)
if not expected.is_private or host != expected or u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("FAIL: test DSN must target exactly the explicitly selected private ECS IP")
if u.path != "/postgres" or q.get("sslmode", [""])[0] != "verify-full" or q.get("sslrootcert", [""])[0] != os.environ["TASKLAB_PG_TLS_DIR"] + "/ca.crt":
    raise SystemExit("FAIL: DSN must use the postgres maintenance database, verify-full, and this run's CA")
if not u.username or not u.password:
    raise SystemExit("FAIL: isolated disposable acceptance DSN must contain its run-scoped admin credentials")
PY

evidence=${TASKLAB_EVIDENCE_DIR:?Set run-scoped evidence directory}
[[ "$evidence" == "/var/lib/tasklab/evidence/$TASKLAB_RUN_ID/postgres-only" ]] || fail 'Evidence directory must exactly match this run ID'
root=$(git rev-parse --show-toplevel)
project="tasklab-postgres-$TASKLAB_RUN_ID"
compose=(docker compose --project-name "$project" --env-file "$TASKLAB_COMPOSE_ENV" -f "$root/deployments/alicloud/compose.postgres-only.yml")
container=$("${compose[@]}" ps -q postgres)
[[ -n "$container" ]] || fail 'run-owned PostgreSQL container is missing'
[[ "$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.project" }}' "$container")" == "$project" ]] || fail 'PostgreSQL container ownership label does not match this run'
[[ "$(docker inspect --format '{{ index .Config.Labels "com.docker.compose.service" }}' "$container")" == postgres ]] || fail 'selected container is not the PostgreSQL service'
[[ "$(docker inspect --format '{{.State.Status}}' "$container")" == running ]] || fail 'PostgreSQL container is not running'
[[ "$(docker inspect --format '{{.State.Health.Status}}' "$container")" == healthy ]] || fail 'PostgreSQL container is not healthy'
[[ "$(docker inspect --format '{{.Config.Image}}' "$container")" == "$TASKLAB_POSTGRES_IMAGE" ]] || fail 'running PostgreSQL image reference differs from the reviewed digest'
image_id=$(docker inspect --format '{{.Image}}' "$container")
repo_digests=$(docker image inspect --format '{{join .RepoDigests "\n"}}' "$image_id")
expected_digest=${TASKLAB_POSTGRES_IMAGE##*@}
grep -Fq "$expected_digest" <<<"$repo_digests" || fail 'running PostgreSQL image RepoDigest does not match the reviewed digest'
postgres_version=$(docker exec -e PGPASSWORD_FILE=/run/secrets/postgres_superuser_password "$container" sh -ec 'export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"; exec psql --no-password -XAt -U postgres -d postgres -c "SHOW server_version"') || fail 'could not read PostgreSQL version'
postgres_system_id=$(docker exec -e PGPASSWORD_FILE=/run/secrets/postgres_superuser_password "$container" sh -ec 'export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"; exec psql --no-password -XAt -U postgres -d postgres -c "SELECT system_identifier::text FROM pg_control_system()"') || fail 'could not read PostgreSQL system identifier'
[[ "$postgres_version" =~ ^[0-9]+\.[0-9]+ ]] || fail 'PostgreSQL server version result is invalid'
[[ "$postgres_system_id" =~ ^[0-9]+$ ]] || fail 'PostgreSQL system identifier is invalid'
schema_version=$(docker exec -e PGPASSWORD_FILE=/run/secrets/postgres_superuser_password "$container" sh -ec 'export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"; exec psql --no-password -XAt -U postgres -d tasklab -c "SELECT COALESCE(MAX(version),0) FROM schema_migrations"') || fail 'could not read migration schema version'
[[ "$schema_version" == 12 ]] || fail "expected schema version 12 before acceptance tests, got $schema_version"

role_sql="SELECT rolname,rolsuper,rolcreatedb,rolcreaterole FROM pg_roles WHERE rolname IN ('tasklab_migrator','tasklab_api','tasklab_worker') ORDER BY rolname"
runtime_role_rows=$(docker exec -e PGPASSWORD_FILE=/run/secrets/postgres_superuser_password -e PGTEST_SQL="$role_sql" "$container" sh -ec 'export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"; unset PGPASSWORD_FILE; exec psql --no-password -XAt -F "|" -U postgres -d postgres -c "$PGTEST_SQL"') || fail 'could not inspect runtime database roles'
expected_roles=$'tasklab_api|f|f|f\ntasklab_migrator|f|f|f\ntasklab_worker|f|f|f'
[[ "$runtime_role_rows" == "$expected_roles" ]] || fail 'API, Worker, and Migrator must exist as distinct non-superuser/non-admin roles'

grant_sql="SELECT has_table_privilege('tasklab_api','tasks','INSERT') AND has_table_privilege('tasklab_api','security_audit_events','INSERT') AND has_table_privilege('tasklab_worker','tasks','UPDATE') AND has_table_privilege('tasklab_worker','task_events','INSERT')"
runtime_grants=$(docker exec -e PGPASSWORD_FILE=/run/secrets/postgres_superuser_password -e PGTEST_SQL="$grant_sql" "$container" sh -ec 'export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"; unset PGPASSWORD_FILE; exec psql --no-password -XAt -U postgres -d tasklab -c "$PGTEST_SQL"') || fail 'could not inspect API/Worker DML grants'
[[ "$runtime_grants" == t ]] || fail 'API/Worker expected DML privileges are missing'

probe_log=$(mktemp "$TASKLAB_SECRET_DIR/ddl-probe.XXXXXX")
chmod 0600 "$probe_log"
trap 'rm -f -- "$probe_log"' EXIT
for role in tasklab_api tasklab_worker; do
  password_secret="/run/secrets/${role}_password"
  ddl_sql="BEGIN; CREATE TABLE public.tasklab_ddl_probe_${TASKLAB_RUN_ID//-/_} (id integer); ROLLBACK;"
  if docker exec -e PGPASSWORD_FILE="$password_secret" -e PGUSER="$role" -e PGDATABASE=tasklab -e PGTEST_SQL="$ddl_sql" \
    "$container" sh -ec 'export PGPASSWORD="$(cat "$PGPASSWORD_FILE")" PGHOST=postgres PGSSLMODE=verify-full PGSSLROOTCERT=/run/tasklab-pg-tls/ca.crt; unset PGPASSWORD_FILE; exec psql --no-password -X --set=ON_ERROR_STOP=1 -c "$PGTEST_SQL"' >"$probe_log" 2>&1; then
    fail "$role unexpectedly had DDL permission (the probe was rolled back)"
  fi
  grep -Fq 'permission denied for schema public' "$probe_log" || fail "$role DDL probe failed for a reason other than the expected permission denial"
done

mkdir -p "$evidence"
chmod 0700 "$evidence"
raw_log=$(mktemp "$TASKLAB_SECRET_DIR/postgres-acceptance.XXXXXX")
chmod 0600 "$raw_log"
trap 'rm -f -- "$raw_log" "$probe_log"' EXIT
export TEST_DATABASE_URL="$(<"$dsn_file")"
export REQUIRE_TEST_DATABASE=1
started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
test_command="GOTOOLCHAIN=local go test -json ./internal/storage/postgres -skip '^TestDatabaseBackupRestoreDrill$' -count=1 -timeout=18m"
set +e
timeout --signal=TERM --kill-after=15s 20m env GOTOOLCHAIN=local go test -json ./internal/storage/postgres -skip '^TestDatabaseBackupRestoreDrill$' -count=1 -timeout=18m > "$raw_log" 2>&1
test_rc=$?
set -e
python3 "$root/scripts/alicloud/redact.py" --secret-dir "$TASKLAB_SECRET_DIR" < "$raw_log" > "$evidence/postgres-test.jsonl"
set +e
python3 "$root/scripts/alicloud/summarize-go-test.py" "$evidence/postgres-test.jsonl" "$evidence/postgres-test-summary.json" \
  --require-test TestEnqueueAndGetTask \
  --require-test TestConcurrentClaimNoDuplicates \
  --require-test TestRenewLeaseRejectsRealStaleSameWorkerGeneration \
  --require-test TestExpiredRunningReclaimIgnoresCapacity \
  --require-test TestMarkSuccessStaleLeaseVersion \
  --require-test TestRetryBackoff \
  --require-test TestRetryTaskReclaimable \
  --require-test TestDLQNotClaimable \
  --require-test TestEffectLedgerKeepsOldGenerationReceiptAsEvidence \
  --require-test TestEffectReconciliationRequiresIdentityAndWritesAtomicAudit \
  --require-test TestAdministrativeMutationAndAuditAreAtomic \
  --require-test TestRuntimeRoleLeastPrivilege \
  --require-test TestRuntimeRoleLeastPrivilege/alter_schema \
  --require-test TestRuntimeGrantScriptRestrictsWorkerQueueColumns \
  --require-test TestWorkerCannotRewriteQueueGovernance \
  --require-test TestMigrationsFreshAndRerun \
  --require-test TestSharedIngressLimiterCoordinatesIndependentInstancesAndBoundsState
summary_rc=$?
set -e
ended=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf '%s\n' "$TASKLAB_SOURCE_SHA" > "$evidence/source-commit.txt"
git -C "$root" rev-parse 'HEAD^{tree}' > "$evidence/source-tree.txt"
printf '%s\n' "$started" > "$evidence/postgres-test-started-utc.txt"
printf '%s\n' "$ended" > "$evidence/postgres-test-ended-utc.txt"
printf '%s\n' "$test_rc" > "$evidence/postgres-test-exit-code.txt"
printf '%s\n' "$test_command" > "$evidence/postgres-test-command.txt"
printf '%s\n' "$TASKLAB_PG_PRIVATE_IP" > "$evidence/postgres-target-private-ip.txt"
(cd "$evidence" && printf '%s\n' "$postgres_version" > postgres-version.txt && printf '%s\n' "$postgres_system_id" > postgres-system-identifier.txt && printf '%s\n' "$schema_version" > schema-version.txt && printf '%s\n' "$TASKLAB_POSTGRES_IMAGE" > postgres-image-reference.txt && printf '%s\n' "$image_id" > postgres-image-id.txt)
(cd "$evidence" && sha256sum postgres-test.jsonl postgres-test-summary.json source-commit.txt source-tree.txt postgres-test-started-utc.txt postgres-test-ended-utc.txt postgres-test-exit-code.txt postgres-test-command.txt postgres-target-private-ip.txt postgres-version.txt postgres-system-identifier.txt schema-version.txt postgres-image-reference.txt postgres-image-id.txt > SHA256SUMS)
[[ $test_rc -eq 0 && $summary_rc -eq 0 ]] || fail "PostgreSQL package acceptance failed or skipped tests (test_exit=$test_rc summary_exit=$summary_rc); inspect redacted evidence"
printf 'PASS: PostgreSQL package acceptance completed without skipped tests; Evidence: %s\n' "$evidence"
