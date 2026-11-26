package testutil

import (
	"context"
	"sync"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// TranslationReply is one scripted provider answer: a response, or an error.
type TranslationReply struct {
	Response enrichment.TranslationResponse
	Err      error
}

// FakeTranslationProvider is a shared enrichment.TranslationProvider for
// tests. Replies are returned in order and the last one repeats; with no
// replies it returns an empty response. Name and Version are configurable
// through the exported fields before first use.
type FakeTranslationProvider struct {
	ProviderName    string
	ProviderVersion string

	mu       sync.Mutex
	replies  []TranslationReply
	enabled  bool
	requests []enrichment.TranslationRequest
}

// NewFakeTranslationProvider returns an enabled provider scripted with replies.
func NewFakeTranslationProvider(replies ...TranslationReply) *FakeTranslationProvider {
	return &FakeTranslationProvider{
		ProviderName:    "openai-compatible",
		ProviderVersion: "fake",
		replies:         replies,
		enabled:         true,
	}
}

func (p *FakeTranslationProvider) Name() string    { return p.ProviderName }
func (p *FakeTranslationProvider) Version() string { return p.ProviderVersion }

// Enabled reports whether the provider is enabled.
func (p *FakeTranslationProvider) Enabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.enabled
}

// SetEnabled turns the provider on or off.
func (p *FakeTranslationProvider) SetEnabled(enabled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.enabled = enabled
}

// Translate returns the next scripted reply and records the request.
func (p *FakeTranslationProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	index := len(p.requests)
	p.requests = append(p.requests, request)
	if len(p.replies) == 0 {
		return enrichment.TranslationResponse{}, nil
	}
	if index >= len(p.replies) {
		index = len(p.replies) - 1
	}
	reply := p.replies[index]
	response := reply.Response
	response.SentenceTranslationTargets = append([]string(nil), response.SentenceTranslationTargets...)
	return response, reply.Err
}

// Calls returns how many times Translate was called.
func (p *FakeTranslationProvider) Calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.requests)
}

// Requests returns a copy of the requests received, in order.
func (p *FakeTranslationProvider) Requests() []enrichment.TranslationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]enrichment.TranslationRequest(nil), p.requests...)
}

var _ enrichment.TranslationProvider = (*FakeTranslationProvider)(nil)
