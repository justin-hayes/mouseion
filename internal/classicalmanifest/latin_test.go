package classicalmanifest

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/olaarchive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latinArchiveName = "opera_latina_adnotata_synthetic.zip"
	latinConllu      = "ola/phi0000.work.conllu"
	latinPaula       = "ola/phi0000.work.paula.xml"
)

// latinToken is one synthetic OLA token.
type latinToken struct{ form, pos, citation string }

// latinWordsIn returns n distinct-looking cited words under one Citation.
func latinWordsIn(citation string, n int) []latinToken {
	tokens := make([]latinToken, n)
	for i := range tokens {
		tokens[i] = latinToken{form: fmt.Sprintf("uerbum%c", 'a'+i%26), pos: "n", citation: citation}
	}
	return tokens
}

// latinSentence ends a run of words with a full stop.
func latinSentence(citation string, words ...latinToken) []latinToken {
	return append(words, latinToken{form: ".", pos: "u", citation: citation})
}

// latinFiles renders the CoNLL-U and PAULA members of the sentences. A token
// whose citation is empty is left out of the PAULA layer.
func latinFiles(sentences ...[]latinToken) (conllu, paula []byte) {
	var c, p strings.Builder
	p.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" + `<paula version="1.1"><header paula_id="synthetic"/><featList xmlns:xlink="http://www.w3.org/1999/xlink" type="cts">` + "\n")
	ref := 0
	for index, sentence := range sentences {
		fmt.Fprintf(&c, "# absolute_id = %d\n# clause_id = c_%d\n# file = synthetic.xml\n", index, index+1)
		for i, token := range sentence {
			ref++
			relation := "ATR"
			if token.pos == "u" {
				relation = "AuxK"
			}
			fmt.Fprintf(&c, "%d\t%s\t%s\t%s\t_\t_\t0\t%s\t_\tt_%d\n", i+1, token.form, token.form, token.pos, relation, ref)
			if token.citation != "" {
				fmt.Fprintf(&p, `<feat id="u_%d" xlink:href="#t_%d" value="%s"/>`+"\n", ref, ref, token.citation)
			}
		}
		c.WriteString("\n")
	}
	p.WriteString("</featList></paula>\n")
	return []byte(c.String()), []byte(p.String())
}

// latinFixture is a synthetic OLA archive with its layers.json and override.
type latinFixture struct {
	upstream string
	layers   []byte
	override classical.Override
	conllu   []byte
	paula    []byte
}

// newLatinFixture writes a zip holding the Work's two members among other
// members, and the layers.json pinning them.
func newLatinFixture(t *testing.T, sentences ...[]latinToken) latinFixture {
	t.Helper()
	conllu, paula := latinFiles(sentences...)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, member := range []struct {
		name string
		data []byte
	}{{"ola/other.txt", bytes.Repeat([]byte("filler "), 50)}, {latinConllu, conllu}, {latinPaula, paula}, {"ola/tail.txt", []byte("tail")}} {
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: member.name, Method: zip.Deflate, Extra: []byte{0x55, 0x58, 0x04, 0x00, 1, 2, 3, 4}})
		require.NoError(t, err)
		_, err = entry.Write(member.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	upstream := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(upstream, latinArchiveName), buffer.Bytes(), 0o600))
	layer := classical.Layer{
		Key: "ola", Release: "v0.2.0", FetchKind: classical.FetchKindArchiveRange,
		ArchiveURL: "https://example.test/" + latinArchiveName, ArchiveSize: int64(buffer.Len()), Record: "10.5281/zenodo.0",
		LicenceClass: classical.LicenceClassAnalysisResource, Restriction: classical.RestrictionUnclear, DeployerRegisterRow: "synthetic OLA layer",
		Files: []classical.LayerFile{
			{Name: latinConllu, WorkTitle: "Gallic War", WorkAuthor: "Caesar", OriginalTitle: "De bello Gallico", EditionLabel: "Synthetic edition", Size: int64(len(conllu)), SHA256: sha256Hex(conllu)},
			{Name: latinPaula, Size: int64(len(paula)), SHA256: sha256Hex(paula)},
		},
	}
	layers, err := json.Marshal(classical.Layers{Layers: []classical.Layer{layer}})
	require.NoError(t, err)
	revision := 1
	fixture := latinFixture{
		upstream: upstream, layers: layers, conllu: conllu, paula: paula,
		override: classical.Override{
			WorkKey: "caesar-gallic-war", WorkTitle: "Gallic War", WorkAuthor: "Caesar", OriginalTitle: "De bello Gallico",
			Language: classical.LanguageLatin, EditionKey: "caesar-synthetic", EditionLabel: "Synthetic edition",
			Source: &classical.SourceSpec{Layer: "ola", FileName: latinConllu, CitationFileName: latinPaula, Format: classical.SourceFormatOLACoNLLU},
			Split:  classical.SplitWholeWork, Revision: &revision, Note: "Synthetic curatorial sign-off.",
		},
	}
	// A Latin Work needs a translation (gate iii), so every fixture carries one
	// covering each section it cites; a test of the gate replaces or drops it.
	translation, pair := coveringTranslation(sentences)
	return fixture.withTranslationData(t, translation, pair)
}

