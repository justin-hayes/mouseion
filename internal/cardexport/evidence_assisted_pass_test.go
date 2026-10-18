package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func evidenceIDs(senses []enrichment.LexicalSense) []string {
	ids := make([]string, 0, len(senses))
	for _, sense := range senses {
		ids = append(ids, sense.EvidenceID)
	}
	return ids
}

// evidenceAssistedPassDeck freezes one Ancient Greek evidence-assisted pass
// projection and returns its frozen deck, work item and run facts.
func evidenceAssistedPassDeck(t *testing.T) (FrozenDeck, WorkItem, RunFacts) {
	t.Helper()
	projections := []CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Anabasis",
		Candidate: domain.SelectionCandidate{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1"},
		Entry: Entry{Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1", Headword: "λόγος",
			Sentence: "ἐν ἀρχῇ ἦν ὁ λόγος καὶ ὁ λόγος ἦν πρὸς τὸν θεόν.", TargetWord: "λόγος",
			PassageContext: "In the beginning was the Word.", PassageCitation: "1.1.1", BrownsonPin: "brownson-pin"},
		Provider: "openai-compatible", ProviderVersion: "model/translation-v16-evidence-assisted-greek",
		TargetLanguage: "en", SkipMeaningEvidence: true, EvidenceAssistedPass: true,
	}}
	deck, _, err := NewPresentation(&lsjStub{found: true, entry: lsjMatchedEntry()}).Freeze(t.Context(), "owner-1", "Anabasis", projections)
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	require.True(t, work[0].Request.RequireEvidenceAssistedPass, "the frozen request uses the evidence-assisted pass contract")
	require.False(t, work[0].Request.RequireContextualGloss)
	require.Equal(t, "In the beginning was the Word.", work[0].Request.PassageContext)
	assert.Equal(t, []string{"n61607.0", "n61607.1", "n61607.2"}, evidenceIDs(work[0].Request.CandidateSenses), "the pass transmits the pinned LSJ senses with their native IDs")
	facts := RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "openai-compatible", ProviderVersion: "model/translation-v16-evidence-assisted-greek"}
	return deck, work[0], facts
}

// TestEvidenceAssistedPassLabelsProviderProposedFields proves a resolved pass
// answer adds a labelled contextual Gloss and sentence translation on top of the
// source-evidence card.
func TestEvidenceAssistedPassLabelsProviderProposedFields(t *testing.T) {
	deck, work, facts := evidenceAssistedPassDeck(t)
	record := enrichment.CacheEntry{
		CacheKey: work.CacheKey, Translation: "word", FallbackGloss: "word",
		SentenceTranslation: "In the beginning was the word.", SentenceTranslationTargets: []string{"word"},
	}
	artifact, diagnostics, err := NewPresentation(&lsjStub{found: true, entry: lsjMatchedEntry()}).Finalize(t.Context(), deck, []StoredResult{{CacheKey: work.CacheKey, Record: record}}, facts)
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)

	note := artifact.Generated[0].Note
	assert.Equal(t, "Provider-proposed: word", note.Gloss)
	assert.Contains(t, note.EnglishSentence, "Provider-proposed: ")
	assert.Contains(t, note.EnglishSentence, "<b>word</b>", "the English target alignment is kept")
	assert.Empty(t, note.English, "the pass never adds a lemma English to a Greek card")
	assert.Contains(t, note.DictionaryEntry, "lsj-gloss", "the full LSJ entry is still shown")
	assert.Empty(t, diagnostics.MeaningOmissions)
}

// TestEvidenceAssistedPassAbstentionKeepsDefaultCard proves a failed or
// abstaining answer leaves the source-evidence card unchanged and produces no
// meaning or provider omission.
func TestEvidenceAssistedPassAbstentionKeepsDefaultCard(t *testing.T) {
	deck, work, facts := evidenceAssistedPassDeck(t)
	omitted := StoredResult{CacheKey: work.CacheKey, OmissionReason: domain.DeckPreparationEvidenceAssistedPassAbstentionReason}
	artifact, diagnostics, err := NewPresentation(&lsjStub{found: true, entry: lsjMatchedEntry()}).Finalize(t.Context(), deck, []StoredResult{omitted}, facts)
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1, "the default card survives an abstention")

	note := artifact.Generated[0].Note
	assert.Empty(t, note.Gloss, "an abstaining pass adds no gloss")
	assert.Empty(t, note.EnglishSentence, "an abstaining pass adds no sentence translation")
	assert.Contains(t, note.DictionaryEntry, "lsj-gloss", "the default card still shows its source evidence")
	assert.Empty(t, diagnostics.MeaningOmissions, "an abstaining pass produces no meaning omission")
}

// TestEvidenceAssistedPassMissingResultKeepsDefaultCard proves a missing provider
// result (for example a failed call) also leaves the default card.
func TestEvidenceAssistedPassMissingResultKeepsDefaultCard(t *testing.T) {
	deck, _, facts := evidenceAssistedPassDeck(t)
	artifact, diagnostics, err := NewPresentation(&lsjStub{found: true, entry: lsjMatchedEntry()}).Finalize(t.Context(), deck, nil, facts)
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)
	assert.Empty(t, artifact.Generated[0].Note.Gloss)
	assert.Empty(t, diagnostics.MeaningOmissions)
}
