package domain

import (
	"errors"
	"fmt"
)

// ErrorKind is the stable classification of an error that crosses the
// application boundary. Its value is intentionally independent of storage or
// provider error text so API clients can make safe decisions.
type ErrorKind string

const (
	ErrorKindMalformedRequest ErrorKind = "malformed_request"
	ErrorKindInvalidArgument  ErrorKind = "invalid_argument"
	ErrorKindNotFound         ErrorKind = "not_found"
	ErrorKindConflict         ErrorKind = "conflict"
	ErrorKindUnauthorized     ErrorKind = "unauthorized"
	ErrorKindForbidden        ErrorKind = "forbidden"
	ErrorKindUnavailable      ErrorKind = "unavailable"
	ErrorKindInternal         ErrorKind = "internal"
)

// Error is a public-safe, typed domain error. Cause is retained for logging
// and errors.Is/errors.As but is never returned to an API client.
type Error struct {
	Kind    ErrorKind
	Code    string
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *Error) Unwrap() error { return e.Cause }

// NewError creates a stable error with a safe client message.
func NewError(kind ErrorKind, code, message string, cause error) *Error {
	return &Error{Kind: kind, Code: code, Message: message, Cause: cause}
}

func InvalidArgument(code, message string, cause error) error {
	return NewError(ErrorKindInvalidArgument, code, message, cause)
}

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
	ErrTaskStateConflict  = errors.New("task state conflict")
	ErrQueueNotFound      = errors.New("queue not found")
	ErrNotImplemented     = errors.New("not implemented")
)
