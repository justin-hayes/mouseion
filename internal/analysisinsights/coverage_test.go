package analysisinsights

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	input    domain.AnalysisCorpusVocabulary
	notReady bool
	progress []domain.StudyProgress
	err      error
}

func (m *memoryStore) GetProjectedCorpusVocabulary(context.Context, string, string) (domain.ProjectedCorpusVocabulary, error) {
	return domain.ProjectedCorpusVocabulary{AnalysisCorpusVocabulary: m.input, Ready: !m.notReady}, m.err
}

func (m *memoryStore) ListStudyProgress(_ context.Context, owner, language string) ([]domain.StudyProgress, error) {
	var result []domain.StudyProgress
	for _, p := range m.progress {
		if p.OwnerID == owner && p.Language == language {
			result = append(result, p)
		}
	}
	return result, nil
}

// matureWord is a word whose scheduled interval has reached maturity.
func matureWord(owner, language, lemma, upos string) domain.StudyProgress {
	return domain.StudyProgress{OwnerID: owner, StudyItem: domain.WordItem(language, lemma, upos, ""),
		Memory: domain.StudyMemory{State: domain.StudyStateReview, ScheduledDays: domain.FluencyMaturityDays}}
}

// learningWord is a word in study that is not yet Mature.
func learningWord(owner, language, lemma, upos string) domain.StudyProgress {
	return domain.StudyProgress{OwnerID: owner, StudyItem: domain.WordItem(language, lemma, upos, ""),
		Memory: domain.StudyMemory{State: domain.StudyStateLearning}}
}

func TestCoverageCountsMatureAgainstInStudyWords(t *testing.T) {
	profile := &domain.TextProfile{SentenceCount: 10, NormalizedTokenCount: 240, MedianSentenceTokenCount: 24, P90SentenceTokenCount: 40, LongSentenceCount: 2}
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 200, DistinctLemmaCount: 6, TextProfile: profile}
	withdrawn := matureWord("alice", "de", "withdrawn", "NOUN")
	withdrawn.Withdrawn = true
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "book", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "mature", UPOS: "NOUN", OccurrenceCount: 80},
			{Language: "de", CanonicalLemma: "learning", UPOS: "NOUN", OccurrenceCount: 40},
			{Language: "de", CanonicalLemma: "withdrawn", UPOS: "NOUN", OccurrenceCount: 20},
			{Language: "de", CanonicalLemma: "other-owner", UPOS: "NOUN", OccurrenceCount: 20},
			{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 5},
			{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 5},
		}},
		progress: []domain.StudyProgress{
			matureWord("alice", "de", "mature", "NOUN"),
			learningWord("alice", "de", "learning", "NOUN"),
			withdrawn,
			matureWord("bob", "de", "other-owner", "NOUN"),
			matureWord("alice", "it", "alpha", "NOUN"),
		},
	}

	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(200), got.AnalyzableTokenCount)
	assert.Equal(t, int64(80), got.MatureTokenCount)
	assert.Equal(t, int64(1), got.MatureLemmaCount)
	assert.Equal(t, int64(120), got.InStudyTokenCount, "in study also counts the Mature words")
	assert.Equal(t, int64(2), got.InStudyLemmaCount)
	assert.Equal(t, int64(120), got.UnknownTokenCount)
	assert.Equal(t, int64(5), got.UnknownLemmaCount)
	assert.Same(t, profile, got.TextProfile)
	// Words already in study leave the eligible pool; a withdrawn word, another
	// owner's progress and another language's progress do not.
	wantTop := []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "other-owner", UPOS: "NOUN", OccurrenceCount: 20},
		{Language: "de", CanonicalLemma: "withdrawn", UPOS: "NOUN", OccurrenceCount: 20},
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 5},
		{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 5},
	}
	assert.Equal(t, wantTop, got.TopUnknownLemmas)
	assert.Equal(t, int64(50), got.Thresholds[0].EligibleTokenCount)
	assert.Equal(t, domain.CoverageProjection{TopLemmaCount: 10, SelectedLemmaCount: 4, OccurrenceCount: 50, EligibleTokenCount: 50, ProjectedTokenCount: 130}, got.UnknownConcentration)
	require.Len(t, got.Projections, 3)
	assert.Equal(t, int64(25), got.Projections[1].TopLemmaCount)
}

func TestCoverageIsNotMatureUntilTheIntervalReachesMaturity(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10, DistinctLemmaCount: 2}
	young := matureWord("alice", "de", "jung", "NOUN")
	young.Memory.ScheduledDays = domain.FluencyMaturityDays - 1
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "jung", UPOS: "NOUN", OccurrenceCount: 6},
			{Language: "de", CanonicalLemma: "alt", UPOS: "NOUN", OccurrenceCount: 4},
		}},
		progress: []domain.StudyProgress{young, matureWord("alice", "de", "alt", "NOUN")},
	}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(4), got.MatureTokenCount)
	assert.Equal(t, int64(10), got.InStudyTokenCount)
}

func TestCoverageMatchesTheFullEffectiveIdentity(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10, DistinctLemmaCount: 3}
	disambiguated := matureWord("alice", "de", "Bank", "NOUN")
	disambiguated.LexicalDiscriminator = "furniture"
	idiom := matureWord("alice", "de", "ins Gras beißen", "")
	idiom.StudyItem = domain.IdiomItem("de", "Bank")
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "Bank", UPOS: "NOUN", LexicalDiscriminator: "furniture", OccurrenceCount: 3},
			{Language: "de", CanonicalLemma: "Bank", UPOS: "NOUN", LexicalDiscriminator: "finance", OccurrenceCount: 2},
			{Language: "de", CanonicalLemma: "Bank", UPOS: "NOUN", OccurrenceCount: 5},
		}},
		progress: []domain.StudyProgress{disambiguated, idiom},
	}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(3), got.MatureTokenCount, "an Idiom with the same text never changes word-token coverage")
	assert.Equal(t, int64(3), got.InStudyTokenCount)
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

