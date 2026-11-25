package ancientgreek

import (
	"os"
	"strings"
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

// A Latin perseus-engN file names its prefatory chapter pr and may suffix a
// section; the reader keeps both as Citation components and still refuses a gap
// in the numbers.
func TestParseReferenceTranslationReadsLatinNamedAndSuffixedSections(t *testing.T) {
	section := func(number, text string) string {
		return `<div type="textpart" subtype="section" n="` + number + `"><p>` + text + `</p></div>`
	}
	chapter := func(n string, sections ...string) string {
		return `<div type="textpart" subtype="chapter" n="` + n + `">` + strings.Join(sections, "") + `</div>`
	}
	book := func(chapters ...string) string {
		return `<TEI><div type="textpart" subtype="book" n="1">` + strings.Join(chapters, "") + `</div></TEI>`
	}

	body := book(
		chapter("pr", section("1", "Preface one."), section("2", "Preface two.")),
		chapter("1", section("1", "Chapter one."), section("1a", "Chapter one, a.")),
	)
	sections, err := ParseReferenceTranslation([]byte(body), "1.pr.1", "1.1.1a")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.pr.1", "1.pr.2", "1.1.1", "1.1.1a"}, citationsInOrder(sections))

	gap := book(chapter("pr", section("1", "One."), section("3", "Three.")))
	_, err = ParseReferenceTranslation([]byte(gap), "1.pr.1", "1.pr.3")
	assertImportCode(t, err, CodeMalformed)
}

func TestMeasuredTranslationReadsGapsAndCoarserKeys(t *testing.T) {
	gapped := []byte(`<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="section" n="1"><p>One.</p></div><div type="textpart" subtype="section" n="3"><p>Three.</p></div></div></TEI>`)
	_, err := ParseReferenceTranslation(gapped, "1.1", "1.3")
	require.Error(t, err, "the strict reader refuses a gap")
	sections, err := ParseReferenceTranslationMeasured(gapped, "1.1", "1.3")
	require.NoError(t, err)
	require.Len(t, sections, 2)
	assert.Equal(t, "1.3", sections[1].Citation)

	coarse := []byte(`<TEI><div type="textpart" subtype="book" n="1"><p>Book one.</p></div></TEI>`)
	sections, err = ParseReferenceTranslationMeasured(coarse, "1.1", "1.3")
	require.NoError(t, err)
	require.Len(t, sections, 1)
	assert.Equal(t, "1", sections[0].Citation)

	_, err = ParseReferenceTranslationMeasured([]byte(`<TEI><div>`), "1.1", "1.3")
	require.Error(t, err)
}

// An editorial textpart (an argument, introduction, chronology or note) is not
// part of the cited text: the reader skips it and everything inside it.
func TestParseReferenceTranslationSkipsEditorialTextparts(t *testing.T) {
	body := `<TEI><div type="translation">` +
		`<div type="textpart" subtype="chapter" n="intro"><p>An introduction.</p>` +
		`<div type="textpart" subtype="section" n="1"><p>Nested editorial text.</p></div></div>` +
		`<div type="textpart" subtype="chapter" n="chronology"><p>A chronology.</p></div>` +
		`<div type="textpart" subtype="chapter" n="1"><p>Chapter one.</p></div>` +
		`<div type="textpart" subtype="chapter" n="note"><p>A note.</p></div>` +
		`<div type="textpart" subtype="chapter" n="2"><p>Chapter two.</p></div>` +
		`</div></TEI>`
	sections, err := ParseReferenceTranslation([]byte(body), "1", "2")
	require.NoError(t, err)
	assert.Equal(t, []string{"1", "2"}, citationsInOrder(sections))
	assert.Equal(t, "Chapter one.", sections[0].Text)

	nested := `<TEI><div type="textpart" subtype="book" n="1">` +
		`<div type="textpart" subtype="section" n="argument"><p>The argument.</p></div>` +
		`<div type="textpart" subtype="section" n="1"><p>One.</p></div></div></TEI>`
	sections, err = ParseReferenceTranslation([]byte(nested), "1.1", "1.1")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.1"}, citationsInOrder(sections))
}

// A speech textpart is a citation level: a speech-keyed translation joins a
// Work cited speech.section.
func TestParseReferenceTranslationReadsSpeechTextparts(t *testing.T) {
	body := `<TEI><div type="translation">` +
		`<div type="textpart" n="1" subtype="speech">` +
		`<div type="textpart" subtype="section" n="argument"><p>The argument.</p></div>` +
		`<div type="textpart" subtype="section" n="1"><p>First speech, one.</p></div>` +
		`<div type="textpart" subtype="section" n="2"><p>First speech, two.</p></div></div>` +
		`<div type="textpart" n="2" subtype="speech">` +
		`<div type="textpart" subtype="section" n="1"><p>Second speech, one.</p></div></div>` +
		`</div></TEI>`
	sections, err := ParseReferenceTranslation([]byte(body), "1.1", "2.1")
	require.NoError(t, err)
	assert.Equal(t, []string{"1.1", "1.2", "2.1"}, citationsInOrder(sections))
	assert.Equal(t, "Second speech, one.", sections[2].Text)
}

// milestoneTranslation renders a translation that marks sections only with
// section milestones, beside chapter milestones the reader ignores.
func milestoneTranslation(paragraphs ...string) []byte {
	return []byte(`<TEI><text><body><head>A title.</head><div type="translation"><head>The speech.</head>` +
		strings.Join(paragraphs, "\n") + `</div></body><back><p>Back matter.</p></back></text></TEI>`)
}

func TestParseReferenceTranslationReadsSectionMilestones(t *testing.T) {
	data := milestoneTranslation(
		`<p><milestone unit="chapter" n="1"/><milestone unit="section" n="1"/>Section one <note>a note</note>begins,</p>`,
		`<p>and continues. <milestone unit="section" n="2"/>Section two.</p>`,
		`<p><milestone unit="chapter" n="2"/><milestone unit="section" n="3"/>Section three.</p>`,
	)
	sections, err := ParseReferenceTranslation(data, "1", "3")
	require.NoError(t, err)
	assert.Equal(t, []string{"1", "2", "3"}, citationsInOrder(sections))
	assert.Equal(t, "Section one begins, and continues.", sections[0].Text)
	assert.Equal(t, "Section three.", sections[2].Text, "a section ends with its translation div")

	sections, err = ParseReferenceTranslationMeasured(data, "1", "3")
	require.NoError(t, err)
	assert.Len(t, sections, 3)
}

func TestMeasuredTranslationReadsGappedMilestonesAndDropsRepeats(t *testing.T) {
	gapped := milestoneTranslation(
		`<p><milestone unit="section" n="1"/>One.</p>`,
		`<p><milestone unit="section" n="3"/>Three.</p>`,
		`<p><milestone unit="section" n="3"/>Three again.</p>`,
	)
	_, err := ParseReferenceTranslation(gapped, "1", "3")
	assertImportCode(t, err, CodeMalformed)
	sections, err := ParseReferenceTranslationMeasured(gapped, "1", "3")
	require.NoError(t, err)
	assert.Equal(t, []string{"1", "3"}, citationsInOrder(sections))
	assert.Equal(t, "Three.", sections[1].Text, "a repeated milestone is dropped, never merged into the earlier section")
}

func TestParseReferenceTranslationMilestoneLimits(t *testing.T) {
	data := milestoneTranslation(`<p><milestone unit="section" n="1"/>One.</p>`)
	_, err := ParseReferenceTranslationMeasured(data, "1.1", "1.1")
	require.Error(t, err, "a section milestone is not read against a deeper Citation range")

	bad := milestoneTranslation(`<p><milestone unit="section" n="x1"/>One.</p>`)
	_, err = ParseReferenceTranslation(bad, "1", "1")
	assertImportCode(t, err, CodeMalformed)

	// A textpart-keyed file is read by its textparts; its milestones are not
	// sections.
	textparts := []byte(`<TEI><div type="textpart" subtype="section" n="1"><p><milestone unit="section" n="7"/>One.</p></div></TEI>`)
	sections, err := ParseReferenceTranslation(textparts, "1", "1")
	require.NoError(t, err)
	assert.Equal(t, []string{"1"}, citationsInOrder(sections))
	assert.Equal(t, "One.", sections[0].Text)
}
