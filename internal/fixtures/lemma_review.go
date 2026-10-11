package fixtures

// Contract status: contractual. Lemma review runs the internal/storecontract
// scenarios (ADR 0088) against this store and the PostgreSQL adapter. Occurrences
// are seeded per Book, and every decision and flag is validated against them.

import (
	"context"
	"maps"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

func fixtureLemmaCorrectionKey(owner, book, analysis string, start, end int64) string {
	return owner + "\x00" + book + "\x00" + analysis + "\x00" + strconv.FormatInt(start, 10) + "\x00" + strconv.FormatInt(end, 10)
}

// lemmaOccurrenceKey names the occurrences one Book's analysis contributes to
// lemma review.
func lemmaOccurrenceKey(owner, bookID string) string {
	return owner + "\x00" + bookID
}

// fixtureLemmaOccurrences builds the two "Weg" occurrences a canned Book's
// analysis contributes, one per sentence, with no learner decision applied.
func fixtureLemmaOccurrences(owner, bookID, corpusID, analysisRunID, sourceDocumentID string) []domain.LemmaReviewOccurrence {
	result := make([]domain.LemmaReviewOccurrence, 0, 2)
	for i, offset := range []int64{4, 20} {
		result = append(result, domain.LemmaReviewOccurrence{
			OwnerID: owner, BookID: bookID, CorpusID: corpusID, AnalysisRunID: analysisRunID,
			SourceDocumentID: sourceDocumentID, StartOffset: offset, EndOffset: offset + 3,
			SentenceOrdinal: int64(i), TokenOrdinal: 1, Surface: "Weg", RawLemma: "Weg",
			CanonicalLemma: "weg", UPOS: "NOUN", SentenceText: "Der Weg führt zum Haus.",
		})
	}
	return result
}

// fixtureCannedLemmaOccurrences seeds the lemma review occurrences of the Books
// the browser fixtures present.
func fixtureCannedLemmaOccurrences() map[string][]domain.LemmaReviewOccurrence {
	canned := []struct{ book, corpus, run, unit string }{
		{BookID, "fixture-corpus", ResultRunID, "fixture-unit"},
		{routeMatchBookID, "fixture-route-match-corpus", "fixture-route-match-run", "fixture-route-match-unit"},
		{LemmaFlagBookID, "fixture-lemma-flag-corpus", "fixture-lemma-flag-run", "fixture-lemma-flag-unit"},
	}
	occurrences := make(map[string][]domain.LemmaReviewOccurrence, len(canned))
	for _, c := range canned {
		occurrences[lemmaOccurrenceKey(OwnerID, c.book)] = fixtureLemmaOccurrences(OwnerID, c.book, c.corpus, c.run, c.unit)
	}
	return occurrences
}

// SeedLemmaReviewOccurrence adds an occurrence to a Book's analysis, so a test can
// review a surface form the browser fixtures do not present.
func (s *Store) SeedLemmaReviewOccurrence(occurrence domain.LemmaReviewOccurrence) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := lemmaOccurrenceKey(occurrence.OwnerID, occurrence.BookID)
	s.lemmaOccurrences[key] = append(s.lemmaOccurrences[key], occurrence)
}

// hasLemmaOccurrenceLocked reports whether occurrence is one the Book's analysis
// was seeded with. The caller holds s.mu.
func (s *Store) hasLemmaOccurrenceLocked(occurrence domain.LemmaReviewOccurrence) bool {
	for _, seeded := range s.lemmaOccurrences[lemmaOccurrenceKey(occurrence.OwnerID, occurrence.BookID)] {
		if seeded.ID() == occurrence.ID() {
			return true
		}
	}
	return false
}

