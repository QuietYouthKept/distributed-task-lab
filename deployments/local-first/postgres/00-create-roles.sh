#!/bin/sh
set -eu

export TASKLAB_MIGRATOR_PASSWORD="$(cat /run/secrets/tasklab_migrator_password)"
export TASKLAB_API_PASSWORD="$(cat /run/secrets/tasklab_api_password)"
export TASKLAB_WORKER_PASSWORD="$(cat /run/secrets/tasklab_worker_password)"

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" --set=ON_ERROR_STOP=1 <<'SQL'
\getenv migrator_password TASKLAB_MIGRATOR_PASSWORD
\getenv api_password TASKLAB_API_PASSWORD
\getenv worker_password TASKLAB_WORKER_PASSWORD

SELECT format('CREATE ROLE tasklab_migrator LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L', :'migrator_password') \gexec
SELECT format('CREATE ROLE tasklab_api LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L', :'api_password') \gexec
SELECT format('CREATE ROLE tasklab_worker LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION PASSWORD %L', :'worker_password') \gexec

REVOKE ALL ON DATABASE tasklab FROM PUBLIC;
GRANT CONNECT ON DATABASE tasklab TO tasklab_migrator, tasklab_api, tasklab_worker;
GRANT USAGE, CREATE ON SCHEMA public TO tasklab_migrator;
GRANT USAGE ON SCHEMA public TO tasklab_api, tasklab_worker;
SQL
