package enrichment

import (
	"context"
	"errors"
	"strings"
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

// IdiomCard is what one idiom-card request sends: the dictionary headword, the
// forms the matcher found in the sentence, the sentence and the entry's bounded
// candidate senses, with no lemma or learner metadata (ADR 0114).
type IdiomCard struct {
	Language, Headword string
	MatchedForms       []string
	Sentence           string
	Senses             []LexicalSense
}

// EnrichIdiomCard sends one idiom-card request (ADR 0137 "Idiom Study
// material"). A confirmed Idiom returns its Gloss and sentence translation; a
// literal_use or ambiguous answer returns no Gloss and says which, a meaning
// omission rather than a provider failure. It returns ErrExternalNotConfigured,
// never an empty result, when external enrichment is off, and a provider
// failure as a retryable error. Whether the payload may be sent at all is the
// caller's gate (ProviderAssessmentSet.MaySend). The answer is not cached here.
func (s *Service) EnrichIdiomCard(ctx context.Context, card IdiomCard) (ContextualMeaning, error) {
	if !s.ExternalConfigured() {
		return ContextualMeaning{}, ErrExternalNotConfigured
	}
	request := TranslationRequest{
		Language: card.Language, TargetLanguage: "en", CanonicalLemma: card.Headword,
		ExampleSentence: card.Sentence, CandidateSenses: CloneLexicalSenses(card.Senses),
		RequireIdiomCard: true, IdiomHeadword: card.Headword, IdiomMatchedForms: append([]string(nil), card.MatchedForms...),
	}
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

// EvidenceAssistedMeaning is the outcome of one evidence-assisted pass request
// for a source-evidence word (ADR 0138 §1): a Gloss that cites the dictionary
// senses it rests on, with the sentence translation and its alignment, or an
// abstention.
type EvidenceAssistedMeaning struct {
	Gloss                      string
	SentenceTranslation        string
	SentenceTranslationTargets []string
	// CitedSenseIDs are the native sense IDs of the senses the Gloss rests on,
	// each one drawn from the candidate senses the request sent.
	CitedSenseIDs []string
	// ContextOnly reports a Gloss given from context with no cited sense, or an
	// abstention.
	ContextOnly      bool
	UnresolvedReason string
	// InvalidCitation reports an answer that cited a sense the request did not
	// send, or one twice. It is not retryable: the evidence cannot defend it. It
	// carries no cited sense, but keeps whatever Gloss and sentence translation
	// came back with the answer (empty when they were unusable).
	InvalidCitation bool
}

// Resolved reports whether the pass produced a Gloss that cites at least one
// dictionary sense. A Gloss from context alone, or an abstention, is not.
func (m EvidenceAssistedMeaning) Resolved() bool {
	return strings.TrimSpace(m.Gloss) != "" && !m.ContextOnly && len(m.CitedSenseIDs) > 0
}

// EnrichEvidenceAssistedPass is the per-word enrichment entry point of the
// evidence-assisted pass (ADR 0101, ADR 0138): the candidate's representative
// sentence and target, the labelled passage context and the candidate's pinned
// dictionary senses with their native IDs are sent, and the answer is a Gloss
// with the senses it cites, or an abstention. It returns
// ErrExternalNotConfigured, not an empty result, when external enrichment is
// off; a provider failure or a malformed answer is returned as an error and is
// retryable, but an answer citing a sense the request did not send, or one
// twice, is the InvalidCitation outcome. Whether the payload may be sent at all is the caller's gate
// (ProviderAssessmentSet.MaySend); the answer is not cached here.
func (s *Service) EnrichEvidenceAssistedPass(ctx context.Context, c Candidate, passageContext string) (EvidenceAssistedMeaning, error) {
	if !s.ExternalConfigured() {
		return EvidenceAssistedMeaning{}, ErrExternalNotConfigured
	}
	request := externalTranslationRequest(c, c.ExampleSentence)
	request.PassageContext = passageContext
	request.RequireEvidenceAssistedPass = true
	request.StudyPass = true
	var metrics ExternalMetrics
	response, err := s.callTranslationWithRetry(ctx, request, &metrics)
	if err == nil {
		response, err = NormalizeTranslationResponse(request, response)
	}
	if errors.Is(err, ErrInvalidEvidenceCitation) {
		return invalidCitationMeaning(err), nil
	}
	if err != nil {
		return EvidenceAssistedMeaning{}, err
	}
	return EvidenceAssistedMeaning{
		Gloss:                      response.Gloss,
		SentenceTranslation:        response.SentenceTranslation,
		SentenceTranslationTargets: append([]string(nil), response.SentenceTranslationTargets...),
		CitedSenseIDs:              append([]string(nil), response.EvidenceIDs...),
		ContextOnly:                response.ContextOnly,
		UnresolvedReason:           response.UnresolvedReason,
	}, nil
}

// invalidCitationMeaning is the outcome of an answer whose citation the evidence
// cannot defend. It keeps the Gloss and sentence translation that came back, as
// long as they pass the same checks a defended answer's do, and drops them
// otherwise.
func invalidCitationMeaning(err error) EvidenceAssistedMeaning {
	meaning := EvidenceAssistedMeaning{InvalidCitation: true}
	citation, ok := errors.AsType[evidenceCitationError](err)
	if !ok {
		return meaning
	}
	returned := citation.response
	gloss, translation := strings.TrimSpace(returned.Gloss), strings.TrimSpace(returned.SentenceTranslation)
	if validateContextualGloss(gloss) != nil || hasMarkup(translation) {
		return meaning
	}
	meaning.Gloss, meaning.SentenceTranslation = gloss, translation
	meaning.SentenceTranslationTargets = ValidateSentenceTranslationTargets(translation, returned.SentenceTranslationTargets)
	return meaning
}
