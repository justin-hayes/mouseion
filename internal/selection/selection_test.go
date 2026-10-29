package selection

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/checked"
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

// failingStore returns the configured error from each Store method so tests can
// exercise the error paths Select uses to wrap store failures.
type failingStore struct {
	knownErr    error
	reservedErr error
	putErr      error
}

func (f failingStore) IsKnownVocabularyIdentity(context.Context, string, string, string, string) (bool, error) {
	return false, f.knownErr
}
func (f failingStore) IsReservedVocabulary(context.Context, string, string, string, string) (bool, error) {
	return false, f.reservedErr
}
func (f failingStore) PutSelectionCandidate(context.Context, domain.SelectionCandidate) (bool, error) {
	return false, f.putErr
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

func TestSelectSkipsBelowMinimumOccurrencesBeforeStoreLookups(t *testing.T) {
	store := &memoryStore{known: map[string]bool{}}
	cfg := DefaultConfig("corpus")
	cfg.MinOccurrences = 2
	corpus := fixture(tok("Haus", "Haus", "NOUN"), tok("Haus", "Haus", "NOUN"), tok("selten", "selten", "ADJ"))

	got, err := NewService(store).Select(context.Background(), "alice", corpus, cfg)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "Haus", got[0].Identity.CanonicalLemma)
	assert.Len(t, store.saved, 1, "an occurrence below the floor must not reach the store")
}

func TestSelectPropagatesStoreFailures(t *testing.T) {
	corpus := fixture(tok("Haus", "Haus", "NOUN"))
	knownFailure := errors.New("known lookup failed")
	reservedFailure := errors.New("reserved lookup failed")
	persistFailure := errors.New("persist failed")
	for name, test := range map[string]struct {
		store   failingStore
		wantErr error
		message string
	}{
		"known lookup":    {failingStore{knownErr: knownFailure}, knownFailure, "get known vocabulary"},
		"reserved lookup": {failingStore{reservedErr: reservedFailure}, reservedFailure, "get reserved vocabulary"},
		"persist":         {failingStore{putErr: persistFailure}, persistFailure, "persist candidate"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewService(test.store).Select(context.Background(), "alice", corpus, DefaultConfig("corpus"))
			require.ErrorIs(t, err, test.wantErr)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestProjectAppliesExactOccurrenceDecisionsWithoutChangingAnalyzerEvidence(t *testing.T) {
	first := analyzer.Token{Surface: "Drachen", RawLemma: "Drach", CanonicalLemma: "drach", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: "book", StartOffset: 10, EndOffset: 17}}
	second := analyzer.Token{Surface: "Drachen", RawLemma: "Drache", CanonicalLemma: "drache", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: "book", StartOffset: 30, EndOffset: 37}}
	corpus := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{
		{Text: "Drachen eins", Tokens: []analyzer.Token{first}},
		{Text: "Drachen zwei", Tokens: []analyzer.Token{second}},
	}}
	originalFirst := corpus.Sentences[0].Tokens[0]
	config := DefaultConfig("corpus")

	uncorrected, err := Project(corpus, config, nil)
	require.NoError(t, err)
	require.Len(t, uncorrected, 2)
	assert.Equal(t, "drach", uncorrected[0].Identity.CanonicalLemma)
	assert.Equal(t, "drache", uncorrected[1].Identity.CanonicalLemma)

	projected, err := Project(corpus, config, []OccurrenceDecision{
		{Occurrence: OccurrenceIdentity{SourceDocumentID: "book", StartOffset: 10, EndOffset: 17}, Lemma: "drache"},
		{Occurrence: OccurrenceIdentity{SourceDocumentID: "book", StartOffset: 30, EndOffset: 37}, Excluded: true},
	})
	require.NoError(t, err)
	require.Len(t, projected, 1)
	assert.Equal(t, "drache", projected[0].Identity.CanonicalLemma)
	assert.Equal(t, 1, projected[0].OccurrenceCount)
	assert.Equal(t, []string{"Drachen"}, projected[0].ObservedForms)
	assert.Equal(t, uint64(10), projected[0].SentenceReferences[0].Location.StartOffset)
	assert.Equal(t, originalFirst, corpus.Sentences[0].Tokens[0], "projection must not rewrite analyzer evidence")

	statistics := AnalyzableStatistics(corpus, config)
	assert.Equal(t, int64(2), statistics.AnalyzableTokenCount, "occurrence decisions must not change the coverage denominator")
}

