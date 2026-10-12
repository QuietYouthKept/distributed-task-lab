#!/bin/sh
set -eu

export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"
unset PGPASSWORD_FILE
test -n "${TASKLAB_WORKER_ID:-}" || { echo 'TASKLAB_WORKER_ID is required' >&2; exit 2; }
psql --no-password --set=ON_ERROR_STOP=1 --set="worker_id=$TASKLAB_WORKER_ID" --file=/opt/tasklab/runtime-grants.sql
