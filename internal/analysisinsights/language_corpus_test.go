package analysisinsights

import (
	"context"
	"reflect"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type languageCorpusStore struct {
	*memoryStore
	evidence    []domain.LanguageCorpusBookEvidence
	knownCalls  int
	activeCalls int
	genCalls    int
}

func (s *languageCorpusStore) ListLanguageCorpusEvidence(_ context.Context, owner, language string) ([]domain.LanguageCorpusBookEvidence, error) {
	var result []domain.LanguageCorpusBookEvidence
	for _, item := range s.evidence {
		if item.Book.OwnerID == owner && item.Book.LanguageState == domain.LanguageChosen && item.Book.LanguageTag == language {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *languageCorpusStore) ListKnownVocabulary(ctx context.Context, owner, language string) ([]domain.KnownVocabulary, error) {
	s.knownCalls++
	return s.memoryStore.ListKnownVocabulary(ctx, owner, language)
}

func (s *languageCorpusStore) ListActiveLearningCampaignVocabulary(ctx context.Context, owner, language string) ([]domain.CampaignVocabulary, error) {
	s.activeCalls++
	return s.memoryStore.ListActiveLearningCampaignVocabulary(ctx, owner, language)
}

func (s *languageCorpusStore) ListLegacyGeneratedVocabulary(ctx context.Context, owner, language string) ([]domain.GeneratedVocabulary, error) {
	s.genCalls++
	return s.memoryStore.ListLegacyGeneratedVocabulary(ctx, owner, language)
}

func corpusBook(owner, id, title, language, source, corpus, run string, stats int64, lemmas []domain.LemmaOccurrence) domain.LanguageCorpusBookEvidence {
	return domain.LanguageCorpusBookEvidence{
		Book:             domain.Book{ID: id, OwnerID: owner, Title: title, LanguageState: domain.LanguageChosen, LanguageTag: language},
		SourceMaterialID: source, SourceLanguage: language, CurrentContentRevisionID: "revision-" + source, CurrentSnapshotID: "snapshot-" + source, CurrentSourceMaterialID: source,
		CurrentAnalysisRunID: run, AnalysisSourceMaterialID: source, CorpusID: corpus, AnalysisRunID: run,
		Statistics: &domain.AnalysisStatistics{AnalyzableTokenCount: stats, DistinctLemmaCount: int64(len(lemmas))}, Lemmas: lemmas,
	}
}

func TestLanguageCorpusAggregatesDeterministicallyAndBatchesVocabulary(t *testing.T) {
	store := &languageCorpusStore{
		memoryStore: &memoryStore{
			known: []domain.KnownVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
		},
		evidence: []domain.LanguageCorpusBookEvidence{
			corpusBook("alice", "b", "Beta", "de", "source-b", "corpus-b", "run-b", 50, []domain.LemmaOccurrence{
				{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 20},
				{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 10},
				{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 10},
			}),
			corpusBook("alice", "a", "Alpha", "de", "source-a", "corpus-a", "run-a", 100, []domain.LemmaOccurrence{
				{Language: "de", CanonicalLemma: "known", UPOS: "NOUN", OccurrenceCount: 30},
				{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 10},
				{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 5},
				{Language: "de", CanonicalLemma: "tie", UPOS: "NOUN", OccurrenceCount: 5},
			}),
			corpusBook("bob", "bob-book", "Other owner", "de", "bob-source", "bob-corpus", "bob-run", 100, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "bob-only", UPOS: "NOUN", OccurrenceCount: 100}}),
		},
	}

	got, err := NewService(store).LanguageCorpus(context.Background(), "alice", "DE")
	if err != nil {
		t.Fatal(err)
	}
	if got.Language != "de" || got.AnalyzedBookCount != 2 || got.KnownTokenCount != 50 || got.AnalyzableTokenCount != 150 {
		t.Fatalf("summary = %+v", got)
	}
	if got.PerBook[0].BookID != "a" || got.PerBook[0].KnownTokenCount != 30 || got.PerBook[1].BookID != "b" || got.PerBook[1].KnownTokenCount != 20 {
		t.Fatalf("per-book spread = %+v", got.PerBook)
	}
	wantTop := []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "alpha", UPOS: "NOUN", OccurrenceCount: 20},
		{Language: "de", CanonicalLemma: "beta", UPOS: "NOUN", OccurrenceCount: 15},
		{Language: "de", CanonicalLemma: "tie", UPOS: "NOUN", OccurrenceCount: 5},
	}
	if !reflect.DeepEqual(got.TopUnknownLemmas, wantTop) {
		t.Fatalf("top unknown = %+v, want %+v", got.TopUnknownLemmas, wantTop)
	}
	if store.knownCalls != 1 || store.activeCalls != 1 || store.genCalls != 1 {
		t.Fatalf("vocabulary calls = known %d active %d generated %d, want one each", store.knownCalls, store.activeCalls, store.genCalls)
	}
}

