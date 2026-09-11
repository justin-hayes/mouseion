package analysisinsights

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	for _, v := range s.known {
		if v.OwnerID == owner && v.Language == language {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *routeStore) ListReservedVocabulary(_ context.Context, owner, language string) ([]domain.DeckPreparationVocabulary, error) {
	var out []domain.DeckPreparationVocabulary
	for _, v := range s.reserved {
		if v.OwnerID == owner && v.Language == language {
			out = append(out, v)
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

func TestJourneyProjectionIsDeterministicAndKeepsIncomparableBooksInPlace(t *testing.T) {
	ctx := context.Background()
	stats := func(n int64) *domain.AnalysisStatistics {
		return &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 2}
	}
	lemma := func(book, language string, known int64) domain.AnalysisCorpusVocabulary {
		return domain.AnalysisCorpusVocabulary{CorpusID: "c-" + book, SourceMaterialID: book, Statistics: stats(100), Lemmas: []domain.LemmaOccurrence{{Language: language, CanonicalLemma: "known-" + book, UPOS: "NOUN", OccurrenceCount: known}, {Language: language, CanonicalLemma: "unknown-" + book, UPOS: "NOUN", OccurrenceCount: 100 - known}}}
	}
	books := []domain.MyBook{
		{Book: domain.Book{ID: "a", OwnerID: "alice"}, Acquired: analyzedRouteSummary("a", "de", "c-a")},
		{Book: domain.Book{ID: "b", OwnerID: "alice"}, Acquired: analyzedRouteSummary("b", "fr", "c-b")},
		{Book: domain.Book{ID: "c", OwnerID: "alice"}, Acquired: analyzedRouteSummary("c", "de", "c-c")},
		{Book: domain.Book{ID: "d", OwnerID: "alice"}, Acquired: analyzedRouteSummary("d", "de", "c-d")},
	}
	store := &routeStore{
		journey: domain.ReadingJourney{OwnerID: "alice", Entries: []domain.ReadingJourneyEntry{{BookID: "a"}, {BookID: "b"}, {BookID: "c"}, {BookID: "d"}}},
		goal:    domain.PrimaryGoal{OwnerID: "alice", Language: "de", BookID: "c"}, books: books,
		corpora:  map[string]domain.AnalysisCorpusVocabulary{"c-a": lemma("a", "de", 50), "c-b": lemma("b", "fr", 90), "c-c": lemma("c", "de", 50), "c-d": {CorpusID: "c-d", SourceMaterialID: "d", Statistics: nil}},
		known:    []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known-a", UPOS: "NOUN"}, {OwnerID: "alice", Language: "de", CanonicalLemma: "known-c", UPOS: "NOUN"}},
		reserved: []domain.DeckPreparationVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "unknown-a", UPOS: "NOUN"}},
	}
	got, err := NewService(store).JourneyProjection(ctx, "alice", "de")
	require.NoError(t, err)
	assert.Equal(t, "de", got.Language)
	assert.Equal(t, "de", store.journeyLanguage)
	assert.Equal(t, 3, got.ComparableCount)
	assert.Equal(t, 1, got.IncomparableCount)
	require.Len(t, got.LearnerOrder, 4)
	for i, want := range []string{"a", "b", "c", "d"} {
		assert.Equal(t, want, got.LearnerOrder[i].BookID, "learner order = %+v", got.LearnerOrder)
	}
	require.Len(t, got.AdvisoryOrder, 4)
	for i, want := range []string{"c", "a", "b", "d"} {
		assert.Equal(t, want, got.AdvisoryOrder[i].BookID, "advisory order = %+v", got.AdvisoryOrder)
	}
	for i := range got.AdvisoryOrder[:3] {
		assert.NotNil(t, got.AdvisoryOrder[i].Rank, "comparable rank[%d] = %+v", i, got.AdvisoryOrder)
	}
	assert.Nil(t, got.AdvisoryOrder[3].Rank, "ranks = %+v", got.AdvisoryOrder)
	assert.True(t, got.LearnerOrder[1].Comparable, "explicit-language projection excluded book = %+v", got.LearnerOrder[1])
	assert.Empty(t, got.LearnerOrder[1].IncomparableReason, "explicit-language projection excluded book = %+v", got.LearnerOrder[1])
	assert.False(t, got.LearnerOrder[3].Comparable, "legacy exclusion = %+v", got.LearnerOrder[3])
	assert.Nil(t, got.LearnerOrder[3].Rank, "legacy exclusion = %+v", got.LearnerOrder[3])
	assert.Equal(t, int64(100), got.LearnerOrder[0].ConditionalCoverage.KnownTokenCount, "conditional coverage = %+v", got.LearnerOrder[0].ConditionalCoverage)
}

func TestJourneyProjectionClassifiesMismatchedCorpusAsEvidenceIntegrityFailure(t *testing.T) {
	store := &routeStore{
		journey: domain.ReadingJourney{OwnerID: "alice", Entries: []domain.ReadingJourneyEntry{{BookID: "book"}}},
		books: []domain.MyBook{{
			Book:     domain.Book{ID: "book", OwnerID: "alice"},
			Acquired: analyzedRouteSummary("source", "de", "corpus"),
		}},
		corpora: map[string]domain.AnalysisCorpusVocabulary{
			"corpus": {
				CorpusID: "corpus", SourceMaterialID: "source",
				Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 1},
				Lemmas:     []domain.LemmaOccurrence{{Language: "it", CanonicalLemma: "ciao", UPOS: "NOUN", OccurrenceCount: 1}},
			},
		},
	}

	result, err := NewService(store).JourneyProjection(context.Background(), "alice", "de")
	require.NoError(t, err)
	require.Len(t, result.LearnerOrder, 1)
	book := result.LearnerOrder[0]
	assert.False(t, book.Comparable, "corpus integrity state = %+v", book)
	assert.Equal(t, "stale/incomplete: corpus evidence is not modeled book language", book.IncomparableReason, "corpus integrity state = %+v", book)
}

