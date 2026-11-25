package classicalmanifest

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/olaarchive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const latinTranslationFile = "perseus-eng2.xml"

// citedSentences is n sentences of ten words each, one per section 1.1 ... 1.n,
// with the citations in unresolved replaced by a suffixed form the one-level
// translation below has no segment for.
func citedSentences(n int, suffixed ...int) [][]latinToken {
	sentences := make([][]latinToken, n)
	for i := range sentences {
		citation := fmt.Sprintf("1.%d", i+1)
		if slices.Contains(suffixed, i+1) {
			citation = fmt.Sprintf("1.%da", i+1)
		}
		sentences[i] = latinSentence(citation, latinWordsIn(citation, 10)...)
	}
	return sentences
}

// translationXML is a perseus-eng2-shaped file with one segment per section
// 1.1 ... 1.n.
func translationXML(n int) []byte {
	var out strings.Builder
	out.WriteString(`<TEI><div type="textpart" subtype="book" n="1">`)
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&out, `<div type="textpart" subtype="section" n="%d"><p>Segment %d.</p></div>`, i, i)
	}
	out.WriteString(`</div></TEI>`)
	return []byte(out.String())
}

// withTranslation pins a synthetic perseus-engN file to the fixture and records
// a spot check, as an admitted Work carries.
func (f latinFixture) withTranslation(t *testing.T, sections int) latinFixture {
	t.Helper()
	data := translationXML(sections)
	require.NoError(t, os.WriteFile(filepath.Join(f.upstream, latinTranslationFile), data, 0o600))
	layers, err := classical.ParseLayers(f.layers)
	require.NoError(t, err)
	layers.Layers = append(layers.Layers, classical.Layer{
		Key: "perseus-eng", Release: "fixture-1", BaseURL: "https://example.test/translation/",
		LicenceClass: classical.LicenceClassSourceContent, Restriction: classical.RestrictionShareAlike, DeployerRegisterRow: "synthetic translation",
		Files: []classical.LayerFile{{Name: latinTranslationFile, Size: int64(len(data)), SHA256: sha256Hex(data), PassageLabel: "Synthetic (2026)", DeckNotice: "Passage context: synthetic."}},
	})
	f.layers, err = json.Marshal(layers)
	require.NoError(t, err)
	f.override.Translation = &classical.TranslationSpec{Layer: "perseus-eng", FileName: latinTranslationFile}
	f.override.SpotCheck = &classical.SpotCheck{Reviewer: "Justin", Pairs: []string{"1.1", "1.2"}}
	return f
}

func TestTranslationJoinPassesWhenEverySentenceResolves(t *testing.T) {
	fixture := newLatinFixture(t, citedSentences(5)...)
	fixture = fixture.withTranslation(t, 5)
	result := generate(t, fixture.options(t, nil))

	require.Len(t, result.Definitions, 1)
	assert.NotNil(t, result.Definitions[0].Translation)
	assert.Contains(t, result.GateReport(), "  - translation-join: pass (5 of 5 sentences, 100.000%, floor 98.000%)")
	assert.Contains(t, result.GateReport(), "  - spot-check: pass (Justin checked 2 pair(s))")
}

func TestTranslationJoinFailsBelowTheFloorAndAReviewerNoteClearsIt(t *testing.T) {
	fixture := newLatinFixture(t, citedSentences(10, 3)...)
	fixture = fixture.withTranslation(t, 10)
	refused := generate(t, fixture.options(t, nil))
	assert.Empty(t, refused.Definitions)
	assert.Contains(t, refused.GateReport(), "translation-join: FAIL (9 of 10 sentences, 90.000%, floor 98.000%)")

	fixture.override.ReviewerNote = "Read in full; the suffixed section is a source oddity."
	cleared := generate(t, fixture.options(t, nil))
	require.Len(t, cleared.Definitions, 1)
	assert.Contains(t, cleared.GateReport(), "cleared by reviewer note")
}

func TestTranslationJoinAcceptsExactlyTheFloor(t *testing.T) {
	// One unresolved sentence in fifty is 98%: at the floor, not below it.
	fixture := newLatinFixture(t, citedSentences(50, 25)...)
	fixture = fixture.withTranslation(t, 50)
	result := generate(t, fixture.options(t, nil))
	require.Len(t, result.Definitions, 1)
	assert.Contains(t, result.GateReport(), "translation-join: pass (49 of 50 sentences, 98.000%")
}