func TestLanguageCorpusUsesVocabularyCategoriesAndRecomputes(t *testing.T) {
	currentSource := "source-current"
	otherSource := "source-other"
	store := &languageCorpusStore{
		memoryStore: &memoryStore{
			known: []domain.KnownVocabulary{
				{OwnerID: "alice", Language: "de", CanonicalLemma: "exact", UPOS: "NOUN"},
				{OwnerID: "alice", Language: "de", CanonicalLemma: "wild"},
			},
			generated: []domain.GeneratedVocabulary{
				{OwnerID: "alice", Language: "de", CanonicalLemma: "elsewhere", UPOS: "NOUN", FirstSourceMaterialID: &otherSource},
				{OwnerID: "alice", Language: "de", CanonicalLemma: "current-generated", UPOS: "NOUN", FirstSourceMaterialID: &currentSource},
				{OwnerID: "alice", Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN"},
			},
			active: []domain.CampaignVocabulary{{OwnerID: "alice", Language: "de", CanonicalLemma: "reserved", UPOS: "NOUN"}},
		},
		evidence: []domain.LanguageCorpusBookEvidence{corpusBook("alice", "categories", "Categories", "de", currentSource, "corpus", "run", 100, []domain.LemmaOccurrence{
			{Language: "de", CanonicalLemma: "exact", UPOS: "NOUN", OccurrenceCount: 10},
			{Language: "de", CanonicalLemma: "wild", UPOS: "NOUN", OccurrenceCount: 10},
			{Language: "de", CanonicalLemma: "wild", UPOS: "VERB", OccurrenceCount: 10},
			{Language: "de", CanonicalLemma: "elsewhere", UPOS: "NOUN", OccurrenceCount: 20},
			{Language: "de", CanonicalLemma: "current-generated", UPOS: "NOUN", OccurrenceCount: 15},
			{Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN", OccurrenceCount: 15},
			{Language: "de", CanonicalLemma: "reserved", UPOS: "NOUN", OccurrenceCount: 15},
		})},
	}

	service := NewService(store)
	got, err := service.LanguageCorpus(context.Background(), "alice", "de")
	if err != nil {
		t.Fatal(err)
	}
	if got.KnownTokenCount != 30 || got.AnalyzableTokenCount != 100 {
		t.Fatalf("category counts = %+v", got)
	}
	wantTop := []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "current-generated", UPOS: "NOUN", OccurrenceCount: 15},
	}
	if !reflect.DeepEqual(got.TopUnknownLemmas, wantTop) {
		t.Fatalf("category top unknown = %+v, want %+v", got.TopUnknownLemmas, wantTop)
	}

	store.known = append(store.known, domain.KnownVocabulary{OwnerID: "alice", Language: "de", CanonicalLemma: "legacy", UPOS: "NOUN"})
	got, err = service.LanguageCorpus(context.Background(), "alice", "de")
	if err != nil {
		t.Fatal(err)
	}
	if got.KnownTokenCount != 45 || len(got.TopUnknownLemmas) != 1 || got.TopUnknownLemmas[0].CanonicalLemma != "current-generated" {
		t.Fatalf("recomputed category view = %+v", got)
	}
}

