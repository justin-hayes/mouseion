package cardexport

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Homographs that share a lemma and part of speech must produce separate cards,
// separate deduplication keys and separate exported identities.
func TestHomographsProduceSeparateCards(t *testing.T) {
	entries := []RenderInput{
		{
			Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB", LexicalDiscriminator: "morpheus:1", Headword: "δέω",
			Sentence: "ἔδει γὰρ αὐτὸν δῆσαι τὸν ἵππον πρὸς τὸ δένδρον.", TargetWord: "δῆσαι",
			Translation: "he had to bind the horse", SentenceTranslation: "For he had to bind the horse to the tree.",
			SourceDocument: "Anabasis", FirstEncounter: 10,
		},
		{
			Language: "grc", CanonicalLemma: "δέω", UPOS: "VERB", LexicalDiscriminator: "morpheus:2", Headword: "δέω",
			Sentence: "οὐδὲν γὰρ αὐτῷ τῶν ἐπιτηδείων ἔδει πρὸς τὸν πόλεμον.", TargetWord: "ἔδει",
			Translation: "he lacked nothing", SentenceTranslation: "For he lacked none of the supplies for the war.",
			SourceDocument: "Anabasis", FirstEncounter: 40,
		},
	}
	artifact, err := renderAccepted(t.Context(), "alice", "Anabasis", entries, nil)
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 2)
	assert.NotEqual(t, artifact.Generated[0].Note.Key, artifact.Generated[1].Note.Key)
	assert.NotEqual(t, artifact.Generated[0].Note.Identity, artifact.Generated[1].Note.Identity)
}

func TestDedupKeyIncludesDiscriminatorOnlyWhenPresent(t *testing.T) {
	assert.Equal(t, DedupKey("grc", "δέω", "VERB", "alice"), DedupKeyWithDiscriminator("grc", "δέω", "VERB", "", "alice"))
	assert.NotEqual(t,
		DedupKeyWithDiscriminator("grc", "δέω", "VERB", "morpheus:1", "alice"),
		DedupKeyWithDiscriminator("grc", "δέω", "VERB", "morpheus:2", "alice"),
	)
}

// An imported headword is shown to the learner instead of the internal key.
func TestImportedHeadwordIsDisplayed(t *testing.T) {
	note, err := makeNote("alice", RenderInput{
		Language: "grc", CanonicalLemma: "λόγοσ", UPOS: "NOUN", LexicalDiscriminator: "morpheus:1", Headword: "λόγος",
		Sentence: "ἐν ἀρχῇ ἦν ὁ λόγος καὶ ὁ λόγος ἦν πρὸς τὸν θεόν.", TargetWord: "λόγος",
	})
	require.NoError(t, err)
	assert.Equal(t, "λόγος", note.Lemma)
	assert.NotContains(t, note.Lemma, "λόγοσ")
}

// Export of an Ancient Greek deck carries the source and dictionary notices.
func TestAncientGreekExportCarriesAttributionAndLicenceNotices(t *testing.T) {
	entries := []RenderInput{{
		Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", Headword: "λόγος",
		Sentence: "ἐν ἀρχῇ ἦν ὁ λόγος καὶ ὁ λόγος ἦν πρὸς τὸν θεόν.", TargetWord: "λόγος",
		DictionaryProviderVersion: "kaikki-2026-09", CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "wikt:1", Gloss: "word"}},
	}}
	attribution := attributionNotices(entries)
	for _, want := range []string{"OGA", "Perseus/canonical-greekLit", "oga_src", "Wiktionary"} {
		assert.Contains(t, attribution, want)
	}
	assert.Contains(t, deckDescription(entries), "Wiktionary")
}
