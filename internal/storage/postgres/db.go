package postgres

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"

	_ "github.com/lib/pq"
)

// migrationsFS embeds the SQL migrations into the binary so they work in a
// compiled Docker image (the source tree is not present at runtime).
//
//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationLockKey is an arbitrary but stable advisory-lock key used to
// serialize migrations across API + multiple workers starting concurrently.
const migrationLockKey = 727901

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

// RunMigrations executes all embedded SQL migration files in name order.
// It takes a PostgreSQL advisory lock first so that the API server and
// multiple worker replicas starting at the same time do not race on the
// same DDL statements.
func RunMigrations(db *sql.DB) error {
	ctx := context.Background()

	// Use a single dedicated connection for both the lock and the migrations,
	// because advisory locks are scoped to a session/connection.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("postgres: acquire conn: %w", err)
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, fmt.Sprintf("SELECT pg_advisory_lock(%d)", migrationLockKey)); err != nil {
		return fmt.Errorf("postgres: advisory lock: %w", err)
	}
	defer conn.ExecContext(ctx, fmt.Sprintf("SELECT pg_advisory_unlock(%d)", migrationLockKey))

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("postgres: read embedded migrations: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		content, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("postgres: read migration %s: %w", entry.Name(), err)
		}

		if _, err := conn.ExecContext(ctx, string(content)); err != nil {
			return fmt.Errorf("postgres: exec migration %s: %w", entry.Name(), err)
		}

		fmt.Printf("migration applied: %s\n", entry.Name())
	}

	return nil
}