func TestJourneyProjectionDoesNotInferLanguageFromJourneyMembers(t *testing.T) {
	store := &routeStore{
		journey: domain.ReadingJourney{OwnerID: "alice", Entries: []domain.ReadingJourneyEntry{{BookID: "book"}}},
		books: []domain.MyBook{{
			Book:     domain.Book{ID: "book", OwnerID: "alice"},
			Acquired: analyzedRouteSummary("source", "de", "corpus"),
		}},
		corpora: map[string]domain.AnalysisCorpusVocabulary{
			"corpus": {
				CorpusID: "corpus", SourceMaterialID: "source",
				Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 1},
				Lemmas:     []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "hallo", UPOS: "INTJ", OccurrenceCount: 1}},
			},
		},
	}

	result, err := NewService(store).JourneyProjection(context.Background(), "alice", "")
	require.NoError(t, err)
	assert.Empty(t, result.Language, "projection inferred language %q", result.Language)
}

func TestJourneyProjectionAnchorsGoalAfterIncomparableAndRecomputesVocabulary(t *testing.T) {
	corpus := func(id, book string, known int64) domain.AnalysisCorpusVocabulary {
		return domain.AnalysisCorpusVocabulary{
			CorpusID: id, SourceMaterialID: book,
			Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: 100, DistinctLemmaCount: 2},
			Lemmas: []domain.LemmaOccurrence{
				{Language: "de", CanonicalLemma: "shared", UPOS: "NOUN", OccurrenceCount: known},
				{Language: "de", CanonicalLemma: "unknown-" + book, UPOS: "VERB", OccurrenceCount: 100 - known},
			},
		}
	}
	books := []domain.MyBook{
		{Book: domain.Book{ID: "missing", OwnerID: "owner"}},
		{Book: domain.Book{ID: "b", OwnerID: "owner"}, Acquired: analyzedRouteSummary("b", "de", "cb")},
		{Book: domain.Book{ID: "c", OwnerID: "owner"}, Acquired: analyzedRouteSummary("c", "de", "cc")},
	}
	store := &routeStore{
		journey: domain.ReadingJourney{OwnerID: "owner", Entries: []domain.ReadingJourneyEntry{{BookID: "missing"}, {BookID: "b"}, {BookID: "c"}}},
		goal:    domain.PrimaryGoal{OwnerID: "owner", Language: "de", BookID: "c"}, books: books,
		corpora: map[string]domain.AnalysisCorpusVocabulary{"cb": corpus("cb", "b", 50), "cc": corpus("cc", "c", 50)},
		known:   []domain.KnownVocabulary{{OwnerID: "owner", Language: "de", CanonicalLemma: "shared", UPOS: ""}},
	}

	result, err := NewService(store).JourneyProjection(context.Background(), "owner", "de")
	require.NoError(t, err)
	require.Len(t, result.AdvisoryOrder, 3)
	assert.Equal(t, "missing", result.AdvisoryOrder[0].BookID, "advisory order = %v", []string{result.AdvisoryOrder[0].BookID, result.AdvisoryOrder[1].BookID, result.AdvisoryOrder[2].BookID})
	assert.Equal(t, "c", result.AdvisoryOrder[1].BookID, "advisory order = %v", []string{result.AdvisoryOrder[0].BookID, result.AdvisoryOrder[1].BookID, result.AdvisoryOrder[2].BookID})
	assert.Equal(t, "b", result.AdvisoryOrder[2].BookID, "advisory order = %v", []string{result.AdvisoryOrder[0].BookID, result.AdvisoryOrder[1].BookID, result.AdvisoryOrder[2].BookID})
	assert.Nil(t, result.AdvisoryOrder[0].Rank, "explanations/ranks = %+v", result.AdvisoryOrder)
	assert.NotEmpty(t, result.AdvisoryOrder[1].PlacementReason, "explanations/ranks = %+v", result.AdvisoryOrder)

	// The same projection call observes current vocabulary changes; no route
	// projection is stored between calls.
	store.known = append(store.known, domain.KnownVocabulary{OwnerID: "owner", Language: "de", CanonicalLemma: "unknown-b", UPOS: "VERB"})
	result, err = NewService(store).JourneyProjection(context.Background(), "owner", "de")
	require.NoError(t, err)
	require.Len(t, result.AdvisoryOrder, 3)
	assert.Equal(t, "c", result.AdvisoryOrder[1].BookID, "recomputed advisory order = %+v", result.AdvisoryOrder)
	assert.Equal(t, "b", result.AdvisoryOrder[2].BookID, "recomputed advisory order = %+v", result.AdvisoryOrder)
	require.NotNil(t, result.AdvisoryOrder[2].Coverage)
	assert.Equal(t, int64(100), result.AdvisoryOrder[2].Coverage.KnownTokenCount, "recomputed coverage = %+v", result.AdvisoryOrder[2].Coverage)
}

