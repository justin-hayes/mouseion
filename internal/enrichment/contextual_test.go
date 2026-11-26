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

func passCandidate() Candidate {
	return Candidate{
		Language: "grc", CanonicalLemma: "λόγοσ", UPOS: "NOUN",
		TargetWord:      "λόγον",
		ExampleSentence: "ἄνθρωπος ὁρᾷ τὸν λόγον.",
		CandidateSenses: []LexicalSense{{EvidenceID: "n1.s1", Source: "lsj", Gloss: "word"}, {EvidenceID: "n1.s2", Source: "lsj", Gloss: "account"}},
	}
}

func TestEnrichEvidenceAssistedPassReturnsAGlossWithItsCitedSenses(t *testing.T) {
	provider := &contextualProvider{enabled: true, response: TranslationResponse{
		Translation: "word", Gloss: "word", EvidenceIDs: []string{"n1.s2"}, SentenceTranslation: "The man sees the word.", SentenceTranslationTargets: []string{"word"},
	}}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, nil)

	meaning, err := service.EnrichEvidenceAssistedPass(context.Background(), passCandidate(), "The man sees the word.")
	require.NoError(t, err)
	assert.True(t, meaning.Resolved())
	assert.Equal(t, "word", meaning.Gloss)
	assert.Equal(t, []string{"n1.s2"}, meaning.CitedSenseIDs)
	assert.Equal(t, "The man sees the word.", meaning.SentenceTranslation)
	assert.Equal(t, []string{"word"}, meaning.SentenceTranslationTargets)
	require.Len(t, provider.requests, 1)
	request := provider.requests[0]
	assert.True(t, request.RequireEvidenceAssistedPass, "the call uses the pass contract")
	assert.False(t, request.RequireContextualGloss)
	assert.Equal(t, "The man sees the word.", request.PassageContext)
	assert.Equal(t, passCandidate().CandidateSenses, request.CandidateSenses)
	assert.Equal(t, "λόγον", request.TargetWord)
}

func TestEnrichEvidenceAssistedPassIsNotResolvedWithoutACitedSense(t *testing.T) {
	cases := map[string]TranslationResponse{
		"context only": {Translation: "word", Gloss: "word", ContextOnly: true, SentenceTranslation: "The man sees the word."},
		"abstention":   {Translation: "word", ContextOnly: true, UnresolvedReason: "no sense fits", SentenceTranslation: "The man sees the word."},
	}
	for name, response := range cases {
		t.Run(name, func(t *testing.T) {
			provider := &contextualProvider{enabled: true, response: response}
			service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, provider, nil)
			meaning, err := service.EnrichEvidenceAssistedPass(context.Background(), passCandidate(), "")
			require.NoError(t, err)
			assert.False(t, meaning.Resolved())
		})
	}
}

func TestEnrichEvidenceAssistedPassReportsAnInvalidCitationAsAnOutcome(t *testing.T) {
	cases := map[string][]string{
		"invented": {"n9.s9"},
		"repeated": {"n1.s2", "n1.s2"},
	}
	for name, ids := range cases {
		t.Run(name, func(t *testing.T) {
			provider := &contextualProvider{enabled: true, response: TranslationResponse{
				Translation: "word", Gloss: "word", EvidenceIDs: ids, SentenceTranslation: "The man sees the word.",
			}}
			service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
			meaning, err := service.EnrichEvidenceAssistedPass(context.Background(), passCandidate(), "")
			require.NoError(t, err, "an answer citing what the evidence does not hold is an outcome, not a retryable failure")
			assert.True(t, meaning.InvalidCitation)
			assert.False(t, meaning.Resolved())
			assert.Equal(t, "word", meaning.Gloss, "the answer's Gloss survives the invalid citation")
			assert.Equal(t, "The man sees the word.", meaning.SentenceTranslation)
			assert.Empty(t, meaning.CitedSenseIDs)
		})
	}
}

func TestEnrichEvidenceAssistedPassKeepsAMalformedAnswerRetryable(t *testing.T) {
	provider := &contextualProvider{enabled: true, response: TranslationResponse{Gloss: "word", EvidenceIDs: []string{"n1.s2"}}}
	service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
	_, err := service.EnrichEvidenceAssistedPass(context.Background(), passCandidate(), "")
	require.Error(t, err)
}

func TestEnrichEvidenceAssistedPassSeparatesNotConfiguredFromAProviderFailure(t *testing.T) {
	off := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, &contextualProvider{enabled: false}, nil)
	_, err := off.EnrichEvidenceAssistedPass(context.Background(), passCandidate(), "")
	require.ErrorIs(t, err, ErrExternalNotConfigured)

	failing := &contextualProvider{enabled: true, err: errors.New("provider unavailable")}
	on := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, failing, nil)
	_, err = on.EnrichEvidenceAssistedPass(context.Background(), passCandidate(), "")
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrExternalNotConfigured)
}