// ListLemmaReviewOccurrences returns the seeded occurrences of surface with the
// learner's decisions and flags applied. An empty surface returns every one.
func (s *Store) ListLemmaReviewOccurrences(_ context.Context, owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seeded := s.lemmaOccurrences[lemmaOccurrenceKey(owner, bookID)]
	result := make([]domain.LemmaReviewOccurrence, 0, len(seeded))
	for _, occurrence := range seeded {
		if surface != "" && occurrence.Surface != surface {
			continue
		}
		key := fixtureLemmaCorrectionKey(owner, bookID, occurrence.AnalysisRunID, occurrence.StartOffset, occurrence.EndOffset)
		occurrence.CorrectedLemma = s.lemmaCorrections[key]
		occurrence.Excluded = s.lemmaExclusions[key]
		if flag, ok := s.lemmaReviewFlags[key]; ok {
			occurrence.ReviewFlagReason = flag.Reason
			occurrence.ReviewFlagProvenance = flag.Provenance
			occurrence.ReviewFlagResolution = flag.Resolution
		}
		result = append(result, occurrence)
	}
	return result, nil
}

// SaveLemmaReviewFlags validates every flag before it records any, as the
// persistence store's transaction does, and never reopens a resolved flag.
func (s *Store) SaveLemmaReviewFlags(_ context.Context, flags []domain.LemmaReviewFlag) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, flag := range flags {
		if strings.TrimSpace(flag.Reason) == "" || !s.hasLemmaOccurrenceLocked(flag.Occurrence) {
			return errNotFound
		}
	}
	for _, flag := range flags {
		o := flag.Occurrence
		key := fixtureLemmaCorrectionKey(o.OwnerID, o.BookID, o.AnalysisRunID, o.StartOffset, o.EndOffset)
		if current, ok := s.lemmaReviewFlags[key]; !ok || current.Resolution == "" {
			s.lemmaReviewFlags[key] = flag
		}
	}
	return nil
}

func (s *Store) PutLemmaDecisions(_ context.Context, decisions []domain.LemmaReviewDecision) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, decision := range decisions {
		occurrence := decision.Occurrence
		if !s.hasLemmaOccurrenceLocked(occurrence) {
			return errNotFound
		}
		if s.lemmaDecisionBlockedByCurrentReadingLocked(occurrence.OwnerID, occurrence.BookID) {
			return errNotFound
		}
		key := fixtureLemmaCorrectionKey(occurrence.OwnerID, occurrence.BookID, occurrence.AnalysisRunID, occurrence.StartOffset, occurrence.EndOffset)
		if s.lemmaCorrections[key] != occurrence.CorrectedLemma || s.lemmaExclusions[key] != occurrence.Excluded {
			return errNotFound
		}
	}
	for _, decision := range decisions {
		occurrence, lemma, excluded := decision.Occurrence, decision.CanonicalLemma, decision.Excluded
		key := fixtureLemmaCorrectionKey(occurrence.OwnerID, occurrence.BookID, occurrence.AnalysisRunID, occurrence.StartOffset, occurrence.EndOffset)
		if excluded {
			delete(s.lemmaCorrections, key)
			s.lemmaExclusions[key] = true
		} else {
			delete(s.lemmaExclusions, key)
			if lemma == occurrence.CanonicalLemma {
				delete(s.lemmaCorrections, key)
			} else {
				s.lemmaCorrections[key] = lemma
			}
		}
		if flag, ok := s.lemmaReviewFlags[key]; ok {
			flag.Resolution = "correct"
			if excluded {
				flag.Resolution = "exclude"
			} else if lemma == occurrence.CanonicalLemma {
				flag.Resolution = "keep"
			}
			s.lemmaReviewFlags[key] = flag
		}
	}
	return nil
}

// lemmaDecisionBlockedByCurrentReadingLocked mirrors the store's gate: a
// vocabulary decision is refused for the Book that is the active German
// current reading. The caller holds s.mu.
func (s *Store) lemmaDecisionBlockedByCurrentReadingLocked(owner, bookID string) bool {
	goal, ok := s.currentReadings[fixtureGoalKey(owner, "de")]
	return ok && goal.IsActive() && goal.BookID == bookID
}

