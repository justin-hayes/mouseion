package ancientgreek

import (
	"os"
	"path/filepath"
	"strconv"
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

func TestSummarizeUnitDerivesPrintedWordsAndTheOpening(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	summary, err := SummarizeUnit(data, "1.1.1", "1.1.2")
	require.NoError(t, err)
	assert.Equal(t, 8, summary.WordCount, "punctuation and ellipsis tokens are not words")
	assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;", summary.Opening, "the Opening uses the importer's reconstruction")
}

func TestSummarizeUnitRefusesAnEmptyRange(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	_, err = SummarizeUnit(data, "3.1.1", "3.1.2")
	var refusal *ImportError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeEmptyCitationRange, refusal.Code)
}

func TestSummarizeUnitSkipsASpeakerLabel(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	tests := []struct {
		name       string
		surface    string
		lemma      string
		elliptical bool
	}{
		{"abbreviation", "ΣΩ", "σω", false},
		{"elliptical abbreviation", "ΜΕΝ", "μεν", true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			labeled := append(sourceSentenceBytes(test.surface, test.lemma, test.elliptical), data...)
			summary, err := SummarizeUnit(labeled, "1.1.1", "1.1.2")
			require.NoError(t, err)
			assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;", summary.Opening)
		})
	}
}

func TestSummarizeUnitRefusesASpeakerLabelOnlyUnit(t *testing.T) {
	_, err := SummarizeUnit(sourceSentenceBytes("ΜΕΝ", "μεν", true), "1.1.1", "1.1.1")
	var refusal *ImportError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeOpeningMissing, refusal.Code)
}

func TestSummarizeUnitRefusesInvalidInputs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	t.Run("malformed source", func(t *testing.T) {
		_, err := SummarizeUnit([]byte("not a source\n"), "1.1.1", "1.1.2")
		require.Error(t, err)
	})
	t.Run("invalid boundary", func(t *testing.T) {
		_, err := SummarizeUnit(data, "not-a-citation", "1.1.2")
		require.Error(t, err)
	})
	t.Run("no printed words", func(t *testing.T) {
		ellipsisOnly := []byte("# source = Xenophon, Xen. Anab._1.1.1\n# text = [0]\n# sent_id = 1\n" +
			"1\t[0]\t[0]\t-\t---------\t_\t0\tPRED\t_\te_1|0|ref=Xen. Anab._1.1.1\n\n")
		_, err := SummarizeUnit(ellipsisOnly, "1.1.1", "1.1.1")
		var refusal *ImportError
		require.ErrorAs(t, err, &refusal)
		assert.Equal(t, CodeEmptyCitationRange, refusal.Code)
	})
}

// sourceSentenceBytes renders one label-only source sentence for the Opening
// tests.
func sourceSentenceBytes(surface, lemma string, elliptical bool) []byte {
	text, next := surface+" .", 2
	if elliptical {
		text, next = surface+" [0] .", 3
	}
	source := "# source = Xenophon, Xen. Anab._1.1.1\n# text = " + text + "\n# sent_id = label\n" +
		"1\t" + surface + "\t" + lemma + "\tn\tn-s---mv-\tCase=v|Gender=m|Number=s\t0\tExD\t_\tt_101|0|ref=Xen. Anab._1.1.1\n"
	if elliptical {
		source += "2\t[0]\t[0]\t-\t---------\t_\t0\tPRED\t_\te_101|0|ref=Xen. Anab._1.1.1\n"
	}
	source += strconv.Itoa(next) + "\t.\t.\tu\tu--------\t_\t1\tAuxK\t_\tt_102|t_101|ref=Xen. Anab._1.1.1\n\n"
	return []byte(source)
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
