package domain

// TaskStatus represents the current state of a task.
type TaskStatus string

const (
	TaskStatusPending TaskStatus = "pending"
	TaskStatusRunning TaskStatus = "running"
	TaskStatusSuccess TaskStatus = "success"
	TaskStatusFailed  TaskStatus = "failed"
	TaskStatusDLQ     TaskStatus = "dlq"
)

// ValidTaskStatuses is the set of all valid task statuses.
var ValidTaskStatuses = map[TaskStatus]bool{
	TaskStatusPending: true,
	TaskStatusRunning: true,
	TaskStatusSuccess: true,
	TaskStatusFailed:  true,
	TaskStatusDLQ:     true,
}

// TaskType categorizes the kind of work a task represents.
type TaskType string

const (
	TaskTypeRAGIngestion TaskType = "rag_ingestion"
	TaskTypeEval         TaskType = "eval"
	TaskTypeBenchmark    TaskType = "benchmark"
)

// ValidTaskTypes is the set of all valid task types.
var ValidTaskTypes = map[TaskType]bool{
	TaskTypeRAGIngestion: true,
	TaskTypeEval:         true,
	TaskTypeBenchmark:    true,
}
