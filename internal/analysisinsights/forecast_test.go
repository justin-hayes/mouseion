package analysisinsights

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type forecastSnapshotStore struct {
	*routeStore
	snapshot []domain.SelectionCandidate
}

func (s *forecastSnapshotStore) ListPrimaryGoalSnapshotVocabulary(context.Context, string, string) ([]domain.SelectionCandidate, error) {
	return s.snapshot, nil
}

func forecastBook(id, language, corpus string) domain.MyBook {
	return domain.MyBook{
		Book:     domain.Book{ID: id, OwnerID: "alice"},
		Acquired: analyzedRouteSummary(id, language, corpus),
	}
}

func forecastInput(source, corpus, language string, count int64, lemmas ...domain.LemmaOccurrence) domain.AnalysisCorpusVocabulary {
	return domain.AnalysisCorpusVocabulary{
		CorpusID: corpus, SourceMaterialID: source,
		Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: count, DistinctLemmaCount: int64(len(lemmas))},
		Lemmas:     lemmas,
	}
}

func TestJourneyForecastUsesExactSequentialIdentityUnion(t *testing.T) {
	store := &routeStore{
		journey: domain.ReadingJourney{OwnerID: "alice", Entries: []domain.ReadingJourneyEntry{{BookID: "first", Position: 1}, {BookID: "second", Position: 2}}},
		books:   []domain.MyBook{forecastBook("first", "de", "c-first"), forecastBook("second", "de", "c-second")},
		corpora: map[string]domain.AnalysisCorpusVocabulary{
			"c-first": forecastInput("first", "c-first", "de", 10,
				domain.LemmaOccurrence{Language: "de", CanonicalLemma: "wild", UPOS: "NOUN", OccurrenceCount: 3},
				domain.LemmaOccurrence{Language: "de", CanonicalLemma: "wild", UPOS: "VERB", OccurrenceCount: 2},
				domain.LemmaOccurrence{Language: "de", CanonicalLemma: "new", UPOS: "NOUN", OccurrenceCount: 4},
				domain.LemmaOccurrence{Language: "de", CanonicalLemma: "overlap", UPOS: "NOUN", OccurrenceCount: 1}),
			"c-second": forecastInput("second", "c-second", "de", 10,
				domain.LemmaOccurrence{Language: "de", CanonicalLemma: "new", UPOS: "NOUN", OccurrenceCount: 3},
				domain.LemmaOccurrence{Language: "de", CanonicalLemma: "overlap", UPOS: "NOUN", OccurrenceCount: 5},
				domain.LemmaOccurrence{Language: "de", CanonicalLemma: "next", UPOS: "VERB", OccurrenceCount: 2}),
		},
		known: []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "wild"}},
	}

	result, err := NewService(store).JourneyForecast(context.Background(), "alice", "de")
	require.NoError(t, err)
	require.Len(t, result.Entries, 2)
	first, second := result.Entries[0], result.Entries[1]
	assert.Equal(t, int64(5), first.Current.KnownTokenCount, "known wildcard must match every UPOS")
	assert.Equal(t, int64(5), first.OnArrival.KnownTokenCount)
	assert.Equal(t, int64(0), second.Current.KnownTokenCount)
	assert.Equal(t, int64(3), second.OnArrival.KnownTokenCount, "the first book contributes only the floor-eligible new identity")
	assert.Equal(t, first.Current, first.AfterGoal, "without a Goal, after-Goal is the current stage")
	assert.Equal(t, second.Current, second.AfterGoal, "without a Goal, after-Goal is the current stage")
}

