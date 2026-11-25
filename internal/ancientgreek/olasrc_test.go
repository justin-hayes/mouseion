package ancientgreek

import (
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// olaToken is one synthetic OLA token: form, lemma, native POS, FEATS, head,
// relation and Citation.
type olaToken struct {
	form, lemma, pos, feats, relation, citation string
	head                                        int
}

// olaFixture renders a synthetic CoNLL-U and PAULA Citation layer for the
// sentences, numbering tokens t_1.. across the file as OLA does.
func olaFixture(sentences [][]olaToken) (conllu, paula []byte) {
	var c, p strings.Builder
	p.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="no"?>` + "\n" + `<!DOCTYPE paula SYSTEM "../../paula_dtd/paula_feat.dtd">` + "\n" +
		`<paula version="1.1"><header paula_id="synthetic"/><featList xmlns:xlink="http://www.w3.org/1999/xlink" type="cts" xml:base="x.tok01.xml">` + "\n")
	ref := 0
	for index, sentence := range sentences {
		fmt.Fprintf(&c, "# absolute_id = %d\n# clause_id = c_%d\n# file = synthetic.xml\n", index, index+1)
		for i, token := range sentence {
			ref++
			fmt.Fprintf(&c, "%d\t%s\t%s\t%s\t_\t%s\t%d\t%s\t_\tt_%d\n", i+1, token.form, token.lemma, token.pos, token.feats, token.head, token.relation, ref)
			fmt.Fprintf(&p, `<feat id="u_%d" xlink:href="#t_%d" value="%s"/>`+"\n", ref, ref, token.citation)
		}
		c.WriteString("\n")
	}
	p.WriteString("</featList></paula>\n")
	return []byte(c.String()), []byte(p.String())
}

func latinSentences() [][]olaToken {
	return [][]olaToken{
		{
			{"Gallia", "Gallia", "n", "Case=n|Gender=f|Number=s", "SBJ", "1.pr.1", 4},
			{"est", "sum", "v", "Mood=i|Number=s|Person=3|Tense=p|Voice=a", "PRED", "1.pr.1", 0},
			{"omnis", "omnis", "a", "Case=n|Gender=f|Number=s", "ATR", "1.pr.1", 1},
			{"diuisa", "diuido", "v", "Case=n|Gender=f|Mood=p|Number=s|Tense=r|Voice=p", "PNOM", "1.pr.1", 2},
			{".", ".", "u", "_", "AuxK", "1.pr.1", 2},
		},
		{
			{"Quarum", "qui", "p", "Case=g|Gender=f|Number=p", "ATR", "1.1a", 2},
			{"unam", "unus", "m", "Case=a|Gender=f|Number=s", "ATR_CO", "1.1a", 3},
			{"incolunt", "incolo1", "v", "Mood=i|Number=p|Person=3|Tense=p|Voice=a", "PRED", "1.1a", 0},
			{"Belgae", "Belgae", "n", "Case=n|Gender=m|Number=p", "SBJ", "1.1a", 3},
			{"ab", "ab", "r", "_", "AuxP", "1.1a", 3},
			{"hoste", "hostis", "n", "Case=b|Gender=m|Number=s|Degree=p", "ADV", "1.1a", 5},
			{",", ",", "u", "_", "AuxX", "1.2", 3},
			{"Aquitani", "Aquitani", "n", "Case=n|Gender=m|Number=p", "SBJ_CO", "1.2", 3},
			{"?", "?", "u", "_", "AuxK", "1.2", 3},
		},
	}
}

func latinDefinition(start string) domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key: "caes-gal", Language: classical.LanguageLatin, WorkTitle: "The Gallic War", WorkAuthor: "Caesar",
		EditionLabel: "Perseus", CitationStart: start, CitationEnd: "1.2",
		SourceFileName: "caes.conllu", SourceFileSize: 1, SourceFileSHA256: strings.Repeat("a", 64),
		InterpretationVersion: classical.InterpretationVersionOLA,
	}
}

func TestParseOlaSourceCutsALatinUnitWithNativeSyntax(t *testing.T) {
	conllu, paula := olaFixture(latinSentences())
	parsed, err := ParseOlaSource(conllu, paula)
	require.NoError(t, err)

	unit, err := parsed.CutUnit(latinDefinition("1.pr.1"))
	require.NoError(t, err)

	assert.Equal(t, SyntaxStateNativeAGDT, unit.SyntaxState)
	assert.Equal(t, "la", unit.Result.Language)
	require.Len(t, unit.Result.Sentences, 2)
	first := unit.Result.Sentences[0]
	assert.Equal(t, "Gallia est omnis diuisa.", first.Text)
	assert.Equal(t, []string{"1.pr.1"}, first.Citations)
	assert.Equal(t, []string{"1.1a", "1.2"}, unit.Result.Sentences[1].Citations)
	assert.Equal(t, "Quarum unam incolunt Belgae ab hoste, Aquitani?", unit.Result.Sentences[1].Text)

	gallia := first.Tokens[0]
	assert.Equal(t, "gallia", gallia.CanonicalLemma)
	assert.Equal(t, "PROPN", gallia.UPOS, "a capitalised noun lemma is a proper name")
	assert.Equal(t, "SBJ", gallia.Dependency)
	assert.Equal(t, uint32(3), gallia.Head)
	assert.Equal(t, map[string]string{"Case": "Nom", "Gender": "Fem", "Number": "Sing"}, gallia.Morphology)
	assert.Equal(t, "diuisa", first.Tokens[3].Surface)
	assert.Equal(t, "Part", first.Tokens[3].Morphology["VerbForm"])

	second := unit.Result.Sentences[1]
	assert.Equal(t, "ATR_CO", second.Tokens[1].Dependency, "relation suffixes are stored verbatim")
	assert.Equal(t, "incolo", second.Tokens[2].CanonicalLemma, "the artefact digit is stripped")
	assert.Empty(t, second.Tokens[2].LexicalDiscriminator)
	assert.Equal(t, "Abl", second.Tokens[5].Morphology["Case"])
	assert.Equal(t, "Pos", second.Tokens[5].Morphology["Degree"])
	assert.Empty(t, unit.Report.UnmappedFeatureValues)
	assert.Contains(t, unit.AnalyzerVersion, classical.InterpretationVersionOLA)
	assert.Equal(t, []string{"t_6", "t_7", "t_8", "t_9", "t_10", "t_11", "t_12", "t_13", "t_14"}, unit.Sources[1].TokenRefs)
}

func TestOlaCitationRangeSelectsSentencesByCitationOrder(t *testing.T) {
	conllu, paula := olaFixture(latinSentences())
	parsed, err := ParseOlaSource(conllu, paula)
	require.NoError(t, err)

	unit, err := parsed.CutUnit(latinDefinition("1.1a"))
	require.NoError(t, err)
	require.Len(t, unit.Result.Sentences, 1, "the preface precedes chapter 1, so it is outside 1.1a-1.2")
}

func TestOlaSourceRefusesBadInput(t *testing.T) {
	conllu, paula := olaFixture(latinSentences())
	tests := map[string]struct{ conllu, paula []byte }{
		"token without a Citation": {conllu, []byte(strings.Replace(string(paula), `#t_3"`, `#t_999"`, 1))},
		"leading-zero Citation":    {conllu, []byte(strings.Replace(string(paula), `value="1.pr.1"`, `value="01.pr.1"`, 1))},
		"unknown named section":    {conllu, []byte(strings.Replace(string(paula), `value="1.pr.1"`, `value="1.zz.1"`, 1))},
		"empty Citation layer":     {conllu, []byte(`<paula><featList></featList></paula>`)},
		"broken XML":               {conllu, []byte(`<paula><featList>`)},
		"short token line":         {[]byte("# absolute_id = 0\n1\tx\n"), paula},
		"unknown feature":          {[]byte(strings.Replace(string(conllu), "Case=n|Gender=f|Number=s", "Case=n|Zodiac=f", 1)), paula},
		"lemma with a multi-digit suffix": {[]byte(strings.Replace(string(conllu), "incolo1", "rex12", 1)), paula},
		"no sentences":             {[]byte("# only a comment\n"), paula},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := ParseOlaSource(input.conllu, input.paula)
			var refusal *classical.PreparationError
			require.ErrorAs(t, err, &refusal)
			assert.Equal(t, classical.CodeMalformed, refusal.Code)
		})
	}
}

