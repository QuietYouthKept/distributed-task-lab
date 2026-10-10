package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/fantasticyi/distributed-task-lab/internal/api/contract"
	"github.com/fantasticyi/distributed-task-lab/internal/domain"
	"github.com/fantasticyi/distributed-task-lab/internal/effects"
)

type EffectHandler struct {
	ledger effects.ReconciliationLedger
	logger *slog.Logger
}

func NewEffectHandler(ledger effects.ReconciliationLedger, logger *slog.Logger) *EffectHandler {
	return &EffectHandler{ledger: ledger, logger: logger}
}

func (h *EffectHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.ledger == nil {
		writeError(w, r, domain.NewError(domain.ErrorKindUnavailable, "effect_reconciliation_unavailable", "effect reconciliation is unavailable", nil))
		return
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if err := json.Unmarshal([]byte(raw), &limit); err != nil || limit < 1 || limit > 500 {
			writeInvalidRequest(w, r, "invalid_limit", "limit must be between 1 and 500")
			return
		}
	}
	records, err := h.ledger.ListUnresolved(r.Context(), limit)
	if err != nil {
		h.logger.Error("effect reconciliation list failed", "error", err)
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contract.EffectsListResponse{Effects: records})
}

func (h *EffectHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	if h.ledger == nil {
		writeError(w, r, domain.NewError(domain.ErrorKindUnavailable, "effect_reconciliation_unavailable", "effect reconciliation is unavailable", nil))
		return
	}
	effectID := r.PathValue("id")
	if effectID == "" {
		writeInvalidRequest(w, r, "missing_effect_id", "effect id is required")
		return
	}
	var request contract.OptionalReasonRequest
	if err := decodeRequest(r, &request); err != nil || strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 1024 || strings.TrimSpace(request.Reason) != request.Reason {
		writeInvalidRequest(w, r, "invalid_resolution_reason", "reason must be 1 to 1024 bytes without surrounding whitespace")
		return
	}
	if err := h.ledger.ResolveManually(r.Context(), effectID, request.Reason); err != nil {
		h.logger.Error("effect reconciliation resolution failed", "effect_id", effectID, "error", err)
		writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, contract.EffectResolutionResponse{EffectID: effectID, Status: "manual_resolution"})
}
