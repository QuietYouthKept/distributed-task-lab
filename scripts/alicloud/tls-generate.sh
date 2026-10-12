#!/usr/bin/env bash
set -euo pipefail

fail() { printf 'FAIL: %s\n' "$1" >&2; exit 1; }
usage() {
  cat <<'EOF'
Usage:
  TASKLAB_RUN_ID=<id> TASKLAB_TLS_ROOT=/etc/tasklab/tls \
    scripts/alicloud/tls-generate.sh --dry-run --api-private-ip <private-ip>
  TASKLAB_RUN_ID=<id> TASKLAB_TLS_ROOT=/etc/tasklab/tls \
    scripts/alicloud/tls-generate.sh --execute --confirm-run-id <same-id> --api-private-ip <private-ip>

Dry-run is the default. --execute creates a new run-scoped short-lived test CA
and API, Identity, and PostgreSQL certificates. The CA private key is stored
outside all mounted TLS directories and is never printed or included in output.
EOF
}

mode=dry-run
confirm=''
api_private_ip=''
while (($#)); do
  case "$1" in
    --dry-run) mode=dry-run; shift ;;
    --execute) mode=execute; shift ;;
    --confirm-run-id) (($# >= 2)) || fail '--confirm-run-id requires a value'; confirm=$2; shift 2 ;;
    --api-private-ip) (($# >= 2)) || fail '--api-private-ip requires a value'; api_private_ip=$2; shift 2 ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; fail "unknown argument: $1" ;;
  esac
done

[[ "${TASKLAB_RUN_ID:-}" =~ ^[A-Za-z0-9][A-Za-z0-9-]{7,39}$ ]] || fail 'TASKLAB_RUN_ID must be a unique 8-40 character run ID'
[[ -n "$api_private_ip" ]] || fail '--api-private-ip is required'
command -v openssl >/dev/null 2>&1 || fail 'OpenSSL is required'
command -v python3 >/dev/null 2>&1 || fail 'Python 3 is required for IP validation'
export TASKLAB_API_PRIVATE_IP=$api_private_ip
python3 - <<'PY'
import ipaddress, os
ip = ipaddress.ip_address(os.environ["TASKLAB_API_PRIVATE_IP"])
if ip.version != 4 or not ip.is_private or ip.is_loopback or ip.is_link_local:
    raise SystemExit("FAIL: API private IP must be a private, non-loopback IPv4 address")
PY

tls_root=${TASKLAB_TLS_ROOT:-/etc/tasklab/tls}
[[ "$tls_root" == /etc/tasklab/tls ]] || fail 'TASKLAB_TLS_ROOT must be /etc/tasklab/tls'
tls_dir="$tls_root/$TASKLAB_RUN_ID"
private_root=${TASKLAB_PRIVATE_ROOT:-/etc/tasklab/private}
[[ "$private_root" == /etc/tasklab/private ]] || fail 'TASKLAB_PRIVATE_ROOT must be /etc/tasklab/private'
ca_dir="$private_root/$TASKLAB_RUN_ID"
if [[ "$mode" == execute ]]; then
  [[ "$confirm" == "$TASKLAB_RUN_ID" ]] || fail 'execute mode requires --confirm-run-id matching TASKLAB_RUN_ID'
  [[ "$(id -u)" == 0 ]] || fail 'execute mode must run as root to create protected /etc/tasklab paths'
fi

for path in "$tls_dir" "$ca_dir"; do
  [[ ! -e "$path" ]] || fail "refusing to overwrite existing run path: $path"
done
[[ ! -L "$tls_root" && ! -L "$private_root" ]] || fail 'TLS/private root must not be a symbolic link'

printf 'Mode: %s\nRun ID: %s\nTLS directory: %s\nCA key directory (not mounted): %s\n' \
  "$mode" "$TASKLAB_RUN_ID" "$tls_dir" "$ca_dir"
printf 'Certificates: API SAN localhost, 127.0.0.1, %s; Identity SAN identity; PostgreSQL SAN postgres, %s.\n' \
  "$api_private_ip" "$api_private_ip"
printf 'CA lifetime: 365 days; leaf lifetime: 30 days. No secrets or private keys are printed.\n'
[[ "$mode" == dry-run ]] && exit 0

umask 077
mkdir -p "$tls_root" "$private_root"
mkdir -m 0700 "$tls_dir" "$ca_dir"
mkdir -m 0700 "$tls_dir/api" "$tls_dir/identity" "$tls_dir/postgres"
tmpdir=$(mktemp -d "$ca_dir/work.XXXXXX")
chmod 0700 "$tmpdir"
cleanup() { rm -rf -- "$tmpdir"; }
trap cleanup EXIT

openssl genrsa -out "$ca_dir/ca.key" 3072 >/dev/null 2>&1
openssl req -x509 -new -sha256 -days 365 -key "$ca_dir/ca.key" \
  -subj "/CN=TaskLab $TASKLAB_RUN_ID Test CA" \
  -addext 'basicConstraints=critical,CA:TRUE' \
  -addext 'keyUsage=critical,keyCertSign,cRLSign' \
  -out "$ca_dir/ca.crt" >/dev/null 2>&1
chmod 0600 "$ca_dir/ca.key"
chmod 0644 "$ca_dir/ca.crt"

issue_leaf() {
  local service=$1 san=$2 directory="$tls_dir/$1" config="$tmpdir/$1.cnf"
  local cert_name key_name ca_name
  case "$service" in
    api) cert_name=api.crt; key_name=api.key; ca_name=test-ca.crt ;;
    identity) cert_name=identity.crt; key_name=identity.key; ca_name=ca.crt ;;
    postgres) cert_name=server.crt; key_name=server.key; ca_name=ca.crt ;;
    *) fail 'internal error: unknown certificate service' ;;
  esac
  cat >"$config" <<EOF
