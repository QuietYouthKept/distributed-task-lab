package domain

import "errors"

// Error types used to classify task failures (stored in TaskFailure.ErrorType).
const (
	ErrorTypeTimeout     = "timeout"
	ErrorTypeExecution   = "execution_error"
	ErrorTypeValidation  = "validation_error"
	ErrorTypeBadPayload  = "bad_payload"
	ErrorTypeUnsupported = "unsupported_task_type"
	ErrorTypePermanent   = "permanent"
)

// IsRetryableErrorType reports whether a failure of the given type should be
// retried. Permanent/validation errors go straight to the DLQ; transient errors
// (timeout, execution, dependency) are retried with backoff.
func IsRetryableErrorType(errorType string) bool {
	switch errorType {
	case ErrorTypeValidation, ErrorTypeBadPayload, ErrorTypeUnsupported, ErrorTypePermanent:
		return false
	default:
		return true
	}
}

// Domain-level errors.
var (
	ErrTaskNotFound       = errors.New("task not found")
	ErrTaskNotClaimed     = errors.New("task is not claimed by this worker")
	ErrTaskNotRunning     = errors.New("task is not in running state")
	ErrInvalidStatus      = errors.New("invalid task status")
	ErrInvalidTaskType    = errors.New("invalid task type")
	ErrMaxAttemptsReached = errors.New("max attempts reached")
	ErrLeaseExpired       = errors.New("lease has expired")
	ErrAlreadyClaimed     = errors.New("task already claimed by another worker")
	ErrNotImplemented     = errors.New("not implemented")
)