func TestLanguageCorpusExcludesEvidenceAndPreservesEmptyReasons(t *testing.T) {
	books := []domain.LanguageCorpusBookEvidence{
		{Book: domain.Book{ID: "missing", OwnerID: "alice", Title: "Missing", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "stale", OwnerID: "alice", Title: "Stale", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, SourceMaterialID: "source", CurrentContentRevisionID: "revision", CurrentSnapshotID: "snapshot", AnalysisSourceMaterialID: "old-source", AnalysisRunID: "old-run", CurrentSourceMaterialID: "source", CurrentAnalysisRunID: "old-run", CorpusID: "old-corpus"},
		{Book: domain.Book{ID: "incomplete", OwnerID: "alice", Title: "Incomplete", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, SourceMaterialID: "source", CurrentContentRevisionID: "revision", CurrentSnapshotID: "snapshot", CorpusID: "incomplete-corpus"},
		{Book: domain.Book{ID: "unavailable", OwnerID: "alice", Title: "Unavailable", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, SourceMaterialID: "source", CurrentAnalysisRunID: "pending"},
		{Book: domain.Book{ID: "different", OwnerID: "alice", Title: "Different", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, SourceMaterialID: "source", CurrentContentRevisionID: "revision", CurrentSnapshotID: "snapshot", SourceLanguage: "it", CorpusID: "different-corpus", Statistics: &domain.AnalysisStatistics{}, Lemmas: []domain.LemmaOccurrence{{Language: "it", CanonicalLemma: "ciao", UPOS: "NOUN", OccurrenceCount: 1}}},
	}
	store := &languageCorpusStore{memoryStore: &memoryStore{}, evidence: books}
	got, err := NewService(store).LanguageCorpus(context.Background(), "alice", "de")
	if err != nil {
		t.Fatal(err)
	}
	if got.AnalyzedBookCount != 0 || got.KnownTokenCount != 0 || got.AnalyzableTokenCount != 0 || len(got.TopUnknownLemmas) != 0 || len(got.PerBook) != len(books) {
		t.Fatalf("empty view = %+v", got)
	}
	wantReasons := map[string]string{
		"missing":     "unavailable: no current acquired source",
		"stale":       "stale: analysis no longer matches the current book scope",
		"incomplete":  "stale/incomplete: corpus statistics unavailable",
		"unavailable": "unavailable: current source content unavailable",
		"different":   "different study language",
	}
	for _, spread := range got.PerBook {
		want := wantReasons[spread.BookID]
		if spread.Included || spread.ExclusionReason != want {
			t.Fatalf("exclusion %s = %+v, want %q", spread.BookID, spread, want)
		}
	}
}

func TestLanguageCorpusDoesNotMergeDuplicateBookAnalyses(t *testing.T) {
	first := corpusBook("alice", "book", "Book", "de", "source", "current-corpus", "current-run", 10, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "current", UPOS: "NOUN", OccurrenceCount: 10}})
	second := corpusBook("alice", "book", "Book", "de", "source", "historical-corpus", "historical-run", 20, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "historical", UPOS: "NOUN", OccurrenceCount: 20}})
	store := &languageCorpusStore{memoryStore: &memoryStore{}, evidence: []domain.LanguageCorpusBookEvidence{first, second}}
	got, err := NewService(store).LanguageCorpus(context.Background(), "alice", "de")
	if err != nil {
		t.Fatal(err)
	}
	if got.AnalyzedBookCount != 1 || got.AnalyzableTokenCount != 10 || got.PerBook[0].CorpusID != "current-corpus" || got.TopUnknownLemmas[0].CanonicalLemma != "current" {
		t.Fatalf("duplicate analysis merge = %+v", got)
	}
}