func TestAWorkWithATranslationNeedsARecordedSpotCheck(t *testing.T) {
	fixture := newLatinFixture(t, citedSentences(5)...)
	fixture = fixture.withTranslation(t, 5)
	fixture.override.SpotCheck = nil
	refused := generate(t, fixture.options(t, nil))
	assert.Empty(t, refused.Definitions)
	assert.Contains(t, refused.Notes, "needs a recorded spot check")

	fixture.override.ReviewerNote = "Read in full."
	assert.Empty(t, generate(t, fixture.options(t, nil)).Definitions, "a reviewer note is not a spot check")

	// A Work already in the manifest never fails quietly.
	fixture.override.ReviewerNote = ""
	fixture.override.SpotCheck = &classical.SpotCheck{Reviewer: "Justin", Pairs: []string{"1.1"}}
	committed := generate(t, fixture.options(t, nil)).Manifest
	fixture.override.SpotCheck = nil
	_, err := Generate(fixture.options(t, committed))
	require.ErrorContains(t, err, "needs a recorded spot check")
}

func TestASpotCheckPairOutsideTheWorkIsRefusedEvenWithAReviewerNote(t *testing.T) {
	fixture := newLatinFixture(t, citedSentences(5)...)
	fixture = fixture.withTranslation(t, 5)
	fixture.override.SpotCheck.Pairs = []string{"1.9"}
	fixture.override.ReviewerNote = "Read in full."
	result := generate(t, fixture.options(t, nil))
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.GateReport(), "spot-check: FAIL (checked pair 1.9 is not a Citation of the Work)")
}

func TestTranslationJoinIsNotEvaluatedWithoutTheTranslationFile(t *testing.T) {
	fixture := newLatinFixture(t, citedSentences(5)...)
	fixture = fixture.withTranslation(t, 5)
	generated := generate(t, fixture.options(t, nil))

	verify := fixture.options(t, generated.Manifest)
	verify.UpstreamDir = t.TempDir()
	verify.AllowMissingUpstream = true
	verified := generate(t, verify)
	assert.Equal(t, string(generated.Manifest), string(verified.Manifest))
}

// lookupOf is a Lewis & Short lookup over a fixed set of headwords.
func lookupOf(headwords ...string) LatinLSLookup {
	set := map[string]bool{}
	for _, headword := range headwords {
		set[headword] = true
	}
	return func(lemma string) bool { return set[lemma] }
}

// lemmasFor lists the ten fixture lemmas uerbuma ... uerbumj.
func lemmasFor(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("uerbum%c", 'a'+i)
	}
	return out
}

func TestDictionaryMatchPassesAtTheFloorCountingOnlyEligibleOccurrences(t *testing.T) {
	// Ten nouns, and a preposition no dictionary entry matches: a part of
	// speech outside the eligible set never counts for or against the Work.
	words := append(latinWordsIn("1.1", 10), latinToken{form: "ad", pos: "r", citation: "1.1"})
	fixture := newLatinFixture(t, latinSentence("1.1", words...))
	options := fixture.options(t, nil)
	options.Generating = true
	options.LatinLS = lookupOf(lemmasFor(9)...)
	result := generate(t, options)
	require.Len(t, result.Definitions, 1)
	assert.Contains(t, result.GateReport(), "dictionary-match: pass (9 of 10 eligible occurrences, 90.000%, floor 90.000%)")
}

func TestDictionaryMatchFailsBelowTheFloorAndAReviewerNoteClearsIt(t *testing.T) {
	fixture := newLatinFixture(t, latinSentence("1.1", latinWordsIn("1.1", 10)...))
	options := fixture.options(t, nil)
	options.Generating = true
	options.LatinLS = lookupOf(lemmasFor(8)...)
	refused := generate(t, options)
	assert.Empty(t, refused.Definitions)
	assert.Contains(t, refused.GateReport(), "dictionary-match: FAIL (8 of 10 eligible occurrences, 80.000%, floor 90.000%)")

	fixture.override.ReviewerNote = "Read in full; the unmatched lemmas are Greek loan words."
	options = fixture.options(t, nil)
	options.Generating = true
	options.LatinLS = lookupOf(lemmasFor(8)...)
	cleared := generate(t, options)
	require.Len(t, cleared.Definitions, 1)
	assert.Contains(t, cleared.GateReport(), "cleared by reviewer note")
}

func TestGeneratingALatinWorkNeedsTheDictionaryIndex(t *testing.T) {
	fixture := newLatinFixture(t, twoBooks()...)
	options := fixture.options(t, nil)
	options.Generating = true
	_, err := Generate(options)
	require.ErrorIs(t, err, errNeedsLSIndex)
}