// TestJourneyProjectionTiesDeterministicOnEqualCoverageAndCarryThresholds
// covers the "equal costs" tie-break (equal current known-token coverage falls
// back to ascending learner position, independent of DB row order) and the
// "unreachable threshold" case (per-book ADR 0025 threshold data is carried
// honestly and must never influence the ordering objective).
func TestJourneyProjectionTiesDeterministicOnEqualCoverageAndCarryThresholds(t *testing.T) {
	ctx := context.Background()
	mkCorpus := func(id, label string, analyzable, knownTokens, eligibleTokens int64) domain.AnalysisCorpusVocabulary {
		return domain.AnalysisCorpusVocabulary{
			CorpusID: id, SourceMaterialID: label,
			Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: analyzable, DistinctLemmaCount: 2},
			Lemmas: []domain.LemmaOccurrence{
				{Language: "de", CanonicalLemma: "known-" + label, UPOS: "NOUN", OccurrenceCount: knownTokens},
				{Language: "de", CanonicalLemma: "unknown-" + label, UPOS: "VERB", OccurrenceCount: eligibleTokens},
			},
		}
	}
	book := func(id string) domain.MyBook {
		return domain.MyBook{
			Book: domain.Book{ID: id, OwnerID: "alice"},
			Acquired: &domain.SourceMaterialSummary{
				Source: domain.SourceMaterial{ID: id, Language: "de", ContentRevisionID: "revision-" + id, ContentSnapshotID: "snapshot-" + id}, CorpusID: "c-" + id,
				AnalysisStatus: "analyzed", AnalysisState: "completed", AnalysisRunID: "run-" + id,
			},
		}
	}
	run := func(entries []string) domain.JourneyProjectionResult {
		journey := make([]domain.ReadingJourneyEntry, 0, len(entries))
		for _, id := range entries {
			journey = append(journey, domain.ReadingJourneyEntry{BookID: id})
		}
		store := &routeStore{
			journey: domain.ReadingJourney{OwnerID: "alice", Entries: journey},
			books:   []domain.MyBook{book("x"), book("y"), book("z")},
			corpora: map[string]domain.AnalysisCorpusVocabulary{
				// x and y have identical current coverage (50 known of 100): an equal-cost tie.
				// z has far lower coverage (no known lemma) against a large denominator and few
				// eligible tokens, so its 95/97/99 thresholds are unreachable.
				"c-x": mkCorpus("c-x", "x", 100, 50, 50),
				"c-y": mkCorpus("c-y", "y", 100, 50, 50),
				"c-z": mkCorpus("c-z", "z", 1000, 50, 20),
			},
			known: []domain.KnownVocabulary{
				{OwnerID: "alice", Language: "de", CanonicalLemma: "known-x", UPOS: "NOUN"},
				{OwnerID: "alice", Language: "de", CanonicalLemma: "known-y", UPOS: "NOUN"},
			},
		}
		got, err := NewService(store).JourneyProjection(ctx, "alice", "de")
		require.NoError(t, err)
		return got
	}

	got := run([]string{"x", "y", "z"})
	require.Len(t, got.LearnerOrder, 3)
	assert.Equal(t, "x", got.LearnerOrder[0].BookID, "learner order changed = %+v", got.LearnerOrder)
	assert.Equal(t, "y", got.LearnerOrder[1].BookID, "learner order changed = %+v", got.LearnerOrder)
	require.Len(t, got.AdvisoryOrder, 3)
	for i, want := range []string{"x", "y", "z"} {
		assert.Equal(t, want, got.AdvisoryOrder[i].BookID, "advisory order = %+v, want [x y z]", got.AdvisoryOrder)
	}
	// Equal-cost tie falls back to ascending learner position, deterministically.
	require.NotNil(t, got.AdvisoryOrder[0].Coverage)
	require.NotNil(t, got.AdvisoryOrder[1].Coverage)
	assert.Equal(t, got.AdvisoryOrder[0].Coverage.KnownTokenCount, got.AdvisoryOrder[1].Coverage.KnownTokenCount, "x and y were not an equal-cost tie: %d vs %d",
		got.AdvisoryOrder[0].Coverage.KnownTokenCount, got.AdvisoryOrder[1].Coverage.KnownTokenCount)
	// Reversing the learner order flips the tie: secondary key is position.
	reversed := run([]string{"y", "x", "z"})
	require.Len(t, reversed.AdvisoryOrder, 3)
	for i, want := range []string{"y", "x", "z"} {
		assert.Equal(t, want, reversed.AdvisoryOrder[i].BookID, "reversed advisory order = %+v, want [y x z]", reversed.AdvisoryOrder)
	}
	// Per-book ADR 0025 threshold data is carried honestly; an unreachable
	// threshold neither excludes the book nor influences the objective.
	z := got.AdvisoryOrder[2]
	require.NotNil(t, z.Coverage, "z carried no thresholds: %+v", z)
	require.NotEmpty(t, z.Coverage.Thresholds, "z carried no thresholds: %+v", z)
	var firstReachable bool
	for _, threshold := range z.Coverage.Thresholds {
		if threshold.TargetPercent == 95 {
			firstReachable = threshold.Reachable
		}
	}
	assert.False(t, firstReachable, "z reported its 95%% threshold as reachable but 50 known + 20 eligible cannot reach it")
	assert.NotEmpty(t, z.PlacementReason, "z should still be ranked by coverage with attributions: %+v", z)
	assert.NotNil(t, z.Rank, "z should still be ranked by coverage with attributions: %+v", z)
	// Idempotent and reproducible on repeated computation from the same evidence.
	again := run([]string{"x", "y", "z"})
	require.Len(t, again.AdvisoryOrder, 3)
	assert.Equal(t, "x", again.AdvisoryOrder[0].BookID, "non-deterministic result: %+v", again.AdvisoryOrder)
	assert.Equal(t, "y", again.AdvisoryOrder[1].BookID, "non-deterministic result: %+v", again.AdvisoryOrder)
}
