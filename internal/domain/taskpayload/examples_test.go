package taskpayload_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/domain/taskpayload"
)

func TestCheckedInPayloadExamplesMatchRegistry(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "..", "examples", "payloads", "*.json"))
	if err != nil {
		t.Fatalf("glob examples: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no payload examples found")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var request struct {
				Type    domain.TaskType            `json:"type"`
				Payload domain.TaskPayloadEnvelope `json:"payload"`
			}
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatalf("decode example: %v", err)
			}
			if _, err := taskpayload.Validate(request.Type, request.Payload); err != nil {
				t.Fatalf("example does not match payload registry: %v", err)
			}
		})
	}
}
