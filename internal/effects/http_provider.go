package effects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// ProviderOutcome describes what the caller can safely conclude after an
// Apply attempt. Unknown is deliberately not treated as a provider failure.
type ProviderOutcome string

const (
	ProviderNotSent           ProviderOutcome = "not_sent"
	ProviderConfirmedSuccess  ProviderOutcome = "confirmed_success"
	ProviderConfirmedFailure  ProviderOutcome = "confirmed_failure"
	ProviderRetryableRejected ProviderOutcome = "retryable_rejected"
	ProviderUnknown           ProviderOutcome = "unknown"
)

type ProviderRequest struct {
	OperationID    string          `json:"operation_id"`
	IdempotencyKey string          `json:"idempotency_key"`
	RequestID      string          `json:"request_id"`
	TaskID         string          `json:"task_id"`
	Generation     int             `json:"generation"`
	Attempt        int             `json:"attempt"`
	Operation      string          `json:"operation"`
	Target         string          `json:"target"`
	Payload        json.RawMessage `json:"payload"`
}

type ProviderReceipt struct {
	OperationID string `json:"operation_id"`
	ReceiptID   string `json:"receipt_id"`
	Status      string `json:"status"`
}

type ProviderApplyResult struct {
	Outcome ProviderOutcome
	Receipt *ProviderReceipt
	Err     error
}

type ReconciliationState string

const (
	ReconciliationPending   ReconciliationState = "pending"
	ReconciliationCompleted ReconciliationState = "completed"
	ReconciliationFailed    ReconciliationState = "failed"
	ReconciliationNotFound  ReconciliationState = "not_found"
	ReconciliationUnknown   ReconciliationState = "unknown"
)

type ReconciliationResult struct {
	State   ReconciliationState
	Receipt *ProviderReceipt
	Err     error
}

// Provider is the provider-independent boundary. Implementations must preserve
// the same idempotency key and request identity across retries. This interface
// is not wired to production task admission until a real provider is approved.
type Provider interface {
	Apply(context.Context, ProviderRequest) ProviderApplyResult
	Reconcile(context.Context, string) ReconciliationResult
}

// HTTPProvider implements the repository's explicit provider contract for
// contract/fault-injection tests and future adapters. It does not assert that
// arbitrary providers use these status-code semantics.
type HTTPProvider struct {
	endpoint string
	client   *http.Client
}

func NewHTTPProvider(endpoint string, client *http.Client) (*HTTPProvider, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("provider endpoint must be an absolute HTTP(S) URL without userinfo, query, or fragment")
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}}
	}
	return &HTTPProvider{endpoint: strings.TrimRight(u.String(), "/"), client: client}, nil
}

