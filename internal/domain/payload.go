package domain

// TaskPayloadEnvelope wraps task-type-specific payload data.
type TaskPayloadEnvelope struct {
	Version string         `json:"version"`
	Kind    string         `json:"kind"`
	Data    map[string]any `json:"data"`
}

// TaskResult holds the outcome of a successfully executed task.
type TaskResult struct {
	Summary   string             `json:"summary"`
	Artifacts map[string]string  `json:"artifacts,omitempty"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
}

// TaskFailure describes why a task execution failed.
type TaskFailure struct {
	ErrorType  string `json:"error_type"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
	OccurredAt string `json:"occurred_at"`
}
