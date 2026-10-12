package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestMigrationDatabaseURLSecretFileAndConflict(t *testing.T) {
	const secretURL = "postgres://tasklab_migrator:very-secret@postgres:5432/tasklab?sslmode=disable"
	get := func(name string) string {
		if name == "MIGRATOR_DATABASE_URL_FILE" {
			return "mounted-secret"
		}
		return ""
	}
	got, err := migrationDatabaseURL(get, func(path string) ([]byte, error) {
		if path != "mounted-secret" {
			t.Fatalf("unexpected secret path %q", path)
		}
		return []byte(secretURL + "\r\n"), nil
	})
	if err != nil || got != secretURL {
		t.Fatalf("secret file URL = %q, %v", got, err)
	}

	get = func(name string) string {
		if name == "MIGRATOR_DATABASE_URL" {
			return secretURL
		}
		if name == "MIGRATOR_DATABASE_URL_FILE" {
			return "mounted-secret"
		}
		return ""
	}
	_, err = migrationDatabaseURL(get, func(string) ([]byte, error) { return nil, nil })
	if err == nil || strings.Contains(err.Error(), "very-secret") {
		t.Fatalf("conflict should be safely rejected without printing credentials: %v", err)
	}
}

func TestMigrationDatabaseURLRejectsUnreadableOrEmptySecret(t *testing.T) {
	get := func(name string) string {
		if name == "MIGRATOR_DATABASE_URL_FILE" {
			return "missing"
		}
		return ""
	}
	_, err := migrationDatabaseURL(get, func(string) ([]byte, error) { return nil, errors.New("file missing") })
	if err == nil || strings.Contains(err.Error(), "missing") {
		t.Fatalf("unreadable secret file should fail without echoing path: %v", err)
	}
	_, err = migrationDatabaseURL(func(string) string { return "" }, func(string) ([]byte, error) { return []byte("\r\n"), nil })
	if err == nil {
		t.Fatal("empty database URL must fail")
	}
}

func TestMigrationJobFailsClosedForWrongProfileAndNonLocalDatabase(t *testing.T) {
	get := func(name string) string {
		switch name {
		case "APP_PROFILE":
			return "production"
		case "MIGRATOR_DATABASE_URL":
			return "postgres://tasklab_migrator:password@postgres:5432/tasklab?sslmode=disable"
		default:
			return ""
		}
	}
	if err := run(context.Background(), get, func(string) ([]byte, error) { return nil, nil }); err == nil {
		t.Fatal("migration command must not accept production in this local-only program")
	}
	for _, raw := range []string{
		"postgres://tasklab_migrator:password@db.example:5432/tasklab?sslmode=disable",
		"postgres://tasklab_migrator:password@postgres:5432/tasklab?sslmode=require",
		"postgres://other:password@postgres:5432/tasklab?sslmode=disable",
	} {
		if err := validateLocalMigratorURL(raw); err == nil {
			t.Fatalf("unsafe local migration URL accepted: %s", raw)
		}
	}
}
