package config

import (
	"strings"
	"testing"
)

func TestDefaultWorkerIDIsUniquePerLoad(t *testing.T) {
	t.Setenv("WORKER_ID", "")
	a, b := Load().WorkerID, Load().WorkerID
	if a == b {
		t.Fatalf("default worker IDs collided: %q", a)
	}
	if !strings.Contains(a, "-") {
		t.Fatalf("worker ID lacks instance identity: %q", a)
	}
}

func TestExplicitWorkerIDOverride(t *testing.T) {
	t.Setenv("WORKER_ID", "stable-instance-id")
	if got := Load().WorkerID; got != "stable-instance-id" {
		t.Fatalf("WorkerID=%q", got)
	}
}
