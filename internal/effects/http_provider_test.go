package effects

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHTTPProviderApplyClassifiesContractResponses(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		want      ProviderOutcome
		wantError bool
	}{
		{name: "completed", status: http.StatusCreated, body: `{"operation_id":"op-1","receipt_id":"receipt-1","status":"completed"}`, want: ProviderConfirmedSuccess},
		{name: "permanent rejection", status: http.StatusUnprocessableEntity, body: `provider-private-token`, want: ProviderConfirmedFailure, wantError: true},
		{name: "retryable rejection", status: http.StatusTooManyRequests, body: `retry later`, want: ProviderRetryableRejected, wantError: true},
		{name: "server outcome unknown", status: http.StatusInternalServerError, body: `temporarily unavailable`, want: ProviderUnknown, wantError: true},
		{name: "malformed success unknown", status: http.StatusOK, body: `{}`, want: ProviderUnknown, wantError: true},
		{name: "redirect is not followed", status: http.StatusTemporaryRedirect, body: ``, want: ProviderUnknown, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/operations" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				assertProviderIdentityHeaders(t, r)
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			provider, err := NewHTTPProvider(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := provider.Apply(context.Background(), testProviderRequest())
			if got.Outcome != tt.want || (got.Err != nil) != tt.wantError {
				t.Fatalf("Apply() = outcome %q, receipt %#v, err %v; want %q, error=%t", got.Outcome, got.Receipt, got.Err, tt.want, tt.wantError)
			}
			if got.Err != nil && strings.Contains(got.Err.Error(), "provider-private-token") {
				t.Fatal("provider response body must not be copied into the returned error")
			}
			if tt.want == ProviderConfirmedSuccess && got.Receipt.ReceiptID != "receipt-1" {
				t.Fatalf("receipt = %#v", got.Receipt)
			}
		})
	}
}

func TestHTTPProviderTimeoutAndCancellationRemainDistinct(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(entered)
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"operation_id":"op-1","receipt_id":"receipt-1","status":"completed"}`))
	}))
	defer server.Close()
	client := &http.Client{Timeout: 10 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	provider, err := NewHTTPProvider(server.URL, client)
	if err != nil {
		t.Fatal(err)
	}
	result := provider.Apply(context.Background(), testProviderRequest())
	<-entered
	if result.Outcome != ProviderUnknown || result.Err == nil {
		t.Fatalf("timeout must remain unknown after the request was sent: %#v", result)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = provider.Apply(ctx, testProviderRequest())
	if result.Outcome != ProviderNotSent || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("pre-dispatch cancellation = %#v", result)
	}
}

func TestHTTPProviderDuplicateAndConflictingIdempotencyKeys(t *testing.T) {
	type saved struct {
		hash, receiptID string
	}
	var mu sync.Mutex
	seen := map[string]saved{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request ProviderRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if prior, ok := seen[request.IdempotencyKey]; ok {
			if prior.hash != string(request.Payload) {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte("idempotency key reused with different payload"))
				return
			}
			_, _ = w.Write([]byte(`{"operation_id":"op-1","receipt_id":"` + prior.receiptID + `","status":"completed"}`))
			return
		}
		seen[request.IdempotencyKey] = saved{hash: string(request.Payload), receiptID: "receipt-stable"}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"operation_id":"op-1","receipt_id":"receipt-stable","status":"completed"}`))
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	first := testProviderRequest()
	second := first
	second.Attempt++
	for _, request := range []ProviderRequest{first, second} {
		got := provider.Apply(context.Background(), request)
		if got.Outcome != ProviderConfirmedSuccess || got.Receipt.ReceiptID != "receipt-stable" {
			t.Fatalf("duplicate with same key and body = %#v", got)
		}
	}
	conflict := first
	conflict.Payload = json.RawMessage(`{"amount":200}`)
	got := provider.Apply(context.Background(), conflict)
	if got.Outcome != ProviderConfirmedFailure || got.Err == nil {
		t.Fatalf("same key with changed payload = %#v", got)
	}
}