func TestSpeakerLabelSpansAreStructuralNonVocabulary(t *testing.T) {
	speaker := domain.UnitRoleSpan{UnitID: "unit", Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 15}
	direction := domain.UnitRoleSpan{UnitID: "unit", Role: domain.SourceStructureRoleDirection, StartOffset: 9, EndOffset: 15}
	labelToken := analyzer.Token{Surface: "fällt", CanonicalLemma: "fallen", UPOS: "VERB", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 9, EndOffset: 15}}
	directionToken := analyzer.Token{Surface: "schlägt", CanonicalLemma: "schlagen", UPOS: "VERB", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 30, EndOffset: 38}}
	standaloneDirectionToken := analyzer.Token{Surface: "auf", CanonicalLemma: "auf", UPOS: "ADV", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 39, EndOffset: 42}}
	properName := analyzer.Token{Surface: "Valentin", CanonicalLemma: "Valentin", UPOS: "PROPN", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 0, EndOffset: 8}}
	corpus := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{
		{Text: "Valentin fällt.", Tokens: []analyzer.Token{properName, labelToken}},
		{Text: "Er schlägt auf.", Tokens: []analyzer.Token{directionToken, standaloneDirectionToken}},
	}}
	cfg := DefaultConfig("corpus")
	cfg.Structure = domain.SnapshotStructure{Recorded: true, RoleSpans: []domain.UnitRoleSpan{speaker, direction}}

	withSpans, err := Project(corpus, cfg, nil)
	require.NoError(t, err)
	require.Len(t, withSpans, 2)
	assert.Equal(t, "auf", withSpans[0].Identity.CanonicalLemma, "a standalone direction stays ordinary eligible text")
	assert.Equal(t, "schlagen", withSpans[1].Identity.CanonicalLemma)
	for _, candidate := range withSpans {
		assert.NotEqual(t, "fallen", candidate.Identity.CanonicalLemma, "a label token never creates an identity")
	}

	cfg.Structure = domain.SnapshotStructure{}
	unstructured, err := Project(corpus, cfg, nil)
	require.NoError(t, err)
	require.Len(t, unstructured, 3)
	assert.Equal(t, []string{"auf", "fallen", "schlagen"}, []string{unstructured[0].Identity.CanonicalLemma, unstructured[1].Identity.CanonicalLemma, unstructured[2].Identity.CanonicalLemma}, "without recorded structure the same evidence keeps its ordinary rules")
}

func TestSpeakerLabelSpansLeaveTheCoverageDenominator(t *testing.T) {
	label := analyzer.Token{Surface: "fällt", CanonicalLemma: "fallen", UPOS: "VERB", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 9, EndOffset: 15}}
	content := analyzer.Token{Surface: "Buch", CanonicalLemma: "Buch", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 20, EndOffset: 24}}
	corpus := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Text: "Valentin fällt. Das Buch.", Tokens: []analyzer.Token{label, content}}}}
	cfg := DefaultConfig("corpus")

	withoutStructure := AnalyzableStatistics(corpus, cfg)
	assert.Equal(t, int64(2), withoutStructure.AnalyzableTokenCount)

	cfg.Structure = domain.SnapshotStructure{Recorded: true, RoleSpans: []domain.UnitRoleSpan{{UnitID: "unit", Role: domain.SourceStructureRoleSpeaker, StartOffset: 9, EndOffset: 15}}}
	withStructure := AnalyzableStatistics(corpus, cfg)
	assert.Equal(t, int64(1), withStructure.AnalyzableTokenCount, "speaker-label tokens are not in the coverage denominator")
	assert.Equal(t, int64(1), withStructure.DistinctLemmaCount)
	assert.Equal(t, int64(2), withStructure.TextProfile.NormalizedTokenCount, "normalized corpus evidence still counts every analyzed token")
}

