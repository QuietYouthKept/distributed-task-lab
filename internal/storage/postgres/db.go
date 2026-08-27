package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

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
	return NewDBWithConfig(databaseURL, DBConfig{})
}

type DBConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

func (c DBConfig) withDefaults() DBConfig {
	if c.MaxOpenConns <= 0 {
		c.MaxOpenConns = 25
	}
	if c.MaxIdleConns < 0 {
		c.MaxIdleConns = 0
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 5
	}
	if c.ConnMaxLifetime <= 0 {
		c.ConnMaxLifetime = 30 * time.Minute
	}
	if c.ConnMaxIdleTime <= 0 {
		c.ConnMaxIdleTime = 5 * time.Minute
	}
	return c
}

func NewDBWithConfig(databaseURL string, cfg DBConfig) (*sql.DB, error) {
	cfg = cfg.withDefaults()
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	db.SetMaxOpenConns(cfg.MaxOpenConns)
	db.SetMaxIdleConns(cfg.MaxIdleConns)
	db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	return db, nil
}

// RunMigrations executes all embedded SQL migration files in name order.
// It takes a PostgreSQL advisory lock first so that the API server and
// multiple worker replicas starting at the same time do not race on the
// same DDL statements.
func RunMigrations(db *sql.DB) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return RunMigrationsContext(ctx, db)
}

// RunMigrationsContext applies forward-only migrations exactly once. Every
// applied file is tracked with a SHA-256 checksum; changing an applied file is
// rejected instead of being silently replayed.
func RunMigrationsContext(ctx context.Context, db *sql.DB) error {

	// Use a single dedicated connection for both the lock and the migrations,
	// because advisory locks are scoped to a session/connection.
	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("postgres: acquire conn: %w", err)
	}
	defer conn.Close()

	for {
		var locked bool
		if err := conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", migrationLockKey).Scan(&locked); err != nil {
			return fmt.Errorf("postgres: advisory lock: %w", err)
		}
		if locked {
			break
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("postgres: migration lock timeout: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, _ = conn.ExecContext(unlockCtx, "SELECT pg_advisory_unlock($1)", migrationLockKey)
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			checksum TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)`); err != nil {
		return fmt.Errorf("postgres: create schema_migrations: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("postgres: read embedded migrations: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	// A tracking-table gap means the database's migration history was edited or
	// partially restored. Replaying an old migration underneath newer schema is
	// not a safe repair strategy, even when today's SQL happens to be idempotent.
	rows, err := conn.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		return fmt.Errorf("postgres: inspect migration history: %w", err)
	}
	var applied []int64
	for rows.Next() {
		var version int64
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			return fmt.Errorf("postgres: scan migration history: %w", err)
		}
		applied = append(applied, version)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("postgres: close migration history: %w", err)
	}
	for i, version := range applied {
		want := int64(i + 1)
		if version != want {
			return fmt.Errorf("postgres: migration history gap: found version %d where %d was required", version, want)
		}
	}
	if len(applied) > len(entries) {
		return fmt.Errorf("postgres: database schema version %d is newer than embedded version %d", applied[len(applied)-1], len(entries))
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		versionText := strings.SplitN(name, "_", 2)[0]
		version, err := strconv.ParseInt(versionText, 10, 64)
		if err != nil {
			return fmt.Errorf("postgres: invalid migration name %s: %w", name, err)
		}
		content, err := migrationsFS.ReadFile(filepath.ToSlash("migrations/" + name))
		if err != nil {
			return fmt.Errorf("postgres: read migration %s: %w", entry.Name(), err)
		}

		sum := sha256.Sum256(content)
		checksum := hex.EncodeToString(sum[:])
		var stored string
		err = conn.QueryRowContext(ctx, "SELECT checksum FROM schema_migrations WHERE version = $1", version).Scan(&stored)
		if err == nil {
			if stored != checksum {
				return fmt.Errorf("postgres: migration %s checksum mismatch", name)
			}
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("postgres: check migration %s: %w", name, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("postgres: begin migration %s: %w", name, err)
		}
		if _, err = tx.ExecContext(ctx, "SET LOCAL lock_timeout = '5s'; SET LOCAL statement_timeout = '25s'"); err == nil {
			_, err = tx.ExecContext(ctx, string(content))
		}
		if err == nil {
			_, err = tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, name, checksum) VALUES ($1, $2, $3)", version, name, checksum)
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("postgres: exec migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("postgres: commit migration %s: %w", name, err)
		}
		fmt.Printf("migration applied: %s\n", name)
	}

	return nil
}