func TestDictionaryGateNeverRunsInVerifyMode(t *testing.T) {
	fixture := newLatinFixture(t, latinSentence("1.1", latinWordsIn("1.1", 10)...))
	options := fixture.options(t, nil)
	options.Generating = true
	options.LatinLS = lookupOf(lemmasFor(10)...)
	generated := generate(t, options)

	// Verify has the archive but neither the index nor the lookup, and
	// reproduces the manifest byte for byte.
	verified := generate(t, fixture.options(t, generated.Manifest))
	assert.Equal(t, string(generated.Manifest), string(verified.Manifest))
	assert.Contains(t, verified.GateReport(), "dictionary-match: not evaluated")
}

func TestJoinGatesOnSyntheticEvidence(t *testing.T) {
	segments := []ancientgreek.ReferenceTranslation{{Citation: "1.2", Text: "two"}, {Citation: "1.3.1", Text: "three one"}}
	gate := translationJoinGate([][]string{{"1.2.5"}, {"1.2"}, {"1.3.1", "1.3.2"}, {"1.4"}}, segments)
	assert.False(t, gate.Passed)
	assert.Contains(t, gate.Detail, "3 of 4 sentences")
	assert.False(t, translationJoinGate(nil, segments).Passed, "a Work with no sentences cannot join")
	assert.False(t, dictionaryMatchGate(nil, lookupOf()).Passed, "a Work with no eligible occurrences cannot match")
	assert.True(t, GateResult{Passed: true, NotEvaluated: true}.Passed)
}

// nestedLatinFixture is newLatinFixture with each member in a stored inner zip
// of the outer archive, as the real OLA archive holds them.
func nestedLatinFixture(t *testing.T, containerMethod uint16, sentences ...[]latinToken) latinFixture {
	t.Helper()
	fixture := newLatinFixture(t, sentences...)
	inner := func(name string, data []byte) []byte {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Extra: []byte{0x55, 0x58, 0x04, 0x00, 1, 2, 3, 4}})
		require.NoError(t, err)
		_, err = entry.Write(data)
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		return buffer.Bytes()
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, member := range []struct {
		name string
		data []byte
	}{{"workspace/other.txt", bytes.Repeat([]byte("filler "), 50)}, {"workspace/conllu.zip", inner(latinConllu, fixture.conllu)}, {"workspace/paula.zip", inner(latinPaula, fixture.paula)}} {
		method := containerMethod
		if member.name == "workspace/other.txt" {
			method = zip.Deflate
		}
		entry, err := writer.CreateHeader(&zip.FileHeader{Name: member.name, Method: method, Extra: []byte{0x55, 0x58, 0x04, 0x00, 1, 2, 3, 4}})
		require.NoError(t, err)
		_, err = entry.Write(member.data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	require.NoError(t, os.WriteFile(filepath.Join(fixture.upstream, latinArchiveName), buffer.Bytes(), 0o600))
	layers, err := classical.ParseLayers(fixture.layers)
	require.NoError(t, err)
	layers.Layers[0].ArchiveSize = int64(buffer.Len())
	fixture.layers, err = json.Marshal(layers)
	require.NoError(t, err)
	fixture.override.Source.Container = "workspace/conllu.zip"
	fixture.override.Source.CitationContainer = "workspace/paula.zip"
	return fixture
}

func TestNestedMembersArePinnedAsAbsoluteOffsetsInTheOuterArchive(t *testing.T) {
	fixture := nestedLatinFixture(t, zip.Store, twoBooks()...)
	result := generate(t, fixture.options(t, nil))
	require.Len(t, result.Definitions, 1)

	archive, err := os.ReadFile(filepath.Join(fixture.upstream, latinArchiveName))
	require.NoError(t, err)
	pin := result.Definitions[0].Source
	for index, want := range [][]byte{fixture.conllu, fixture.paula} {
		member := pin.Members[index]
		start, end, err := olaarchive.Window(member, int64(len(archive)))
		require.NoError(t, err)
		data, err := olaarchive.Extract(archive[start:end+1], member)
		require.NoError(t, err)
		assert.Equal(t, want, data, "the single-level reader fetches the nested member unchanged")
	}

	// The pin does not record the nesting, so verify needs no archive.
	verify := fixture.options(t, result.Manifest)
	verify.UpstreamDir = t.TempDir()
	verify.AllowMissingUpstream = true
	assert.Equal(t, string(result.Manifest), string(generate(t, verify).Manifest))
}

func TestANestedZipThatIsNotStoredIsRefused(t *testing.T) {
	fixture := nestedLatinFixture(t, zip.Deflate, twoBooks()...)
	result := generate(t, fixture.options(t, nil))
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "is not stored")
}
