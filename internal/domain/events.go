package domain

// TaskEventType categorizes the kind of lifecycle event.
type TaskEventType string

const (
	EventSubmitted   TaskEventType = "submitted"
	EventClaimed     TaskEventType = "claimed"
	EventSucceeded   TaskEventType = "succeeded"
	EventFailed      TaskEventType = "failed"
	EventRetried     TaskEventType = "retried"
	EventMovedToDLQ  TaskEventType = "moved_to_dlq"
	EventCancelled   TaskEventType = "cancelled"
	EventRequeued    TaskEventType = "requeued"
)

// TaskEvent is an audit record of a task lifecycle transition.
type TaskEvent struct {
	ID        int            `json:"id"`
	TaskID    string         `json:"task_id"`
	EventType TaskEventType  `json:"event_type"`
	WorkerID  *string        `json:"worker_id,omitempty"`
	Message   string         `json:"message"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	CreatedAt string         `json:"created_at"`
}
