package ancientgreek

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brownsonFixture returns the bytes of the synthetic perseus-eng2-shaped
// fixture. The real pinned file never enters the repository or CI.
func brownsonFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/perseus-eng2-section.xml")
	require.NoError(t, err)
	return data
}

func TestParseReferenceTranslationExtractsCitedSectionsWithNotesRemoved(t *testing.T) {
	data := brownsonFixture(t)
	sections, err := ParseReferenceTranslation(data, "1.1.1", "1.10.19")
	require.NoError(t, err)

	byCitation := map[string]string{}
	for _, section := range sections {
		byCitation[section.Citation] = section.Text
		assert.NotContains(t, section.Text, "editor's note", "a Perseus note is removed")
	}
	assert.Equal(t, []string{"1.1.1", "1.2.16", "1.2.17", "1.2.18"}, citationsInOrder(sections))
	assert.Equal(t, "The men Cyrus led [into the country] were Greeks, and Tarsus was their goal.", byCitation["1.1.1"], "bracketed material is kept")
	assert.Equal(t, "He halted his chariot in front of the phalanx and gave orders.", byCitation["1.2.17"], "the note is removed without merging words")
}

func citationsInOrder(translations []ReferenceTranslation) []string {
	citations := make([]string, 0, len(translations))
	for _, translation := range translations {
		citations = append(citations, translation.Citation)
	}
	return citations
}

func TestParseReferenceTranslationRefusesMalformedInput(t *testing.T) {
	cases := map[string]string{
		"invalid XML":        `<?xml version="1.0"?><TEI><div`,
		"section not number": `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2"><div type="textpart" subtype="section" n="x"><p>text</p></div></div></div></TEI>`,
		"no sections":        `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2"></div></div></TEI>`,
		"empty section":      `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2"><div type="textpart" subtype="section" n="1"></div></div></div></TEI>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseReferenceTranslation([]byte(body), "1.1.1", "1.10.19")
			assertImportCode(t, err, CodeMalformed)
			assert.NotContains(t, err.Error(), "chariot", "a refusal never carries source text")
		})
	}
}

func TestParseReferenceTranslationRefusesStructuralNoise(t *testing.T) {
	cases := map[string]string{
		"section without book or chapter": `<TEI><div type="textpart" subtype="section" n="17"><p>text</p></div></TEI>`,
		"duplicate cited section": `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
			`<div type="textpart" subtype="section" n="17"><p>first</p></div>` +
			`<div type="textpart" subtype="section" n="17"><p>second</p></div></div></div></TEI>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseReferenceTranslation([]byte(body), "1.2.16", "1.2.18")
			assertImportCode(t, err, CodeMalformed)
		})
	}
}

func TestParseReferenceTranslationDropsALeadingNoteWithoutMergingWords(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
		`<div type="textpart" subtype="section" n="17"><note anchored="true">opening note</note>Words remain.</div></div></div></TEI>`
	sections, err := ParseReferenceTranslation([]byte(body), "1.2.17", "1.2.17")
	require.NoError(t, err)
	require.Len(t, sections, 1)
	assert.Equal(t, "Words remain.", sections[0].Text, "a leading note leaves the text unchanged")
}

func TestParseReferenceTranslationParsesSectionOnlyTextparts(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="section" n="1"><p>First.</p></div>` +
		`<div type="textpart" subtype="section" n="2"><p>Second.</p></div>` +
		`<div type="textpart" subtype="section" n="3"><p>Third.</p></div></TEI>`
	sections, err := ParseReferenceTranslation([]byte(body), "1", "3")
	require.NoError(t, err)
	assert.Equal(t, []string{"1", "2", "3"}, citationsInOrder(sections))
	assert.Equal(t, "Second.", sections[1].Text)
}

func TestParseReferenceTranslationParsesDeeperCitationShapes(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
		`<div type="textpart" subtype="section" n="17">` +
		`<div type="textpart" subtype="subsection" n="3"><p>First part.</p></div>` +
		`<div type="textpart" subtype="subsection" n="4"><p>Second part.</p></div>` +
		`</div></div></div></TEI>`
	sections, err := ParseReferenceTranslation([]byte(body), "1.2.17.3", "1.2.17.4")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.2.17.3", "1.2.17.4"}, citationsInOrder(sections))
}

func TestParseReferenceTranslationRefusesAMissingSection(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
		`<div type="textpart" subtype="section" n="16"><p>Sixteen.</p></div>` +
		`<div type="textpart" subtype="section" n="18"><p>Eighteen.</p></div>` +
		`</div></div></TEI>`
	_, err := ParseReferenceTranslation([]byte(body), "1.2.16", "1.2.18")
	assertImportCode(t, err, CodeMalformed)
}

func TestParseReferenceTranslationRefusesAMissingFirstOrLastSection(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
		`<div type="textpart" subtype="section" n="17"><p>Seventeen.</p></div>` +
		`</div></div></TEI>`
	for name, citationRange := range map[string][2]string{
		"missing first": {"1.2.16", "1.2.17"},
		"missing last":  {"1.2.17", "1.2.18"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseReferenceTranslation([]byte(body), citationRange[0], citationRange[1])
			assertImportCode(t, err, CodeMalformed)
		})
	}
}

func TestParseReferenceTranslationRefusesAMissingWholeChapter(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="book" n="1">` +
		`<div type="textpart" subtype="chapter" n="2"><div type="textpart" subtype="section" n="17"><p>Two.</p></div></div>` +
		`<div type="textpart" subtype="chapter" n="4"><div type="textpart" subtype="section" n="1"><p>Four.</p></div>` +
		`<div type="textpart" subtype="section" n="2"><p>Four again.</p></div></div>` +
		`</div></TEI>`
	_, err := ParseReferenceTranslation([]byte(body), "1.2.17", "1.4.2")
	assertImportCode(t, err, CodeMalformed)
}

func TestParseReferenceTranslationRefusesAnExtraDivision(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
		`<div type="textpart" subtype="section" n="17">` +
		`<div type="textpart" subtype="subsection" n="3"><p>An extra division.</p></div>` +
		`</div></div></div></TEI>`
	_, err := ParseReferenceTranslation([]byte(body), "1.2.17", "1.2.18")
	assertImportCode(t, err, CodeMalformed)
}

func TestParseReferenceTranslationRefusesADepthThatDoesNotAlign(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="section" n="17"><p>Section only.</p></div></TEI>`
	_, err := ParseReferenceTranslation([]byte(body), "1.2.17", "1.2.18")
	assertImportCode(t, err, CodeMalformed)
}
