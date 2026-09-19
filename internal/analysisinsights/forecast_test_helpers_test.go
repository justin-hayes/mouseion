package analysisinsights

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type routeStore struct {
	journey         domain.ReadingJourney
	journeyLanguage string
	goal            domain.PrimaryGoal
	books           []domain.MyBook
	corpora         map[string]domain.AnalysisCorpusVocabulary
	known           []domain.KnownVocabulary
	reserved        []domain.DeckPreparationVocabulary
}

func (s *routeStore) GetReadingJourney(_ context.Context, _, language string) (domain.ReadingJourney, error) {
	s.journeyLanguage = language
	return s.journey, nil
}

func (s *routeStore) GetPrimaryGoal(context.Context, string, string) (domain.PrimaryGoal, error) {
	return s.goal, nil
}

func (s *routeStore) ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error) {
	return s.books, nil
}

func (s *routeStore) GetAnalysisCorpusVocabulary(_ context.Context, _ string, id string) (domain.AnalysisCorpusVocabulary, error) {
	return s.corpora[id], nil
}

func (s *routeStore) ListKnownVocabulary(_ context.Context, owner, language string) ([]domain.KnownVocabulary, error) {
	var out []domain.KnownVocabulary
	for _, vocabulary := range s.known {
		if vocabulary.OwnerID == owner && vocabulary.Language == language {
			out = append(out, vocabulary)
		}
	}
	return out, nil
}

func (s *routeStore) ListReservedVocabulary(_ context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	var out []domain.DeckPreparationVocabulary
	for _, vocabulary := range s.reserved {
		if vocabulary.OwnerID == owner && vocabulary.Language == language {
			out = append(out, vocabulary)
		}
	}
	return out, nil
}

func (s *routeStore) ListUnattachedGeneratedVocabulary(context.Context, string, string) ([]domain.GeneratedVocabulary, error) {
	return nil, nil
}

func analyzedRouteSummary(id, language, corpus string) *domain.SourceMaterialSummary {
	return &domain.SourceMaterialSummary{
		Source:         domain.SourceMaterial{ID: id, Language: language, ContentRevisionID: "revision-" + id, ContentSnapshotID: "snapshot-" + id},
		AnalysisStatus: "analyzed",
		AnalysisState:  "completed",
		AnalysisRunID:  "run-" + id,
		CorpusID:       corpus,
	}
}
