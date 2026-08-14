package domain

// QueueConfig describes the governance policy for a named queue.
type QueueConfig struct {
	Name               string  `json:"name"`
	MaxConcurrency     int     `json:"max_concurrency"`
	RateLimitPerSecond float64 `json:"rate_limit_per_second"`
	Paused             bool    `json:"paused"`
}

// DefaultQueueName is the queue tasks are assigned to when none is specified.
const DefaultQueueName = "default"
