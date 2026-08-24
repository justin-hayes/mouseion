package analysisinsights

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type memoryStore struct {
	input     domain.AnalysisCorpusVocabulary
	known     []domain.KnownVocabulary
	generated []domain.GeneratedVocabulary
	err       error
}

func (m *memoryStore) GetAnalysisCorpusVocabulary(context.Context, string, string) (domain.AnalysisCorpusVocabulary, error) {
	return m.input, m.err
}
func (m *memoryStore) ListKnownVocabulary(_ context.Context, owner, language string) ([]domain.KnownVocabulary, error) {
	var result []domain.KnownVocabulary
	for _, word := range m.known {
		if word.OwnerID == owner && word.Language == language {
			result = append(result, word)
		}
	}
	return result, nil
}
func (m *memoryStore) ListGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	var result []domain.GeneratedVocabulary
	for _, word := range m.generated {
		if word.OwnerID == owner && word.Language == language {
			result = append(result, word)
		}
	}
	return result, nil
}

func TestCoverageUsesPersistedDenominatorAndVocabularyCategories(t *testing.T) {
	currentBook, otherBook := "book-current", "book-other"
	profile := &domain.TextProfile{SentenceCount: 10, NormalizedTokenCount: 240, MedianSentenceTokenCount: 24, P90SentenceTokenCount: 40, LongSentenceCount: 2}
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 200, DistinctLemmaCount: 8, TextProfile: profile}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: currentBook, Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 80},
			{Language: "de", CanonicalLemma: "wild", UPOS: "NOUN", OccurrenceCount: 20},
			{Language: "de", CanonicalLemma: "wild", UPOS: "VERB", OccurrenceCount: 10},
			{Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", OccurrenceCount: 40},
			{Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN", OccurrenceCount: 20},
			{Language: "de", CanonicalLemma: "repeat", UPOS: "NOUN", OccurrenceCount: 20},
			{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 5},
			{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 5},
		}},
		known: []domain.KnownVocabulary{
			{OwnerID: "alice", Language: "de", CanonicalLemma: "known", UPOS: "NOUN"},
			{OwnerID: "alice", Language: "de", CanonicalLemma: "wild"},
			{OwnerID: "bob", Language: "de", CanonicalLemma: "generated", UPOS: "NOUN"},
		},
		generated: []domain.GeneratedVocabulary{
			{OwnerID: "alice", Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", FirstSourceMaterialID: &otherBook},
			{OwnerID: "alice", Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN"},
			{OwnerID: "alice", Language: "de", CanonicalLemma: "repeat", UPOS: "NOUN", FirstSourceMaterialID: &currentBook},
		},
	}

	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	if err != nil {
		t.Fatal(err)
	}
	if got.AnalyzableTokenCount != 200 || got.DistinctLemmaCount != 8 || got.KnownTokenCount != 110 || got.KnownLemmaCount != 3 || got.UnknownTokenCount != 90 || got.UnknownLemmaCount != 5 {
		t.Fatalf("coverage = %+v", got)
	}
	if got.TextProfile != profile {
		t.Fatalf("text profile = %+v, want persisted profile", got.TextProfile)
	}
	want := []domain.CoverageThreshold{
		{TargetPercent: 95, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30},
		{TargetPercent: 97, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30},
		{TargetPercent: 99, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30},
	}
	if !reflect.DeepEqual(got.Thresholds, want) {
		t.Fatalf("thresholds = %+v, want %+v", got.Thresholds, want)
	}
	wantTop := []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "repeat", UPOS: "NOUN", OccurrenceCount: 20},
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 5},
		{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 5},
	}
	if !reflect.DeepEqual(got.TopUnknownLemmas, wantTop) {
		t.Fatalf("top unknown = %+v, want %+v", got.TopUnknownLemmas, wantTop)
	}
	if got.UnknownConcentration != (domain.CoverageProjection{TopLemmaCount: 10, SelectedLemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, ProjectedTokenCount: 140}) {
		t.Fatalf("concentration = %+v", got.UnknownConcentration)
	}
	if len(got.Projections) != 3 || got.Projections[0].ProjectedTokenCount != 140 || got.Projections[1].TopLemmaCount != 25 || got.Projections[2].TopLemmaCount != 50 {
		t.Fatalf("projections = %+v", got.Projections)
	}
}

func TestCoverageThresholdsUseExactMathAndDeterministicTies(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 6}
	store := &memoryStore{input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "book", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "eins", UPOS: "NOUN", OccurrenceCount: 94},
		{Language: "de", CanonicalLemma: "zwei", UPOS: "NOUN", OccurrenceCount: 2},
		{Language: "de", CanonicalLemma: "drei", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 1},
		{Language: "de", CanonicalLemma: "gamma", UPOS: "NOUN", OccurrenceCount: 1},
	}}}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := []int64{2, 3, 5}
	wantOccurrences := []int64{96, 97, 99}
	for i := range got.Thresholds {
		if got.Thresholds[i].LemmaCount != wantCounts[i] || got.Thresholds[i].OccurrenceCount != wantOccurrences[i] {
			t.Fatalf("threshold %d = %+v", i, got.Thresholds[i])
		}
	}
	wantOrder := []string{"eins", "zwei", "alpha", "beta", "drei"}
	for i, want := range wantOrder {
		if got.TopUnknownLemmas[i].CanonicalLemma != want {
			t.Fatalf("top unknown order = %+v", got.TopUnknownLemmas)
		}
	}
}

func TestCoverageRequiresPersistedStatistics(t *testing.T) {
	_, err := NewService(&memoryStore{}).Coverage(context.Background(), "alice", "legacy")
	if !errors.Is(err, ErrStatisticsUnavailable) {
		t.Fatalf("error = %v", err)
	}
}
