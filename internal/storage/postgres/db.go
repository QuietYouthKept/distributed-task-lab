package postgres

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

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

// RunMigrations executes all SQL migration files in order.
func RunMigrations(db *sql.DB) error {
	migrationsDir := migrationsDir()
	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("postgres: read migrations dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}

		path := filepath.Join(migrationsDir, entry.Name())
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("postgres: read migration %s: %w", entry.Name(), err)
		}

		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("postgres: exec migration %s: %w", entry.Name(), err)
		}

		fmt.Printf("migration applied: %s\n", entry.Name())
	}

	return nil
}

// migrationsDir returns the absolute path to the migrations directory.
func migrationsDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "migrations")
}
