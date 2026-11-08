package cardexport

import (
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/stretchr/testify/require"
)

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

func keptIdiomHeadwords(t *testing.T, specs []rankedIdiomSpec, bound int) []string {
	t.Helper()
	projections, lexicon := rankedIdiomFixture(t, specs)
	deck, _, err := NewPresentation(lexicon).Freeze(t.Context(), "owner-1", "Libro", projections,
		WithIdiomCards([]string{"it"}, "idiom-v1"), withIdiomCardBound(bound))
	require.NoError(t, err)
	headwords := make([]string, 0, bound)
	for _, item := range deck.IdiomWork("it") {
		headwords = append(headwords, item.Headword)
	}
	return headwords
}

func TestFreezeRanksNounOnlyIdiomsAfterEveryOtherIdiom(t *testing.T) {
	headwords := keptIdiomHeadwords(t, []rankedIdiomSpec{
		{anchor: "sistema", other: "solare", occurrences: 40, start: 100, entries: [][2]string{nounWiktionary}},
		{anchor: "acqua", other: "mare", occurrences: 30, start: 200, entries: [][2]string{nounWiktionary}},
		{anchor: "via", other: "di", occurrences: 2, start: 300, entries: [][2]string{advWiktionary}},
		{anchor: "anche", other: "se", occurrences: 1, start: 400, entries: [][2]string{advWiktionary}},
	}, 3)

	require.Equal(t, []string{"via di", "anche se", "sistema solare"}, headwords)
}

func TestFreezeKeepsUnclearPartsOfSpeechInTheFirstTier(t *testing.T) {
	headwords := keptIdiomHeadwords(t, []rankedIdiomSpec{
		{anchor: "sistema", other: "solare", occurrences: 40, start: 100, entries: [][2]string{nounWiktionary}},
		{anchor: "su", other: "giù", occurrences: 1, start: 200, entries: [][2]string{advWiktionary, nounWiktionary}},
		{anchor: "in", other: "disparte", occurrences: 1, start: 300, entries: [][2]string{noPOS}},
		{anchor: "kano", other: "matia", occurrences: 1, start: 400, entries: [][2]string{nounWiktionary, reviewed}},
	}, 3)

	require.Equal(t, []string{"su giù", "in disparte", "kano matia"}, headwords)
}

func TestFreezeKeepsTheExistingOrderWithinEachTier(t *testing.T) {
	headwords := keptIdiomHeadwords(t, []rankedIdiomSpec{
		{anchor: "acqua", other: "mare", occurrences: 3, start: 200, entries: [][2]string{nounWiktionary}},
		{anchor: "sistema", other: "solare", occurrences: 3, start: 100, entries: [][2]string{nounWiktionary}},
		{anchor: "via", other: "di", occurrences: 2, start: 300, entries: [][2]string{advWiktionary}},
		{anchor: "anche", other: "se", occurrences: 5, start: 400, entries: [][2]string{advWiktionary}},
	}, 4)

	require.Equal(t, []string{"anche se", "via di", "sistema solare", "acqua mare"}, headwords)
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
		{Language: "it", Headword: "via di", AnchorCanonicalLemma: "via", AnchorUPOS: "NOUN", AnchorOccurrences: 2, Kept: true},
		{Language: "it", Headword: "sistema solare", AnchorCanonicalLemma: "sistema", AnchorUPOS: "NOUN", AnchorOccurrences: 40, NounOnly: true},
	}, diagnostics.IdiomRanking)
	require.Equal(t, 1, diagnostics.IdiomOverflow)
}
