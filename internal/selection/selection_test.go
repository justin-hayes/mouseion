package selection

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type memoryStore struct {
	known    map[string]bool
	reserved map[string]bool
	saved    []domain.SelectionCandidate
}

func (m *memoryStore) key(owner, lang, lemma, upos string) string {
	return owner + "/" + lang + "/" + lemma + "/" + upos
}
func (m *memoryStore) IsKnownVocabularyIdentity(_ context.Context, o, l, x, p string) (bool, error) {
	return m.known[m.key(o, l, x, p)], nil
}
func (m *memoryStore) IsReservedVocabulary(_ context.Context, o, l, x, p string) (bool, error) {
	return m.reserved[m.key(o, l, x, p)], nil
}
func (m *memoryStore) PutSelectionCandidate(_ context.Context, c domain.SelectionCandidate) (bool, error) {
	m.saved = append(m.saved, c)
	return true, nil
}

func tok(surface, lemma, upos string) analyzer.Token {
	return analyzer.Token{Surface: surface, CanonicalLemma: lemma, UPOS: upos}
}
func fixture(tokens ...analyzer.Token) analyzer.Result {
	sentences := make([]analyzer.Sentence, len(tokens))
	for i, t := range tokens {
		sentences[i] = analyzer.Sentence{Text: t.Surface, Tokens: []analyzer.Token{t}, Location: analyzer.SourceLocation{SourceDocumentID: "book"}}
	}
	return analyzer.Result{Language: "de", Sentences: sentences}
}

func TestDefaultRulesFiltersAggregationAndDeterminism(t *testing.T) {
	store := &memoryStore{known: map[string]bool{}}
	svc := NewService(store)
	cfg := DefaultConfig("corpus-1")
	corpus := fixture(tok("Häuser", "Haus", "NOUN"), tok("Haus", "Haus", "noun"), tok("selten", "selten", "ADJ"), tok("der", "der", "DET"), tok("Berlin", "Berlin", "PROPN"))
	got, err := svc.Select(context.Background(), "alice", corpus, cfg)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "Haus", got[0].Identity.CanonicalLemma)
	assert.Equal(t, "selten", got[1].Identity.CanonicalLemma)
	assert.Equal(t, []string{"Haus", "Häuser"}, got[0].ObservedForms)
	assert.Equal(t, 2, got[0].OccurrenceCount)
	assert.Len(t, got[0].SentenceReferences, 2)
}

func TestDefaultRulesExcludeSeparableParticlesButKeepAdverbs(t *testing.T) {
	store := &memoryStore{known: map[string]bool{}}
	corpus := fixture(
		analyzer.Token{Surface: "steht", CanonicalLemma: "aufstehen", UPOS: "VERB"},
		analyzer.Token{Surface: "auf", CanonicalLemma: "auf", UPOS: "ADV", Dependency: "compound:prt"},
		analyzer.Token{Surface: "dort", CanonicalLemma: "dort", UPOS: "ADV", Dependency: "advmod"},
	)

	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("corpus-1"))

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "aufstehen", got[0].Identity.CanonicalLemma)
	assert.Equal(t, "dort", got[1].Identity.CanonicalLemma)
}

func TestAnalyzableStatisticsUsesSelectionFiltersBeforeLearnerExclusions(t *testing.T) {
	corpus := fixture(
		tok("Häuser", "Haus", "NOUN"),
		tok("Haus", "Haus", "noun"),
		tok("laufen", "laufen", "VERB"),
		tok("der", "der", "DET"),
		tok("leer", " ", "ADJ"),
		tok("5", "5", "NOUN"),
		tok("B2", "B2", "NOUN"),
	)
	got := AnalyzableStatistics(corpus, DefaultConfig("corpus-1"))
	want := domain.AnalysisStatistics{AnalyzableTokenCount: 4, DistinctLemmaCount: 3, TextProfile: &domain.TextProfile{SentenceCount: 7, NormalizedTokenCount: 7, MedianSentenceTokenCount: 1, P90SentenceTokenCount: 1}}
	assert.Equal(t, want, got)
}

func TestItalianFixtureFiltersAndAggregatesVocabulary(t *testing.T) {
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
			{Surface: "Maria", CanonicalLemma: "maria", UPOS: "PROPN"},
			{Surface: "berrà", CanonicalLemma: "bere", UPOS: "VERB"},
			{Surface: "damme", CanonicalLemma: "dare", UPOS: "VERB"},
			{Surface: "lo", CanonicalLemma: "lo", UPOS: "PRON"},
			{Surface: "!", CanonicalLemma: "!", UPOS: "PUNCT"},
		},
	}}}
	store := &memoryStore{known: map[string]bool{}, reserved: map[string]bool{}}
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

