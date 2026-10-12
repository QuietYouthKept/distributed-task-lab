#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
[[ "${1:-}" == --confirm-run-id && "${2:-}" == "${TASKLAB_RUN_ID:-}" ]] || fail 'Expected --confirm-run-id <same TASKLAB_RUN_ID>'
[[ "${TASKLAB_SOURCE_SHA:-}" =~ ^[0-9a-f]{40}$ ]] || fail 'TASKLAB_SOURCE_SHA must be the reviewed commit SHA'
root=$(git rev-parse --show-toplevel)
[[ "$(git -C "$root" rev-parse HEAD)" == "$TASKLAB_SOURCE_SHA" ]] || fail 'checkout HEAD differs from TASKLAB_SOURCE_SHA'
[[ -z "$(git -C "$root" status --porcelain)" ]] || fail 'checkout must be clean before destructive isolated acceptance setup'
[[ "${TASKLAB_SECRET_DIR:-}" == "/etc/tasklab/secrets/$TASKLAB_RUN_ID" ]] || fail 'secret directory must match the run ID exactly'
dsn_file=${TASKLAB_ACCEPTANCE_DSN_FILE:-}
[[ "$dsn_file" == "$TASKLAB_SECRET_DIR/postgres-acceptance-dsn" && -s "$dsn_file" ]] || fail 'run-scoped PostgreSQL maintenance DSN file is required'
[[ "${TASKLAB_PG_PRIVATE_IP:-}" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] || fail 'TASKLAB_PG_PRIVATE_IP must be the isolated PostgreSQL private IPv4'
[[ -f /proc/meminfo ]] || fail '/proc/meminfo is unavailable'
mem_kib=$(awk '/MemAvailable:/ {print $2}' /proc/meminfo)
(( mem_kib >= 6291456 )) || fail 'at least 6 GiB available RAM required; no test started'
command -v go >/dev/null 2>&1 || fail 'Go toolchain is required'
command -v timeout >/dev/null 2>&1 || fail 'GNU timeout is required'
command -v python3 >/dev/null 2>&1 || fail 'Python 3 is required'

export TASKLAB_DSN_FILE="$TASKLAB_ACCEPTANCE_DSN_FILE"
export TASKLAB_EXPECTED_PRIVATE_IP="$TASKLAB_PG_PRIVATE_IP"
python3 - <<'PY'
import ipaddress, os, pathlib, urllib.parse
u = urllib.parse.urlparse(pathlib.Path(os.environ["TASKLAB_DSN_FILE"]).read_text().strip())
expected = ipaddress.ip_address(os.environ["TASKLAB_EXPECTED_PRIVATE_IP"])
host = ipaddress.ip_address(u.hostname or "")
q = urllib.parse.parse_qs(u.query)
if not expected.is_private or host != expected or u.scheme not in ("postgres", "postgresql"):
    raise SystemExit("FAIL: DSN must target exactly the explicitly selected private PostgreSQL host")
ca_file = q.get("sslrootcert", [""])[0]
if u.path != "/postgres" or q.get("sslmode", [""])[0] != "verify-full" or not ca_file or not pathlib.Path(ca_file).is_file():
    raise SystemExit("FAIL: DSN must use maintenance database postgres with sslmode=verify-full and a CA")
if not u.username or not u.password:
    raise SystemExit("FAIL: run-scoped disposable acceptance credentials are required")
PY

evidence=${TASKLAB_EVIDENCE_DIR:?Set run-scoped Evidence output directory}
[[ "$evidence" == "/var/lib/tasklab/evidence/$TASKLAB_RUN_ID/distributed-claim" ]] || fail 'Evidence directory must exactly match this run ID'
mkdir -p "$evidence"
chmod 0700 "$evidence"
raw_log=$(mktemp "$TASKLAB_SECRET_DIR/claim-acceptance.XXXXXX")
chmod 0600 "$raw_log"
trap 'rm -f -- "$raw_log"' EXIT
export TEST_DATABASE_URL="$(<"$TASKLAB_ACCEPTANCE_DSN_FILE")"
export REQUIRE_TEST_DATABASE=1
export TASKLAB_CLAIM_WORKERS=${TASKLAB_CLAIM_WORKERS:-100}
export TASKLAB_CLAIM_TASKS=${TASKLAB_CLAIM_TASKS:-10000}
export TASKLAB_CLAIM_TIMEOUT_SECONDS=${TASKLAB_CLAIM_TIMEOUT_SECONDS:-900}
seeds=${TASKLAB_CLAIM_SEEDS:-10}
[[ "$TASKLAB_CLAIM_WORKERS" =~ ^[0-9]+$ && "$TASKLAB_CLAIM_WORKERS" -ge 1 && "$TASKLAB_CLAIM_WORKERS" -le 100 ]] || fail 'TASKLAB_CLAIM_WORKERS must be in 1..100'
[[ "$TASKLAB_CLAIM_TASKS" =~ ^[0-9]+$ && "$TASKLAB_CLAIM_TASKS" -ge 1 && "$TASKLAB_CLAIM_TASKS" -le 100000 ]] || fail 'TASKLAB_CLAIM_TASKS must be in 1..100000'
[[ "$TASKLAB_CLAIM_TIMEOUT_SECONDS" =~ ^[0-9]+$ && "$TASKLAB_CLAIM_TIMEOUT_SECONDS" -ge 1 && "$TASKLAB_CLAIM_TIMEOUT_SECONDS" -le 3600 ]] || fail 'TASKLAB_CLAIM_TIMEOUT_SECONDS must be in 1..3600'
[[ "$seeds" =~ ^[0-9]+$ && "$seeds" -ge 1 && "$seeds" -le 10 ]] || fail 'TASKLAB_CLAIM_SEEDS must be in 1..10'

go_version=$(go version)
started=$(date -u +%Y-%m-%dT%H:%M:%SZ)
test_command="GOTOOLCHAIN=local go test -json ./internal/storage/postgres -run '^TestConcurrentClaimNoDuplicates$' -count=$seeds -timeout=3h"
set +e
timeout --signal=TERM --kill-after=15s 3h env GOTOOLCHAIN=local go test -json ./internal/storage/postgres -run '^TestConcurrentClaimNoDuplicates$' -count="$seeds" -timeout=2h45m >"$raw_log" 2>&1
test_rc=$?
set -e
python3 "$root/scripts/alicloud/redact.py" --secret-dir "$TASKLAB_SECRET_DIR" <"$raw_log" >"$evidence/claim-test.jsonl"
set +e
python3 "$root/scripts/alicloud/summarize-go-test.py" "$evidence/claim-test.jsonl" "$evidence/claim-test-summary.json" --require-test TestConcurrentClaimNoDuplicates
summary_rc=$?
set -e
ended=$(date -u +%Y-%m-%dT%H:%M:%SZ)
printf '%s\n' "$TASKLAB_SOURCE_SHA" >"$evidence/source-commit.txt"
git -C "$root" rev-parse 'HEAD^{tree}' >"$evidence/source-tree.txt"
printf '%s\n' "$go_version" >"$evidence/go-version.txt"
printf '%s\n' "$test_command" >"$evidence/test-command.txt"
printf '%s\n' "$test_rc" >"$evidence/test-exit-code.txt"
printf '%s\n' "$TASKLAB_PG_PRIVATE_IP" >"$evidence/postgres-target-private-ip.txt"
printf '%s\n' "$TASKLAB_CLAIM_WORKERS" >"$evidence/workers.txt"
printf '%s\n' "$TASKLAB_CLAIM_TASKS" >"$evidence/tasks-per-seed.txt"
printf '%s\n' "$seeds" >"$evidence/seeds.txt"
printf '%s\n' "$started" >"$evidence/started-utc.txt"
printf '%s\n' "$ended" >"$evidence/ended-utc.txt"
(cd "$evidence" && sha256sum claim-test.jsonl claim-test-summary.json source-commit.txt source-tree.txt go-version.txt test-command.txt test-exit-code.txt postgres-target-private-ip.txt workers.txt tasks-per-seed.txt seeds.txt started-utc.txt ended-utc.txt >SHA256SUMS)
[[ "$test_rc" -eq 0 && "$summary_rc" -eq 0 ]] || fail "claim acceptance failed, skipped or empty (test_exit=$test_rc summary_exit=$summary_rc); inspect redacted evidence"
printf 'PASS: bounded %s-worker x %s-task x %s-seed Claim/Fenced ACK acceptance; evidence=%s\n' "$TASKLAB_CLAIM_WORKERS" "$TASKLAB_CLAIM_TASKS" "$seeds" "$evidence"
