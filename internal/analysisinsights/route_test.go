package analysisinsights

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type routeStore struct {
	journey         domain.ReadingJourney
	journeyLanguage string
	goal            domain.PrimaryGoal
	books           []domain.MyBook
	corpora         map[string]domain.AnalysisCorpusVocabulary
	known           []domain.KnownVocabulary
	active          []domain.CampaignVocabulary
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
func (s *routeStore) ListActiveLearningCampaignVocabulary(_ context.Context, owner, language string) ([]domain.CampaignVocabulary, error) {
	var out []domain.CampaignVocabulary
	for _, v := range s.active {
		if v.OwnerID == owner && v.Language == language {
			out = append(out, v)
		}
	}
	return out, nil
}
func (s *routeStore) ListLegacyGeneratedVocabulary(context.Context, string, string) ([]domain.GeneratedVocabulary, error) {
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
		corpora: map[string]domain.AnalysisCorpusVocabulary{"c-a": lemma("a", "de", 50), "c-b": lemma("b", "fr", 90), "c-c": lemma("c", "de", 50), "c-d": {CorpusID: "c-d", SourceMaterialID: "d", Statistics: nil}},
		known:   []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known-a", UPOS: "NOUN"}, {OwnerID: "alice", Language: "de", CanonicalLemma: "known-c", UPOS: "NOUN"}},
		active:  []domain.CampaignVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "unknown-a", UPOS: "NOUN"}},
	}
	got, err := NewService(store).JourneyProjection(ctx, "alice", "de")
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != "de" || store.journeyLanguage != "de" || got.ComparableCount != 3 || got.IncomparableCount != 1 {
		t.Fatalf("summary = %+v", got)
	}
	for i, want := range []string{"a", "b", "c", "d"} {
		if got.LearnerOrder[i].BookID != want {
			t.Fatalf("learner order = %+v", got.LearnerOrder)
		}
	}
	for i, want := range []string{"c", "a", "b", "d"} {
		if got.AdvisoryOrder[i].BookID != want {
			t.Fatalf("advisory order = %+v", got.AdvisoryOrder)
		}
	}
	for i := range got.AdvisoryOrder[:3] {
		if got.AdvisoryOrder[i].Rank == nil {
			t.Fatalf("comparable rank[%d] = %+v", i, got.AdvisoryOrder)
		}
	}
	if got.AdvisoryOrder[3].Rank != nil {
		t.Fatalf("ranks = %+v", got.AdvisoryOrder)
	}
	if !got.LearnerOrder[1].Comparable || got.LearnerOrder[1].IncomparableReason != "" {
		t.Fatalf("explicit-language projection excluded book = %+v", got.LearnerOrder[1])
	}
	if got.LearnerOrder[3].Comparable || got.LearnerOrder[3].Rank != nil {
		t.Fatalf("legacy exclusion = %+v", got.LearnerOrder[3])
	}
	if got.LearnerOrder[0].ConditionalCoverage.KnownTokenCount != 100 {
		t.Fatalf("conditional coverage = %+v", got.LearnerOrder[0].ConditionalCoverage)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	book := result.LearnerOrder[0]
	if book.Comparable || book.IncomparableReason != "stale/incomplete: corpus evidence is not modeled book language" {
		t.Fatalf("corpus integrity state = %+v", book)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if result.Language != "" {
		t.Fatalf("projection inferred language %q", result.Language)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if got := []string{result.AdvisoryOrder[0].BookID, result.AdvisoryOrder[1].BookID, result.AdvisoryOrder[2].BookID}; got[0] != "missing" || got[1] != "c" || got[2] != "b" {
		t.Fatalf("advisory order = %v", got)
	}
	if result.AdvisoryOrder[0].Rank != nil || result.AdvisoryOrder[1].PlacementReason == "" {
		t.Fatalf("explanations/ranks = %+v", result.AdvisoryOrder)
	}

	// The same projection call observes current vocabulary changes; no route
	// projection is stored between calls.
	store.known = append(store.known, domain.KnownVocabulary{OwnerID: "owner", Language: "de", CanonicalLemma: "unknown-b", UPOS: "VERB"})
	result, err = NewService(store).JourneyProjection(context.Background(), "owner", "de")
	if err != nil {
		t.Fatal(err)
	}
	if result.AdvisoryOrder[1].BookID != "c" || result.AdvisoryOrder[2].BookID != "b" {
		t.Fatalf("recomputed advisory order = %+v", result.AdvisoryOrder)
	}
	if result.AdvisoryOrder[2].Coverage == nil || result.AdvisoryOrder[2].Coverage.KnownTokenCount != 100 {
		t.Fatalf("recomputed coverage = %+v", result.AdvisoryOrder[2].Coverage)
	}
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
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	got := run([]string{"x", "y", "z"})
	if got.LearnerOrder[0].BookID != "x" || got.LearnerOrder[1].BookID != "y" {
		t.Fatalf("learner order changed = %+v", got.LearnerOrder)
	}
	for i, want := range []string{"x", "y", "z"} {
		if got.AdvisoryOrder[i].BookID != want {
			t.Fatalf("advisory order = %+v, want [x y z]", got.AdvisoryOrder)
		}
	}
	// Equal-cost tie falls back to ascending learner position, deterministically.
	if got.AdvisoryOrder[0].Coverage.KnownTokenCount != got.AdvisoryOrder[1].Coverage.KnownTokenCount {
		t.Fatalf("x and y were not an equal-cost tie: %d vs %d",
			got.AdvisoryOrder[0].Coverage.KnownTokenCount, got.AdvisoryOrder[1].Coverage.KnownTokenCount)
	}
	// Reversing the learner order flips the tie: secondary key is position.
	reversed := run([]string{"y", "x", "z"})
	for i, want := range []string{"y", "x", "z"} {
		if reversed.AdvisoryOrder[i].BookID != want {
			t.Fatalf("reversed advisory order = %+v, want [y x z]", reversed.AdvisoryOrder)
		}
	}
	// Per-book ADR 0025 threshold data is carried honestly; an unreachable
	// threshold neither excludes the book nor influences the objective.
	z := got.AdvisoryOrder[2]
	if z.Coverage == nil || len(z.Coverage.Thresholds) == 0 {
		t.Fatalf("z carried no thresholds: %+v", z)
	}
	var firstReachable bool
	for _, threshold := range z.Coverage.Thresholds {
		if threshold.TargetPercent == 95 {
			firstReachable = threshold.Reachable
		}
	}
	if firstReachable {
		t.Fatal("z reported its 95%% threshold as reachable but 50 known + 20 eligible cannot reach it")
	}
	if z.PlacementReason == "" || z.Rank == nil {
		t.Fatalf("z should still be ranked by coverage with attributions: %+v", z)
	}
	// Idempotent and reproducible on repeated computation from the same evidence.
	if again := run([]string{"x", "y", "z"}); again.AdvisoryOrder[0].BookID != "x" || again.AdvisoryOrder[1].BookID != "y" {
		t.Fatalf("non-deterministic result: %+v", again.AdvisoryOrder)
	}
}
