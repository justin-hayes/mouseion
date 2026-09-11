package selection

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	states   map[string]string
	known    map[string]bool
	reserved map[string]bool
	saved    []domain.SelectionCandidate
}

func (m *memoryStore) key(owner, lang, lemma, upos string) string {
	return owner + "/" + lang + "/" + lemma + "/" + upos
}
func (m *memoryStore) GetVocabularyStateByIdentity(_ context.Context, o, l, x, p string) (domain.VocabularyState, error) {
	v, ok := m.states[m.key(o, l, x, p)]
	if !ok {
		return domain.VocabularyState{}, persistence.ErrNotFound
	}
	return domain.VocabularyState{State: v}, nil
}
func (m *memoryStore) IsKnownVocabularyIdentity(_ context.Context, o, l, x, p string) (bool, error) {
	return m.known[m.key(o, l, x, p)], nil
}
func (m *memoryStore) IsReservedVocabulary(_ context.Context, o, l, x, p string) (bool, error) {
	return m.reserved[m.key(o, l, x, p)], nil
}
func (m *memoryStore) PutSelectionCandidate(_ context.Context, c domain.SelectionCandidate) (bool, error) {
	m.saved = append(m.saved, c)
	m.states[m.key(c.OwnerID, c.Language, c.CanonicalLemma, c.UPOS)] = "candidate"
	return true, nil
}

func tok(surface, lemma, upos string, ner bool) analyzer.Token {
	var entity *string
	if ner {
		v := "PERSON"
		entity = &v
	}
	return analyzer.Token{Surface: surface, CanonicalLemma: lemma, UPOS: upos, NamedEntity: entity}
}
func fixture(tokens ...analyzer.Token) analyzer.Result {
	sentences := make([]analyzer.Sentence, len(tokens))
	for i, t := range tokens {
		sentences[i] = analyzer.Sentence{Text: t.Surface, Tokens: []analyzer.Token{t}, Location: analyzer.SourceLocation{SourceDocumentID: "book"}}
	}
	return analyzer.Result{Language: "de", Sentences: sentences}
}

func TestDefaultRulesFiltersAggregationAndDeterminism(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}}
	svc := NewService(store)
	cfg := DefaultConfig("corpus-1")
	corpus := fixture(tok("Häuser", "Haus", "NOUN", false), tok("Haus", "Haus", "noun", false), tok("selten", "selten", "ADJ", false), tok("der", "der", "DET", false), tok("Berlin", "Berlin", "PROPN", false), tok("Anna", "Anna", "NOUN", true))
	got, err := svc.Select(context.Background(), "alice", corpus, cfg)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Haus", got[0].Identity.CanonicalLemma)
	assert.Equal(t, "selten", got[1].Identity.CanonicalLemma)
	assert.Equal(t, []string{"Haus", "Häuser"}, got[0].ObservedForms)
	assert.Equal(t, 2, got[0].OccurrenceCount)
	assert.Len(t, got[0].SentenceReferences, 2)
}

func TestAnalyzableStatisticsUsesSelectionFiltersBeforeVocabularyState(t *testing.T) {
	corpus := fixture(
		tok("Häuser", "Haus", "NOUN", false),
		tok("Haus", "Haus", "noun", false),
		tok("laufen", "laufen", "VERB", false),
		tok("der", "der", "DET", false),
		tok("Anna", "Anna", "NOUN", true),
		tok("leer", " ", "ADJ", false),
		tok("5", "5", "NOUN", false),
		tok("B2", "B2", "NOUN", false),
	)
	got := AnalyzableStatistics(corpus, DefaultConfig("corpus-1"))
	want := domain.AnalysisStatistics{AnalyzableTokenCount: 4, DistinctLemmaCount: 3, TextProfile: &domain.TextProfile{SentenceCount: 8, NormalizedTokenCount: 8, MedianSentenceTokenCount: 1, P90SentenceTokenCount: 1}}
	assert.Equal(t, want, got)
}