func TestJourneyForecastAppliesGoalSnapshotAndLowerBounds(t *testing.T) {
	base := &routeStore{
		journey: domain.ReadingJourney{OwnerID: "alice", Entries: []domain.ReadingJourneyEntry{
			{BookID: "goal", Position: 1}, {BookID: "first", Position: 2}, {BookID: "missing", Position: 3}, {BookID: "last", Position: 4},
		}},
		goal:  domain.PrimaryGoal{OwnerID: "alice", Language: "de", BookID: "goal", SnapshotID: "snapshot"},
		books: []domain.MyBook{forecastBook("goal", "de", "c-goal"), forecastBook("first", "de", "c-first"), forecastBook("last", "de", "c-last")},
		corpora: map[string]domain.AnalysisCorpusVocabulary{
			"c-goal":  forecastInput("goal", "c-goal", "de", 10, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 2}, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "goal-word", UPOS: "VERB", OccurrenceCount: 3}, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "wild", UPOS: "NOUN", OccurrenceCount: 5}),
			"c-first": forecastInput("first", "c-first", "de", 10, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "goal-word", UPOS: "VERB", OccurrenceCount: 4}, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "first-word", UPOS: "NOUN", OccurrenceCount: 4}, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "below-floor", UPOS: "NOUN", OccurrenceCount: 2}),
			"c-last":  forecastInput("last", "c-last", "de", 8, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "goal-word", UPOS: "VERB", OccurrenceCount: 1}, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "first-word", UPOS: "NOUN", OccurrenceCount: 3}, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "last-word", UPOS: "NOUN", OccurrenceCount: 4}),
		},
		known: []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
	}
	store := &forecastSnapshotStore{routeStore: base, snapshot: []domain.SelectionCandidate{{OwnerID: "alice", Language: "de", CanonicalLemma: "goal-word", UPOS: "VERB"}}}

	result, err := NewService(store).JourneyForecast(context.Background(), "alice", "de")
	require.NoError(t, err)
	require.Len(t, result.Entries, 4)
	goal, first, missing, last := result.Entries[0], result.Entries[1], result.Entries[2], result.Entries[3]
	assert.Equal(t, int64(2), goal.Current.KnownTokenCount)
	assert.Equal(t, int64(5), goal.AfterGoal.KnownTokenCount)
	assert.Equal(t, int64(2), goal.OnArrival.KnownTokenCount)
	assert.Equal(t, int64(4), first.OnArrival.KnownTokenCount, "the frozen Goal identity is accepted before the next book")
	assert.Equal(t, int64(0), first.Current.KnownTokenCount)
	assert.Equal(t, int64(4), first.AfterGoal.KnownTokenCount)
	assert.NotEmpty(t, missing.UnavailableReason)
	assert.Nil(t, missing.Current)
	assert.True(t, last.LowerBound)
	assert.Equal(t, int64(4), last.OnArrival.KnownTokenCount, "Goal plus the first book are unioned exactly")
}

func TestJourneyForecastKeepsLanguageIntegrityAndEmptyGoalSnapshot(t *testing.T) {
	store := &forecastSnapshotStore{routeStore: &routeStore{
		journey: domain.ReadingJourney{OwnerID: "alice", Entries: []domain.ReadingJourneyEntry{{BookID: "book", Position: 1}}},
		goal:    domain.PrimaryGoal{OwnerID: "alice", Language: "de", BookID: "book", SnapshotID: "empty"},
		books:   []domain.MyBook{forecastBook("book", "de", "corpus")},
		corpora: map[string]domain.AnalysisCorpusVocabulary{"corpus": forecastInput("book", "corpus", "de", 4, domain.LemmaOccurrence{Language: "it", CanonicalLemma: "ciao", UPOS: "NOUN", OccurrenceCount: 4})},
	}, snapshot: nil}
	result, err := NewService(store).JourneyForecast(context.Background(), "alice", "de")
	require.NoError(t, err)
	require.Len(t, result.Entries, 1)
	assert.Contains(t, result.Entries[0].UnavailableReason, "Journey language")
	assert.Nil(t, result.Entries[0].Current)

	store.corpora["corpus"] = forecastInput("book", "corpus", "de", 4, domain.LemmaOccurrence{Language: "de", CanonicalLemma: "word", UPOS: "NOUN", OccurrenceCount: 4})
	result, err = NewService(store).JourneyForecast(context.Background(), "alice", "de")
	require.NoError(t, err)
	assert.Equal(t, result.Entries[0].Current, result.Entries[0].AfterGoal, "an empty snapshot adds no transition")
}