func TestCoverageThresholdUsesWholeBookDenominatorRatherThanStudyPool(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 4}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "book", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "mature", UPOS: "NOUN", OccurrenceCount: 90},
			{Language: "de", CanonicalLemma: "one", UPOS: "NOUN", OccurrenceCount: 5},
			{Language: "de", CanonicalLemma: "two", UPOS: "VERB", OccurrenceCount: 3},
			{Language: "de", CanonicalLemma: "three", UPOS: "ADJ", OccurrenceCount: 2},
		}},
		progress: []domain.StudyProgress{matureWord("alice", "de", "mature", "NOUN")},
	}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	wantThresholds := []struct{ lemmaCount, occurrenceCount int64 }{{1, 5}, {2, 8}, {3, 10}}
	require.Len(t, got.Thresholds, len(wantThresholds))
	for i, want := range wantThresholds {
		threshold := got.Thresholds[i]
		assert.True(t, threshold.Reachable, "%d%% threshold = %+v", threshold.TargetPercent, threshold)
		assert.Equal(t, want.lemmaCount, threshold.LemmaCount, "%d%% threshold = %+v", threshold.TargetPercent, threshold)
		assert.Equal(t, want.occurrenceCount, threshold.OccurrenceCount, "%d%% threshold = %+v", threshold.TargetPercent, threshold)
	}
}

func TestCoverageClampsMatureOverflowAndProjectedCoverage(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10, DistinctLemmaCount: 2}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{SourceMaterialID: "book", Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "mature", UPOS: "NOUN", OccurrenceCount: 12},
			{Language: "de", CanonicalLemma: "unknown", UPOS: "VERB", OccurrenceCount: 4},
		}},
		progress: []domain.StudyProgress{matureWord("alice", "de", "mature", "NOUN")},
	}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(10), got.MatureTokenCount)
	assert.Equal(t, int64(10), got.InStudyTokenCount)
	assert.Zero(t, got.UnknownTokenCount)
	for _, projection := range got.Projections {
		assert.LessOrEqual(t, projection.ProjectedTokenCount, got.AnalyzableTokenCount, "projection exceeds denominator: %+v", projection)
	}
}

func TestCoverageRequiresPersistedStatistics(t *testing.T) {
	_, err := NewService(&memoryStore{}).Coverage(context.Background(), "alice", "legacy")
	assert.ErrorIs(t, err, ErrStatisticsUnavailable, "error = %v", err)
}

func TestCoverageKeepsExcludedOccurrencesInTheDenominator(t *testing.T) {
	// Projected counts omit excluded occurrences; the persisted analyzable
	// total still includes them, so they stay unmastered rather than vanishing.
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10, DistinctLemmaCount: 2}
	store := &memoryStore{input: domain.AnalysisCorpusVocabulary{Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "Straße", UPOS: "NOUN", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 4},
	}}, progress: []domain.StudyProgress{matureWord("alice", "de", "Haus", "NOUN")}}
	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Equal(t, int64(10), got.AnalyzableTokenCount)
	assert.Equal(t, int64(4), got.MatureTokenCount)
	assert.Equal(t, int64(6), got.UnknownTokenCount)
}

func TestCoverageIsUpdatingWhileCountsAreNotReady(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 10}
	store := &memoryStore{notReady: true, input: domain.AnalysisCorpusVocabulary{Statistics: statistics}}
	_, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	assert.ErrorIs(t, err, ErrCountsUpdating)
}

func TestCoverageCountsWordsLeftBeforeTargetCoverage(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 5}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "mature", UPOS: "NOUN", OccurrenceCount: 50},
			{Language: "de", CanonicalLemma: "learning", UPOS: "NOUN", OccurrenceCount: 40},
			{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 6},
			{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 3},
			{Language: "de", CanonicalLemma: "gamma", UPOS: "NOUN", OccurrenceCount: 1},
		}},
		progress: []domain.StudyProgress{
			matureWord("alice", "de", "mature", "NOUN"),
			learningWord("alice", "de", "learning", "NOUN"),
		},
	}

	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	// 90 of 100 tokens are in study; Target coverage is 98%, so alpha (6) is
	// the first word counted and beta (3) the second, reaching 99.
	assert.Equal(t, int(domain.StudyTargetCoveragePercent), got.ToTarget.TargetPercent)
	assert.Equal(t, int64(2), got.ToTarget.LemmaCount)
	assert.Equal(t, int64(9), got.ToTarget.OccurrenceCount)
	assert.True(t, got.ToTarget.Reachable)
}

func TestCoverageAtTargetNeedsNoMoreWords(t *testing.T) {
	statistics := &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 2}
	store := &memoryStore{
		input: domain.AnalysisCorpusVocabulary{Statistics: statistics, Lemmas: []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "mature", UPOS: "NOUN", OccurrenceCount: 98},
			{Language: "de", CanonicalLemma: "rare", UPOS: "NOUN", OccurrenceCount: 2},
		}},
		progress: []domain.StudyProgress{matureWord("alice", "de", "mature", "NOUN")},
	}

	got, err := NewService(store).Coverage(context.Background(), "alice", "corpus")
	require.NoError(t, err)
	assert.Zero(t, got.ToTarget.LemmaCount)
	assert.True(t, got.ToTarget.Reachable)
}