func TestHTTPProviderConnectionResetAfterRequestIsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Idempotency-Key") != "key-1" {
			t.Errorf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Error("test server does not support connection hijacking")
			return
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Errorf("hijack response: %v", err)
			return
		}
		_ = conn.Close()
	}))
	defer server.Close()
	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := provider.Apply(context.Background(), testProviderRequest())
	if got.Outcome != ProviderUnknown || got.Err == nil {
		t.Fatalf("connection reset after request must remain unknown: %#v", got)
	}
}

func TestHTTPProviderReconciliationStates(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   ReconciliationState
	}{
		{name: "pending", status: http.StatusOK, body: `{"state":"pending"}`, want: ReconciliationPending},
		{name: "completed", status: http.StatusOK, body: `{"state":"completed","receipt":{"operation_id":"op-1","receipt_id":"receipt-1","status":"completed"}}`, want: ReconciliationCompleted},
		{name: "provider failed", status: http.StatusOK, body: `{"state":"failed","receipt":{"operation_id":"op-1","receipt_id":"receipt-2","status":"failed"}}`, want: ReconciliationFailed},
		{name: "completed missing receipt identity", status: http.StatusOK, body: `{"state":"completed","receipt":{"receipt_id":"receipt-3","status":"completed"}}`, want: ReconciliationUnknown},
		{name: "completed receipt has inconsistent status", status: http.StatusOK, body: `{"state":"completed","receipt":{"operation_id":"op-1","receipt_id":"receipt-4","status":"pending"}}`, want: ReconciliationUnknown},
		{name: "failed receipt has inconsistent status", status: http.StatusOK, body: `{"state":"failed","receipt":{"operation_id":"op-1","receipt_id":"receipt-5","status":"completed"}}`, want: ReconciliationUnknown},
		{name: "missing", status: http.StatusNotFound, want: ReconciliationNotFound},
		{name: "uncertain", status: http.StatusOK, body: `{"state":"unknown"}`, want: ReconciliationUnknown},
		{name: "query unavailable", status: http.StatusServiceUnavailable, body: `unavailable`, want: ReconciliationUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/operations/key-1" || r.Header.Get("Idempotency-Key") != "key-1" {
					t.Errorf("unexpected reconciliation request: %s %s", r.Method, r.URL.String())
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			provider, err := NewHTTPProvider(server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			got := provider.Reconcile(context.Background(), "key-1")
			if got.State != tt.want {
				t.Fatalf("Reconcile() = %#v; want state %q", got, tt.want)
			}
		})
	}
}

func TestHTTPProviderRejectsUnsafeEndpointAndInvalidRequest(t *testing.T) {
	for _, endpoint := range []string{"", "ftp://provider.example", "http://user:secret@provider.example", "http://provider.example?token=secret"} {
		if _, err := NewHTTPProvider(endpoint, nil); err == nil {
			t.Errorf("NewHTTPProvider(%q) unexpectedly accepted endpoint", endpoint)
		}
	}
	provider, err := NewHTTPProvider("http://provider.example", nil)
	if err != nil {
		t.Fatal(err)
	}
	got := provider.Apply(context.Background(), ProviderRequest{})
	if got.Outcome != ProviderNotSent || got.Err == nil {
		t.Fatalf("invalid request = %#v", got)
	}
}

func testProviderRequest() ProviderRequest {
	return ProviderRequest{OperationID: "op-1", IdempotencyKey: "key-1", RequestID: "req-1", TaskID: "task-1", Generation: 2, Attempt: 3, Operation: "notification", Target: "user-1", Payload: json.RawMessage(`{"message":"hello"}`)}
}

func assertProviderIdentityHeaders(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Idempotency-Key") != "key-1" || r.Header.Get("X-Request-ID") != "req-1" || r.Header.Get("X-Task-ID") != "task-1" || r.Header.Get("X-Lease-Generation") != "2" || r.Header.Get("X-Attempt") != "3" {
		t.Errorf("provider identity headers = %#v", r.Header)
	}
}
