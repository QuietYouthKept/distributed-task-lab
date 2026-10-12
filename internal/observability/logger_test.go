package observability

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestLoggerRedactsCredentialFieldsAndConfiguredSecrets(t *testing.T) {
	var output bytes.Buffer
	logger := newTestLogger(&output, "known-db-password", "known-provider-key")
	logger.Info("diagnostic",
		"api_token", "field-token-value",
		"endpoint", "postgres://service:known-db-password@db.example:5432/tasks?sslmode=require",
		"error", errors.New("provider failed: Authorization: Bearer dynamic.jwt.token api_key=known-provider-key"),
		"message", "ordinary diagnostic",
	)
	encoded := output.String()
	for _, secret := range []string{"field-token-value", "known-db-password", "known-provider-key", "dynamic.jwt.token"} {
		if strings.Contains(encoded, secret) {
			t.Fatalf("structured logger output retained a credential (value length %d)", len(secret))
		}
	}
	for _, expected := range []string{"[REDACTED]", "ordinary diagnostic"} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("structured logger output omitted expected safe marker %q", expected)
		}
	}
}

func newTestLogger(output *bytes.Buffer, secrets ...string) *slog.Logger {
	return slog.New(newJSONHandler(output, secrets...))
}
