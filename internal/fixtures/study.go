package fixtures

// Contract status: contractual. Study candidate and Study material
// transitions must match internal/storecontract scenarios shared with
// PostgreSQL (ADR 0088).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListStudyCandidates derives the Study stream's candidates from the analyzed
// Books' seeded occurrences, applying the learner's lemma corrections and
// exclusions. Known and Reserved vocabulary are not consulted.
func (s *Store) ListStudyCandidates(_ context.Context, owner, language string) ([]domain.StudyCandidate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := s.currentReadings[fixtureGoalKey(owner, language)]
	tallies := map[domain.StudyIdentity]*domain.StudyCandidate{}
	for _, book := range s.books {
		if book.Source.OwnerID != owner || book.Source.Language != language || book.BookID == "" {
			continue
		}
		inReading := current.IsActive() && current.BookID == book.BookID
		for _, occurrence := range s.lemmaOccurrences[lemmaOccurrenceKey(owner, book.BookID)] {
			identity, counts := s.effectiveStudyIdentityLocked(owner, book.BookID, language, occurrence)
			if !counts {
				continue
			}
			entry := tallies[identity]
			if entry == nil {
				entry = &domain.StudyCandidate{StudyIdentity: identity}
				tallies[identity] = entry
			}
			entry.PersonalOccurrences++
			entry.InCurrentReading = entry.InCurrentReading || inReading
		}
	}
	candidates := make([]domain.StudyCandidate, 0, len(tallies))
	for _, entry := range tallies {
		candidates = append(candidates, *entry)
	}
	return candidates, nil
}

// effectiveStudyIdentityLocked applies the learner's decision to one occurrence:
// an exclusion removes it and a correction replaces its lemma. The caller holds
// s.mu.
func (s *Store) effectiveStudyIdentityLocked(owner, bookID, language string, occurrence domain.LemmaReviewOccurrence) (domain.StudyIdentity, bool) {
	key := fixtureLemmaCorrectionKey(owner, bookID, occurrence.AnalysisRunID, occurrence.StartOffset, occurrence.EndOffset)
	if s.lemmaExclusions[key] {
		return domain.StudyIdentity{}, false
	}
	identity := domain.StudyIdentity{Language: language, CanonicalLemma: occurrence.CanonicalLemma, UPOS: strings.ToUpper(occurrence.UPOS), LexicalDiscriminator: occurrence.LexicalDiscriminator}
	if corrected := s.lemmaCorrections[key]; corrected != "" {
		identity.CanonicalLemma, identity.LexicalDiscriminator = corrected, ""
	}
	return identity, true
}

// ListCurrentStudyMaterial returns every word's current Study material.
func (s *Store) ListCurrentStudyMaterial(_ context.Context, owner, language string) ([]domain.StudyMaterial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	material := make([]domain.StudyMaterial, 0, len(s.studyMaterial))
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.Language == language {
			material = append(material, cloneStudyMaterial(item))
		}
	}
	return material, nil
}

// GetStudyMaterial returns one Study material record.
func (s *Store) GetStudyMaterial(_ context.Context, owner, id string) (domain.StudyMaterial, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.ID == id {
			return cloneStudyMaterial(item), nil
		}
	}
	return domain.StudyMaterial{}, errNotFound
}

// CountStudyMaterial reports the ready, preparing and unpreparable words.
func (s *Store) CountStudyMaterial(_ context.Context, owner, language string) (domain.StudyMaterialCounts, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var counts domain.StudyMaterialCounts
	for _, item := range s.studyMaterial {
		if item.OwnerID != owner || item.Language != language {
			continue
		}
		switch item.State {
		case domain.StudyMaterialReady:
			counts.Ready++
		case domain.StudyMaterialPending:
			counts.Preparing++
		case domain.StudyMaterialUnpreparable:
			counts.Unpreparable++
		}
	}
	return counts, nil
}

