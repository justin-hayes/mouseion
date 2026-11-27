package fixtures

// Contract status: contractual. Study corrections (ADR 0134 §6) must match the
// internal/storecontract scenarios shared with PostgreSQL (ADR 0088).

import (
	"context"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListStudyOccurrences returns the occurrences that resolve to the word's
// identity under current evidence: the analyzed Books' seeded occurrences with
// the learner's corrections and exclusions applied, and the occurrence a
// browser fixture seeds a candidate without a Book from.
func (s *Store) ListStudyOccurrences(_ context.Context, owner string, identity domain.StudyItem) ([]domain.StudyOccurrenceRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var refs []domain.StudyOccurrenceRef
	for _, book := range s.books {
		if book.Source.OwnerID != owner || book.Source.Language != identity.Language || book.BookID == "" || !s.fixtureBookActiveLocked(owner, book.BookID) {
			continue
		}
		for _, occurrence := range s.lemmaOccurrences[lemmaOccurrenceKey(owner, book.BookID)] {
			effective, counts := s.effectiveStudyIdentityLocked(owner, book.BookID, identity.Language, occurrence)
			if counts && effective == identity {
				refs = append(refs, domain.StudyOccurrenceRef{
					BookID: occurrence.BookID, AnalysisRunID: occurrence.AnalysisRunID, CorpusID: occurrence.CorpusID,
					SentenceOrdinal: occurrence.SentenceOrdinal, TokenOrdinal: occurrence.TokenOrdinal,
					UnitID: occurrence.SourceDocumentID, StartOffset: occurrence.StartOffset, EndOffset: occurrence.EndOffset,
				})
			}
		}
	}
	for _, item := range s.studyCandidates {
		if item.owner == owner && item.StudyItem == identity {
			refs = append(refs, fixtureStudyDefaultSource)
		}
	}
	return refs, nil
}

// FindStudyOccurrences returns the occurrences that decide a correction check:
// the source when it still resolves to the identity, else one valid occurrence.
func (s *Store) FindStudyOccurrences(ctx context.Context, owner string, identity domain.StudyItem, source domain.StudyOccurrenceRef) ([]domain.StudyOccurrenceRef, error) {
	valid, err := s.ListStudyOccurrences(ctx, owner, identity)
	if err != nil {
		return nil, err
	}
	return domain.StudyEvidenceWitnesses(valid, source), nil
}

// SupersedeStudyMaterial replaces a word's current Study material after a
// correction. A record that is missing, foreign or already superseded is
// domain.ErrNotFound.
func (s *Store) SupersedeStudyMaterial(_ context.Context, owner, materialID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.ID == materialID && !s.studySuperseded[item.ID] {
			if s.studySuperseded == nil {
				s.studySuperseded = map[string]bool{}
			}
			s.studySuperseded[item.ID] = true
			return nil
		}
	}
	return errNotFound
}

// SetStudyWordWithdrawn withdraws a word by a correction at now, or returns it
// when withdrawn is false. Its progress is kept either way.
func (s *Store) SetStudyWordWithdrawn(_ context.Context, owner, progressID string, withdrawn bool, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyProgress {
		if item := &s.studyProgress[i]; item.OwnerID == owner && item.ID == progressID {
			item.Withdrawn, item.WithdrawnReason = withdrawn, ""
			return nil
		}
	}
	return errNotFound
}

// CompleteStudyReintroductions moves each word whose replacement Study
// material is ready onto it, with the new Gloss as its learned meaning and the
// word marked corrected at now. A word whose replacement material cannot be
// prepared is withdrawn. The FSRS state is untouched.
func (s *Store) CompleteStudyReintroductions(_ context.Context, owner, language string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyProgress {
		item := &s.studyProgress[i]
		if item.OwnerID != owner || item.Language != language || item.Withdrawn {
			continue
		}
		for _, material := range s.studyMaterial {
			if material.OwnerID != owner || material.StudyItem != item.StudyItem || s.studySuperseded[material.ID] || material.ID == item.MaterialID {
				continue
			}
			switch material.State {
			case domain.StudyMaterialReady:
				item.MaterialID, item.LearnedMeaning, item.CorrectedAt, item.UpdatedAt = material.ID, material.Gloss, now.UTC(), now.UTC()
				item.LearnedCitations = material.Cited.Clone()
			case domain.StudyMaterialUnpreparable:
				s.withdrawUnpreparableLocked(owner, item, now)
			case domain.StudyMaterialPending:
			}
		}
	}
	return nil
}

// withdrawUnpreparableLocked withdraws a word whose replacement material cannot
// be prepared, with the reason "no suitable context left" when its contexts were
// reported as unsuitable. The caller holds s.mu.
func (s *Store) withdrawUnpreparableLocked(owner string, item *domain.StudyProgress, now time.Time) {
	item.Withdrawn, item.UpdatedAt = true, now.UTC()
	if s.hasRetiringReportLocked(owner, item.ID) {
		item.WithdrawnReason = domain.StudyWithdrawnNoSuitableContext
	}
}

// StudyOccurrenceLocator completes an occurrence with its locator. A token the
// analyses do not hold is domain.ErrNotFound.
func (s *Store) StudyOccurrenceLocator(_ context.Context, owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.studyLocatorLocked(owner, ref)
}

// studyLocatorLocked completes an occurrence with its locator. The caller holds
// s.mu.
func (s *Store) studyLocatorLocked(owner string, ref domain.StudyOccurrenceRef) (domain.StudyOccurrenceRef, error) {
	for key, occurrences := range s.lemmaOccurrences {
		if !strings.HasPrefix(key, owner+"\x00") {
			continue
		}
		for _, occurrence := range occurrences {
			if occurrence.AnalysisRunID == ref.AnalysisRunID && occurrence.CorpusID == ref.CorpusID &&
				occurrence.SentenceOrdinal == ref.SentenceOrdinal && occurrence.TokenOrdinal == ref.TokenOrdinal {
				ref.UnitID, ref.StartOffset, ref.EndOffset = occurrence.SourceDocumentID, occurrence.StartOffset, occurrence.EndOffset
				return ref, nil
			}
		}
	}
	return domain.StudyOccurrenceRef{}, errNotFound
}

// RelocateStudyMaterial names the occurrence a word's ready, current material
// now comes from. The sentence, Gloss and translation are kept.
func (s *Store) RelocateStudyMaterial(_ context.Context, owner, materialID string, ref domain.StudyOccurrenceRef) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID == owner && item.ID == materialID && item.State == domain.StudyMaterialReady && !s.studySuperseded[item.ID] {
			item.AnalysisRunID, item.CorpusID, item.SentenceOrdinal, item.TokenOrdinal = ref.AnalysisRunID, ref.CorpusID, ref.SentenceOrdinal, ref.TokenOrdinal
			return nil
		}
	}
	return errNotFound
}

// SetStudyMaterialEvidence records the occurrences a preparation attempt saw
// on a pending record.
func (s *Store) SetStudyMaterialEvidence(_ context.Context, owner, materialID, evidence string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID == owner && item.ID == materialID && item.State == domain.StudyMaterialPending {
			item.EvidenceFingerprint = evidence
			return nil
		}
	}
	return errNotFound
}