func TestProjectRejectsAmbiguousOrMalformedOccurrenceDecisions(t *testing.T) {
	occurrence := OccurrenceIdentity{SourceDocumentID: "book", StartOffset: 1, EndOffset: 2}
	for name, decisions := range map[string][]OccurrenceDecision{
		"no action":       {{Occurrence: occurrence}},
		"both actions":    {{Occurrence: occurrence, Lemma: "haus", Excluded: true}},
		"missing source":  {{Occurrence: OccurrenceIdentity{StartOffset: 1, EndOffset: 2}, Lemma: "haus"}},
		"reversed span":   {{Occurrence: OccurrenceIdentity{SourceDocumentID: "book", StartOffset: 2, EndOffset: 1}, Lemma: "haus"}},
		"unknown span":    {{Occurrence: OccurrenceIdentity{SourceDocumentID: "book", StartOffset: 20, EndOffset: 21}, Lemma: "haus"}},
		"duplicate exact": {{Occurrence: occurrence, Lemma: "haus"}, {Occurrence: occurrence, Excluded: true}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Project(analyzer.Result{Language: "de"}, DefaultConfig("corpus"), decisions)
			assert.ErrorIs(t, err, ErrInvalidDecision)
		})
	}
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

func TestDefaultRulesExcludeRecoveredAdvmodParticles(t *testing.T) {
	store := &memoryStore{known: map[string]bool{}}
	corpus, err := analyzer.FromProto(&mouseionv1.NormalizedCorpus{
		SchemaVersion: "1.1.0",
		Language:      "de",
		Sentences: []*mouseionv1.Sentence{{
			Text: "Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.",
			Tokens: []*mouseionv1.Token{
				{Surface: "Großmutter", CanonicalLemma: "großmutter", Pos: "NOUN", Dependency: "nsubj", Head: 1},
				{Surface: "sieht", CanonicalLemma: "fernsehen", Pos: "VERB", Dependency: "root", Head: 1},
				{Surface: "im", CanonicalLemma: "in", Pos: "ADP", Dependency: "case", Head: 3},
				{Surface: "Wohnzimmer", CanonicalLemma: "wohnzimmer", Pos: "NOUN", Dependency: "obl", Head: 1},
				{Surface: "fern", CanonicalLemma: "fern", Pos: "ADV", Dependency: "advmod", Head: 1},
				{Surface: "ohne", CanonicalLemma: "ohne", Pos: "ADP", Dependency: "case", Head: 6},
				{Surface: "Bild", CanonicalLemma: "bild", Pos: "NOUN", Dependency: "obl", Head: 1},
				{Surface: "und", CanonicalLemma: "und", Pos: "CCONJ", Dependency: "cc", Head: 6},
				{Surface: "Ton", CanonicalLemma: "ton", Pos: "NOUN", Dependency: "conj", Head: 6},
			},
		}},
	})
	require.NoError(t, err)

	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("corpus-1"))

	require.NoError(t, err)
	require.Len(t, got, 5)
	assert.Equal(t, "bild", got[0].Identity.CanonicalLemma)
	assert.Equal(t, "fernsehen", got[1].Identity.CanonicalLemma)
	assert.Equal(t, "großmutter", got[2].Identity.CanonicalLemma)
	assert.Equal(t, "ton", got[3].Identity.CanonicalLemma)
	assert.Equal(t, "wohnzimmer", got[4].Identity.CanonicalLemma)
	for _, candidate := range got {
		assert.NotEqual(t, "fern", candidate.Identity.CanonicalLemma, "a recovered particle is not its own candidate")
	}
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
		{Identity: Identity{"it", "acqua", "NOUN", ""}, OccurrenceCount: 1},
		{Identity: Identity{"it", "bere", "VERB", ""}, OccurrenceCount: 2},
		{Identity: Identity{"it", "dare", "VERB", ""}, OccurrenceCount: 1},
		{Identity: Identity{"it", "uomo", "NOUN", ""}, OccurrenceCount: 2},
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