[req]
prompt = no
distinguished_name = dn
req_extensions = v3_req
[dn]
CN = $service.$TASKLAB_RUN_ID.tasklab.test
[v3_req]
basicConstraints = critical,CA:FALSE
keyUsage = critical,digitalSignature,keyEncipherment
extendedKeyUsage = serverAuth
subjectAltName = $san
EOF
  openssl genrsa -out "$directory/$key_name" 2048 >/dev/null 2>&1
  openssl req -new -sha256 -key "$directory/$key_name" -config "$config" \
    -out "$tmpdir/$service.csr" >/dev/null 2>&1
  openssl x509 -req -sha256 -days 30 -in "$tmpdir/$service.csr" \
    -CA "$ca_dir/ca.crt" -CAkey "$ca_dir/ca.key" -CAcreateserial \
    -extfile "$config" -extensions v3_req -out "$directory/$cert_name" >/dev/null 2>&1
  install -m 0644 "$ca_dir/ca.crt" "$directory/$ca_name"
  chmod 0600 "$directory/$key_name"
  chmod 0644 "$directory/$cert_name"
  openssl verify -CAfile "$directory/$ca_name" "$directory/$cert_name" >/dev/null
  case "$service" in
    api)
      openssl x509 -in "$directory/$cert_name" -noout -checkhost localhost -checkip 127.0.0.1 -checkip "$api_private_ip" >/dev/null ;;
    identity)
      openssl x509 -in "$directory/$cert_name" -noout -checkhost identity >/dev/null ;;
    postgres)
      openssl x509 -in "$directory/$cert_name" -noout -checkhost postgres -checkip "$api_private_ip" >/dev/null ;;
  esac
  [[ "$(stat -c '%a' "$directory/$key_name")" == 600 ]] || fail "$service private key permissions are not 0600"
}

issue_leaf api "DNS:localhost,IP:127.0.0.1,IP:$api_private_ip"
issue_leaf identity 'DNS:identity'
issue_leaf postgres "DNS:postgres,IP:$api_private_ip"

printf 'PASS: TLS material created for run %s. CA private key remains at %s/ca.key; do not mount or export that directory.\n' \
  "$TASKLAB_RUN_ID" "$ca_dir"
