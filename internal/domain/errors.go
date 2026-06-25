package domain

import "errors"

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
	ErrNotImplemented     = errors.New("not implemented in Phase 1")
)