func TestBookDeckEligibilityAcrossBooksBoundary(t *testing.T) {
	eligibility := NewEligibility(nil, nil)
	allows := func(inBook int, across int64) bool {
		return eligibility.AllowsBookDeckCandidate(domain.SelectionCandidate{
			Language: "de", CanonicalLemma: "wort", UPOS: "NOUN", OccurrenceCount: inBook,
		}, across)
	}
	assert.True(t, allows(3, 3), "three in-Book occurrences qualify without extra evidence")
	assert.False(t, allows(2, 14), "fourteen across Books does not meet the threshold")
	assert.True(t, allows(2, 15), "fifteen across Books qualifies")
	assert.False(t, allows(1, 14), "a single occurrence needs the same across-Books threshold")
	assert.True(t, allows(1, 15), "a single occurrence qualifies with fifteen across Books")
	assert.False(t, allows(0, 100), "an identity absent from the Book never qualifies")
	assert.True(t, allows(4, 0), "existing three-plus eligibility is unchanged")

	known := NewEligibility([]domain.KnownVocabulary{{Language: "de", CanonicalLemma: "wort", UPOS: ""}}, nil)
	assert.False(t, known.AllowsBookDeckCandidate(domain.SelectionCandidate{
		Language: "de", CanonicalLemma: "wort", UPOS: "NOUN", OccurrenceCount: 2,
	}, 15), "lemma-wide Known vocabulary excludes every POS")
	reserved := NewEligibility(nil, []domain.DeckPreparationVocabulary{{Language: "de", CanonicalLemma: "wort", UPOS: "NOUN"}})
	assert.False(t, reserved.AllowsBookDeckCandidate(domain.SelectionCandidate{
		Language: "de", CanonicalLemma: "wort", UPOS: "NOUN", OccurrenceCount: 2,
	}, 15), "reserved identities remain excluded")
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

// frenchFixture builds a French corpus, one sentence per token, so aggregation
// is deterministic and language-scoped.
func frenchFixture(tokens ...analyzer.Token) analyzer.Result {
	sentences := make([]analyzer.Sentence, len(tokens))
	for i, t := range tokens {
		sentences[i] = analyzer.Sentence{Text: t.Surface, Tokens: []analyzer.Token{t}, Location: analyzer.SourceLocation{SourceDocumentID: "livre"}}
	}
	return analyzer.Result{Language: "fr", Sentences: sentences}
}

func TestFrenchFixtureFiltersAndAggregatesVocabulary(t *testing.T) {
	corpus := frenchFixture(
		tok("L'", "le", "DET"),
		tok("homme", "homme", "NOUN"),
		tok("d'", "de", "ADP"),
		tok("accord", "accord", "NOUN"),
		tok("qu'", "que", "PRON"),
		tok("s'", "se", "PRON"),
		tok("habille", "habiller", "VERB"),
		tok("va", "aller", "VERB"),
		tok("t", "il", "PRON"),
		tok("ne", "ne", "ADV"),
		tok("pas", "pas", "ADV"),
		tok("pas", "pas", "NOUN"),
		tok("près", "près", "ADV"),
		tok("là", "là", "ADV"),
		tok("où", "où", "ADV"),
		tok("au", "à", "ADP"),
		tok("au", "le", "DET"),
		tok("du", "de", "ADP"),
		tok("du", "le", "DET"),
		tok("des", "de", "ADP"),
		tok("des", "les", "DET"),
		tok("aux", "à", "ADP"),
		tok("aux", "les", "DET"),
	)
	store := &memoryStore{known: map[string]bool{}, reserved: map[string]bool{}}
	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("french-corpus"))
	require.NoError(t, err)

	counts := map[string]int{}
	upos := map[string]string{}
	for _, candidate := range got {
		counts[candidate.Identity.CanonicalLemma] = candidate.OccurrenceCount
		upos[candidate.Identity.CanonicalLemma] = candidate.Identity.UPOS
	}
	assert.Equal(t, map[string]int{
		"accord": 1, "aller": 1, "habiller": 1, "homme": 1,
		"pas": 1, "près": 1, "là": 1, "où": 1,
	}, counts)
	assert.Equal(t, "NOUN", upos["pas"], "NOUN pas stays eligible")
	for _, candidate := range got {
		if candidate.Identity.UPOS == "ADV" {
			assert.NotContains(t, []string{"ne", "pas"}, candidate.Identity.CanonicalLemma, "ADV negation never becomes vocabulary")
		}
	}
	for _, lemma := range []string{"le", "les", "de", "que", "se", "il", "à"} {
		assert.NotContains(t, counts, lemma, "article, preposition and pronoun components are not content words")
	}
}

