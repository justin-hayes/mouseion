package analysisinsights

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	input     domain.AnalysisCorpusVocabulary
	known     []domain.KnownVocabulary
	generated []domain.GeneratedVocabulary
	reserved  []domain.DeckPreparationVocabulary
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
func (m *memoryStore) ListUnattachedGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	var result []domain.GeneratedVocabulary
	for _, word := range m.generated {
		if word.OwnerID == owner && word.Language == language {
			result = append(result, word)
		}
	}
	return result, nil
}
func (m *memoryStore) ListReservedVocabulary(_ context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	var result []domain.DeckPreparationVocabulary
	for _, word := range m.reserved {
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
	require.NoError(t, err)
	assert.Equal(t, int64(200), got.AnalyzableTokenCount)
	assert.Equal(t, int64(8), got.DistinctLemmaCount)
	assert.Equal(t, int64(110), got.KnownTokenCount)
	assert.Equal(t, int64(3), got.KnownLemmaCount)
	assert.Equal(t, int64(90), got.UnknownTokenCount)
	assert.Equal(t, int64(5), got.UnknownLemmaCount)
	assert.Same(t, profile, got.TextProfile, "text profile = %+v, want persisted profile", got.TextProfile)
	want := []domain.CoverageThreshold{
		{TargetPercent: 95, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, Reachable: false},
		{TargetPercent: 97, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, Reachable: false},
		{TargetPercent: 99, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, Reachable: false},
	}
	assert.Equal(t, want, got.Thresholds, "thresholds = %+v, want %+v", got.Thresholds, want)
	wantTop := []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "repeat", UPOS: "NOUN", OccurrenceCount: 20},
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 5},
		{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 5},
	}
	assert.Equal(t, wantTop, got.TopUnknownLemmas, "top unknown = %+v, want %+v", got.TopUnknownLemmas, wantTop)
	assert.Equal(t, domain.CoverageProjection{TopLemmaCount: 10, SelectedLemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, ProjectedTokenCount: 140}, got.UnknownConcentration, "concentration = %+v", got.UnknownConcentration)
	require.Len(t, got.Projections, 3)
	assert.Equal(t, int64(140), got.Projections[0].ProjectedTokenCount)
	assert.Equal(t, int64(25), got.Projections[1].TopLemmaCount)
	assert.Equal(t, int64(50), got.Projections[2].TopLemmaCount)
}

func TestCoverageUsesResultingCorpus(t *testing.T) {
	known := []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "bekannt", UPOS: "NOUN"}}
	full := &memoryStore{known: known, input: domain.AnalysisCorpusVocabulary{
		Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 2},
		Lemmas:     []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "bekannt", UPOS: "NOUN", OccurrenceCount: 50}, {Language: "de", CanonicalLemma: "anhang", UPOS: "NOUN", OccurrenceCount: 50}},
	}}
	fullCoverage, err := NewService(full).Coverage(context.Background(), "alice", "full")
	require.NoError(t, err)
	assert.Equal(t, int64(50), fullCoverage.KnownTokenCount)
	assert.Equal(t, int64(100), fullCoverage.AnalyzableTokenCount)
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
	require.NoError(t, err)
	wantCounts := []int64{2, 3, 5}
	wantOccurrences := []int64{96, 97, 99}
	require.Len(t, got.Thresholds, len(wantCounts))
	for i := range wantCounts {
		assert.Equal(t, wantCounts[i], got.Thresholds[i].LemmaCount, "threshold %d = %+v", i, got.Thresholds[i])
		assert.Equal(t, wantOccurrences[i], got.Thresholds[i].OccurrenceCount, "threshold %d = %+v", i, got.Thresholds[i])
		assert.True(t, got.Thresholds[i].Reachable, "threshold %d = %+v", i, got.Thresholds[i])
	}
	wantOrder := []string{"eins", "zwei", "alpha", "beta", "drei"}
	require.Len(t, got.TopUnknownLemmas, len(wantOrder))
	for i, want := range wantOrder {
		assert.Equal(t, want, got.TopUnknownLemmas[i].CanonicalLemma, "top unknown order = %+v", got.TopUnknownLemmas)
	}
}

func TestItalianCoverageUsesAggregatedLemmaOccurrences(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 6, DistinctLemmaCount: 4}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "libro", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "it", CanonicalLemma: "bere", UPOS: "VERB", OccurrenceCount: 2},
			{Language: "it", CanonicalLemma: "uomo", UPOS: "NOUN", OccurrenceCount: 2},
			{Language: "it", CanonicalLemma: "acqua", UPOS: "NOUN", OccurrenceCount: 1},
			{Language: "it", CanonicalLemma: "dare", UPOS: "VERB", OccurrenceCount: 1},
		}},
		known: []domain.KnownVocabulary{{OwnerID: "alice", Language: "it", CanonicalLemma: "uomo", UPOS: "NOUN"}},
	}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus-it")
	require.NoError(t, err)
	assert.Equal(t, int64(2), got.KnownTokenCount)
	assert.Equal(t, int64(4), got.UnknownTokenCount)
	assert.Equal(t, int64(1), got.KnownLemmaCount)
	assert.Equal(t, int64(3), got.UnknownLemmaCount)
	require.Len(t, got.Thresholds, 3)
	assert.True(t, got.Thresholds[1].Reachable)
	assert.Equal(t, int64(4), got.Thresholds[1].OccurrenceCount)
	assert.Equal(t, int64(3), got.Thresholds[1].LemmaCount)
}

