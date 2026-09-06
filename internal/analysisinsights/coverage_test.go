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
	active    []domain.CampaignVocabulary
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
func (m *memoryStore) ListLegacyGeneratedVocabulary(_ context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	var result []domain.GeneratedVocabulary
	for _, word := range m.generated {
		if word.OwnerID == owner && word.Language == language {
			result = append(result, word)
		}
	}
	return result, nil
}
func (m *memoryStore) ListActiveLearningCampaignVocabulary(_ context.Context, owner, language string) ([]domain.CampaignVocabulary, error) {
	var result []domain.CampaignVocabulary
	for _, word := range m.active {
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
		{TargetPercent: 95, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, Reachable: false},
		{TargetPercent: 97, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, Reachable: false},
		{TargetPercent: 99, LemmaCount: 3, OccurrenceCount: 30, EligibleTokenCount: 30, Reachable: false},
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

func TestCoverageUsesResultingCorpus(t *testing.T) {
	known := []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "bekannt", UPOS: "NOUN"}}
	full := &memoryStore{known: known, input: domain.AnalysisCorpusVocabulary{
		Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 2},
		Lemmas:     []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "bekannt", UPOS: "NOUN", OccurrenceCount: 50}, {Language: "de", CanonicalLemma: "anhang", UPOS: "NOUN", OccurrenceCount: 50}},
	}}
	fullCoverage, err := NewService(full).Coverage(context.Background(), "alice", "full")
	if err != nil {
		t.Fatal(err)
	}
	if fullCoverage.KnownTokenCount != 50 || fullCoverage.AnalyzableTokenCount != 100 {
		t.Fatalf("full=%+v", fullCoverage)
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
		if got.Thresholds[i].LemmaCount != wantCounts[i] || got.Thresholds[i].OccurrenceCount != wantOccurrences[i] || !got.Thresholds[i].Reachable {
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
	if err != nil {
		t.Fatal(err)
	}
	if got.KnownTokenCount != 2 || got.UnknownTokenCount != 4 || got.KnownLemmaCount != 1 || got.UnknownLemmaCount != 3 {
		t.Fatalf("Italian coverage = %+v", got)
	}
	if !got.Thresholds[1].Reachable || got.Thresholds[1].OccurrenceCount != 4 || got.Thresholds[1].LemmaCount != 3 {
		t.Fatalf("Italian 97%% threshold = %+v", got.Thresholds[1])
	}
}

func TestCoverageSeparatesActiveCampaignProjectionAndReleasesAbandonedVocabulary(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 3}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "future", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 50},
			{Language: "de", CanonicalLemma: "active", UPOS: "VERB", OccurrenceCount: 30},
			{Language: "de", CanonicalLemma: "released", UPOS: "ADJ", OccurrenceCount: 20},
		}},
		known:  []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
		active: []domain.CampaignVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "active", UPOS: "VERB"}},
	}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	if err != nil {
		t.Fatal(err)
	}
	if got.KnownTokenCount != 50 || got.KnownLemmaCount != 1 || got.ActiveCampaignTokenCount != 30 || got.ActiveCampaignLemmaCount != 1 || got.UnknownTokenCount != 50 || got.UnknownLemmaCount != 2 {
		t.Fatalf("campaign coverage = %+v", got)
	}
	if len(got.TopUnknownLemmas) != 1 || got.TopUnknownLemmas[0].CanonicalLemma == "active" {
		t.Fatalf("unfinished active vocabulary was treated as current or deck-eligible: %+v", got.TopUnknownLemmas)
	}
	if len(got.TopUnknownLemmas) != 1 || got.TopUnknownLemmas[0].CanonicalLemma != "released" {
		t.Fatalf("deck-eligible vocabulary = %+v", got.TopUnknownLemmas)
	}
	store.active = nil // abandonment releases the reservation without persisting mastery
	got, err = NewService(store).Coverage(context.Background(), "alice", "corpus")
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveCampaignTokenCount != 0 || len(got.TopUnknownLemmas) != 2 || got.TopUnknownLemmas[0].CanonicalLemma != "active" {
		t.Fatalf("coverage after abandonment = %+v", got)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	threshold97 := got.Thresholds[1]
	if !threshold97.Reachable || threshold97.LemmaCount != 2 || threshold97.OccurrenceCount != 8 {
		t.Fatalf("97%% whole-book threshold = %+v, want two lemmas (the deck pool's 97%% prefix would require all three)", threshold97)
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
	if err != nil {
		t.Fatal(err)
	}
	if got.KnownTokenCount != 10 || got.UnknownTokenCount != 0 {
		t.Fatalf("coverage counts = known %d unknown %d", got.KnownTokenCount, got.UnknownTokenCount)
	}
	for _, projection := range got.Projections {
		if projection.ProjectedTokenCount <= got.AnalyzableTokenCount {
			continue
		}
		t.Fatalf("projection exceeds denominator: %+v", projection)
	}
}

func TestCoverageRequiresPersistedStatistics(t *testing.T) {
	_, err := NewService(&memoryStore{}).Coverage(context.Background(), "alice", "legacy")
	if !errors.Is(err, ErrStatisticsUnavailable) {
		t.Fatalf("error = %v", err)
	}
}

func TestCoverageExcludesStaleNonLexicalLemmaRows(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10, DistinctLemmaCount: 3}
	store := &memoryStore{input: domain.AnalysisCorpusVocabulary{Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "5", UPOS: "NOUN", OccurrenceCount: 6},
		{Language: "de", CanonicalLemma: "Straße", UPOS: "NOUN", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "B2", UPOS: "NOUN", OccurrenceCount: 1},
	}}}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	if err != nil {
		t.Fatal(err)
	}
	if got.AnalyzableTokenCount != 4 || got.DistinctLemmaCount != 2 || got.UnknownTokenCount != 4 || len(got.TopUnknownLemmas) != 2 || got.TopUnknownLemmas[0].CanonicalLemma != "Straße" {
		t.Fatalf("coverage = %+v", got)
	}
}
