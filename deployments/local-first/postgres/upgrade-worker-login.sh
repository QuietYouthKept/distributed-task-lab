#!/bin/sh
set -eu

: "${PGPASSWORD_FILE:?set the one-shot database administrator password file}"
: "${TASKLAB_WORKER_A_PASSWORD_FILE:?set the per-Worker A password file}"
export TASKLAB_WORKER_LOGIN=tasklab_worker_a

export PGPASSWORD="$(cat "$PGPASSWORD_FILE")"
export TASKLAB_WORKER_A_PASSWORD="$(cat "$TASKLAB_WORKER_A_PASSWORD_FILE")"
unset PGPASSWORD_FILE TASKLAB_WORKER_A_PASSWORD_FILE

# This one-shot script must run as the database administrator in the approved
# maintenance window, after old Workers are drained and before new Workers start.
psql --no-password --set=ON_ERROR_STOP=1 <<'SQL'
\getenv worker_password TASKLAB_WORKER_A_PASSWORD
\getenv worker_login TASKLAB_WORKER_LOGIN

ALTER ROLE tasklab_worker NOLOGIN;
SELECT format('CREATE ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L', :'worker_login', :'worker_password')
WHERE NOT EXISTS (SELECT 1 FROM pg_catalog.pg_roles WHERE rolname = :'worker_login')
\gexec
SELECT format('ALTER ROLE %I LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L', :'worker_login', :'worker_password')
\gexec
SELECT format('GRANT tasklab_worker TO %I', :'worker_login')
\gexec
SQL

unset TASKLAB_WORKER_A_PASSWORD
