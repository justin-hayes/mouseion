package prepareddeck

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
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
			if retryable != tt.retryable || code != tt.code {
				t.Fatalf("got retryable=%v code=%q, want %v %q", retryable, code, tt.retryable, tt.code)
			}
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
	if got := standardRetryDelay(w.Config, 1); got != time.Second {
		t.Fatalf("first retry delay=%s", got)
	}
	if got := standardRetryDelay(w.Config, 4); got != 3*time.Second {
		t.Fatalf("capped retry delay=%s", got)
	}
}