// ReadLemmaReviewProposal derives deterministic before and after counts from the
// fixture Book's vocabulary, with no other Books contributing and nothing Reserved.
func (s *Store) ReadLemmaReviewProposal(ctx context.Context, proposal domain.LemmaReviewProposal) (domain.LemmaReviewPreview, error) {
	occurrences, err := s.ListLemmaReviewOccurrences(ctx, proposal.OwnerID, proposal.BookID, proposal.Surface)
	if err != nil {
		return domain.LemmaReviewPreview{}, err
	}
	before := map[domain.LemmaReviewIdentity]int64{}
	if len(occurrences) > 0 {
		vocabulary, vocabularyErr := s.GetProjectedCorpusVocabulary(ctx, proposal.OwnerID, occurrences[0].CorpusID)
		if vocabularyErr != nil {
			return domain.LemmaReviewPreview{}, vocabularyErr
		}
		for _, item := range vocabulary.Lemmas {
			before[domain.LemmaReviewIdentity{Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS}] = item.OccurrenceCount
		}
	}
	after := maps.Clone(before)
	for _, decision := range proposal.Decisions() {
		if current := selection.ReviewIdentityNow(proposal.Language, decision.Occurrence); current != (selection.Identity{}) {
			after[domain.LemmaReviewIdentity(current)]--
		}
		if next := selection.ReviewIdentityAfter(proposal.Language, decision.Occurrence, decision); next != (selection.Identity{}) {
			after[domain.LemmaReviewIdentity(next)]++
		}
	}
	known, err := s.ListKnownVocabulary(ctx, proposal.OwnerID, proposal.Language)
	if err != nil {
		return domain.LemmaReviewPreview{}, err
	}
	knownSet := make(map[domain.LemmaReviewIdentity]bool, len(known))
	for _, item := range known {
		knownSet[domain.LemmaReviewIdentity{Language: item.Language, CanonicalLemma: item.CanonicalLemma, UPOS: item.UPOS}] = true
	}
	affected := selection.ProposalIdentities(proposal)
	states := make([]domain.LemmaReviewIdentityState, 0, len(affected))
	for _, identity := range affected {
		states = append(states, domain.LemmaReviewIdentityState{
			Identity: identity, InBook: before[identity], AfterInBook: after[identity],
			Known: knownSet[identity] || knownSet[domain.LemmaReviewIdentity{Language: identity.Language, CanonicalLemma: identity.CanonicalLemma}],
		})
	}
	return domain.LemmaReviewPreview{States: states, Fingerprint: domain.LemmaReviewFingerprint(proposal, occurrences, states)}, nil
}

// PutLemmaDecisionProposal applies a proposal only when its state still matches
// the preview's fingerprint, as the persistence store does.
func (s *Store) PutLemmaDecisionProposal(ctx context.Context, proposal domain.LemmaReviewProposal, expected string) error {
	if len(proposal.Occurrences) == 0 {
		return errNotFound
	}
	s.mu.Lock()
	blocked := s.lemmaDecisionBlockedByCurrentReadingLocked(proposal.OwnerID, proposal.BookID)
	s.mu.Unlock()
	if blocked {
		return domain.ErrLemmaDecisionCurrentReading
	}
	preview, err := s.ReadLemmaReviewProposal(ctx, proposal)
	if err != nil {
		return err
	}
	if preview.Fingerprint != expected {
		return domain.ErrLemmaReviewPreviewStale
	}
	return s.PutLemmaDecisions(ctx, proposal.Decisions())
}

// hasUnresolvedLemmaReviewFlag reports whether any seeded occurrence of the Book
// carries a flag the learner has not resolved.
func (s *Store) hasUnresolvedLemmaReviewFlag(owner, bookID string) bool {
	for _, occurrence := range s.lemmaOccurrences[lemmaOccurrenceKey(owner, bookID)] {
		key := fixtureLemmaCorrectionKey(owner, bookID, occurrence.AnalysisRunID, occurrence.StartOffset, occurrence.EndOffset)
		if flag, ok := s.lemmaReviewFlags[key]; ok && flag.Resolution == "" {
			return true
		}
	}
	return false
}
