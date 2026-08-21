// Package analyzertest provides reusable test implementations and contract
// checks for analyzer backends.
package analyzertest

import (
	"context"
	"sync"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

// Fake is an analyzer controlled by a test-supplied function.
type Fake struct {
	AnalyzeFunc func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error)

	mu       sync.Mutex
	requests []analyzer.AnalyzeRequest
}

// Analyze records the whole-document request and delegates to AnalyzeFunc.
func (fake *Fake) Analyze(ctx context.Context, request analyzer.AnalyzeRequest) (analyzer.Result, error) {
	fake.mu.Lock()
	fake.requests = append(fake.requests, request)
	fake.mu.Unlock()
	return fake.AnalyzeFunc(ctx, request)
}

// Requests returns a snapshot of requests received by the fake.
func (fake *Fake) Requests() []analyzer.AnalyzeRequest {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	return append([]analyzer.AnalyzeRequest(nil), fake.requests...)
}

var _ analyzer.Analyzer = (*Fake)(nil)
