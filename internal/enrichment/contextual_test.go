package enrichment

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextualProvider struct {
	enabled  bool
	response TranslationResponse
	err      error
	requests []TranslationRequest
}

func (p *contextualProvider) Name() string    { return "contextual" }
func (p *contextualProvider) Version() string { return "1" }
func (p *contextualProvider) Enabled() bool   { return p.enabled }
func (p *contextualProvider) Translate(_ context.Context, request TranslationRequest) (TranslationResponse, error) {
	p.requests = append(p.requests, request)
	return p.response, p.err
}

func contextualCandidate() Candidate {
	return Candidate{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		TargetWord:      "Haus",
		ExampleSentence: "Das Haus steht am Ende der Strasse.",
	}
}

func TestEnrichExternalContextualRequiresAGloss(t *testing.T) {
	provider := &contextualProvider{enabled: true, response: TranslationResponse{
		Translation: "house", Gloss: "a building", ContextOnly: true, SentenceTranslation: "The house is at the end of the street.", SentenceTranslationTargets: []string{"house"},
	}}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, nil)

	meaning, err := service.EnrichExternalContextual(context.Background(), contextualCandidate())
	require.NoError(t, err)
	assert.True(t, meaning.Resolved())
	assert.Equal(t, "a building", meaning.Gloss)
	assert.Equal(t, "The house is at the end of the street.", meaning.SentenceTranslation)
	require.Len(t, provider.requests, 1)
	assert.True(t, provider.requests[0].RequireContextualGloss)
	assert.Equal(t, "Haus", provider.requests[0].TargetWord)
}

func TestEnrichExternalContextualReportsAMeaningOmission(t *testing.T) {
	provider := &contextualProvider{enabled: true, response: TranslationResponse{
		ContextOnly: true, SentenceTranslation: "The house.", UnresolvedKind: ContextualGlossUnresolvedLemmaMismatch, UnresolvedReason: "the sentence uses another word",
	}}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, nil)

	meaning, err := service.EnrichExternalContextual(context.Background(), contextualCandidate())
	require.NoError(t, err)
	assert.False(t, meaning.Resolved())
	assert.Equal(t, ContextualGlossUnresolvedLemmaMismatch, meaning.UnresolvedKind)
}

func TestEnrichExternalContextualSeparatesNotConfiguredFromAProviderFailure(t *testing.T) {
	off := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, &contextualProvider{enabled: false}, nil)
	_, err := off.EnrichExternalContextual(context.Background(), contextualCandidate())
	require.ErrorIs(t, err, ErrExternalNotConfigured)

	failing := &contextualProvider{enabled: true, err: errors.New("provider unavailable")}
	on := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, failing, nil)
	_, err = on.EnrichExternalContextual(context.Background(), contextualCandidate())
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrExternalNotConfigured)
}
