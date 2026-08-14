package worker

// ExecutionError wraps a handler error with retryability classification. It lets
// a handler signal that a failure is permanent (non-retryable) and should go
// straight to the DLQ instead of being retried.
type ExecutionError struct {
	Err       error
	ErrorType string
	Retryable bool
}

func (e *ExecutionError) Error() string { return e.Err.Error() }
func (e *ExecutionError) Unwrap() error { return e.Err }

// NewNonRetryableError wraps err as a non-retryable (permanent) failure.
func NewNonRetryableError(err error, errorType string) *ExecutionError {
	return &ExecutionError{Err: err, ErrorType: errorType, Retryable: false}
}
