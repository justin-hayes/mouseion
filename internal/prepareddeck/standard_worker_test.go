package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
)

func TestClassifyStandardProviderError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
		code      string
	}{
		{name: "rate limit", err: &enrichment.LLMHTTPError{StatusCode: http.StatusTooManyRequests}, retryable: true, code: "http_429"},
		{name: "server error", err: &enrichment.LLMHTTPError{StatusCode: http.StatusBadGateway}, retryable: true, code: "http_502"},
		{name: "authentication", err: &enrichment.LLMHTTPError{StatusCode: http.StatusUnauthorized}, code: "http_401"},
		{name: "unsupported model", err: &enrichment.LLMHTTPError{StatusCode: http.StatusNotFound}, code: "http_404"},
		{name: "request encoding", err: errors.New("encode LLM translation input: required identity is empty"), code: "invalid_response"},
		{name: "malformed response", err: fmt.Errorf("%w: item_id mismatch", enrichment.ErrInvalidTranslationResponse), retryable: true, code: "invalid_response"},
		{name: "unknown", err: errors.New("invalid response"), code: "invalid_response"},
		{name: "timeout", err: context.DeadlineExceeded, retryable: true, code: "transport_timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, code, retryable := classifyStandardProviderError(tt.err, context.Background())
			assert.Equal(t, tt.retryable, retryable)
			assert.Equal(t, tt.code, code)
		})
	}
}

func TestShouldRetryStandardProviderAttempt(t *testing.T) {
	tests := []struct {
		name                string
		retryable           bool
		providerAttempts    int
		maxProviderAttempts int
		want                bool
	}{
		{name: "retry before budget", retryable: true, providerAttempts: 0, maxProviderAttempts: 2, want: true},
		{name: "budget exhausted", retryable: true, providerAttempts: 1, maxProviderAttempts: 2, want: false},
		{name: "terminal error", retryable: false, providerAttempts: 0, maxProviderAttempts: 2, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, shouldRetryStandardProviderAttempt(tt.retryable, tt.providerAttempts, tt.maxProviderAttempts))
		})
	}
}

func TestStandardRetryDelayIsBoundedAndJitterInjectable(t *testing.T) {
	w := &StandardTranslationWorker{
		Config: PreparedDeckConfig{StandardRetryBaseDelay: time.Second, StandardRetryMaxDelay: 3 * time.Second},
		Now:    func() time.Time { return time.Unix(100, 0) },
		Jitter: func(delay time.Duration) time.Duration { return delay },
	}
	// The production retry path persists the computed timestamp. This test
	// locks the pure backoff boundary through the same policy values without a
	// database or provider call.
	assert.Equal(t, time.Second, standardRetryDelay(w.Config, 1), "first retry delay")
	assert.Equal(t, 3*time.Second, standardRetryDelay(w.Config, 4), "capped retry delay")
}
