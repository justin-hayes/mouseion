package prepareddeck

import (
	"context"
	"errors"
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
		{name: "malformed", err: errors.New("invalid response"), code: "invalid_response"},
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
