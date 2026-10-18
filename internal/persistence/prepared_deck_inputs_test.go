package persistence

import (
	"testing"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAnalysisFromTokenEvidenceKeepsStoredSentenceNumbers pins ADR 0106's freeze
// evidence rule at the loader seam: a sentence the corpus holds without tokens
// keeps its number, so a later token's slice index is its stored ordinal.
func TestAnalysisFromTokenEvidenceKeepsStoredSentenceNumbers(t *testing.T) {
	analysis, err := analysisFromTokenEvidence([]sqlcgen.ListAnalysisTokenEvidenceRow{
		{Language: "de", SentenceOrdinal: 0, TokenOrdinal: 0, Surface: "Haus", CanonicalLemma: "haus", Upos: "NOUN", Morphology: []byte(`{}`), UnitID: "unit", SentenceText: "Das Haus steht."},
		{Language: "de", SentenceOrdinal: 2, TokenOrdinal: 0, Surface: "geht", CanonicalLemma: "gehen", Upos: "VERB", Morphology: []byte(`{}`), UnitID: "unit", SentenceText: "Er geht nach Hause."},
	})
	require.NoError(t, err)
	require.Len(t, analysis.Sentences, 3)
	assert.Equal(t, "Das Haus steht.", analysis.Sentences[0].Text)
	assert.Empty(t, analysis.Sentences[1].Text, "the tokenless sentence keeps its number")
	assert.Empty(t, analysis.Sentences[1].Tokens)
	assert.Equal(t, "Er geht nach Hause.", analysis.Sentences[2].Text, "a later token keeps its stored sentence ordinal")
	require.Len(t, analysis.Sentences[2].Tokens, 1)
}

// TestAnalysisFromTokenEvidenceRejectsNegativeSentenceOrdinal guards the padding
// against a malformed persisted ordinal.
func TestAnalysisFromTokenEvidenceRejectsNegativeSentenceOrdinal(t *testing.T) {
	_, err := analysisFromTokenEvidence([]sqlcgen.ListAnalysisTokenEvidenceRow{
		{Language: "de", SentenceOrdinal: -1, TokenOrdinal: 0, Surface: "Haus", CanonicalLemma: "haus", Upos: "NOUN", Morphology: []byte(`{}`)},
	})
	require.ErrorContains(t, err, "invalid sentence ordinal")
}
