// Command migrate applies the repository's embedded forward-only SQL
// migrations using an explicitly supplied local migrator connection. It is
// intentionally separate from API and Worker startup.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/storage/postgres"
)

func main() {
	if err := run(context.Background(), os.Getenv, os.ReadFile); err != nil {
		// Never print the connection URL or a driver error which might include
		// credentials. Detailed database errors can contain secret-bearing DSNs.
		fmt.Fprintln(os.Stderr, "migration job failed; inspect database health and migration state")
		os.Exit(1)
	}
	fmt.Println("migration job completed")
}

type getenv func(string) string
type readFile func(string) ([]byte, error)

func run(parent context.Context, get getenv, read readFile) error {
	profile := get("APP_PROFILE")
	if profile != "staging-sim" {
		return errors.New("migration job is restricted to APP_PROFILE=staging-sim")
	}
	databaseURL, err := migrationDatabaseURL(get, read)
	if err != nil {
		return err
	}
	if err := validateLocalMigratorURL(databaseURL); err != nil {
		return err
	}

	db, err := postgres.NewDBWithConfig(databaseURL, postgres.DBConfig{MaxOpenConns: 1, MaxIdleConns: 1})
	if err != nil {
		return errors.New("could not connect using the local migration account")
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	return postgres.RunMigrationsContext(ctx, db)
}

func migrationDatabaseURL(get getenv, read readFile) (string, error) {
	direct, file := get("MIGRATOR_DATABASE_URL"), get("MIGRATOR_DATABASE_URL_FILE")
	if direct != "" && file != "" {
		return "", errors.New("MIGRATOR_DATABASE_URL and MIGRATOR_DATABASE_URL_FILE cannot both be set")
	}
	if file != "" {
		contents, err := read(file)
		if err != nil {
			return "", errors.New("MIGRATOR_DATABASE_URL_FILE could not be read")
		}
		direct = strings.TrimRight(string(contents), "\r\n")
	}
	if strings.TrimSpace(direct) == "" {
		return "", errors.New("MIGRATOR_DATABASE_URL or MIGRATOR_DATABASE_URL_FILE is required")
	}
	return direct, nil
}

func validateLocalMigratorURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		return errors.New("migration database URL must use PostgreSQL")
	}
	if u.Hostname() != "postgres" || u.Port() != "5432" || u.Query().Get("sslmode") != "disable" {
		return errors.New("migration database URL must target the isolated postgres:5432 service with sslmode=disable")
	}
	if u.User == nil || u.User.Username() != "tasklab_migrator" {
		return errors.New("migration database URL must use the dedicated tasklab_migrator account")
	}
	if _, ok := u.User.Password(); !ok {
		return errors.New("migration database URL must include the local migrator password")
	}
	return nil
}