func TestCoverageSeparatesReservedProjectionAndReleasesReservation(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 3}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "future", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 50},
			{Language: "de", CanonicalLemma: "reserved", UPOS: "VERB", OccurrenceCount: 30},
			{Language: "de", CanonicalLemma: "released", UPOS: "ADJ", OccurrenceCount: 20},
		}},
		known:    []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
		reserved: []domain.DeckPreparationVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "reserved", UPOS: "VERB"}},
	}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(50), got.KnownTokenCount)
	assert.Equal(t, int64(1), got.KnownLemmaCount)
	assert.Equal(t, int64(30), got.ReservedTokenCount)
	assert.Equal(t, int64(1), got.ReservedLemmaCount)
	assert.Equal(t, int64(50), got.UnknownTokenCount)
	assert.Equal(t, int64(2), got.UnknownLemmaCount)
	require.Len(t, got.TopUnknownLemmas, 1)
	assert.NotEqual(t, "reserved", got.TopUnknownLemmas[0].CanonicalLemma, "unfinished reserved vocabulary was treated as current or deck-eligible: %+v", got.TopUnknownLemmas)
	require.Len(t, got.TopUnknownLemmas, 1)
	assert.Equal(t, "released", got.TopUnknownLemmas[0].CanonicalLemma, "deck-eligible vocabulary = %+v", got.TopUnknownLemmas)
	store.reserved = nil // release releases the reservation without persisting mastery
	got, err = NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Zero(t, got.ReservedTokenCount)
	require.Len(t, got.TopUnknownLemmas, 2)
	assert.Equal(t, "reserved", got.TopUnknownLemmas[0].CanonicalLemma, "coverage after release = %+v", got)
}

func TestCoverageThresholdUsesWholeBookDenominatorRatherThanDeckPool(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 4}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "book", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 90},
			{Language: "de", CanonicalLemma: "one", UPOS: "NOUN", OccurrenceCount: 5},
			{Language: "de", CanonicalLemma: "two", UPOS: "VERB", OccurrenceCount: 3},
			{Language: "de", CanonicalLemma: "three", UPOS: "ADJ", OccurrenceCount: 2},
		}},
		known: []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
	}

	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	wantThresholds := []struct {
		lemmaCount      int64
		occurrenceCount int64
	}{
		{lemmaCount: 1, occurrenceCount: 5},
		{lemmaCount: 2, occurrenceCount: 8},
		{lemmaCount: 3, occurrenceCount: 10},
	}
	require.Len(t, got.Thresholds, len(wantThresholds))
	for i, want := range wantThresholds {
		threshold := got.Thresholds[i]
		assert.True(t, threshold.Reachable, "%d%% whole-book threshold = %+v, want %d lemmas and %d occurrences", threshold.TargetPercent, threshold, want.lemmaCount, want.occurrenceCount)
		assert.Equal(t, want.lemmaCount, threshold.LemmaCount, "%d%% whole-book threshold = %+v, want %d lemmas and %d occurrences", threshold.TargetPercent, threshold, want.lemmaCount, want.occurrenceCount)
		assert.Equal(t, want.occurrenceCount, threshold.OccurrenceCount, "%d%% whole-book threshold = %+v, want %d lemmas and %d occurrences", threshold.TargetPercent, threshold, want.lemmaCount, want.occurrenceCount)
	}
}

func TestCoverageClampsKnownOverflowAndProjectedCoverage(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10, DistinctLemmaCount: 2}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "book", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 12},
			{Language: "de", CanonicalLemma: "unknown", UPOS: "VERB", OccurrenceCount: 4},
		}},
		known: []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
	}

	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(10), got.KnownTokenCount, "coverage counts = known %d unknown %d", got.KnownTokenCount, got.UnknownTokenCount)
	assert.Zero(t, got.UnknownTokenCount, "coverage counts = known %d unknown %d", got.KnownTokenCount, got.UnknownTokenCount)
	for _, projection := range got.Projections {
		assert.LessOrEqual(t, projection.ProjectedTokenCount, got.AnalyzableTokenCount, "projection exceeds denominator: %+v", projection)
	}
}

func TestCoverageRequiresPersistedStatistics(t *testing.T) {
	_, err := NewService(&memoryStore{}).Coverage(context.Background(), "alice", "legacy")
	assert.ErrorIs(t, err, ErrStatisticsUnavailable, "error = %v", err)
}

func TestCoverageExcludesStaleNonLexicalLemmaRows(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10, DistinctLemmaCount: 3}
	store := &memoryStore{input: domain.AnalysisCorpusVocabulary{Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "5", UPOS: "NOUN", OccurrenceCount: 6},
		{Language: "de", CanonicalLemma: "Straße", UPOS: "NOUN", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "B2", UPOS: "NOUN", OccurrenceCount: 1},
	}}}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(4), got.AnalyzableTokenCount)
	assert.Equal(t, int64(2), got.DistinctLemmaCount)
	assert.Equal(t, int64(4), got.UnknownTokenCount)
	require.Len(t, got.TopUnknownLemmas, 2)
	assert.Equal(t, "Straße", got.TopUnknownLemmas[0].CanonicalLemma, "coverage = %+v", got)
}
