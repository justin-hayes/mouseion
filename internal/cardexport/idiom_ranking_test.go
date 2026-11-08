package cardexport

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type idiomLookupStub struct {
	entries map[string][]idiom.Entry
	err     error
	calls   []string
}

func (s *idiomLookupStub) IdiomEntries(_ context.Context, _, word string) ([]idiom.Entry, error) {
	s.calls = append(s.calls, word)
	return s.entries[word], s.err
}

func testIdiomEntry() idiom.Entry {
	return idiom.Entry{Language: "it", Headword: "in gamba", UPOS: "ADJ", Words: []string{"in", "gamba"}, Source: "wiktionary", Version: "fixture-v1"}
}

func gambaCandidate() domain.SelectionCandidate {
	return domain.SelectionCandidate{Language: "it", CanonicalLemma: "gamba", UPOS: "NOUN", ObservedForms: []byte(`["gamba"]`)}
}

func gambaTokens() []analyzer.Token {
	return []analyzer.Token{
		{Surface: "un", CanonicalLemma: "un", Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 2}},
		{Surface: "ragazzo", CanonicalLemma: "ragazzo", Location: analyzer.SourceLocation{StartOffset: 3, EndOffset: 10}},
		{Surface: "in", CanonicalLemma: "in", Location: analyzer.SourceLocation{StartOffset: 11, EndOffset: 13}},
		{Surface: "gamba", CanonicalLemma: "gamba", Location: analyzer.SourceLocation{StartOffset: 14, EndOffset: 19}},
	}
}

func acceptedGambaEvidence(sentence string, tokens []analyzer.Token) sentencequality.SentenceEvidence {
	return sentencequality.SentenceEvidence{Sentence: sentence, Target: "gamba", Tokens: tokens, Quality: sentencequality.SentenceQuality{Accepted: true}}
}

// TestIdiomSentencePreferenceDetectsTheTargetInsideACandidate pins the leading
// rule at the seam, including the literal sentence it prefers.
func TestIdiomSentencePreferenceDetectsTheTargetInsideACandidate(t *testing.T) {
	lookup := &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}
	prefer, err := idiomSentencePreferenceFor(t.Context(), lookup, map[string]bool{"it": true}, gambaCandidate())
	require.NoError(t, err)

	t.Run("the idiom sentence is not preferred", func(t *testing.T) {
		require.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("the literal sentence is preferred", func(t *testing.T) {
		require.True(t, prefer(acceptedGambaEvidence("la gamba", []analyzer.Token{
			{Surface: "la", CanonicalLemma: "la", Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 2}},
			{Surface: "gamba", CanonicalLemma: "gamba", Location: analyzer.SourceLocation{StartOffset: 3, EndOffset: 8}},
		})))
	})
}

// TestIdiomSentencePreferenceIsInertWithoutUsableInput pins every fallback that
// keeps the existing sentence choice: switch off, no provider, no entries,
// malformed forms, rejected evidence, and an unanchorable target.
func TestIdiomSentencePreferenceIsInertWithoutUsableInput(t *testing.T) {
	t.Run("switch off", func(t *testing.T) {
		lookup := &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}
		prefer, err := idiomSentencePreferenceFor(t.Context(), lookup, map[string]bool{"de": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
		assert.Empty(t, lookup.calls)
	})
	t.Run("no idiom provider", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), nil, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("no entries", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{}, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("malformed observed forms", func(t *testing.T) {
		candidate := gambaCandidate()
		candidate.ObservedForms = []byte(`{`)
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}, map[string]bool{"it": true}, candidate)
		require.NoError(t, err)
		assert.False(t, prefer(acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())))
	})
	t.Run("rejected evidence", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		evidence := acceptedGambaEvidence("un ragazzo in gamba", gambaTokens())
		evidence.Quality.Accepted = false
		assert.False(t, prefer(evidence))
	})
	t.Run("unanchorable target", func(t *testing.T) {
		prefer, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}, map[string]bool{"it": true}, gambaCandidate())
		require.NoError(t, err)
		assert.False(t, prefer(sentencequality.SentenceEvidence{Sentence: "un ragazzo in gamba", Target: "gamba", Quality: sentencequality.SentenceQuality{Accepted: true}}))
	})
}

// TestIdiomSentencePreferencePropagatesLookupFailure pins the durable error path.
func TestIdiomSentencePreferencePropagatesLookupFailure(t *testing.T) {
	_, err := idiomSentencePreferenceFor(t.Context(), &idiomLookupStub{err: errors.New("corrupt table")}, map[string]bool{"it": true}, gambaCandidate())
	require.Error(t, err)
	assert.ErrorContains(t, err, "corrupt table")
}

