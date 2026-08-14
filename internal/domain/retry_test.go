package domain

import (
	"testing"
	"time"
)

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		name       string
		base       time.Duration
		max        time.Duration
		multiplier float64
		attempt    int
		want       time.Duration
	}{
		{"first retry", time.Second, 0, 2.0, 1, time.Second},
		{"second retry", time.Second, 0, 2.0, 2, 2 * time.Second},
		{"third retry", time.Second, 0, 2.0, 3, 4 * time.Second},
		{"capped", time.Second, 3 * time.Second, 2.0, 5, 3 * time.Second},
		{"default multiplier", time.Second, 0, 0, 2, 2 * time.Second},
		{"default base", 0, 0, 2.0, 1, time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExponentialBackoff(tt.base, tt.max, tt.multiplier, tt.attempt)
			if got != tt.want {
				t.Errorf("ExponentialBackoff(%v, %v, %v, %d) = %v, want %v", tt.base, tt.max, tt.multiplier, tt.attempt, got, tt.want)
			}
		})
	}
}

func TestIsRetryableErrorType(t *testing.T) {
	retryable := []string{ErrorTypeTimeout, ErrorTypeExecution, "network_error", "dependency_timeout", ""}
	nonRetryable := []string{ErrorTypeValidation, ErrorTypeBadPayload, ErrorTypeUnsupported, ErrorTypePermanent}

	for _, et := range retryable {
		if !IsRetryableErrorType(et) {
			t.Errorf("IsRetryableErrorType(%q) = false, want true", et)
		}
	}
	for _, et := range nonRetryable {
		if IsRetryableErrorType(et) {
			t.Errorf("IsRetryableErrorType(%q) = true, want false", et)
		}
	}
}
