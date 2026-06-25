package observability

import (
	"log/slog"
	"os"
)

// Logger returns a structured JSON logger.
func Logger() *slog.Logger {
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})
	return slog.New(handler)
}

// Metrics holds basic counters for task processing.
type Metrics struct {
	PendingCount int64
	RunningCount int64
	SuccessCount int64
	FailedCount  int64
	DLQCount     int64
	RetryCount   int64

	TaskDurationMs []float64
}

// MetricsSnapshot provides a point-in-time view of metrics.
type MetricsSnapshot struct {
	Pending int64 `json:"pending_count"`
	Running int64 `json:"running_count"`
	Success int64 `json:"success_count"`
	Failed  int64 `json:"failed_count"`
	DLQ     int64 `json:"dlq_count"`
	Retry   int64 `json:"retry_count"`
}