func TestCachedIdiomIndexLooksUpEachWordOnce(t *testing.T) {
	stub := &idiomLookupStub{entries: map[string][]idiom.Entry{"gamba": {testIdiomEntry()}}}
	cached := newCachedIdiomIndex(stub)
	for range 3 {
		entries, err := idiomEntriesForSentence(context.Background(), cached, "it", gambaTokens())
		require.NoError(t, err)
		require.Len(t, entries, 1)
	}
	require.Equal(t, []string{"un", "ragazzo", "in", "gamba"}, stub.calls, "each word reaches the index once per freeze")

	failing := newCachedIdiomIndex(&idiomLookupStub{err: errors.New("index closed")})
	_, err := failing.IdiomEntries(context.Background(), "it", "gamba")
	require.ErrorContains(t, err, "index closed")
}

// rankedIdiomSpec is one two-word idiom anchored on its first word. Entries
// lists the dictionary entries for the headword as (UPOS, source) pairs.
type rankedIdiomSpec struct {
	anchor, other string
	occurrences   int
	start         uint64
	entries       [][2]string
}

func rankedIdiomFixture(t *testing.T, specs []rankedIdiomSpec) ([]CandidateProjection, fakeIdiomLexicon) {
	t.Helper()
	lexicon := fakeIdiomLexicon{entries: make(map[string][]idiom.Entry)}
	projections := make([]CandidateProjection, 0, len(specs))
	for _, spec := range specs {
		headword := spec.anchor + " " + spec.other
		for _, pair := range spec.entries {
			entry := idiom.Entry{
				Language: "it", Headword: headword, UPOS: pair[0], Words: []string{spec.anchor, spec.other},
				Senses: []enrichment.LexicalSense{{EvidenceID: "e-" + spec.anchor, Gloss: "gloss for " + headword}},
				Source: pair[1], Version: "fixture-v1",
			}
			for _, word := range entry.Words {
				lexicon.entries["it\x00"+word] = append(lexicon.entries["it\x00"+word], entry)
			}
		}
		sentence, tokens := idiomBoundSentence(spec.start, spec.anchor, spec.other)
		observed, err := json.Marshal([]string{spec.anchor})
		require.NoError(t, err)
		projections = append(projections, CandidateProjection{
			OwnerID: "owner-1", DeckName: "Libro",
			Candidate: domain.SelectionCandidate{
				OwnerID: "owner-1", CorpusID: "corpus-1", Language: "it",
				CanonicalLemma: spec.anchor, UPOS: "NOUN", OccurrenceCount: spec.occurrences,
				FirstEncounter: int64(spec.start), ObservedForms: observed, //nolint:gosec // fixture offsets are small
			},
			Entry: Entry{
				OwnerID: "owner-1", Language: "it", CanonicalLemma: spec.anchor, UPOS: "NOUN",
				CorpusID: "corpus-1", SentenceOrdinal: 1, Sentence: sentence, TargetWord: spec.anchor,
				SentenceTokens: tokens, SentenceStartOffset: spec.start, FirstEncounter: int64(spec.start), //nolint:gosec // fixture offsets are small
				SourceDocument: "Libro",
			},
			Sentences: map[int64]analyzer.Sentence{1: {Text: sentence, Tokens: tokens, Location: analyzer.SourceLocation{StartOffset: spec.start}}},
			Provider:  "fixture", ProviderVersion: "lemma-v1", TargetLanguage: "en", RequireContextualGloss: true,
		})
	}
	return projections, lexicon
}

var (
	nounWiktionary = [2]string{"NOUN", "wiktionary"}
	advWiktionary  = [2]string{"ADV", "wiktionary"}
	noPOS          = [2]string{"", "wiktionary"}
	reviewed       = [2]string{"", "reviewed-idioms"}
)

// freezeRankedIdioms freezes the fixture under bound and returns the kept
// idiom headwords in card order and every idiom headword in bound order.
func freezeRankedIdioms(t *testing.T, specs []rankedIdiomSpec, bound int) (kept, ranked []string) {
	t.Helper()
	projections, lexicon := rankedIdiomFixture(t, specs)
	deck, diagnostics, err := NewPresentation(lexicon).Freeze(t.Context(), "owner-1", "Libro", projections,
		WithIdiomCards([]string{"it"}, "idiom-v1"), withIdiomCardBound(bound))
	require.NoError(t, err)
	for _, item := range deck.IdiomWork("it") {
		kept = append(kept, item.Headword)
	}
	for _, rank := range diagnostics.IdiomRanking {
		ranked = append(ranked, rank.Headword)
	}
	return kept, ranked
}

