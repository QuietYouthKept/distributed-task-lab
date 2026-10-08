package application

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"testing"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/lib/pq"
)

func TestWrapQueueErrorClassifiesDatabaseUnavailable(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "deadline exceeded", err: context.DeadlineExceeded},
		{name: "closed connection", err: sql.ErrConnDone},
		{name: "bad connection", err: driver.ErrBadConn},
		{name: "postgres connection exception", err: &pq.Error{Code: "08006"}},
		{name: "postgres connection pool exhausted", err: &pq.Error{Code: "53300"}},
		{name: "postgres shutdown", err: &pq.Error{Code: "57P01"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wrapQueueError("task service: get", fmt.Errorf("storage: %w", tt.err))

			var domainErr *domain.Error
			if !errors.As(err, &domainErr) {
				t.Fatalf("expected *domain.Error, got %T: %v", err, err)
			}
			if domainErr.Kind != domain.ErrorKindUnavailable {
				t.Errorf("kind = %q, want %q", domainErr.Kind, domain.ErrorKindUnavailable)
			}
			if domainErr.Code != "database_unavailable" {
				t.Errorf("code = %q, want database_unavailable", domainErr.Code)
			}
			if !errors.Is(err, tt.err) {
				t.Errorf("original cause is not preserved: %v", err)
			}
		})
	}
}

func TestWrapQueueErrorPreservesDomainClassification(t *testing.T) {
	original := domain.NewError(domain.ErrorKindConflict, "task_state_conflict", "task state conflict", domain.ErrTaskStateConflict)
	err := wrapQueueError("task service: cancel", original)

	var domainErr *domain.Error
	if !errors.As(err, &domainErr) {
		t.Fatalf("expected wrapped *domain.Error, got %T: %v", err, err)
	}
	if domainErr != original {
		t.Fatal("domain error was replaced instead of wrapped")
	}
	if !errors.Is(err, domain.ErrTaskStateConflict) {
		t.Fatal("domain cause is not preserved")
	}
}

func TestWrapQueueErrorLeavesNonTransientFailureInternal(t *testing.T) {
	original := errors.New("query syntax failure")
	err := wrapQueueError("task service: list", original)

	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		t.Fatalf("unexpected public domain classification: %+v", domainErr)
	}
	if !errors.Is(err, original) {
		t.Fatal("original cause is not preserved")
	}
}