// coveringTranslation is a perseus-eng2-shaped file with a segment for every
// book.section the sentences cite, and the first such Citation for the spot
// check.
func coveringTranslation(sentences [][]latinToken) ([]byte, string) {
	var order []string
	sections := map[string][]string{}
	first := ""
	for _, sentence := range sentences {
		for _, token := range sentence {
			book, section, ok := strings.Cut(token.citation, ".")
			if !ok || strings.Contains(section, ".") || slices.Contains(sections[book], section) {
				continue
			}
			if _, seen := sections[book]; !seen {
				order = append(order, book)
			}
			sections[book] = append(sections[book], section)
			if first == "" {
				first = token.citation
			}
		}
	}
	var out strings.Builder
	out.WriteString(`<TEI>`)
	for _, book := range order {
		fmt.Fprintf(&out, `<div type="textpart" subtype="book" n="%s">`, book)
		for _, section := range sections[book] {
			fmt.Fprintf(&out, `<div type="textpart" subtype="section" n="%s"><p>Segment %s.%s.</p></div>`, section, book, section)
		}
		out.WriteString(`</div>`)
	}
	out.WriteString(`</TEI>`)
	return []byte(out.String()), first
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// options generates from the fixture's archive.
func (f latinFixture) options(t *testing.T, previous []byte) Options {
	t.Helper()
	return Options{Layers: f.layers, Overrides: overrideFile(t, f.override), Manifest: previous, UpstreamDir: f.upstream}
}

// twoBooks is a clean Work: book 1 then book 2, with a praenomen and a real
// one-letter word among the words.
func twoBooks() [][]latinToken {
	return [][]latinToken{
		latinSentence("1.1", append([]latinToken{{"Gallia", "n", "1.1"}, {"M", "n", "1.1"}, {"a", "r", "1.1"}}, latinWordsIn("1.1", 7)...)...),
		latinSentence("1.2", latinWordsIn("1.2", 10)...),
		latinSentence("2.1", latinWordsIn("2.1", 10)...),
		latinSentence("2.3", latinWordsIn("2.3", 10)...),
	}
}

func TestLatinGenerationPinsTheMembersFromTheArchive(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	result := generate(t, fixture.options(t, nil))

	require.Len(t, result.Definitions, 1)
	pin := result.Definitions[0].Source
	require.Len(t, pin.Members, 2)
	archive, err := os.ReadFile(filepath.Join(fixture.upstream, latinArchiveName))
	require.NoError(t, err)
	for index, want := range [][]byte{fixture.conllu, fixture.paula} {
		member := pin.Members[index]
		assert.Equal(t, []string{latinConllu, latinPaula}[index], member.Name)
		assert.EqualValues(t, len(want), member.Size)
		assert.Equal(t, sha256Hex(want), member.SHA256)
		// The pin is exactly what preparation will fetch and verify.
		start, end, err := olaarchive.Window(member, int64(len(archive)))
		require.NoError(t, err)
		data, err := olaarchive.Extract(archive[start:end+1], member)
		require.NoError(t, err)
		assert.Equal(t, want, data)
	}
	assert.Equal(t, latinConllu, pin.FileName)
	assert.Equal(t, pin.Members[0].SHA256, pin.SHA256)
	assert.Equal(t, classical.SourceFormatOLACoNLLU, pin.Format)
	assert.Positive(t, pin.Members[1].HeaderOffset)
	assert.Greater(t, pin.Members[1].HeaderOffset, pin.Members[0].HeaderOffset)
}

func TestLatinGenerationWritesTheDefinitionFields(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	definition := generate(t, fixture.options(t, nil)).Definitions[0]

	assert.Equal(t, classical.LanguageLatin, definition.Language)
	assert.Equal(t, "De bello Gallico", definition.OriginalTitle)
	assert.Equal(t, classical.InterpretationVersionOLA, definition.InterpretationVersion)
	assert.Equal(t, "1.1", definition.CitationStart)
	assert.Equal(t, "2.3", definition.CitationEnd)
	assert.Equal(t, 40, definition.WordCount)
	assert.Equal(t, "Gallia M a uerbuma uerbumb uerbumc uerbumd uerbume uerbumf uerbumg.", definition.Opening)
	assert.Nil(t, definition.Contents, "this Work's override does not name books")
}

func TestLatinOriginalTitleIsKeptOnlyWhenItDiffersFromTheWorkTitle(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	fixture.override.WorkTitle = "De bello Gallico"
	fixture.override.EditionKey = "caesar-same-title"
	layers, err := classical.ParseLayers(fixture.layers)
	require.NoError(t, err)
	layers.Layers[0].Files[0].WorkTitle = "De bello Gallico"
	fixture.layers, err = json.Marshal(layers)
	require.NoError(t, err)

	definition := generate(t, fixture.options(t, nil)).Definitions[0]
	assert.Empty(t, definition.OriginalTitle)
}

func TestLatinConstituentBooksAppearOnlyForSchemesThatNameBooks(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	fixture.override.Contents = classical.ContentsBooks
	definition := generate(t, fixture.options(t, nil)).Definitions[0]

	assert.Equal(t, 2, definition.ConstituentBooks)
	require.Len(t, definition.Contents, 2)
	assert.Equal(t, classical.ConstituentBookSpan{Book: 1, CitationStart: "1.1", CitationEnd: "1.2", WordCount: 20}, definition.Contents[0])
	assert.Equal(t, classical.ConstituentBookSpan{Book: 2, CitationStart: "2.1", CitationEnd: "2.3", WordCount: 20}, definition.Contents[1])

	// A speech's outer level is a chapter: without contents: books the same
	// text has one constituent book and no book spans.
	fixture.override.Contents = ""
	flat := generate(t, fixture.options(t, nil)).Definitions[0]
	assert.Equal(t, 1, flat.ConstituentBooks)
	assert.Empty(t, flat.Contents)

	// Naming books for a Work with one outer number is a mistake.
	single := newLatinFixture(t, latinSentence("1.1", latinWordsIn("1.1", 5)...), latinSentence("1.2", latinWordsIn("1.2", 5)...))
	single.override.Contents = classical.ContentsBooks
	_, err := Generate(single.options(t, nil))
	require.ErrorIs(t, err, errContentsOverride)
}

func TestLatinGateRefusesAnUncitedTokenEvenWithAReviewerNote(t *testing.T) {
	sentences := twoBooks()
	sentences[1][2].citation = ""
	fixture := newLatinFixture(t, sentences...)
	_, err := Generate(fixture.options(t, nil))
	require.NoError(t, err, "a new candidate that fails a gate is skipped, not fatal")
	result := generate(t, fixture.options(t, nil))
	assert.Empty(t, result.Definitions)
	require.Len(t, result.Gates, 1)
	assert.Equal(t, "- Gates for caesar-gallic-war:", result.Gates[0].Lines()[0])
	assert.Contains(t, result.Notes, "1 of 44 tokens have no Citation")
	assert.Contains(t, result.Notes, "Rejected new candidate caesar-gallic-war")

	fixture.override.ReviewerNote = "Read; fine."
	assert.Empty(t, generate(t, fixture.options(t, nil)).Definitions, "an uncited token cannot be cleared")
}

func TestLatinGateRefusesBackwardsCitationsUntilAReviewerClearsThem(t *testing.T) {
	sentences := twoBooks()
	sentences = append(sentences, latinSentence("1.2", latinWordsIn("1.2", 4)...))
	fixture := newLatinFixture(t, sentences...)

	refused := generate(t, fixture.options(t, nil))
	assert.Empty(t, refused.Definitions)
	assert.Contains(t, refused.Notes, "citations: FAIL (Citations run backwards 1 time(s), first at 1.2)")
	assert.Contains(t, refused.Notes, "  - excluded")

	fixture.override.ReviewerNote = "Read in full; the source repeats a passage."
	cleared := generate(t, fixture.options(t, nil))
	require.Len(t, cleared.Definitions, 1)
	assert.Contains(t, cleared.GateReport(), "citations: FAIL")
	assert.Contains(t, cleared.GateReport(), "cleared by reviewer note: Read in full; the source repeats a passage.")
	assert.True(t, cleared.Gates[0].Admitted())
}

func TestLatinGateRefusesASplitWordRateAboveTheThreshold(t *testing.T) {
	split := latinWordsIn("1.1", 90)
	for _, letter := range []string{"x", "q", "b"} {
		split = append(split, latinToken{form: letter, pos: "n", citation: "1.1"})
	}
	fixture := newLatinFixture(t, latinSentence("1.1", split...))

	refused := generate(t, fixture.options(t, nil))
	assert.Empty(t, refused.Definitions)
	assert.Contains(t, refused.Notes, "split-words: FAIL (3 of 93 words, 3.226%, limit 2.000%)")

	fixture.override.ReviewerNote = "Abbreviation-heavy; read in full."
	cleared := generate(t, fixture.options(t, nil))
	require.Len(t, cleared.Definitions, 1)
	assert.Contains(t, cleared.Notes, "cleared by reviewer note")
}

func TestLatinGateAcceptsRealOneLetterWordsAndPraenomina(t *testing.T) {
	words := latinWordsIn("1.1", 20)
	for _, letter := range []string{"a", "e", "o", "i", "M", "C", "L"} {
		words = append(words, latinToken{form: letter, pos: "n", citation: "1.1"})
	}
	fixture := newLatinFixture(t, latinSentence("1.1", words...))
	result := generate(t, fixture.options(t, nil))
	require.Len(t, result.Definitions, 1)
	assert.Contains(t, result.Notes, "split-words: pass (0 of 27 words")
}

func TestLatinGateReportListsEveryGateOfEveryWork(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	report := generate(t, fixture.options(t, nil)).GateReport()
	assert.Equal(t, strings.Join([]string{
		"- Gates for caesar-gallic-war:",
		"  - citations: pass (44 tokens, all cited, none backwards)",
		"  - split-words: pass (0 of 40 words, 0.000%, limit 2.000%)",
		"  - translation-join: pass (4 of 4 sentences, 100.000%, floor 98.000%)",
		"  - spot-check: pass (Justin checked 1 pair(s))",
		"  - dictionary-match: not evaluated (no dictionary index was given; generation evaluates it)",
		"",
	}, "\n"), report)
}

func TestLatinVerifyReproducesTheCommittedManifestWithoutTheArchive(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	fixture.override.Contents = classical.ContentsBooks
	generated := generate(t, fixture.options(t, nil))

	verify := fixture.options(t, generated.Manifest)
	verify.UpstreamDir = t.TempDir()
	verify.AllowMissingUpstream = true
	verified := generate(t, verify)
	assert.Equal(t, string(generated.Manifest), string(verified.Manifest))
	assert.Contains(t, verified.GateReport(), "not evaluated")

	// Generating again from the archive also reproduces it byte for byte.
	assert.Equal(t, string(generated.Manifest), string(generate(t, fixture.options(t, generated.Manifest)).Manifest))

	// A hand-edited pin is caught even without the archive.
	verify.Manifest = bytes.Replace(generated.Manifest, []byte(`"sha256":"`), []byte(`"sha256":"0`), 1)
	_, err := Generate(verify)
	require.Error(t, err)
}

func TestLatinGenerationRefusesAnArchiveThatIsNotThePinnedOne(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	archive := filepath.Join(fixture.upstream, latinArchiveName)
	require.NoError(t, os.WriteFile(archive, []byte("not a zip"), 0o600))
	result := generate(t, fixture.options(t, nil))
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "Rejected new candidate caesar-gallic-war")
}