func TestItalianFixtureFiltersAndAggregatesVocabulary(t *testing.T) {
	person := "S-PER"
	corpus := analyzer.Result{Language: "it", Sentences: []analyzer.Sentence{{
		Text: "L'uomo e gli uomini bevono dell'acqua; Maria berrà e dammelo!",
		Tokens: []analyzer.Token{
			{Surface: "L'", CanonicalLemma: "il", UPOS: "DET"},
			{Surface: "uomo", CanonicalLemma: "uomo", UPOS: "NOUN"},
			{Surface: "uomini", CanonicalLemma: "uomo", UPOS: "NOUN"},
			{Surface: "bevono", CanonicalLemma: "bere", UPOS: "VERB"},
			{Surface: "dell'", CanonicalLemma: "di", UPOS: "ADP"},
			{Surface: "acqua", CanonicalLemma: "acqua", UPOS: "NOUN"},
			{Surface: ";", CanonicalLemma: ";", UPOS: "PUNCT"},
			{Surface: "Maria", CanonicalLemma: "maria", UPOS: "PROPN", NamedEntity: &person},
			{Surface: "berrà", CanonicalLemma: "bere", UPOS: "VERB"},
			{Surface: "damme", CanonicalLemma: "dare", UPOS: "VERB"},
			{Surface: "lo", CanonicalLemma: "lo", UPOS: "PRON"},
			{Surface: "!", CanonicalLemma: "!", UPOS: "PUNCT"},
		},
	}}}
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}, reserved: map[string]bool{}}
	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("italian-corpus"))
	require.NoError(t, err)
	want := []Candidate{
		{Identity: Identity{"it", "acqua", "NOUN"}, OccurrenceCount: 1},
		{Identity: Identity{"it", "bere", "VERB"}, OccurrenceCount: 2},
		{Identity: Identity{"it", "dare", "VERB"}, OccurrenceCount: 1},
		{Identity: Identity{"it", "uomo", "NOUN"}, OccurrenceCount: 2},
	}
	require.Len(t, got, len(want))
	for i := range want {
		assert.Equal(t, want[i].Identity, got[i].Identity, "Italian candidate %d", i)
		assert.Equal(t, want[i].OccurrenceCount, got[i].OccurrenceCount, "Italian candidate %d", i)
	}
	assert.Equal(t, []string{"uomini", "uomo"}, got[3].ObservedForms)
	statistics := AnalyzableStatistics(corpus, DefaultConfig("italian-corpus"))
	assert.Equal(t, int64(6), statistics.AnalyzableTokenCount)
	assert.Equal(t, int64(4), statistics.DistinctLemmaCount)
	assert.Equal(t, int64(12), statistics.TextProfile.NormalizedTokenCount)
}

func TestAnalyzableStatisticsComputesExplainableSentenceProfile(t *testing.T) {
	result := analyzer.Result{Sentences: []analyzer.Sentence{{}, {Tokens: make([]analyzer.Token, 10)}, {Tokens: make([]analyzer.Token, 20)}, {Tokens: make([]analyzer.Token, 36)}}}
	got := AnalyzableStatistics(result, DefaultConfig("corpus")).TextProfile
	want := &domain.TextProfile{SentenceCount: 4, NormalizedTokenCount: 66, EmptySentenceCount: 1, MedianSentenceTokenCount: 15, P90SentenceTokenCount: 36, LongSentenceCount: 1}
	assert.Equal(t, want, got)
}

func TestDefaultIncludesSingletonAndConfigOverridesFilters(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}}
	cfg := DefaultConfig("c")
	assert.Equal(t, 1, cfg.MinOccurrences, "MinOccurrences=%d, want 1", cfg.MinOccurrences)
	cfg.AllowedPOS["PROPN"] = true
	cfg.IncludeNamedEntities = true
	got, err := NewService(store).Select(context.Background(), "alice", fixture(tok("Berlin", "Berlin", "PROPN", true)), cfg)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].OccurrenceCount)
	assert.Len(t, store.saved, 1)
}

func TestOwnerScopedExclusions(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}}
	for _, state := range []string{"known", "ignored"} {
		store.states[store.key("alice", "de", state, "NOUN")] = state
	}
	store.known[store.key("alice", "de", "importiert", "NOUN")] = true
	corpus := fixture(tok("known", "known", "NOUN", false), tok("known", "known", "NOUN", false), tok("ignored", "ignored", "NOUN", false), tok("ignored", "ignored", "NOUN", false), tok("generated", "generated", "NOUN", false), tok("generated", "generated", "NOUN", false), tok("importiert", "importiert", "NOUN", false), tok("importiert", "importiert", "NOUN", false))
	svc := NewService(store)
	alice, err := svc.Select(context.Background(), "alice", corpus, DefaultConfig("a"))
	require.NoError(t, err)
	require.Len(t, alice, 1)
	assert.Equal(t, "generated", alice[0].Identity.CanonicalLemma)
	bob, err := svc.Select(context.Background(), "bob", corpus, DefaultConfig("b"))
	require.NoError(t, err)
	assert.Len(t, bob, 4)
}

func TestLegacyGeneratedStateDoesNotSuppressCandidate(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}}
	store.states[store.key("alice", "de", "Haus", "NOUN")] = "generated"

	got, err := NewService(store).Select(context.Background(), "alice", fixture(tok("Haus", "Haus", "NOUN", false)), DefaultConfig("book"))
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Len(t, store.saved, 1)
}

func TestReservedVocabularyIsOwnerScoped(t *testing.T) {
	store := &memoryStore{states: map[string]string{}, known: map[string]bool{}, reserved: map[string]bool{}}
	store.reserved[store.key("alice", "de", "Haus", "NOUN")] = true
	corpus := fixture(tok("Haus", "Haus", "NOUN", false))

	alice, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("next-book"))
	require.NoError(t, err)
	assert.Empty(t, alice)
	bob, err := NewService(store).Select(context.Background(), "bob", corpus, DefaultConfig("bob-book"))
	require.NoError(t, err)
	assert.Len(t, bob, 1)
}

func TestInvalidConfig(t *testing.T) {
	_, err := NewService(&memoryStore{}).Select(context.Background(), "", analyzer.Result{}, SelectionConfig{})
	assert.ErrorIs(t, err, ErrInvalidConfig)
}