func TestOccurrenceEligibleExcludesFrenchNegationAdverbs(t *testing.T) {
	cfg := DefaultConfig("")
	assert.False(t, OccurrenceEligible(cfg, "fr", "ne", "ADV", "advmod"))
	assert.False(t, OccurrenceEligible(cfg, "fr-FR", "pas", "ADV", "advmod"))
	assert.True(t, OccurrenceEligible(cfg, "fr", "pas", "NOUN", "obj"), "NOUN pas stays eligible")
	assert.True(t, OccurrenceEligible(cfg, "fr", "près", "ADV", "advmod"))
	assert.True(t, OccurrenceEligible(cfg, "fr", "où", "ADV", "advmod"))
	assert.True(t, OccurrenceEligible(cfg, "de", "pas", "ADV", "advmod"), "other languages are unaffected")
}

func TestFrenchBaseLemmaGroupsPronominalAndPlainUses(t *testing.T) {
	corpus := frenchFixture(
		tok("lave", "laver", "VERB"),
		tok("me", "moi", "PRON"),
		tok("lave", "laver", "VERB"),
		tok("s'", "se", "PRON"),
		tok("en", "en", "PRON"),
		tok("va", "aller", "VERB"),
	)
	store := &memoryStore{known: map[string]bool{}, reserved: map[string]bool{}}
	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("french-corpus"))
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "aller", got[0].Identity.CanonicalLemma)
	assert.Equal(t, 1, got[0].OccurrenceCount)
	assert.Equal(t, "laver", got[1].Identity.CanonicalLemma)
	assert.Equal(t, 2, got[1].OccurrenceCount, "pronominal and plain uses share the base verb identity")
	for _, candidate := range got {
		assert.Empty(t, candidate.Identity.LexicalDiscriminator, "base-lemma grouping carries no discriminator")
	}
}

func TestFrenchKnownAllerCoversReflexiveUse(t *testing.T) {
	store := &memoryStore{
		known:    map[string]bool{"alice/fr/aller/VERB": true},
		reserved: map[string]bool{},
	}
	corpus := frenchFixture(tok("va", "aller", "VERB"))

	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("french-corpus"))

	require.NoError(t, err)
	assert.Empty(t, got, "Known aller covers s'en aller")
}

func TestFrenchNegationAdverbsStayOutOfTheCoverageDenominator(t *testing.T) {
	corpus := frenchFixture(tok("homme", "homme", "NOUN"), tok("ne", "ne", "ADV"), tok("pas", "pas", "ADV"))

	statistics := AnalyzableStatistics(corpus, DefaultConfig("french-corpus"))

	assert.Equal(t, int64(1), statistics.AnalyzableTokenCount, "excluded negation does not inflate the denominator")
	assert.Equal(t, int64(1), statistics.DistinctLemmaCount)
}

