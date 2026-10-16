package ancientgreek

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capturedWriter records the candidate the seam hands to the writer.
type capturedWriter struct {
	request ImportWriteRequest
	result  ImportWriteResult
	err     error
}

func (w *capturedWriter) WriteImportedAnalysis(_ context.Context, request ImportWriteRequest) (ImportWriteResult, error) {
	w.request = request
	return w.result, w.err
}

// fixtureDefinition pins the fixture file's exact size and digest.
func fixtureDefinition(t *testing.T) domain.ReadingUnitDefinition {
	t.Helper()
	data := readFixture(t)
	definition := AnabasisBook1
	definition.SourceFileSize = int64(len(data))
	sum := sha256.Sum256(data)
	definition.SourceFileSHA256 = hex.EncodeToString(sum[:])
	return definition
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "oga-src-book1.conllu"))
	require.NoError(t, err)
	return data
}

// definitionForBytes pins arbitrary bytes, so a malformed or hostile input still
// passes the pin check and fails later for the intended reason.
func definitionForBytes(data []byte) domain.ReadingUnitDefinition {
	definition := AnabasisBook1
	definition.SourceFileSize = int64(len(data))
	sum := sha256.Sum256(data)
	definition.SourceFileSHA256 = hex.EncodeToString(sum[:])
	return definition
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.conllu")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func TestImportInterpretsFixture(t *testing.T) {
	writer := &capturedWriter{}
	request := ImportRequest{OwnerID: "owner", BookID: "book", SourcePath: writeTemp(t, readFixture(t)), Definition: fixtureDefinition(t)}

	report, err := Import(context.Background(), request, writer)
	require.NoError(t, err)

	assert.Equal(t, 2, report.Sentences)
	assert.Equal(t, 10, report.Tokens)
	assert.Equal(t, 1, report.EllipsisTokensSkipped)
	assert.Equal(t, 2, report.ProperNamesDerived)
	assert.Equal(t, 2, report.NumberedLemmaTypes)
	assert.Equal(t, 1, report.BareLemmasWithNumberedSibling)
	assert.Equal(t, []FeatureValueCount{{Feature: "Mood", Value: "n", Count: 1}, {Feature: "Tense", Value: "r", Count: 1}}, report.UnmappedFeatureValues)
	assert.Equal(t, "book", report.BookID)
	assert.False(t, report.Reimported)

	// The reconstruction drops the ellipsis token and the space before `·` and
	// `;`, and separates sentences with one newline.
	assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;\nλέγει Κῦρος λέλυκε λύειν", writer.request.FullText)
	assert.Equal(t, SyntaxStateAbsent, writer.request.SyntaxState)
	assert.Equal(t, ImportAnalyzerName, writer.request.AnalyzerName)
	assert.Contains(t, writer.request.AnalyzerVersion, PosMappingVersion)
	assert.Equal(t, domain.ClassicalUnitID(AnabasisBook1.Key), writer.request.UnitID)

	first := writer.request.Result.Sentences[0]
	require.Len(t, first.Tokens, 6)
	assert.Equal(t, "λέγω", first.Tokens[0].CanonicalLemma)
	assert.Equal(t, "morpheus:1", first.Tokens[0].LexicalDiscriminator)
	assert.Equal(t, "λέγω", first.Tokens[0].Headword)
	assert.Equal(t, "VERB", first.Tokens[0].UPOS)
	assert.Equal(t, "PROPN", first.Tokens[1].UPOS)
	assert.Equal(t, "κῦροσ", first.Tokens[1].CanonicalLemma)
	assert.Equal(t, "κῦρος", first.Tokens[1].Headword)
	assert.Equal(t, "PUNCT", first.Tokens[2].UPOS)
	assert.Equal(t, "CCONJ", first.Tokens[3].UPOS)
	assert.Equal(t, "morpheus:2", first.Tokens[4].LexicalDiscriminator)
	assert.Equal(t, "PUNCT", first.Tokens[5].UPOS)
	assert.Equal(t, absentSyntaxDependency, first.Tokens[0].Dependency)
	assert.Zero(t, first.Tokens[0].Head)

	second := writer.request.Result.Sentences[1]
	require.Len(t, second.Tokens, 4)
	assert.Equal(t, "λέγω", second.Tokens[0].CanonicalLemma)
	assert.Empty(t, second.Tokens[0].LexicalDiscriminator, "a bare sibling keeps no discriminator")
	assert.Equal(t, map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Tense": "Pres", "Voice": "Act"}, second.Tokens[0].Morphology)
	assert.Equal(t, map[string]string{"Mood": "Ind", "Number": "Sing", "Person": "3", "Voice": "Act"}, second.Tokens[2].Morphology, "an unmapped Tense value is omitted")
	assert.Equal(t, map[string]string{"Tense": "Pres", "Voice": "Act"}, second.Tokens[3].Morphology, "an unmapped Mood value is omitted")
}

func TestImportRefusesPin(t *testing.T) {
	definition := fixtureDefinition(t)
	definition.SourceFileSHA256 = strings.Repeat("0", 64)
	_, err := Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, readFixture(t)), Definition: definition}, &capturedWriter{})
	assertImportCode(t, err, CodePinMismatch)

	definition = fixtureDefinition(t)
	definition.SourceFileSize = definition.SourceFileSize + 1
	_, err = Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, readFixture(t)), Definition: definition}, &capturedWriter{})
	assertImportCode(t, err, CodePinMismatch)
}

