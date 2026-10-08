package handlers

import (
	"net/http"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
)

func writeError(w http.ResponseWriter, r *http.Request, err error) {
	contract.WriteError(w, r, err)
}

func writeInvalidRequest(w http.ResponseWriter, r *http.Request, code, message string) {
	writeError(w, r, domain.InvalidArgument(code, message, nil))
}

func writeMalformedRequest(w http.ResponseWriter, r *http.Request, code, message string) {
	writeError(w, r, domain.NewError(domain.ErrorKindMalformedRequest, code, message, nil))
}
