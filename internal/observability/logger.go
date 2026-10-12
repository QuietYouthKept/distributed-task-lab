package observability

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// Logger returns a structured JSON logger.
func Logger(secrets ...string) *slog.Logger {
	return slog.New(newJSONHandler(os.Stdout, secrets...))
}

func newJSONHandler(output io.Writer, secrets ...string) slog.Handler {
	return slog.NewJSONHandler(output, &slog.HandlerOptions{
		Level: slog.LevelInfo,
		ReplaceAttr: func(_ []string, attr slog.Attr) slog.Attr {
			if sensitiveAttribute(attr.Key) {
				attr.Value = slog.StringValue("[REDACTED]")
				return attr
			}
			if attr.Value.Kind() == slog.KindString {
				attr.Value = slog.StringValue(redactText(attr.Value.String(), secrets))
			} else if attr.Value.Kind() == slog.KindAny {
				if err, ok := attr.Value.Any().(error); ok {
					attr.Value = slog.StringValue(redactText(err.Error(), secrets))
				} else if text, ok := attr.Value.Any().(fmt.Stringer); ok {
					attr.Value = slog.StringValue(redactText(text.String(), secrets))
				}
			}
			return attr
		},
	})
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
