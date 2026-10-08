package application

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/lib/pq"
)

// wrapQueueError keeps domain errors intact while translating transient
// database failures into the stable public dependency-unavailable contract.
// The original error remains available to logs through errors.Unwrap.
func wrapQueueError(operation string, err error) error {
	if err == nil {
		return nil
	}

	var domainErr *domain.Error
	if errors.As(err, &domainErr) {
		return fmt.Errorf("%s: %w", operation, err)
	}

	if isDatabaseUnavailable(err) {
		return domain.NewError(
			domain.ErrorKindUnavailable,
			"database_unavailable",
			"database temporarily unavailable",
			fmt.Errorf("%s: %w", operation, err),
		)
	}

	return fmt.Errorf("%s: %w", operation, err)
}

func isDatabaseUnavailable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, sql.ErrConnDone) ||
		errors.Is(err, driver.ErrBadConn) {
		return true
	}

	var networkErr net.Error
	if errors.As(err, &networkErr) && networkErr.Timeout() {
		return true
	}

	var postgresErr *pq.Error
	if errors.As(err, &postgresErr) {
		switch postgresErr.Code.Class() {
		case "08", // connection exception
			"53", // insufficient resources, including too many connections
			"57": // operator intervention, including shutdown/crash
			return true
		}
	}

	return false
}
