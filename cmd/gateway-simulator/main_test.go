package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSimulatorSecretFileAndConflict(t *testing.T) {
	secret := "fixture-secret-value"
	path := filepath.Join(t.TempDir(), "gateway-key")
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GATEWAY_SIMULATOR_API_KEY", "")
	t.Setenv("GATEWAY_SIMULATOR_API_KEY_FILE", path)
	got, err := secretFromEnv("GATEWAY_SIMULATOR_API_KEY")
	if err != nil || got != secret {
		t.Fatalf("secretFromEnv() = %q, %v", got, err)
	}
	t.Setenv("GATEWAY_SIMULATOR_API_KEY", "another-secret")
	_, err = secretFromEnv("GATEWAY_SIMULATOR_API_KEY")
	if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "another-secret") {
		t.Fatalf("conflict should fail without exposing values, got %v", err)
	}
}