func (p *HTTPProvider) Apply(ctx context.Context, in ProviderRequest) ProviderApplyResult {
	if err := validateProviderRequest(in); err != nil {
		return ProviderApplyResult{Outcome: ProviderNotSent, Err: err}
	}
	if err := ctx.Err(); err != nil {
		return ProviderApplyResult{Outcome: ProviderNotSent, Err: err}
	}
	body, err := json.Marshal(in)
	if err != nil {
		return ProviderApplyResult{Outcome: ProviderNotSent, Err: err}
	}
	var wroteRequest atomic.Bool
	trace := &httptrace.ClientTrace{WroteRequest: func(httptrace.WroteRequestInfo) { wroteRequest.Store(true) }}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, trace), http.MethodPost, p.endpoint+"/operations", bytes.NewReader(body))
	if err != nil {
		return ProviderApplyResult{Outcome: ProviderNotSent, Err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", in.IdempotencyKey)
	req.Header.Set("X-Request-ID", in.RequestID)
	req.Header.Set("X-Task-ID", in.TaskID)
	req.Header.Set("X-Lease-Generation", fmt.Sprint(in.Generation))
	req.Header.Set("X-Attempt", fmt.Sprint(in.Attempt))
	resp, err := p.client.Do(req)
	if err != nil {
		outcome := ProviderUnknown
		if !wroteRequest.Load() {
			outcome = ProviderNotSent
		}
		return ProviderApplyResult{Outcome: outcome, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return ProviderApplyResult{Outcome: ProviderRetryableRejected, Err: fmt.Errorf("provider rejected request for retry (HTTP %d)", resp.StatusCode)}
	}
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		return ProviderApplyResult{Outcome: ProviderConfirmedFailure, Err: fmt.Errorf("provider rejected request (HTTP %d)", resp.StatusCode)}
	}
	if resp.StatusCode >= 500 || (resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated) {
		return ProviderApplyResult{Outcome: ProviderUnknown, Err: fmt.Errorf("provider returned ambiguous HTTP status %d", resp.StatusCode)}
	}
	receipt, err := decodeReceipt(resp.Body)
	if err != nil || receipt.Status != "completed" || receipt.OperationID != in.OperationID {
		if err == nil {
			err = errors.New("provider success response did not match the operation")
		}
		return ProviderApplyResult{Outcome: ProviderUnknown, Err: err}
	}
	return ProviderApplyResult{Outcome: ProviderConfirmedSuccess, Receipt: receipt}
}

func (p *HTTPProvider) Reconcile(ctx context.Context, idempotencyKey string) ReconciliationResult {
	if strings.TrimSpace(idempotencyKey) == "" {
		return ReconciliationResult{State: ReconciliationUnknown, Err: errors.New("idempotency key is required")}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.endpoint+"/operations/"+url.PathEscape(idempotencyKey), nil)
	if err != nil {
		return ReconciliationResult{State: ReconciliationUnknown, Err: err}
	}
	req.Header.Set("Idempotency-Key", idempotencyKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return ReconciliationResult{State: ReconciliationUnknown, Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ReconciliationResult{State: ReconciliationNotFound}
	}
	if resp.StatusCode >= 500 || resp.StatusCode >= 300 {
		return ReconciliationResult{State: ReconciliationUnknown, Err: fmt.Errorf("provider reconciliation returned HTTP %d", resp.StatusCode)}
	}
	var reply struct {
		State   ReconciliationState `json:"state"`
		Receipt ProviderReceipt     `json:"receipt"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&reply); err != nil {
		return ReconciliationResult{State: ReconciliationUnknown, Err: err}
	}
	switch reply.State {
	case ReconciliationPending, ReconciliationUnknown:
		return ReconciliationResult{State: reply.State}
	case ReconciliationCompleted, ReconciliationFailed:
		if reply.Receipt.ReceiptID == "" {
			return ReconciliationResult{State: ReconciliationUnknown, Err: errors.New("provider reconciliation response omitted receipt identity")}
		}
		return ReconciliationResult{State: reply.State, Receipt: &reply.Receipt}
	default:
		return ReconciliationResult{State: ReconciliationUnknown, Err: fmt.Errorf("provider returned unsupported reconciliation state %q", reply.State)}
	}
}

func validateProviderRequest(in ProviderRequest) error {
	if in.OperationID == "" || in.IdempotencyKey == "" || in.RequestID == "" || in.TaskID == "" || in.Generation < 1 || in.Attempt < 1 || in.Operation == "" || in.Target == "" || len(in.Payload) == 0 || !json.Valid(in.Payload) {
		return errors.New("provider request is missing valid operation, idempotency, task, lease, attempt, or payload identity")
	}
	return nil
}

func decodeReceipt(r io.Reader) (*ProviderReceipt, error) {
	var receipt ProviderReceipt
	if err := json.NewDecoder(io.LimitReader(r, 64<<10)).Decode(&receipt); err != nil {
		return nil, err
	}
	if receipt.OperationID == "" || receipt.ReceiptID == "" || receipt.Status == "" {
		return nil, errors.New("provider response omitted receipt identity or status")
	}
	return &receipt, nil
}