func TestImportRefusesOversize(t *testing.T) {
	definition := fixtureDefinition(t)
	definition.SourceFileSize = 16
	definition.SourceFileSHA256 = strings.Repeat("0", 64)
	_, err := Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, readFixture(t)), Definition: definition}, &capturedWriter{})
	assertImportCode(t, err, CodeOversize)
}

func TestImportRefusesMalformedInput(t *testing.T) {
	cases := map[string]string{
		"short line":         "# source = Xenophon, Xen. Anab._1.1.1\n# text = x\n# sent_id = 1\n1\tλέγει\n",
		"unknown comment":    "# source = Xenophon, Xen. Anab._1.1.1\n# text = x\n# sent_id = 1\n# newdoc = x\n1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_1|0|ref=Xen. Anab._1.1.1\n",
		"missing citation":   "# source = Xenophon, Xen. Anab._1.1.1\n# text = x\n# sent_id = 1\n1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_1|0\n",
		"short postag":       "# source = Xenophon, Xen. Anab._1.1.1\n# text = x\n# sent_id = 1\n1\tλέγει\tλέγω\tv\tv3spia--\t_\t0\tPRED\t_\tt_1|0|ref=Xen. Anab._1.1.1\n",
		"line before header": "1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_1|0|ref=Xen. Anab._1.1.1\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			data := []byte(content)
			_, err := Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, data), Definition: definitionForBytes(data)}, &capturedWriter{})
			assertImportCode(t, err, CodeMalformed)
		})
	}
}

func TestImportRefusesUnknownNativePOS(t *testing.T) {
	inRange := []byte("# source = Xenophon, Xen. Anab._1.1.1\n# text = x\n# sent_id = 1\n1\tλέγει\tλέγω\tz\tv3spia---\t_\t0\tPRED\t_\tt_1|0|ref=Xen. Anab._1.1.1\n")
	_, err := Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, inRange), Definition: definitionForBytes(inRange)}, &capturedWriter{})
	assertImportCode(t, err, CodeUnknownPOS)

	// An unknown native POS outside the imported citation range stops the
	// import too: the mapping never guesses, wherever the token sits.
	outOfRange := []byte("# source = Xenophon, Xen. Anab._2.1.1\n# text = x\n# sent_id = 1\n1\tλέγει\tλέγω\tz\tv3spia---\t_\t0\tPRED\t_\tt_1|0|ref=Xen. Anab._2.1.1\n")
	_, err = Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, outOfRange), Definition: definitionForBytes(outOfRange)}, &capturedWriter{})
	assertImportCode(t, err, CodeUnknownPOS)
}

func TestImportRefusesStraddlingSentence(t *testing.T) {
	data := []byte(strings.Join([]string{
		"# source = Xenophon, Xen. Anab._1.10.19",
		"# text = x",
		"# sent_id = 1",
		"1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_1|0|ref=Xen. Anab._1.10.19",
		"2\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_2|0|ref=Xen. Anab._2.1.1",
		"",
	}, "\n"))
	_, err := Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, data), Definition: definitionForBytes(data)}, &capturedWriter{})
	assertImportCode(t, err, CodeCitationStraddle)
}

