package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"
	"maps"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
)

func fixtureLemmaCorrectionKey(owner, book, analysis string, start, end int64) string {
	return owner + "\x00" + book + "\x00" + analysis + "\x00" + strconv.FormatInt(start, 10) + "\x00" + strconv.FormatInt(end, 10)
}

func (s *Store) ListLemmaReviewOccurrences(_ context.Context, owner, bookID, surface string) ([]domain.LemmaReviewOccurrence, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != OwnerID || (surface != "" && surface != "Weg") || (bookID != BookID && bookID != routeMatchBookID && bookID != LemmaFlagBookID) {
		return nil, nil
	}
	if surface == "" {
		surface = "Weg"
	}
	corpusID, analysisRunID, sourceDocumentID := "fixture-corpus", ResultRunID, "fixture-unit"
	if bookID == routeMatchBookID {
		corpusID, analysisRunID, sourceDocumentID = "fixture-route-match-corpus", "fixture-route-match-run", "fixture-route-match-unit"
	}
	if bookID == LemmaFlagBookID {
		corpusID, analysisRunID, sourceDocumentID = "fixture-lemma-flag-corpus", "fixture-lemma-flag-run", "fixture-lemma-flag-unit"
	}
	result := make([]domain.LemmaReviewOccurrence, 0, 2)
	for i, offset := range []int64{4, 20} {
		start, end := offset, offset+3
		occurrence := domain.LemmaReviewOccurrence{
			OwnerID: owner, BookID: bookID, CorpusID: corpusID, AnalysisRunID: analysisRunID,
			SourceDocumentID: sourceDocumentID, StartOffset: start, EndOffset: end,
			SentenceOrdinal: int64(i), TokenOrdinal: 1, Surface: surface, RawLemma: "Weg",
			CanonicalLemma: "weg", UPOS: "NOUN", SentenceText: "Der Weg führt zum Haus.",
			CorrectedLemma: s.lemmaCorrections[fixtureLemmaCorrectionKey(owner, bookID, analysisRunID, start, end)],
			Excluded:       s.lemmaExclusions[fixtureLemmaCorrectionKey(owner, bookID, analysisRunID, start, end)],
		}
		if flag, ok := s.lemmaReviewFlags[fixtureLemmaCorrectionKey(owner, bookID, analysisRunID, start, end)]; ok {
			occurrence.ReviewFlagReason = flag.Reason
			occurrence.ReviewFlagProvenance = flag.Provenance
			occurrence.ReviewFlagResolution = flag.Resolution
		}
		result = append(result, occurrence)
	}
	return result, nil
}

func (s *Store) SaveLemmaReviewFlags(_ context.Context, flags []domain.LemmaReviewFlag) error {
	s.mu.Lock()
	defer s.mu.Unlock()
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
		validAnalysis := occurrence.BookID == BookID && occurrence.AnalysisRunID == ResultRunID || occurrence.BookID == routeMatchBookID && occurrence.AnalysisRunID == "fixture-route-match-run" || occurrence.BookID == LemmaFlagBookID && occurrence.AnalysisRunID == "fixture-lemma-flag-run"
		if occurrence.OwnerID != OwnerID || !validAnalysis || occurrence.Surface != "Weg" {
			return errNotFound
		}
		if goal, ok := s.currentReadings[fixtureGoalKey(occurrence.OwnerID, "de")]; ok && goal.IsActive() && goal.BookID == occurrence.BookID {
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
		return persistence.ErrLemmaDecisionCurrentReading
	}
	preview, err := s.ReadLemmaReviewProposal(ctx, proposal)
	if err != nil {
		return err
	}
	if preview.Fingerprint != expected {
		return persistence.ErrLemmaReviewPreviewStale
	}
	return s.PutLemmaDecisions(ctx, proposal.Decisions())
}

func (s *Store) hasUnresolvedLemmaReviewFlag(owner, bookID string) bool {
	runID := ""
	if bookID == routeMatchBookID {
		runID = "fixture-route-match-run"
	}
	if bookID == LemmaFlagBookID {
		runID = "fixture-lemma-flag-run"
	}
	if bookID == BookID {
		runID = ResultRunID
	}
	for key, flag := range s.lemmaReviewFlags {
		if strings.HasPrefix(key, owner+"\x00"+bookID+"\x00"+runID+"\x00") && flag.Resolution == "" {
			return true
		}
	}
	return false
}
