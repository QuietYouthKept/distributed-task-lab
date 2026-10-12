// Package artifacts provides a bounded, content-verified object abstraction
// for worker inputs and durable outputs.
package artifacts

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrNotFound       = errors.New("artifact not found")
	ErrConflict       = errors.New("artifact key already contains different content")
	ErrUnknownOutcome = errors.New("artifact write outcome is unknown")
	ErrAccessDenied   = errors.New("artifact access denied")
	ErrInvalidKey     = errors.New("invalid artifact key")
	ErrTooLarge       = errors.New("artifact exceeds configured size limit")
	ErrCorrupt        = errors.New("artifact content or metadata is corrupt")
)

type ObjectInfo struct {
	// Key is the logical key accepted by the Store API. Configured bucket and
	// prefix details remain private so a returned artifact:// reference can be
	// passed back to the same Store without prefix duplication.
	Key         string `json:"key"`
	SHA256      string `json:"sha256"`
	Size        int64  `json:"size"`
	ContentType string `json:"content_type,omitempty"`
	VersionID   string `json:"version_id,omitempty"`
}

type Store interface {
	PutIfAbsent(context.Context, string, []byte, string) (ObjectInfo, error)
	Get(context.Context, string) ([]byte, ObjectInfo, error)
	Head(context.Context, string) (ObjectInfo, error)
	Delete(context.Context, string) error
}

type StoreError struct {
	Operation string
	Key       string
	Kind      error
	Cause     error
}

func (e *StoreError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("artifact %s %q: %v", e.Operation, e.Key, e.Cause)
	}
	return fmt.Sprintf("artifact %s %q: %v", e.Operation, e.Key, e.Kind)
}
func (e *StoreError) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Kind
}
func (e *StoreError) Is(target error) bool { return target == e.Kind || errors.Is(e.Cause, target) }

func wrapStoreError(operation, key string, kind, cause error) error {
	return &StoreError{Operation: operation, Key: key, Kind: kind, Cause: cause}
}