func TestFreezeRanksNounOnlyIdiomsAfterEveryOtherIdiom(t *testing.T) {
	kept, _ := freezeRankedIdioms(t, []rankedIdiomSpec{
		{anchor: "sistema", other: "solare", occurrences: 40, start: 100, entries: [][2]string{nounWiktionary}},
		{anchor: "acqua", other: "mare", occurrences: 30, start: 200, entries: [][2]string{nounWiktionary}},
		{anchor: "via", other: "di", occurrences: 2, start: 300, entries: [][2]string{advWiktionary}},
		{anchor: "anche", other: "se", occurrences: 1, start: 400, entries: [][2]string{advWiktionary}},
	}, 3)

	require.ElementsMatch(t, []string{"via di", "anche se", "sistema solare"}, kept)
}

func TestFreezeKeepsUnclearPartsOfSpeechInTheFirstTier(t *testing.T) {
	kept, _ := freezeRankedIdioms(t, []rankedIdiomSpec{
		{anchor: "sistema", other: "solare", occurrences: 40, start: 100, entries: [][2]string{nounWiktionary}},
		{anchor: "su", other: "giù", occurrences: 1, start: 200, entries: [][2]string{advWiktionary, nounWiktionary}},
		{anchor: "in", other: "disparte", occurrences: 1, start: 300, entries: [][2]string{noPOS}},
		{anchor: "kano", other: "matia", occurrences: 1, start: 400, entries: [][2]string{nounWiktionary, reviewed}},
	}, 3)

	require.ElementsMatch(t, []string{"su giù", "in disparte", "kano matia"}, kept)
}

func TestFreezeTiersDecideOnlyWhichIdiomsAreKept(t *testing.T) {
	kept, ranked := freezeRankedIdioms(t, []rankedIdiomSpec{
		{anchor: "acqua", other: "mare", occurrences: 3, start: 200, entries: [][2]string{nounWiktionary}},
		{anchor: "sistema", other: "solare", occurrences: 3, start: 100, entries: [][2]string{nounWiktionary}},
		{anchor: "via", other: "di", occurrences: 2, start: 300, entries: [][2]string{advWiktionary}},
		{anchor: "anche", other: "se", occurrences: 5, start: 400, entries: [][2]string{advWiktionary}},
	}, 4)

	t.Run("each tier keeps the existing order", func(t *testing.T) {
		require.Equal(t, []string{"anche se", "via di", "sistema solare", "acqua mare"}, ranked)
	})
	t.Run("kept idioms keep the existing card order", func(t *testing.T) {
		require.Equal(t, []string{"anche se", "sistema solare", "acqua mare", "via di"}, kept)
	})
}

func TestNounOnlyCheckPropagatesIndexFailureAndIgnoresAnEmptyHeadword(t *testing.T) {
	_, err := idiomIsNounOnly(t.Context(), &idiomLookupStub{err: errors.New("index unavailable")}, "it", "in gamba")
	require.ErrorContains(t, err, "index unavailable")
	nounOnly, err := idiomIsNounOnly(t.Context(), &idiomLookupStub{}, "it", " ")
	require.NoError(t, err)
	require.False(t, nounOnly)
}

func TestFreezeDiagnosticsReportTheFullIdiomRanking(t *testing.T) {
	projections, lexicon := rankedIdiomFixture(t, []rankedIdiomSpec{
		{anchor: "sistema", other: "solare", occurrences: 40, start: 100, entries: [][2]string{nounWiktionary}},
		{anchor: "via", other: "di", occurrences: 2, start: 300, entries: [][2]string{advWiktionary}},
	})
	_, diagnostics, err := NewPresentation(lexicon).Freeze(t.Context(), "owner-1", "Libro", projections,
		WithIdiomCards([]string{"it"}, "idiom-v1"), withIdiomCardBound(1))
	require.NoError(t, err)

	require.Equal(t, []IdiomRank{
		{Language: "it", Headword: "via di", AnchorCanonicalLemma: "via", AnchorUPOS: "NOUN", AnchorOccurrences: 2},
		{Language: "it", Headword: "sistema solare", AnchorCanonicalLemma: "sistema", AnchorUPOS: "NOUN", AnchorOccurrences: 40, NounOnly: true},
	}, diagnostics.IdiomRanking)
	require.Equal(t, 1, diagnostics.IdiomOverflow)
}