func TestGreekFixtureFiltersAndAggregatesVocabulary(t *testing.T) {
	corpus := analyzer.Result{Language: "el", Sentences: []analyzer.Sentence{{
		Text: "ΣΤΟ σπίτι ο Νίκος διαβάζει ιστορίες. Οι φίλοι γελούν.",
		Tokens: []analyzer.Token{
			{Surface: "ΣΤΟ", CanonicalLemma: "σε", UPOS: "ADP"},
			{Surface: "ΣΤΟ", CanonicalLemma: "ο", UPOS: "DET"},
			{Surface: "σπίτι", CanonicalLemma: "σπίτι", UPOS: "NOUN"},
			{Surface: "ο", CanonicalLemma: "ο", UPOS: "DET"},
			{Surface: "Νίκος", CanonicalLemma: "νίκοσ", UPOS: "PROPN"},
			{Surface: "διαβάζει", CanonicalLemma: "διαβάζω", UPOS: "VERB"},
			{Surface: "ιστορίες", CanonicalLemma: "ιστορία", UPOS: "NOUN"},
			{Surface: ".", CanonicalLemma: ".", UPOS: "PUNCT"},
			{Surface: "Οι", CanonicalLemma: "ο", UPOS: "DET"},
			{Surface: "φίλοι", CanonicalLemma: "φίλοσ", UPOS: "NOUN"},
			{Surface: "γελούν", CanonicalLemma: "γελάω", UPOS: "VERB"},
			{Surface: "τον", CanonicalLemma: "ο", UPOS: "PRON"},
			{Surface: "και", CanonicalLemma: "και", UPOS: "CCONJ"},
			{Surface: "!", CanonicalLemma: "!", UPOS: "PUNCT"},
		},
	}}}
	store := &memoryStore{known: map[string]bool{}, reserved: map[string]bool{}}
	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("greek-corpus"))
	require.NoError(t, err)
	counts := make(map[string]int)
	for _, candidate := range got {
		counts[candidate.Identity.CanonicalLemma] = candidate.OccurrenceCount
	}
	assert.Equal(t, map[string]int{"γελάω": 1, "διαβάζω": 1, "ιστορία": 1, "σπίτι": 1, "φίλοσ": 1}, counts)
	assert.NotContains(t, counts, "σε")
	assert.NotContains(t, counts, "ο")
	assert.NotContains(t, counts, "και")
	assert.NotContains(t, counts, "νίκοσ")
	assert.NotContains(t, counts, ".")
	statistics := AnalyzableStatistics(corpus, DefaultConfig("greek-corpus"))
	assert.Equal(t, int64(5), statistics.AnalyzableTokenCount)
	assert.Equal(t, int64(5), statistics.DistinctLemmaCount)
}

func TestAnalyzableStatisticsComputesExplainableSentenceProfile(t *testing.T) {
	result := analyzer.Result{Sentences: []analyzer.Sentence{{}, {Tokens: make([]analyzer.Token, 10)}, {Tokens: make([]analyzer.Token, 20)}, {Tokens: make([]analyzer.Token, 36)}}}
	got := AnalyzableStatistics(result, DefaultConfig("corpus")).TextProfile
	want := &domain.TextProfile{SentenceCount: 4, NormalizedTokenCount: 66, EmptySentenceCount: 1, MedianSentenceTokenCount: 15, P90SentenceTokenCount: 36, LongSentenceCount: 1}
	assert.Equal(t, want, got)
}

func TestDefaultIncludesSingletonAndConfigOverridesFilters(t *testing.T) {
	store := &memoryStore{known: map[string]bool{}}
	cfg := DefaultConfig("c")
	assert.Equal(t, 1, cfg.MinOccurrences, "MinOccurrences=%d, want 1", cfg.MinOccurrences)
	cfg.AllowedPOS["PROPN"] = true
	got, err := NewService(store).Select(context.Background(), "alice", fixture(tok("Berlin", "Berlin", "PROPN")), cfg)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 1, got[0].OccurrenceCount)
	assert.Len(t, store.saved, 1)
}