func TestLatinGateStopsTheRunForAWorkAlreadyInTheManifest(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	committed := generate(t, fixture.options(t, nil)).Manifest

	// The Work's text now fails the split-word gate; withdrawing its Books
	// quietly would strand the learners who own them.
	split := latinWordsIn("1.1", 40)
	for _, letter := range []string{"x", "q", "b"} {
		split = append(split, latinToken{form: letter, pos: "n", citation: "1.1"})
	}
	changed := newLatinFixture(t, latinSentence("1.1", split...))
	_, err := Generate(changed.options(t, committed))
	var gate *GateError
	require.ErrorAs(t, err, &gate)
	assert.Equal(t, []string{GateSplitWord}, []string{gate.Report.Failed()[0].Name})
}

// writeLatinGolden regenerates the checked-in Latin fixture when MOUSEION_UPDATE_GOLDEN is set.
func writeLatinGolden(t *testing.T) {
	t.Helper()
	fixture := newLatinFixture(t, twoBooks()...)
	fixture.override.Contents = classical.ContentsBooks
	result := generate(t, fixture.options(t, nil))
	dir := filepath.Join("testdata", "latin")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "layers.json"), fixture.layers, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "overrides.jsonl"), overrideFile(t, fixture.override), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.jsonl"), result.Manifest, 0o600))
}

