#!/usr/bin/env bash
set -euo pipefail

# MinIO community images are no longer published. Build this CI-only S3 test
# server from a source version and commit pinned in the official Go module proxy.
readonly MINIO_MODULE_VERSION='v0.0.0-20251015172955-9e49d5e7a648'
readonly MINIO_MODULE_SUM='h1:6TdolSCLSs2nwm8i0PpWDqf9iX2Ty9WQK8wmr7dCnUM='
readonly MINIO_MOD_SUM='h1:yCWDkwWO9IWpGsT4mreDDN/B/QVmK2zC666uInRAcqE='
readonly MINIO_SOURCE_COMMIT='9e49d5e7a648f00e26f2246f4dc28e6b07f8c84a'
readonly MINIO_SOURCE_TAG='RELEASE.2025-10-15T17-29-55Z'
readonly MINIO_REPOSITORY='github.com/minio/minio'

runner_temp="${RUNNER_TEMP:-${TMPDIR:-/tmp}}"
build_root="$(mktemp -d "${runner_temp%/}/tasklab-minio-build.XXXXXX")"
trap 'rm -rf -- "$build_root"' EXIT

export GOBIN="$build_root/bin"
mkdir -p "$GOBIN" "$build_root/context"
export CGO_ENABLED=0
export GOOS=linux
export GOARCH=amd64
export GOTOOLCHAIN=go1.26.9
module_info="$(go mod download -json "${MINIO_REPOSITORY}@${MINIO_MODULE_VERSION}")"
module_dir="$(printf '%s\n' "$module_info" | sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p')"
module_sum="$(printf '%s\n' "$module_info" | sed -n 's/^[[:space:]]*"Sum": "\(.*\)",$/\1/p')"
mod_sum="$(printf '%s\n' "$module_info" | sed -n 's/^[[:space:]]*"GoModSum": "\(.*\)"$/\1/p')"
test -n "$module_dir"
test "$module_sum" = "$MINIO_MODULE_SUM"
test "$mod_sum" = "$MINIO_MOD_SUM"
if command -v cygpath >/dev/null 2>&1; then
  module_dir="$(cygpath --unix "$module_dir")"
fi
test -f "$module_dir/LICENSE"
minio_binary="$GOBIN/minio"
(
  cd "$module_dir"
  go build -trimpath -buildvcs=false -o "$minio_binary" .
)
test -f "$minio_binary"
cp "$minio_binary" "$build_root/context/minio"
cp "$module_dir/LICENSE" "$build_root/context/LICENSE"

echo "MinIO upstream source tag: $MINIO_SOURCE_TAG"
echo "MinIO upstream commit: $MINIO_SOURCE_COMMIT"
echo "MinIO Go module version: $MINIO_MODULE_VERSION"
echo "MinIO Go module checksum: $module_sum"
echo "MinIO go.mod checksum: $mod_sum"
echo "License: AGPL-3.0 (upstream LICENSE included in image)"
go version
go version -m "$minio_binary" | sed -n \
  -e '1,3p' \
  -e '/build[[:space:]]\+CGO_ENABLED=0/p' \
  -e '/build[[:space:]]\+GOARCH=amd64/p' \
  -e '/build[[:space:]]\+GOOS=linux/p' \
  -e '/build[[:space:]]\+-trimpath=true/p'
sha256sum "$minio_binary"

image_tag="tasklab-minio-ci:${GITHUB_RUN_ID:-local}-${GITHUB_RUN_ATTEMPT:-0}"
if [[ -n "${GITHUB_ENV:-}" ]]; then
  printf 'MINIO_CI_IMAGE=%s\n' "$image_tag" >> "$GITHUB_ENV"
fi
docker build --pull --no-cache \
  --build-arg "MINIO_SOURCE_TAG=$MINIO_SOURCE_TAG" \
  --build-arg "MINIO_SOURCE_COMMIT=$MINIO_SOURCE_COMMIT" \
  --build-arg "MINIO_MODULE_VERSION=$MINIO_MODULE_VERSION" \
  --build-arg "MINIO_MODULE_SUM=$MINIO_MODULE_SUM" \
  --tag "$image_tag" \
  --file "${GITHUB_WORKSPACE:-.}/.github/minio-ci.Dockerfile" \
  "$build_root/context"

echo "Built ephemeral MinIO image tag: $image_tag"
docker image inspect "$image_tag" --format 'Local image ID (not a registry digest): {{.Id}}'
