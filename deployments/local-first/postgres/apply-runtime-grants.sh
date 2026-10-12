#!/bin/sh
set -eu

export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"
unset PGPASSWORD_FILE
psql --no-password --set=ON_ERROR_STOP=1 --file=/opt/tasklab/runtime-grants.sql
