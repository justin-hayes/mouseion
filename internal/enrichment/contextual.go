package enrichment

import (
	"context"
	"errors"
)

// ErrExternalNotConfigured reports that no translation provider can be used:
// administrator configuration, learner consent or the provider itself is
// missing. A caller must treat it as "not ready", never as an omission.
var ErrExternalNotConfigured = errors.New("enrichment: external enrichment is not configured")

// ContextualMeaning is the outcome of one contextual-Gloss request. A resolved
// request has a Gloss and no UnresolvedKind. An unresolved one has no Gloss and
// says why, which is a meaning omission rather than a provider failure.
type ContextualMeaning struct {
	Gloss                      string
	SentenceTranslation        string
	SentenceTranslationTargets []string
	UnresolvedKind             ContextualGlossUnresolvedKind
	UnresolvedReason           string
}

// Resolved reports whether the request produced a Gloss.
func (m ContextualMeaning) Resolved() bool { return m.Gloss != "" }

// EnrichExternalContextual is the per-word enrichment entry point with a
// contextual Gloss required (the modern-language contract, ADR 0133): the
// candidate's example sentence is sent, and the answer is either a resolved
// Gloss with the sentence translation or a meaning omission. It returns
// ErrExternalNotConfigured, not an empty result, when external enrichment is
// off, so a caller never mistakes it for an omission. A provider failure is
// returned as an error and is retryable. The answer is not cached here: the
// caller keeps it durably with the word.
func (s *Service) EnrichExternalContextual(ctx context.Context, c Candidate) (ContextualMeaning, error) {
	if !s.ExternalConfigured() {
		return ContextualMeaning{}, ErrExternalNotConfigured
	}
	request := externalTranslationRequest(c, c.ExampleSentence)
	request.RequireContextualGloss = true
	var metrics ExternalMetrics
	response, err := s.callTranslationWithRetry(ctx, request, &metrics)
	if err != nil {
		return ContextualMeaning{}, err
	}
	response, err = NormalizeTranslationResponse(request, response)
	if err != nil {
		return ContextualMeaning{}, err
	}
	return ContextualMeaning{
		Gloss:                      response.Gloss,
		SentenceTranslation:        response.SentenceTranslation,
		SentenceTranslationTargets: append([]string(nil), response.SentenceTranslationTargets...),
		UnresolvedKind:             response.UnresolvedKind,
		UnresolvedReason:           response.UnresolvedReason,
	}, nil
}
