package ancientgreek

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLatinEvidenceListsSentenceCitationsAndOccurrences(t *testing.T) {
	conllu, paula := olaFixture(latinSentences())
	source, err := ParseLatinManifestSource(conllu, paula)
	require.NoError(t, err)
	evidence, err := source.LatinEvidence()
	require.NoError(t, err)

	assert.Equal(t, [][]string{{"1.pr.1"}, {"1.1a", "1.2"}}, evidence.SentenceCitations)
	require.Len(t, evidence.Occurrences, 14)
	assert.Equal(t, LatinOccurrence{CanonicalLemma: "gallia", Headword: "Gallia", UPOS: "PROPN", Dependency: "SBJ"}, evidence.Occurrences[0])
	assert.Equal(t, LatinOccurrence{CanonicalLemma: "incolo", Headword: "incolo", UPOS: "VERB", Dependency: "PRED"}, evidence.Occurrences[7])
}

func TestLatinEvidenceRefusesAGreekWork(t *testing.T) {
	_, err := (&ManifestSource{}).LatinEvidence()
	require.Error(t, err)
}