func TestOlaSourceRefusesUnknownPartOfSpeech(t *testing.T) {
	sentences := latinSentences()
	sentences[0][0].pos = "q"
	conllu, paula := olaFixture(sentences)
	parsed, err := ParseOlaSource(conllu, paula)
	require.NoError(t, err)
	_, err = parsed.CutUnit(latinDefinition("1.pr.1"))
	var refusal *classical.PreparationError
	require.ErrorAs(t, err, &refusal)
	assert.Equal(t, classical.CodeUnknownPOS, refusal.Code)
}

func TestOlaDocumentRefusesAGreekDefinition(t *testing.T) {
	conllu, paula := olaFixture(latinSentences())
	parsed, err := ParseOlaSource(conllu, paula)
	require.NoError(t, err)
	definition := latinDefinition("1.pr.1")
	definition.Language = classical.Language
	_, err = parsed.CutUnit(definition)
	require.Error(t, err)
}

func TestLatinHelpersLeaveGreekRulesAlone(t *testing.T) {
	headword, discriminator := decodeLemmaFor("et1", true)
	assert.Equal(t, "et", headword)
	assert.Empty(t, discriminator)
	headword, _ = decodeLemmaFor("rex1", true)
	assert.Equal(t, "rex", headword)
	headword, _ = decodeLemmaFor("rex12", true)
	assert.Equal(t, "rex1", headword, "only one trailing digit is stripped")
	headword, discriminator = decodeLemmaFor("λόγος2", false)
	assert.Equal(t, "λόγος", headword)
	assert.Equal(t, "morpheus:2", discriminator)
	assert.True(t, noSpaceBeforeFor("?", true))
	assert.False(t, noSpaceBeforeFor("?", false), "a Latin question mark is not Greek punctuation")
	assert.True(t, noSpaceBeforeFor(";", false))
}
