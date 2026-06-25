package postgres

import (
	"database/sql"
	"fmt"

	_ "github.com/lib/pq"
)

// NewDB opens a PostgreSQL connection and verifies it with a ping.
func NewDB(databaseURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	return db, nil
}

// RunMigrations executes the schema creation DDL.
func RunMigrations(db *sql.DB) error {
	_, err := db.Exec(migrationsSQL)
	if err != nil {
		return fmt.Errorf("postgres: migrate: %w", err)
	}
	return nil
}

const migrationsSQL = `
CREATE TABLE IF NOT EXISTS tasks (
    id                  TEXT PRIMARY KEY,
    type                TEXT NOT NULL,
    status              TEXT NOT NULL DEFAULT 'pending',
    payload             JSONB NOT NULL DEFAULT '{}',
    result              JSONB,
    priority            INTEGER NOT NULL DEFAULT 0,
    max_attempts        INTEGER NOT NULL DEFAULT 3,
    attempt             INTEGER NOT NULL DEFAULT 0,
    timeout_seconds     INTEGER NOT NULL DEFAULT 60,
    retry_delay_seconds INTEGER NOT NULL DEFAULT 5,
    leased_by           TEXT,
    lease_expires_at    TIMESTAMPTZ,
    next_run_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at          TIMESTAMPTZ,
    finished_at         TIMESTAMPTZ,
    last_error          TEXT,
    dlq_reason          TEXT,
    idempotency_key     TEXT
);

CREATE INDEX IF NOT EXISTS idx_tasks_status_next_run
    ON tasks (status, next_run_at, priority);

CREATE INDEX IF NOT EXISTS idx_tasks_status_lease
    ON tasks (status, lease_expires_at);

CREATE INDEX IF NOT EXISTS idx_tasks_type
    ON tasks (type);

CREATE INDEX IF NOT EXISTS idx_tasks_created_at
    ON tasks (created_at);

CREATE INDEX IF NOT EXISTS idx_tasks_idempotency_key
    ON tasks (idempotency_key);
`
