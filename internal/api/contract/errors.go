// Package contract owns HTTP response shapes shared by router middleware and
// endpoint handlers. Keeping it below api avoids an import cycle between the
// router and handlers packages.
package contract

import (
	"errors"
	"net/http"

	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ErrorResponse struct {
	Error     APIError `json:"error"`
	RequestID string   `json:"request_id"`
}

// WriteError maps a domain error to the public, versionable error envelope.
// It deliberately never serializes err.Error(), which may contain SQL or a
// provider error returned by an untrusted dependency.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	status, body := ClassifyError(err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = jsonEncoder(w, ErrorResponse{Error: body, RequestID: RequestID(r.Context())})
}

// WriteRateLimitError emits the stable, request-correlated rate-limit response.
func WriteRateLimitError(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = jsonEncoder(w, ErrorResponse{Error: APIError{Code: "rate_limited", Message: "request rate limit exceeded"}, RequestID: RequestID(r.Context())})
}

func ClassifyError(err error) (int, APIError) {
	var typed *domain.Error
	if errors.As(err, &typed) {
		switch typed.Kind {
		case domain.ErrorKindMalformedRequest:
			return http.StatusBadRequest, APIError{Code: typed.Code, Message: typed.Message}
		case domain.ErrorKindInvalidArgument:
			return http.StatusUnprocessableEntity, APIError{Code: typed.Code, Message: typed.Message}
		case domain.ErrorKindNotFound:
			return http.StatusNotFound, APIError{Code: typed.Code, Message: typed.Message}
		case domain.ErrorKindConflict:
			return http.StatusConflict, APIError{Code: typed.Code, Message: typed.Message}
		case domain.ErrorKindUnauthorized:
			return http.StatusUnauthorized, APIError{Code: typed.Code, Message: typed.Message}
		case domain.ErrorKindForbidden:
			return http.StatusForbidden, APIError{Code: typed.Code, Message: typed.Message}
		case domain.ErrorKindUnavailable:
			return http.StatusServiceUnavailable, APIError{Code: typed.Code, Message: typed.Message}
		}
	}
	switch {
	case errors.Is(err, domain.ErrTaskNotFound), errors.Is(err, domain.ErrQueueNotFound):
		return http.StatusNotFound, APIError{Code: "not_found", Message: "requested resource was not found"}
	case errors.Is(err, domain.ErrTaskNotClaimed), errors.Is(err, domain.ErrTaskNotRunning), errors.Is(err, domain.ErrAlreadyClaimed), errors.Is(err, domain.ErrTaskStateConflict):
		return http.StatusConflict, APIError{Code: "state_conflict", Message: "operation conflicts with the current task state"}
	default:
		return http.StatusInternalServerError, APIError{Code: "internal_error", Message: "internal server error"}
	}
}
