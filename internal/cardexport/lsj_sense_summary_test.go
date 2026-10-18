package cardexport

import (
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testLSJSummaryProviderVersion = "model/translation-v17-lsj-sense-summary-greek"

// lsjSummaryDeck freezes one Ancient Greek pass projection whose LSJ entry is
// also offered a context-free sense summary.
func lsjSummaryDeck(t *testing.T) (FrozenDeck, WorkItem, RunFacts) {
	t.Helper()
	projections := []CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Anabasis",
		Candidate: domain.SelectionCandidate{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1"},
		Entry: Entry{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1", Headword: "λόγος",
			Sentence: "ἐν ἀρχῇ ἦν ὁ λόγος καὶ ὁ λόγος ἦν πρὸς τὸν θεόν.", TargetWord: "λόγος"},
		Provider: "openai-compatible", ProviderVersion: "model/translation-v16-evidence-assisted-greek",
		TargetLanguage: "en", SkipMeaningEvidence: true, EvidenceAssistedPass: true,
		LSJSummaryProviderVersion: testLSJSummaryProviderVersion,
	}}
	deck, _, err := NewPresentation(&lsjStub{found: true, entry: lsjMatchedEntry()}).Freeze(t.Context(), "owner-1", "Anabasis", projections)
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	facts := RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "openai-compatible", ProviderVersion: "model/translation-v16-evidence-assisted-greek"}
	return deck, work[0], facts
}

func lsjSummarySenseIDs(senses []enrichment.LSJSenseSummarySense) []string {
	ids := make([]string, 0, len(senses))
	for _, sense := range senses {
		ids = append(ids, sense.ID)
	}
	return ids
}

// TestLSJSenseSummaryFrozenRequestCarriesOnlyTheEntry proves the frozen summary
// request carries the pinned entry's headword and senses and nothing derived from
// the Book sentence, and that its cache identity is the (pin, entry ID, prompt
// version) triple.
func TestLSJSenseSummaryFrozenRequestCarriesOnlyTheEntry(t *testing.T) {
	_, work, _ := lsjSummaryDeck(t)

	require.True(t, work.SummaryRequest.RequireLSJSenseSummary)
	assert.Equal(t, "λόγος", work.SummaryRequest.Headword)
	assert.Empty(t, work.SummaryRequest.ExampleSentence, "the summary is written without the sentence")
	assert.Empty(t, work.SummaryRequest.PassageContext)
	assert.Empty(t, work.SummaryRequest.CandidateSenses)
	assert.Empty(t, work.SummaryRequest.TargetWord)
	assert.Equal(t, []string{"n61607.0", "n61607.1", "n61607.2"}, lsjSummarySenseIDs(work.SummaryRequest.LSJSenses))
	assert.Equal(t, enrichment.LSJSenseSummaryKey{Pin: testLSJPin, EntryID: "n61607", PromptVersion: testLSJSummaryProviderVersion}, work.SummaryKey)
}

// TestLSJSenseSummaryRendersAboveTheFullEntry proves an accepted summary is
// shown above the full LSJ entry, which is never replaced.
func TestLSJSenseSummaryRendersAboveTheFullEntry(t *testing.T) {
	deck, work, facts := lsjSummaryDeck(t)
	summary := enrichment.LSJSenseSummary{EntryID: "n61607", Pin: testLSJPin, Lines: []enrichment.LSJSenseSummaryLine{{SenseIDs: []string{"n61607.0"}, Text: "word or speech"}}}
	deck = deck.WithLSJSenseSummaries(map[enrichment.LSJSenseSummaryKey]enrichment.LSJSenseSummary{work.SummaryKey: summary})

	artifact, _, err := NewPresentation(&lsjStub{found: true, entry: lsjMatchedEntry()}).Finalize(t.Context(), deck, []StoredResult{{CacheKey: work.CacheKey, OmissionReason: domain.DeckPreparationEvidenceAssistedPassAbstentionReason}}, facts)
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)

	note := artifact.Generated[0].Note
	assert.Contains(t, note.LSJSenseSummary, lsjSenseSummaryLabel, "the summary is labelled provider-written")
	assert.Contains(t, note.LSJSenseSummary, "word or speech")
	assert.Less(t, strings.Index(note.LSJSenseSummary, lsjSenseSummaryLabel), strings.Index(note.LSJSenseSummary, "word or speech"), "the label precedes the summary lines")
	assert.Contains(t, note.DictionaryEntry, "lsj-gloss", "the full LSJ entry is still shown")
}

// TestWithLSJSenseSummariesDiscardsUnknownSenseID proves a summary that cites a
// sense ID that is not in the entry is discarded, so the card keeps its full
// entry and shows no summary.
func TestWithLSJSenseSummariesDiscardsUnknownSenseID(t *testing.T) {
	deck, work, facts := lsjSummaryDeck(t)
	summary := enrichment.LSJSenseSummary{EntryID: "n61607", Pin: testLSJPin, Lines: []enrichment.LSJSenseSummaryLine{{SenseIDs: []string{"n61607.missing"}, Text: "word or speech"}}}
	deck = deck.WithLSJSenseSummaries(map[enrichment.LSJSenseSummaryKey]enrichment.LSJSenseSummary{work.SummaryKey: summary})

	artifact, _, err := NewPresentation(&lsjStub{found: true, entry: lsjMatchedEntry()}).Finalize(t.Context(), deck, []StoredResult{{CacheKey: work.CacheKey, OmissionReason: domain.DeckPreparationEvidenceAssistedPassAbstentionReason}}, facts)
	require.NoError(t, err)
	note := artifact.Generated[0].Note
	assert.Empty(t, note.LSJSenseSummary, "an unknown sense ID discards the summary")
	assert.Contains(t, note.DictionaryEntry, "lsj-gloss", "the full entry survives the discarded summary")
}

// TestLSJSenseSummaryKeyIgnoresDeckAndSentence proves the summary cache identity
// is the pinned entry alone, so the same entry reuses one summary across decks
// and occurrences.
func TestLSJSenseSummaryKeyIgnoresDeckAndSentence(t *testing.T) {
	first := renderInputFromEntry(Entry{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", Headword: "λόγος", Sentence: "one λόγος.", DictionaryEntryID: "n61607", DictionaryPin: testLSJPin, LSJSummaryProviderVersion: testLSJSummaryProviderVersion, CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "n61607.0"}}})
	second := renderInputFromEntry(Entry{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", Headword: "λόγος", Sentence: "another λόγος entirely.", DictionaryEntryID: "n61607", DictionaryPin: testLSJPin, LSJSummaryProviderVersion: testLSJSummaryProviderVersion, CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "n61607.0"}}})
	assert.Equal(t, lsjSenseSummaryKey(first), lsjSenseSummaryKey(second))
	assert.NotEqual(t, enrichment.LSJSenseSummaryKey{}, lsjSenseSummaryKey(first))
}

// TestLSJSenseSummaryKeysNeedsAnOfferedSummary proves a deck only reports the
// per-entry summary keys it is offered: the pass deck without the summary
// assessment reports none.
func TestLSJSenseSummaryKeysNeedsAnOfferedSummary(t *testing.T) {
	deck, work, _ := lsjSummaryDeck(t)
	assert.Equal(t, []enrichment.LSJSenseSummaryKey{work.SummaryKey}, deck.LSJSenseSummaryKeys())

	passDeck, _, _ := evidenceAssistedPassDeck(t)
	assert.Empty(t, passDeck.LSJSenseSummaryKeys(), "a pass deck with no offered summary reports no keys")
}
