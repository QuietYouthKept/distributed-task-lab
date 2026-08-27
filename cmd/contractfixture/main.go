// Command contractfixture emits the canonical GET /tasks/{id} JSON shape from
// the real domain.Task type for cross-language consumer contract tests.
package main

import (
	"encoding/json"
	"os"
	"time"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func main() {
	key := "strategyops:replay:fixture"
	lastError := "worker failed"
	dlqReason := "max attempts exceeded"
	t := domain.Task{
		ID: "job-1", Type: domain.TaskTypeStrategyReplay, Status: domain.TaskStatusDLQ,
		Queue:       domain.DefaultQueueName,
		Payload:     domain.TaskPayloadEnvelope{Version: "1", Kind: "strategy_replay", Data: map[string]any{"proposal_id": "p-123", "proposal_version": 1}},
		Result:      &domain.TaskResult{Summary: "fixture", Artifacts: map[string]string{"replay_result": "{}"}, Metrics: map[string]float64{}},
		MaxAttempts: 3, Attempt: 3, LeaseVersion: 3,
		NextRunAt: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		CreatedAt: time.Date(2026, 8, 21, 0, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 21, 0, 0, 1, 0, time.UTC),
		LastError: &lastError, DLQReason: &dlqReason, IdempotencyKey: &key,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(t); err != nil {
		panic(err)
	}
}