// ReserveStudyMaterial starts a word's material as pending, reporting false
// when the word already has current material.
func (s *Store) ReserveStudyMaterial(_ context.Context, owner string, identity domain.StudyIdentity) (string, bool, error) {
	identity.CanonicalLemma = strings.TrimSpace(identity.CanonicalLemma)
	identity.UPOS = strings.ToUpper(strings.TrimSpace(identity.UPOS))
	identity.LexicalDiscriminator = strings.TrimSpace(identity.LexicalDiscriminator)
	identity.Language = strings.ToLower(strings.TrimSpace(identity.Language))
	if identity.Language == "" || identity.CanonicalLemma == "" || identity.UPOS == "" {
		return "", false, domain.ErrInvalidStudyMaterial
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.StudyIdentity == identity {
			return "", false, nil
		}
	}
	id := fmt.Sprintf("fixture-study-material-%d", len(s.studyMaterial)+1)
	now := time.Now().UTC()
	s.studyMaterial = append(s.studyMaterial, domain.StudyMaterial{ID: id, OwnerID: owner, StudyIdentity: identity, State: domain.StudyMaterialPending, CreatedAt: now, UpdatedAt: now})
	return id, true, nil
}

// RecordStudyMaterialReady stores a pending word's prepared material.
func (s *Store) RecordStudyMaterialReady(_ context.Context, owner, id string, content domain.StudyMaterialReadyContent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID != owner || item.ID != id || item.State != domain.StudyMaterialPending {
			continue
		}
		item.State = domain.StudyMaterialReady
		item.BookID, item.AnalysisRunID, item.CorpusID = content.BookID, content.AnalysisRunID, content.CorpusID
		item.SentenceOrdinal, item.TokenOrdinal = content.SentenceOrdinal, content.TokenOrdinal
		item.Sentence, item.TargetWord, item.Gloss = content.Sentence, content.TargetWord, content.Gloss
		item.SentenceTranslation = content.SentenceTranslation
		item.SentenceTranslationTargets = append([]string{}, content.SentenceTranslationTargets...)
		item.UpdatedAt = time.Now().UTC()
		return nil
	}
	return errNotFound
}

// RecordStudyMaterialUnpreparable marks a pending word unpreparable.
func (s *Store) RecordStudyMaterialUnpreparable(_ context.Context, owner, id, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return domain.ErrInvalidStudyMaterial
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyMaterial {
		item := &s.studyMaterial[i]
		if item.OwnerID != owner || item.ID != id || item.State != domain.StudyMaterialPending {
			continue
		}
		item.State, item.OmissionReason, item.UpdatedAt = domain.StudyMaterialUnpreparable, strings.TrimSpace(reason), time.Now().UTC()
		return nil
	}
	return errNotFound
}

func cloneStudyMaterial(item domain.StudyMaterial) domain.StudyMaterial {
	item.SentenceTranslationTargets = append([]string{}, item.SentenceTranslationTargets...)
	return item
}

// Study is the fixture server's Study preparation seam. It reserves pending
// material for the next words in stream order and never prepares it, since the
// fixture server binds no provider or River.
type Study struct{ Store *Store }

// EnqueueStudyPreparation reserves material for the next words in stream order.
func (s Study) EnqueueStudyPreparation(ctx context.Context, owner, language string) error {
	candidates, err := s.Store.ListStudyCandidates(ctx, owner, language)
	if err != nil {
		return err
	}
	material, err := s.Store.ListCurrentStudyMaterial(ctx, owner, language)
	if err != nil {
		return err
	}
	states := make(map[domain.StudyIdentity]domain.StudyMaterialState, len(material))
	for _, item := range material {
		states[item.StudyIdentity] = item.State
	}
	for _, target := range domain.StudyPreparationTargets(domain.OrderStudyStream(candidates), states) {
		if _, _, err := s.Store.ReserveStudyMaterial(ctx, owner, target.StudyIdentity); err != nil {
			return err
		}
	}
	return nil
}