func TestImportComputesProperNamesOverTheWholeWork(t *testing.T) {
	data := []byte(strings.Join([]string{
		"# source = Xenophon, Xen. Anab._1.1.1",
		"# text = x",
		"# sent_id = 1",
		"1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_1|0|ref=Xen. Anab._1.1.1",
		"2\tχώρα\tχώρα\tn\tn-s---fn-\tCase=n|Gender=f|Number=s\t1\tSBJ\t_\tt_2|t_1|ref=Xen. Anab._1.1.1",
		"",
		"# source = Xenophon, Xen. Anab._2.1.1",
		"# text = x",
		"# sent_id = 2",
		"1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_3|0|ref=Xen. Anab._2.1.1",
		"2\tΧώρα\tχώρα\tn\tn-s---fn-\tCase=n|Gender=f|Number=s\t1\tSBJ\t_\tt_4|t_3|ref=Xen. Anab._2.1.1",
		"",
		"# source = Xenophon, Xen. Anab._2.1.2",
		"# text = x",
		"# sent_id = 3",
		"1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_5|0|ref=Xen. Anab._2.1.2",
		"2\tΧώρα\tχώρα\tn\tn-s---fn-\tCase=n|Gender=f|Number=s\t1\tSBJ\t_\tt_6|t_5|ref=Xen. Anab._2.1.2",
		"",
		"# source = Xenophon, Xen. Anab._2.1.3",
		"# text = x",
		"# sent_id = 4",
		"1\tλέγει\tλέγω\tv\tv3spia---\t_\t0\tPRED\t_\tt_7|0|ref=Xen. Anab._2.1.3",
		"2\tΧώρα\tχώρα\tn\tn-s---fn-\tCase=n|Gender=f|Number=s\t1\tSBJ\t_\tt_8|t_7|ref=Xen. Anab._2.1.3",
		"",
	}, "\n"))
	writer := &capturedWriter{}
	_, err := Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, data), Definition: definitionForBytes(data)}, writer)
	require.NoError(t, err)
	require.Len(t, writer.request.Result.Sentences, 1, "only the Book 1 sentence is imported")
	assert.Equal(t, "PROPN", writer.request.Result.Sentences[0].Tokens[1].UPOS, "the whole-work capitalization makes the in-range noun a proper name")
}

func TestNoSpaceBefore(t *testing.T) {
	for _, surface := range []string{",", ".", ";", "\u00b7", "\u037e", "\u0387"} {
		assert.True(t, noSpaceBefore(surface), "%q", surface)
	}
	for _, surface := range []string{"λέγει", "("} {
		assert.False(t, noSpaceBefore(surface), "%q", surface)
	}
}

func TestDecodeLemma(t *testing.T) {
	cases := map[string][2]string{
		"δέω2": {"δέω", "morpheus:2"},
		"λέγω": {"λέγω", ""},
		"ἦ1":   {"ἦ", "morpheus:1"},
		"εἰμί": {"εἰμί", ""},
	}
	for lemma, want := range cases {
		headword, discriminator := decodeLemma(lemma)
		assert.Equal(t, want[0], headword, lemma)
		assert.Equal(t, want[1], discriminator, lemma)
	}
}

func TestMapFeatures(t *testing.T) {
	morphology, unmapped := mapFeatures("v3ppie---")
	assert.Equal(t, map[string]string{"Mood": "Ind", "Number": "Plur", "Person": "3", "Tense": "Pres", "Voice": "Mid"}, morphology)
	assert.Empty(t, unmapped)

	morphology, unmapped = mapFeatures("n-s---fn-")
	assert.Equal(t, map[string]string{"Case": "Nom", "Gender": "Fem", "Number": "Sing"}, morphology)
	assert.Empty(t, unmapped)

	_, unmapped = mapFeatures("v3sria---")
	assert.Equal(t, []FeatureValueCount{{Feature: "Tense", Value: "r"}}, unmapped)
}

func TestMapUPOSMapsEveryNativeValue(t *testing.T) {
	cases := []struct {
		pos      string
		relation string
		want     string
	}{
		{"n", "", "NOUN"},
		{"v", "", "VERB"},
		{"a", "", "ADJ"},
		{"d", "", "ADV"},
		{"l", "", "DET"},
		{"g", "", "PART"},
		{"c", "AuxC", "SCONJ"},
		{"c", "COORD", "CCONJ"},
		{"r", "", "ADP"},
		{"p", "", "PRON"},
		{"m", "", "NUM"},
		{"i", "", "INTJ"},
		{"u", "", "PUNCT"},
	}
	for _, tc := range cases {
		upos, err := mapUPOS(tc.pos, tc.relation, false)
		require.NoError(t, err)
		assert.Equal(t, tc.want, upos)
	}
	upos, err := mapUPOS("n", "SBJ", true)
	require.NoError(t, err)
	assert.Equal(t, "PROPN", upos)
	_, err = mapUPOS("z", "", false)
	assertImportCode(t, err, CodeUnknownPOS)
}