func TestOwnerScopedExclusions(t *testing.T) {
	store := &memoryStore{known: map[string]bool{}}
	store.known[store.key("alice", "de", "known", "NOUN")] = true
	store.known[store.key("alice", "de", "importiert", "NOUN")] = true
	corpus := fixture(tok("known", "known", "NOUN"), tok("known", "known", "NOUN"), tok("ignored", "ignored", "NOUN"), tok("ignored", "ignored", "NOUN"), tok("generated", "generated", "NOUN"), tok("generated", "generated", "NOUN"), tok("importiert", "importiert", "NOUN"), tok("importiert", "importiert", "NOUN"))
	svc := NewService(store)
	alice, err := svc.Select(context.Background(), "alice", corpus, DefaultConfig("a"))
	require.NoError(t, err)
	require.Len(t, alice, 2)
	assert.Equal(t, "generated", alice[0].Identity.CanonicalLemma)
	assert.Equal(t, "ignored", alice[1].Identity.CanonicalLemma)
	bob, err := svc.Select(context.Background(), "bob", corpus, DefaultConfig("b"))
	require.NoError(t, err)
	assert.Len(t, bob, 4)
}

func TestEligibilityUsesKnownWildcardAndLanguageScopedReservations(t *testing.T) {
	eligibility := NewEligibility(
		[]domain.KnownVocabulary{{Language: "de", CanonicalLemma: "wissen", UPOS: ""}},
		[]domain.DeckPreparationVocabulary{{Language: "de", CanonicalLemma: "reserviert", UPOS: "NOUN"}},
	)
	assert.False(t, eligibility.Allows(domain.SelectionCandidate{Language: "de", CanonicalLemma: "wissen", UPOS: "VERB", OccurrenceCount: 3}, 3))
	assert.True(t, eligibility.Allows(domain.SelectionCandidate{Language: "it", CanonicalLemma: "wissen", UPOS: "VERB", OccurrenceCount: 3}, 3))
	assert.False(t, eligibility.Allows(domain.SelectionCandidate{Language: "de", CanonicalLemma: "reserviert", UPOS: "NOUN", OccurrenceCount: 3}, 3))
	assert.True(t, eligibility.Allows(domain.SelectionCandidate{Language: "de", CanonicalLemma: "reserviert", UPOS: "VERB", OccurrenceCount: 3}, 3))
	assert.True(t, eligibility.Allows(domain.SelectionCandidate{Language: "it", CanonicalLemma: "reserviert", UPOS: "NOUN", OccurrenceCount: 3}, 3))
	overlap := NewEligibility(
		[]domain.KnownVocabulary{{Language: "de", CanonicalLemma: "overlap", UPOS: "NOUN"}},
		[]domain.DeckPreparationVocabulary{{Language: "de", CanonicalLemma: "overlap", UPOS: "NOUN"}},
	)
	assert.False(t, overlap.Allows(domain.SelectionCandidate{Language: "de", CanonicalLemma: "overlap", UPOS: "NOUN", OccurrenceCount: 3}, 3))
	assert.False(t, eligibility.Allows(domain.SelectionCandidate{Language: "de", CanonicalLemma: "häufig", UPOS: "NOUN", OccurrenceCount: 2}, 3))
}

func TestGreekEligibilityKeepsKnownAndReservedVocabularyOwnerAndLanguageScoped(t *testing.T) {
	eligibility := NewEligibility(
		[]domain.KnownVocabulary{{OwnerID: "alice", Language: "el", CanonicalLemma: "σπίτι", UPOS: "NOUN"}},
		[]domain.DeckPreparationVocabulary{{OwnerID: "alice", Language: "el", CanonicalLemma: "πλατεία", UPOS: "NOUN"}},
	)
	assert.False(t, eligibility.Allows(domain.SelectionCandidate{Language: "el", CanonicalLemma: "σπίτι", UPOS: "NOUN", OccurrenceCount: 3}, 3))
	assert.False(t, eligibility.Allows(domain.SelectionCandidate{Language: "el", CanonicalLemma: "πλατεία", UPOS: "NOUN", OccurrenceCount: 3}, 3))
	assert.True(t, eligibility.Allows(domain.SelectionCandidate{Language: "de", CanonicalLemma: "σπίτι", UPOS: "NOUN", OccurrenceCount: 3}, 3))
	assert.True(t, eligibility.Allows(domain.SelectionCandidate{Language: "el", CanonicalLemma: "σπίτι", UPOS: "VERB", OccurrenceCount: 3}, 3))
}

func TestReservedVocabularyIsOwnerScoped(t *testing.T) {
	store := &memoryStore{known: map[string]bool{}, reserved: map[string]bool{}}
	store.reserved[store.key("alice", "de", "Haus", "NOUN")] = true
	corpus := fixture(tok("Haus", "Haus", "NOUN"))

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
