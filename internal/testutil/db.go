// Package testutil provides helpers for integration tests that need a
// PostgreSQL database. Each call to EnsureDatabase provisions a fresh,
// uniquely-named database so test packages can run in parallel without
// interfering with each other.
package testutil

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"

	_ "github.com/lib/pq"
)

// Database is a uniquely-created test database.
type Database struct {
	Name string
	URL  string
	base string
}

// EnsureDatabase creates a uniquely-named PostgreSQL database derived from the
// maintenance database in baseURL, and returns a connection URL to it.
func EnsureDatabase(baseURL, prefix string) (*Database, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}

	admin, err := sql.Open("postgres", baseURL)
	if err != nil {
		return nil, fmt.Errorf("open admin: %w", err)
	}
	if err := admin.Ping(); err != nil {
		admin.Close()
		return nil, fmt.Errorf("ping admin: %w", err)
	}

	name := fmt.Sprintf("%s_%d_%d", prefix, os.Getpid(), time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE " + name); err != nil {
		admin.Close()
		return nil, fmt.Errorf("create db: %w", err)
	}
	admin.Close()

	u.Path = "/" + name
	return &Database{Name: name, URL: u.String(), base: baseURL}, nil
}

// Drop drops the database, force-terminating any lingering connections.
func (d *Database) Drop() {
	admin, err := sql.Open("postgres", d.base)
	if err != nil {
		return
	}
	defer admin.Close()
	_, _ = admin.Exec("DROP DATABASE IF EXISTS " + d.Name + " WITH (FORCE)")
}

// DefaultBaseURL returns the maintenance database URL, honoring TEST_DATABASE_URL.
// The URL must point to an existing database the caller can create databases in
// (the "postgres" maintenance database always exists).
func DefaultBaseURL() string {
	if u := os.Getenv("TEST_DATABASE_URL"); u != "" {
		return u
	}
	return "postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable"
}