func TestFrenchVocabularyIsOwnerAndLanguageScoped(t *testing.T) {
	store := &memoryStore{
		known: map[string]bool{
			"alice/de/aller/VERB": true,
			"bob/fr/aller/VERB":   true,
		},
		reserved: map[string]bool{"alice/it/aller/VERB": true},
	}
	corpus := frenchFixture(tok("va", "aller", "VERB"))

	got, err := NewService(store).Select(context.Background(), "alice", corpus, DefaultConfig("french-corpus"))

	require.NoError(t, err)
	require.Len(t, got, 1, "another owner's or language's vocabulary must not exclude a French identity")
	assert.Equal(t, "aller", got[0].Identity.CanonicalLemma)
}

func TestInvalidConfig(t *testing.T) {
	_, err := NewService(&memoryStore{}).Select(context.Background(), "", analyzer.Result{}, SelectionConfig{})
	assert.ErrorIs(t, err, ErrInvalidConfig)
}

func TestProjectRejectsInvalidConfig(t *testing.T) {
	allowed := map[string]bool{"NOUN": true}
	for name, test := range map[string]struct {
		corpus analyzer.Result
		config SelectionConfig
	}{
		"zero minimum":     {analyzer.Result{Language: "de"}, SelectionConfig{MinOccurrences: 0, AllowedPOS: allowed}},
		"no allowed parts": {analyzer.Result{Language: "de"}, SelectionConfig{MinOccurrences: 1}},
		"missing language": {analyzer.Result{}, SelectionConfig{MinOccurrences: 1, AllowedPOS: allowed}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Project(test.corpus, test.config, nil)
			assert.ErrorIs(t, err, ErrInvalidConfig)
		})
	}
}

// TestProjectWithholdsOnlyStudyContextOverlappingSpeakerLabels holds ADR 0103's
// sentence and vocabulary rules: a label-overlapping token is structural
// non-vocabulary, while other eligible words in the sentence still count and may
// be carded from another acceptable occurrence. Analyzer sentence boundaries are
// untouched.
func TestProjectWithholdsOnlyStudyContextOverlappingSpeakerLabels(t *testing.T) {
	unit := "epub-unit-v1:0:chapter"
	labeled := analyzer.Sentence{
		Text:     "Valentin fällt, das Buch liegt hier.",
		Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 0, EndOffset: 36},
		Tokens: []analyzer.Token{
			{Surface: "Valentin", CanonicalLemma: "Valentin", UPOS: "PROPN", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 0, EndOffset: 8}},
			{Surface: "fällt", CanonicalLemma: "fallen", UPOS: "VERB", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 9, EndOffset: 14}},
			{Surface: "Buch", CanonicalLemma: "Buch", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 20, EndOffset: 24}},
			{Surface: "liegt", CanonicalLemma: "liegen", UPOS: "VERB", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 25, EndOffset: 30}},
		},
	}
	// The second sentence shares the same source line (only a space separates
	// them) but is a standalone direction, not a speaker label.
	direction := analyzer.Sentence{
		Text:     "Das Buch liegt hier.",
		Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 37, EndOffset: 57},
		Tokens: []analyzer.Token{
			{Surface: "Buch", CanonicalLemma: "Buch", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 41, EndOffset: 45}},
			{Surface: "liegt", CanonicalLemma: "liegen", UPOS: "VERB", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 46, EndOffset: 51}},
		},
	}
	corpus := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{labeled, direction}}
	structure := domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet, RoleSpans: []domain.UnitRoleSpan{
		{UnitID: unit, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 15},
		{UnitID: unit, Role: domain.SourceStructureRoleDirection, StartOffset: 37, EndOffset: 57},
	}}
	config := DefaultConfig("corpus")
	config.Structure = structure

	projected, err := Project(corpus, config, nil)
	require.NoError(t, err)
	byLemma := map[string]Candidate{}
	for _, candidate := range projected {
		byLemma[candidate.Identity.CanonicalLemma] = candidate
	}
	require.Contains(t, byLemma, "Buch")
	assert.Equal(t, 2, byLemma["Buch"].OccurrenceCount, "an eligible direction word in a labeled sentence still counts")
	require.Len(t, byLemma["Buch"].SentenceReferences, 1, "only the acceptable occurrence supplies context")
	assert.Equal(t, direction.Text, byLemma["Buch"].SentenceReferences[0].Text)
	assert.Equal(t, 2, byLemma["liegen"].OccurrenceCount)
	require.Len(t, byLemma["liegen"].SentenceReferences, 1)
	assert.Equal(t, direction.Text, byLemma["liegen"].SentenceReferences[0].Text)
	assert.NotContains(t, byLemma, "fallen", "a label-overlapping token is structural non-vocabulary")

	// Without recorded structure (v1), the same corpus behaves exactly as
	// today: no restriction can be derived.
	v1, err := Project(corpus, DefaultConfig("corpus"), nil)
	require.NoError(t, err)
	for _, candidate := range v1 {
		switch candidate.Identity.CanonicalLemma {
		case "fallen":
			assert.Len(t, candidate.SentenceReferences, 1)
		case "Buch":
			assert.Len(t, candidate.SentenceReferences, 2)
		}
	}
}