func TestAsciiValue(t *testing.T) {
	assert.Equal(t, "empty", asciiValue(""))
	assert.Equal(t, "n", asciiValue("n"))
	assert.Equal(t, "non-ascii", asciiValue("λ"))
	assert.Equal(t, "non-ascii", asciiValue("\n"))
}

func TestParseCitationRejectsNonNumbers(t *testing.T) {
	_, ok := parseCitation("1.x")
	assert.False(t, ok)
	_, ok = parseCitation("1.-2")
	assert.False(t, ok)
	citation, ok := parseCitation("1.10.19")
	require.True(t, ok)
	assert.Equal(t, []int{1, 10, 19}, citation)
	assert.Negative(t, compareCitation([]int{1, 1, 1}, []int{1, 2}))
	assert.Positive(t, compareCitation([]int{1, 2}, []int{1, 1, 9}))
	assert.Zero(t, compareCitation([]int{1, 1}, []int{1, 1, 0}))
}

func TestImportRequiresWriterAndDefinition(t *testing.T) {
	_, err := Import(context.Background(), ImportRequest{}, nil)
	require.ErrorContains(t, err, "import writer is required")

	definition := fixtureDefinition(t)
	definition.Key = ""
	_, err = Import(context.Background(), ImportRequest{SourcePath: writeTemp(t, readFixture(t)), Definition: definition}, &capturedWriter{})
	require.Error(t, err)
}

func TestImportReportsUnreadablePath(t *testing.T) {
	_, err := Import(context.Background(), ImportRequest{SourcePath: filepath.Join(t.TempDir(), "missing.conllu"), Definition: fixtureDefinition(t)}, &capturedWriter{})
	assertImportCode(t, err, CodeReadFailed)
}

func TestValidateRejectsBrokenSpans(t *testing.T) {
	sentence := func(text string, start, end uint64, tokens ...analyzer.Token) analyzer.Sentence {
		return analyzer.Sentence{Text: text, Location: analyzer.SourceLocation{StartOffset: start, EndOffset: end}, Tokens: tokens}
	}
	token := func(surface string, start, end uint64) analyzer.Token {
		return analyzer.Token{Surface: surface, Location: analyzer.SourceLocation{StartOffset: start, EndOffset: end}}
	}
	valid := interpretation{FullText: "ab cd", Result: analyzer.Result{Sentences: []analyzer.Sentence{sentence("ab cd", 0, 5, token("ab", 0, 2), token("cd", 3, 5))}}}
	require.NoError(t, valid.validate())

	cases := map[string]interpretation{
		"span beyond text":      {FullText: "ab", Result: analyzer.Result{Sentences: []analyzer.Sentence{sentence("ab", 0, 3)}}},
		"sentence mismatch":     {FullText: "ab", Result: analyzer.Result{Sentences: []analyzer.Sentence{sentence("xy", 0, 2)}}},
		"token leaves sentence": {FullText: "ab", Result: analyzer.Result{Sentences: []analyzer.Sentence{sentence("ab", 0, 2, token("ab", 0, 3))}}},
		"surface mismatch":      {FullText: "ab", Result: analyzer.Result{Sentences: []analyzer.Sentence{sentence("ab", 0, 2, token("xy", 0, 2))}}},
		"shared span":           {FullText: "ab", Result: analyzer.Result{Sentences: []analyzer.Sentence{sentence("ab", 0, 2, token("a", 0, 1), token("b", 0, 1))}}},
	}
	for name, broken := range cases {
		t.Run(name, func(t *testing.T) {
			require.Error(t, broken.validate())
		})
	}
}

func assertImportCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var importErr *ImportError
	require.ErrorAs(t, err, &importErr)
	assert.Equal(t, code, importErr.Code)
	assert.NotContains(t, err.Error(), "λέγ", "a refusal never carries source text")
}