func TestVerifyReproducesTheCheckedInLatinManifestWithoutTheArchive(t *testing.T) {
	if os.Getenv("MOUSEION_UPDATE_GOLDEN") != "" {
		writeLatinGolden(t)
	}
	dir := filepath.Join("testdata", "latin")
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // fixture path in the repository.
		require.NoError(t, err)
		return data
	}
	manifest := read("manifest.jsonl")
	result := generate(t, Options{Layers: read("layers.json"), Overrides: read("overrides.jsonl"), Manifest: manifest, AllowMissingUpstream: true})
	assert.Equal(t, string(manifest), string(result.Manifest))
	require.Len(t, result.Definitions, 1)
	assert.Equal(t, classical.LanguageLatin, result.Definitions[0].Language)

	// Dropping contents: books from the override is drift offline too.
	var override classical.Override
	require.NoError(t, json.Unmarshal(read("overrides.jsonl"), &override))
	override.Contents = ""
	_, err := Generate(Options{Layers: read("layers.json"), Overrides: overrideFile(t, override), Manifest: manifest, AllowMissingUpstream: true})
	require.ErrorIs(t, err, errContentsOverride)
}

func TestLatinMalformedCitationLayerIsReportedNotDropped(t *testing.T) {
	sentences := twoBooks()
	sentences[0][0].citation = "not a citation!"
	result := generate(t, newLatinFixture(t, sentences...).options(t, nil))
	require.Len(t, result.Gates, 1)
	assert.False(t, result.Gates[0].Admitted())
	assert.Contains(t, result.GateReport(), "unreadable")
}

func TestLatinWorkMayOpenWithANamedSection(t *testing.T) {
	fixture := newLatinFixture(t,
		latinSentence("pr.1", latinWordsIn("pr.1", 6)...),
		latinSentence("1.1", latinWordsIn("1.1", 6)...),
	)
	definition := generate(t, fixture.options(t, nil)).Definitions[0]
	assert.Equal(t, "pr.1", definition.CitationStart)
	assert.Equal(t, "1.1", definition.CitationEnd)
}
