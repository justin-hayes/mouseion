package ancientgreek

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProposeUnitsSplitsByTopLevelCitation(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	units, err := ProposeUnits(data)
	require.NoError(t, err)
	require.Len(t, units, 2)
	assert.Equal(t, UnitRange{Start: "1.1.1", End: "1.1.2"}, units[0])
	assert.Equal(t, UnitRange{Start: "2.1.1", End: "2.1.1"}, units[1])
}

func TestUnitWordCountCountsPrintedWordsAndRefusesEmptyRanges(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	words, err := UnitWordCount(data, "1.1.1", "1.1.2")
	require.NoError(t, err)
	assert.Equal(t, 8, words, "punctuation and ellipsis tokens are not words")

	_, err = UnitWordCount(data, "3.1.1", "3.1.2")
	var refusal *ImportError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeEmptyCitationRange, refusal.Code)
}

func TestRecoverableSourceAcceptsARangeHeader(t *testing.T) {
	assert.True(t, recoverableSource("Xenophon, Xen. Anab._1.2.17-1.2.18"))
	assert.True(t, recoverableSource("Xenophon, Xen. Anab._1.2.17"))
	assert.False(t, recoverableSource("Xenophon, Xen. Anab._1.2.17-"))
	assert.False(t, recoverableSource("Xenophon, Xen. Anab._not-a-citation"))
}

func TestInspectTranslationAlignsTheWholeRange(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "perseus-eng2-section.xml"))
	require.NoError(t, err)
	sections, err := InspectTranslation(data, "1.2.16", "1.2.18")
	require.NoError(t, err)
	assert.Equal(t, 3, sections)

	_, err = InspectTranslation(data, "1.2.16", "1.2.19")
	var refusal *ImportError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeMalformed, refusal.Code)
}
