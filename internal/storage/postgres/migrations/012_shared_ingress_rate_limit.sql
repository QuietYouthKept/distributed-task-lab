-- Optional cross-instance API token buckets. Rows are fixed-capacity slots so
-- hostile or accidental subject churn cannot grow shared state without bound.
CREATE TABLE IF NOT EXISTS api_ingress_rate_limit_buckets (
    slot INTEGER PRIMARY KEY CHECK (slot BETWEEN 1 AND 10000),
    principal_hash BYTEA,
    tokens DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (tokens >= 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT statement_timestamp(),
    last_admitted BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_ingress_rate_limit_principal
    ON api_ingress_rate_limit_buckets(principal_hash)
    WHERE principal_hash IS NOT NULL;
