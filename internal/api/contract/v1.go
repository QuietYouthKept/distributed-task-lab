package contract

import (
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/effects"
)

// The v1 DTOs below name response/request shapes that were previously emitted
// as anonymous maps or inline structs. Their JSON fields intentionally match
// the existing wire representation; they provide a single type-level source
// for OpenAPI drift tests without changing the API's values or status codes.

type TaskListResponse struct {
	Tasks      []domain.Task `json:"tasks"`
	NextCursor string        `json:"next_cursor"`
	HasMore    bool          `json:"has_more"`
}

type TaskActionResponse struct {
	Status string `json:"status"`
	TaskID string `json:"task_id"`
}

type QueueActionResponse struct {
	Status string `json:"status"`
	Queue  string `json:"queue"`
}

type EffectsListResponse struct {
	Effects []effects.Record `json:"effects"`
}

type EffectResolutionResponse struct {
	EffectID string `json:"effect_id"`
	Status   string `json:"status"`
}

type OptionalReasonRequest struct {
	Reason string `json:"reason"`
}

type QueueConfigRequest struct {
	MaxConcurrency     int     `json:"max_concurrency"`
	RateLimitPerSecond float64 `json:"rate_limit_per_second"`
	Burst              int     `json:"burst"`
	AgingAfterSeconds  int     `json:"aging_after_seconds"`
	AgingPriorityBoost int     `json:"aging_priority_boost"`
}

type TaskStatsResponse map[domain.TaskStatus]int64
