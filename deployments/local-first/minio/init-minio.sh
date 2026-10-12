#!/bin/sh
set -eu

export MINIO_ROOT_USER="$(cat "$MINIO_ROOT_USER_FILE")"
export MINIO_ROOT_PASSWORD="$(cat "$MINIO_ROOT_PASSWORD_FILE")"
export ARTIFACT_S3_ACCESS_KEY="$(cat "$ARTIFACT_S3_ACCESS_KEY_FILE")"
export ARTIFACT_S3_SECRET_KEY="$(cat "$ARTIFACT_S3_SECRET_KEY_FILE")"

mc alias set tasklab http://minio:9000 "$MINIO_ROOT_USER" "$MINIO_ROOT_PASSWORD" >/dev/null
mc mb --ignore-existing tasklab/tasklab-artifacts >/dev/null
mc admin user add tasklab "$ARTIFACT_S3_ACCESS_KEY" "$ARTIFACT_S3_SECRET_KEY" >/dev/null 2>&1
mc admin policy create tasklab tasklab-artifact-object-rw /opt/tasklab/artifact-policy.json >/dev/null 2>&1
mc admin policy attach tasklab tasklab-artifact-object-rw --user "$ARTIFACT_S3_ACCESS_KEY" >/dev/null
mc alias remove tasklab >/dev/null
unset MINIO_ROOT_USER MINIO_ROOT_PASSWORD ARTIFACT_S3_ACCESS_KEY ARTIFACT_S3_SECRET_KEY
