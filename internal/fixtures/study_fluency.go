package fixtures

// Contract status: contractual. Fluency context and response transitions must
// match internal/storecontract scenarios shared with PostgreSQL (ADR 0088).

import (
	"context"
	"fmt"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// StudyFluencyResponse is one stored answer to a Fluency context.
type StudyFluencyResponse struct {
	ID, OwnerID, ContextID string
	Response               domain.FluencyResponse
	RespondedAt            time.Time
}

// GetStudyProgress returns one word's Study progress.
func (s *Store) GetStudyProgress(_ context.Context, owner, id string) (domain.StudyProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.studyProgress {
		if item.OwnerID == owner && item.ID == id {
			return item, nil
		}
	}
	return domain.StudyProgress{}, errNotFound
}

// ListStudyFluencyContexts returns every Fluency context of the learner's words
// in the study language, with when each was last answered.
func (s *Store) ListStudyFluencyContexts(_ context.Context, owner, language string) ([]domain.FluencyContext, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	words := map[string]bool{}
	for _, item := range s.studyProgress {
		if item.OwnerID == owner && item.Language == language {
			words[item.ID] = true
		}
	}
	var contexts []domain.FluencyContext
	for _, item := range s.studyFluencyContexts {
		if item.OwnerID != owner || !words[item.ProgressID] {
			continue
		}
		item.SentenceTranslationTargets = append([]string{}, item.SentenceTranslationTargets...)
		for _, response := range s.studyFluencyResponses {
			if response.OwnerID == owner && response.ContextID == item.ID && response.RespondedAt.After(item.LastResponseAt) {
				item.LastResponseAt = response.RespondedAt
			}
		}
		contexts = append(contexts, item)
	}
	return contexts, nil
}

// RecordStudyFluencyContext stores one assessed occurrence of a mature word.
func (s *Store) RecordStudyFluencyContext(_ context.Context, owner string, record domain.FluencyContextRecord) error {
	if record.Agreement != domain.FluencyAgrees && record.Agreement != domain.FluencyDisagrees {
		return fmt.Errorf("record study fluency context: unknown agreement %q", record.Agreement)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	known := false
	for _, item := range s.studyProgress {
		known = known || (item.OwnerID == owner && item.ID == record.ProgressID)
	}
	if !known {
		return errNotFound
	}
	if !s.fluencyContextFitsLocked(owner, record) {
		return domain.ErrFluencyContextNotStored
	}
	s.studyFluencyContexts = append(s.studyFluencyContexts, domain.FluencyContext{
		ID: fmt.Sprintf("fixture-fluency-context-%d", len(s.studyFluencyContexts)+1), OwnerID: owner, ProgressID: record.ProgressID,
		Agreement: record.Agreement,
		BookID:    record.BookID, AnalysisRunID: record.AnalysisRunID, CorpusID: record.CorpusID,
		SentenceOrdinal: record.SentenceOrdinal, TokenOrdinal: record.TokenOrdinal,
		Sentence: record.Sentence, TargetWord: record.TargetWord, Gloss: record.Gloss, SentenceTranslation: record.SentenceTranslation,
		SentenceTranslationTargets: append([]string{}, record.SentenceTranslationTargets...),
		Form:                       record.Form, CreatedAt: time.Now().UTC(),
	})
	return nil
}

// fluencyContextFitsLocked reports whether the context may be stored: the
// occurrence is not already assessed for the word, and an agreeing context
// leaves the word within its limit. The caller holds s.mu.
func (s *Store) fluencyContextFitsLocked(owner string, record domain.FluencyContextRecord) bool {
	agreeing := 0
	for _, item := range s.studyFluencyContexts {
		if item.OwnerID != owner || item.ProgressID != record.ProgressID {
			continue
		}
		if item.AnalysisRunID == record.AnalysisRunID && item.SentenceOrdinal == record.SentenceOrdinal && item.TokenOrdinal == record.TokenOrdinal {
			return false
		}
		if item.Agreement == domain.FluencyAgrees {
			agreeing++
		}
	}
	return record.Agreement != domain.FluencyAgrees || agreeing < domain.FluencyContextsPerWord
}

// RecordStudyFluencyResponse appends the learner's answer to one of their
// agreeing Fluency contexts. It writes nothing else.
func (s *Store) RecordStudyFluencyResponse(_ context.Context, owner string, record domain.FluencyResponseRecord) error {
	if _, ok := domain.ParseFluencyResponse(string(record.Response)); !ok {
		return fmt.Errorf("record study fluency response: unknown response %q", record.Response)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.studyFluencyContexts {
		if item.OwnerID == owner && item.ID == record.ContextID && item.Agreement == domain.FluencyAgrees {
			s.studyFluencyResponses = append(s.studyFluencyResponses, StudyFluencyResponse{
				ID: fmt.Sprintf("fixture-fluency-response-%d", len(s.studyFluencyResponses)+1), OwnerID: owner,
				ContextID: record.ContextID, Response: record.Response, RespondedAt: record.RespondedAt.UTC(),
			})
			return nil
		}
	}
	return errNotFound
}

// ListStudyFluencyResponses returns a context's answers, oldest first.
func (s *Store) ListStudyFluencyResponses(_ context.Context, owner, contextID string) ([]StudyFluencyResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var responses []StudyFluencyResponse
	for _, item := range s.studyFluencyResponses {
		if item.OwnerID == owner && item.ContextID == contextID {
			responses = append(responses, item)
		}
	}
	return responses, nil
}

// SeedMatureStudyWord puts a word in study as already mature, with its Study
// material, so a browser fixture can show Fluency without weeks of reviews. The
// word is on a 30-day schedule, due long after now.
func (s *Store) SeedMatureStudyWord(owner string, identity domain.StudyIdentity, content domain.StudyMaterialReadyContent, now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	materialID := fmt.Sprintf("fixture-study-material-%d", len(s.studyMaterial)+1)
	s.studyMaterial = append(s.studyMaterial, domain.StudyMaterial{
		ID: materialID, OwnerID: owner, StudyIdentity: identity, State: domain.StudyMaterialReady,
		BookID: content.BookID, AnalysisRunID: content.AnalysisRunID, CorpusID: content.CorpusID,
		SentenceOrdinal: content.SentenceOrdinal, TokenOrdinal: content.TokenOrdinal,
		Sentence: content.Sentence, TargetWord: content.TargetWord, Gloss: content.Gloss,
		SentenceTranslation: content.SentenceTranslation, SentenceTranslationTargets: content.SentenceTranslationTargets,
		CreatedAt: now, UpdatedAt: now,
	})
	id := fmt.Sprintf("fixture-study-progress-%d", len(s.studyProgress)+1)
	s.studyProgress = append(s.studyProgress, domain.StudyProgress{
		ID: id, OwnerID: owner, StudyIdentity: identity, MaterialID: materialID,
		IntroducedAt: now.Add(-40 * 24 * time.Hour), LearnedMeaning: content.Gloss,
		Memory: domain.StudyMemory{
			State: domain.StudyStateReview, Due: now.Add(30 * 24 * time.Hour), Stability: 30, Difficulty: 5,
			ScheduledDays: 30, Reps: 6, LastReview: now.Add(-24 * time.Hour),
		},
		CreatedAt: now, UpdatedAt: now,
	})
	return id
}

// SeedFluencyContext stores a ready Fluency context for a word, for a browser
// fixture.
func (s *Store) SeedFluencyContext(owner, progressID string, record domain.FluencyContextRecord) error {
	record.ProgressID = progressID
	return s.RecordStudyFluencyContext(context.Background(), owner, record)
}