// TestProjectKeepsAnExplicitEmptyReferenceListForLabelOnlyContext proves a
// candidate can count an eligible direction word while its only sentence
// overlaps a speaker label. The empty reference list is persisted as [] rather
// than JSON null, so preparation fact loaders can expand it unconditionally.
func TestProjectKeepsAnExplicitEmptyReferenceListForLabelOnlyContext(t *testing.T) {
	const unit = "epub-unit-v1:0:mixed"
	const text = "Mephistopheles mit seltsamen Geberden."
	labelIndex := strings.Index(text, "Mephistopheles")
	require.GreaterOrEqual(t, labelIndex, 0)
	labelStart, err := checked.Uint64FromInt(utf8.RuneCountInString(text[:labelIndex]))
	require.NoError(t, err)
	labelLength, err := checked.Uint64FromInt(utf8.RuneCountInString("Mephistopheles"))
	require.NoError(t, err)
	labelEnd := labelStart + labelLength
	wordIndex := strings.Index(text, "Geberden")
	require.GreaterOrEqual(t, wordIndex, 0)
	wordStart, err := checked.Uint64FromInt(utf8.RuneCountInString(text[:wordIndex]))
	require.NoError(t, err)
	wordLength, err := checked.Uint64FromInt(utf8.RuneCountInString("Geberden"))
	require.NoError(t, err)
	textLength, err := checked.Uint64FromInt(len([]rune(text)))
	require.NoError(t, err)
	sentence := analyzer.Sentence{
		Text:     text,
		Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: 0, EndOffset: textLength},
		Tokens: []analyzer.Token{
			{Surface: "Mephistopheles", CanonicalLemma: "Mephistopheles", UPOS: "PROPN", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: labelStart, EndOffset: labelEnd}},
			{Surface: "Geberden", CanonicalLemma: "Geberde", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: wordStart, EndOffset: wordStart + wordLength}},
		},
	}
	corpus := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{sentence}}
	config := DefaultConfig("corpus")
	config.Structure = domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet, RoleSpans: []domain.UnitRoleSpan{
		{UnitID: unit, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 14},
	}}

	projected, err := Project(corpus, config, nil)
	require.NoError(t, err)
	require.Len(t, projected, 1)
	assert.Equal(t, "Geberde", projected[0].Identity.CanonicalLemma)
	assert.Equal(t, 1, projected[0].OccurrenceCount)
	assert.NotNil(t, projected[0].SentenceReferences)
	assert.Empty(t, projected[0].SentenceReferences)
	encoded, err := json.Marshal(projected[0].SentenceReferences)
	require.NoError(t, err)
	assert.JSONEq(t, "[]", string(encoded))
}
