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

func TestSpeakerLabelRuleRecognisesOnlyDialogueAbbreviations(t *testing.T) {
	tests := []struct {
		name      string
		tokens    []sourceToken
		wantLabel bool
	}{
		{"abbreviation", []sourceToken{{surface: "ΣΩ", pos: "n"}, {surface: ".", pos: "u"}}, true},
		{"elliptical abbreviation", []sourceToken{{surface: "ΜΕΝ", pos: "n"}, {surface: "[0]", pos: "-"}, {surface: ".", pos: "u"}}, true},
		{"punctuated abbreviation", []sourceToken{{surface: "Σ-Ω", pos: "n"}, {surface: ".", pos: "u"}}, true},
		{"capitalised word", []sourceToken{{surface: "Ἀρταξέρξης", pos: "n"}, {surface: ".", pos: "u"}}, false},
		{"lowercase word", []sourceToken{{surface: "εἶεν", pos: "d"}, {surface: "·", pos: "u"}}, false},
		{"sentence", []sourceToken{{surface: "λέγει", pos: "v"}, {surface: "Κῦρος", pos: "n"}, {surface: ".", pos: "u"}}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.wantLabel, isSpeakerLabel(sourceSentence{tokens: test.tokens}))
		})
	}
}

func TestSummarizeUnitRefusesASpeakerLabelOnlyUnit(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)

	summary, err := SummarizeUnit(data, "1.1.1", "1.1.2")
	require.NoError(t, err)
	assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;", summary.Opening)

	label := sourceSentenceBytes("ΜΕΝ", "μεν")
	labeled := append(append([]byte(nil), label...), data...)
	summary, err = SummarizeUnit(labeled, "1.1.1", "1.1.2")
	require.NoError(t, err)
	assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;", summary.Opening, "a speaker label is skipped, with or without an elliptical node")

	_, err = SummarizeUnit(label, "1.1.1", "1.1.1")
	var refusal *ImportError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeOpeningMissing, refusal.Code)
}

func TestSummarizeUnitRefusesInvalidInputs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)

	_, err = SummarizeUnit([]byte("not a source\n"), "1.1.1", "1.1.2")
	require.Error(t, err, "a malformed source is refused")

	_, err = SummarizeUnit(data, "not-a-citation", "1.1.2")
	require.Error(t, err, "an invalid unit boundary is refused")

	ellipsisOnly := []byte("# source = Xenophon, Xen. Anab._1.1.1\n# text = [0]\n# sent_id = 1\n" +
		"1\t[0]\t[0]\t-\t---------\t_\t0\tPRED\t_\te_1|0|ref=Xen. Anab._1.1.1\n\n")
	_, err = SummarizeUnit(ellipsisOnly, "1.1.1", "1.1.1")
	var refusal *ImportError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeEmptyCitationRange, refusal.Code)
}

func TestUnitOpeningSkipsAnEmptySentenceAndRefusesAMalformedCitation(t *testing.T) {
	doc := &sourceDocument{sentences: []sourceSentence{
		{},
		{tokens: []sourceToken{{surface: "x", pos: "n", citation: "not-a-citation"}}},
	}}
	_, err := unitOpening(doc, []int{1, 1, 1}, []int{1, 1, 1})
	var refusal *ImportError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, CodeMalformed, refusal.Code)
}

// sourceSentenceBytes renders one label-only source sentence for the Opening
// tests.
func sourceSentenceBytes(surface, lemma string) []byte {
	return []byte("# source = Xenophon, Xen. Anab._1.1.1\n# text = " + surface + " [0] .\n# sent_id = label\n" +
		"1\t" + surface + "\t" + lemma + "\tn\tn-s---mv-\tCase=v|Gender=m|Number=s\t0\tExD\t_\tt_101|0|ref=Xen. Anab._1.1.1\n" +
		"2\t[0]\t[0]\t-\t---------\t_\t0\tPRED\t_\te_101|0|ref=Xen. Anab._1.1.1\n" +
		"3\t.\t.\tu\tu--------\t_\t1\tAuxK\t_\tt_102|t_101|ref=Xen. Anab._1.1.1\n\n")
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
