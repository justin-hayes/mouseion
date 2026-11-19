package classicalmanifest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureInputs reads the reviewed fixture tree.
func fixtureInputs(t *testing.T) ([]byte, []byte, string) {
	t.Helper()
	layers, err := os.ReadFile(filepath.Join("testdata", "layers.json"))
	require.NoError(t, err)
	overrides, err := os.ReadFile(filepath.Join("testdata", "overrides.jsonl"))
	require.NoError(t, err)
	return layers, overrides, filepath.Join("testdata", "upstream")
}

// fixtureLayers reads only the fixture layers.json.
func fixtureLayers(t *testing.T) []byte {
	t.Helper()
	layers, _, _ := fixtureInputs(t)
	return layers
}

// copyUpstream copies the fixture files into a temporary upstream tree.
func copyUpstream(t *testing.T) string {
	t.Helper()
	_, _, original := fixtureInputs(t)
	dir := t.TempDir()
	for _, name := range []string{"work.conllu", "work.xml"} {
		data, err := os.ReadFile(filepath.Join(original, name)) //nolint:gosec // fixture path in the repository.
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o600)) //nolint:gosec // test temp path.
	}
	return dir
}

// baseOverride is the fixture Work override.
func baseOverride() classical.Override {
	revision := 1
	return classical.Override{
		WorkKey:      "alpha",
		WorkTitle:    "Alpha",
		WorkAuthor:   "Author",
		GreekTitle:   "Ἄλφα",
		EditionKey:   "alpha-test",
		EditionLabel: "Test Edition",
		Source:       &classical.SourceSpec{Layer: "fixture-source", FileName: "work.conllu", Format: classical.SourceFormatOGACoNLLU},
		Translation:  &classical.TranslationSpec{Layer: "fixture-translation", FileName: "work.xml"},
		Split:        classical.SplitConstituentBooks,
		Books:        []int{1},
		Revision:     &revision,
		Note:         "Fixture curatorial sign-off.",
	}
}

// overrideFile renders overrides.
func overrideFile(t *testing.T, overrides ...classical.Override) []byte {
	t.Helper()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	for _, override := range overrides {
		require.NoError(t, encoder.Encode(override))
	}
	return buffer.Bytes()
}

// layersForUpstream re-pins every fixture file after a test mutates it.
func layersForUpstream(t *testing.T, data []byte, upstream string) []byte {
	t.Helper()
	layers, err := classical.ParseLayers(data)
	require.NoError(t, err)
	for index := range layers.Layers {
		for fileIndex := range layers.Layers[index].Files {
			name := layers.Layers[index].Files[fileIndex].Name
			_, size, digest, err := readUpstream(filepath.Join(upstream, name))
			require.NoError(t, err)
			layers.Layers[index].Files[fileIndex].Size = size
			layers.Layers[index].Files[fileIndex].SHA256 = digest
		}
	}
	encoded, err := json.Marshal(layers)
	require.NoError(t, err)
	return encoded
}

// generate runs the generator and fails the test on an unexpected error.
func generate(t *testing.T, options Options) Result {
	t.Helper()
	result, err := Generate(options)
	require.NoError(t, err)
	return result
}

func TestGenerateAndVerifyProduceDeterministicSortedJSONL(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	overrides := overrideFile(t, baseOverride())
	options := Options{Layers: layers, Overrides: overrides, UpstreamDir: upstream}

	first := generate(t, options)
	second := generate(t, options)
	assert.Equal(t, first.Manifest, second.Manifest)
	require.Len(t, first.Definitions, 1)
	assert.Equal(t, "alpha-1", first.Definitions[0].Key)
	assert.Equal(t, "alpha", first.Definitions[0].WorkKey)
	assert.Equal(t, "alpha-test", first.Definitions[0].EditionKey)
	assert.Equal(t, 1, *first.Definitions[0].ConstituentBook)
	assert.Equal(t, 2, first.Definitions[0].ConstituentBooks)
	assert.Equal(t, 8, first.Definitions[0].WordCount)
	assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;", first.Definitions[0].Opening)
	require.NotNil(t, first.Definitions[0].Translation)
	assert.Contains(t, first.Notes, "Crossing sentences: none")
	assert.Contains(t, first.Notes, "translation available")

	verified := generate(t, Options{Layers: layers, Overrides: overrides, Manifest: first.Manifest, AllowMissingUpstream: true})
	assert.Equal(t, first.Manifest, verified.Manifest)
	assert.Equal(t, first.Definitions[0].Opening, verified.Definitions[0].Opening, "a run without upstream retains the committed Opening")

	definition, err := classical.ParseDefinitions(first.Manifest)
	require.NoError(t, err)
	assert.Equal(t, first.Definitions, definition)
}

func TestGeneratedKeysAreSortedAcrossWorks(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	beta := baseOverride()
	beta.WorkKey = "zeta"
	beta.WorkTitle = "Zeta"
	beta.EditionKey = "zeta-test"
	beta.Source = &classical.SourceSpec{Layer: "fixture-source", FileName: "work.conllu", Format: classical.SourceFormatOGACoNLLU}
	overrides := overrideFile(t, baseOverride(), beta)
	result := generate(t, Options{Layers: layers, Overrides: overrides, UpstreamDir: upstream})
	require.Len(t, result.Definitions, 2)
	assert.Equal(t, []string{"alpha-1", "zeta-1"}, []string{result.Definitions[0].Key, result.Definitions[1].Key})
}

func TestPinMismatchStopsAnExistingEntry(t *testing.T) {
	layers := fixtureLayers(t)
	upstream := copyUpstream(t)
	overrides := overrideFile(t, baseOverride())
	options := Options{Layers: layers, Overrides: overrides, UpstreamDir: upstream}
	first := generate(t, options)
	appendFile(t, filepath.Join(upstream, "work.conllu"), "\n")
	options.Manifest = first.Manifest
	_, err := Generate(options)
	require.ErrorContains(t, err, "pin mismatch")
}

func TestInadmissibleSourceLayerIsRefused(t *testing.T) {
	layers, overrides, upstream := fixtureInputs(t)
	layers = bytes.Replace(layers, []byte(`"restriction": "share-alike"`), []byte(`"restriction": "nc"`), 1)
	_, err := Generate(Options{Layers: layers, Overrides: overrides, UpstreamDir: upstream})
	require.ErrorContains(t, err, "not redistributable")
}

func TestUnparseableFileIsSkippedForANewCandidateAndStopsAnExistingEntry(t *testing.T) {
	layers := fixtureLayers(t)
	upstream := copyUpstream(t)
	overrides := overrideFile(t, baseOverride())
	options := Options{Layers: layers, Overrides: overrides, UpstreamDir: upstream}
	result := generate(t, options)
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "work.conllu"), []byte("not a conllu file\n"), 0o600))
	options.Layers = layersForUpstream(t, layers, upstream)
	options.Manifest = nil
	skipped := generate(t, options)
	assert.Empty(t, skipped.Definitions)
	assert.Contains(t, skipped.Notes, "Rejected new candidate alpha")
	options.Manifest = result.Manifest
	_, err := Generate(options)
	require.ErrorContains(t, err, "existing alpha failed a check")
}

func TestUnknownNativePOSIsRefused(t *testing.T) {
	layers := fixtureLayers(t)
	upstream := copyUpstream(t)
	data, err := os.ReadFile(filepath.Join(upstream, "work.conllu")) //nolint:gosec // test temp path.
	require.NoError(t, err)
	data = bytes.Replace(data, []byte("\tv\tv3spia---"), []byte("\tz\tv3spia---"), 1)
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "work.conllu"), data, 0o600)) //nolint:gosec // test temp path.
	result := generate(t, Options{Layers: layersForUpstream(t, layers, upstream), Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "source_unknown_pos")
}

func TestMissingSelectedBookIsSkippedForANewCandidate(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Books = []int{9}
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "selected book 9 has no citation range")

	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "selected book 9 has no citation range")
}

func TestUnassignedSentenceStopsAnExistingWork(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	override := baseOverride()
	override.Units = []classical.UnitSpec{{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.1.2"}}
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "sentence_unassigned")
}

func TestOversizeFileStopsAnExistingEntry(t *testing.T) {
	layers := fixtureLayers(t)
	upstream := copyUpstream(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	require.NoError(t, os.Truncate(filepath.Join(upstream, "work.conllu"), MaxSourceBytes+1))
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "larger than")
}

func TestTranslationMisalignmentIsRefused(t *testing.T) {
	layers := fixtureLayers(t)
	upstream := copyUpstream(t)
	data, err := os.ReadFile(filepath.Join(upstream, "work.xml")) //nolint:gosec // test temp path.
	require.NoError(t, err)
	data = bytes.Replace(data, []byte(`<div type="textpart" subtype="section" n="2"><p>Book one, section two.</p></div>`), nil, 1)
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "work.xml"), data, 0o600)) //nolint:gosec // test temp path.
	result := generate(t, Options{Layers: layersForUpstream(t, layers, upstream), Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "cited range ends at a missing section")
}

func TestContentChangeNeedsARevisionBump(t *testing.T) {
	layers := fixtureLayers(t)
	upstream := copyUpstream(t)
	options := Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream}
	first := generate(t, options)
	appendFile(t, filepath.Join(upstream, "work.conllu"), "\n")
	options.Layers = layersForUpstream(t, layers, upstream)
	options.Manifest = first.Manifest
	_, err := Generate(options)
	require.ErrorContains(t, err, "content changed without a revision bump")

	revision := 2
	override := baseOverride()
	override.Revision = &revision
	options.Overrides = overrideFile(t, override)
	updated := generate(t, options)
	require.Len(t, updated.Definitions, 1)
	assert.Equal(t, 2, updated.Definitions[0].Revision)
}

func TestDecreasedRevisionIsRefused(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	revision := 2
	override := baseOverride()
	override.Revision = &revision
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	lower := 1
	override.Revision = &lower
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "revision decreased")
}

func TestCuratorialChangeNeedsNoRevision(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	override := baseOverride()
	override.WorkTitle = "Alpha Revised"
	updated := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), Manifest: first.Manifest, UpstreamDir: upstream})
	require.Len(t, updated.Definitions, 1)
	assert.Equal(t, "Alpha Revised", updated.Definitions[0].WorkTitle)
	assert.Equal(t, 1, updated.Definitions[0].Revision)
}

func TestRemovedEntryBecomesAWithdrawnTombstone(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	removed := generate(t, Options{Layers: layers, Overrides: nil, Manifest: first.Manifest, AllowMissingUpstream: true})
	require.Len(t, removed.Definitions, 1)
	tombstone := removed.Definitions[0]
	assert.True(t, tombstone.Withdrawn)
	assert.Equal(t, "alpha-1", tombstone.Key)
	assert.Equal(t, first.Definitions[0].Revision, tombstone.Revision)
	assert.Equal(t, first.Definitions[0].Source, tombstone.Source)
}

func TestOverrideDepartureWithoutANoteIsRefused(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	firstOverride := baseOverride()
	firstOverride.Note = ""
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, firstOverride), UpstreamDir: upstream})
	departing := firstOverride
	departing.WorkTitle = "A different title"
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, departing), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "departs from the upstream Work title without a note")
}

func TestUnitOverrideDepartureWithoutANoteIsRefused(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	unit := classical.Override{Key: "alpha-1", GreekTitle: "Ἄλφα ἄλλη"}
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride(), unit), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "departs from its Work Greek title without a note")
}

func TestWholeWorkSplitCreatesOneUnit(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Split = classical.SplitWholeWork
	override.Books = nil
	override.Translation = nil
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.Len(t, result.Definitions, 1)
	definition := result.Definitions[0]
	assert.Equal(t, "alpha", definition.Key)
	assert.Nil(t, definition.ConstituentBook)
	assert.Equal(t, 1, definition.ConstituentBooks)
	assert.Equal(t, "1.1.1", definition.CitationStart)
	assert.Equal(t, "2.1.1", definition.CitationEnd)
	assert.Equal(t, 10, definition.WordCount)
	assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;", definition.Opening)
}

func TestGenerateOpeningSkipsASpeakerLabel(t *testing.T) {
	cases := []struct {
		name  string
		label string
	}{
		{"abbreviation", speakerLabelSentence("ΣΩ", "σω", false)},
		{"elliptical abbreviation", speakerLabelSentence("ΜΕΝ", "μεν", true)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			layers := fixtureLayers(t)
			upstream := copyUpstream(t)
			original, err := os.ReadFile(filepath.Join(upstream, "work.conllu")) //nolint:gosec // test temp path.
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(upstream, "work.conllu"), append([]byte(testCase.label), original...), 0o600)) //nolint:gosec // test temp path.

			result := generate(t, Options{Layers: layersForUpstream(t, layers, upstream), Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
			require.Len(t, result.Definitions, 1)
			assert.Equal(t, "λέγει Κῦρος· καὶ λέγουσι;", result.Definitions[0].Opening)
		})
	}
}

func TestGenerateFailsWhenAUnitHasOnlySpeakerLabels(t *testing.T) {
	layers := fixtureLayers(t)
	upstream := copyUpstream(t)
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "work.conllu"), []byte(speakerLabelSentence("ΜΕΝ", "μεν", true)), 0o600))
	override := baseOverride()
	override.Translation = nil

	_, err := Generate(Options{Layers: layersForUpstream(t, layers, upstream), Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.ErrorContains(t, err, "opening_missing", "the generator fails rather than guessing an Opening")
}

func TestVerifyWithoutUpstreamFailsWhenTheCommittedOpeningIsMissing(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	definitions, err := classical.ParseDefinitions(first.Manifest)
	require.NoError(t, err)
	definitions[0].Opening = ""
	withoutOpening, err := classical.RenderDefinitions(definitions)
	require.NoError(t, err)

	_, err = Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), Manifest: withoutOpening, AllowMissingUpstream: true})
	require.ErrorContains(t, err, "has no generated Opening", "a run without upstream cannot invent a missing Opening")
}

func TestOpeningChangeNeedsNoRevisionBump(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	definitions, err := classical.ParseDefinitions(first.Manifest)
	require.NoError(t, err)
	definitions[0].Opening = "a drifted Opening"
	tampered, err := classical.RenderDefinitions(definitions)
	require.NoError(t, err)

	updated := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), Manifest: tampered, UpstreamDir: upstream})
	assert.Equal(t, first.Manifest, updated.Manifest, "the source-derived Opening replaces the drifted one")
	assert.Equal(t, 1, updated.Definitions[0].Revision, "an Opening change alone needs no revision bump")
}

// speakerLabelSentence renders one source sentence that is only a dialogue
// speaker's abbreviation, optionally with an elliptical node.
func speakerLabelSentence(surface, lemma string, elliptical bool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# source = Xenophon, Xen. Anab._1.1.1\n# text = %s", surface)
	if elliptical {
		b.WriteString(" [0]")
	}
	b.WriteString(" .\n# sent_id = label\n")
	fmt.Fprintf(&b, "1\t%s\t%s\tn\tn-s---mv-\tCase=v|Gender=m|Number=s\t0\tExD\t_\tt_101|0|ref=Xen. Anab._1.1.1\n", surface, lemma)
	terminal := 2
	if elliptical {
		b.WriteString("2\t[0]\t[0]\t-\t---------\t_\t0\tPRED\t_\te_101|0|ref=Xen. Anab._1.1.1\n")
		terminal = 3
	}
	fmt.Fprintf(&b, "%d\t.\t.\tu\tu--------\t_\t1\tAuxK\t_\tt_102|t_101|ref=Xen. Anab._1.1.1\n\n", terminal)
	return b.String()
}

func TestExplicitUnitSplitSelectsOneBook(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Units = []classical.UnitSpec{
		{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.1.2"},
		{Book: 2, CitationStart: "2.1.1", CitationEnd: "2.1.1"},
	}
	override.Books = []int{2}
	override.Translation = nil
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.Len(t, result.Definitions, 1)
	assert.Equal(t, "alpha-2", result.Definitions[0].Key)
	assert.Equal(t, 2, result.Definitions[0].ConstituentBooks)
	assert.Equal(t, 2, result.Definitions[0].WordCount)
}

func TestNewWorkWithoutUpstreamFilesIsSkipped(t *testing.T) {
	layers, _, _ := fixtureInputs(t)
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride())})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "no upstream directory")
}

func TestChangedEditionKeyNeedsANewReadingUnitKey(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	override := baseOverride()
	override.EditionKey = "alpha-other-edition"
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "changed its Work or Edition key")
}

func TestUnitRevisionCanBumpWithoutContentChange(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	revision := 2
	unit := classical.Override{Key: "alpha-1", Revision: &revision}
	updated := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride(), unit), Manifest: first.Manifest, UpstreamDir: upstream})
	require.Len(t, updated.Definitions, 1)
	assert.Equal(t, 2, updated.Definitions[0].Revision)
}

func TestWorkLayerAndFormatRefusals(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Source.Layer = "missing"
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.ErrorContains(t, err, "unknown source layer")

	override = baseOverride()
	override.Source.Format = "epub"
	_, err = Generate(Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.ErrorContains(t, err, "unsupported source format")

	override = baseOverride()
	override.WorkTitle = ""
	_, err = Generate(Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.ErrorContains(t, err, "needs workTitle")
}

func TestDuplicateOverridesAreRefused(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride(), baseOverride()), UpstreamDir: upstream})
	require.ErrorContains(t, err, "duplicate work override")

	unit := classical.Override{Key: "alpha-1"}
	_, err = Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride(), unit, unit), UpstreamDir: upstream})
	require.ErrorContains(t, err, "duplicate unit override")
}

func TestChangedCitationRangeNeedsANewKey(t *testing.T) {
	previous := classical.Definition{Key: "alpha-1", CitationStart: "1.1.1", CitationEnd: "1.1.2"}
	next := previous
	next.CitationEnd = "1.1.3"
	require.ErrorContains(t, checkIdentity(previous, next), "changed its citation range")
}

func TestEmptyCitationRangeIsSkippedForANewCandidate(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Units = []classical.UnitSpec{
		{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.1.2"},
		{Book: 2, CitationStart: "2.1.1", CitationEnd: "2.1.1"},
		{Book: 3, CitationStart: "3.1.1", CitationEnd: "3.1.2"},
	}
	override.Books = []int{3}
	override.Translation = nil
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "citation_range_empty")
}

func TestExplicitUnitCannotSpanConstituentBooks(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Units = []classical.UnitSpec{{Book: 1, CitationStart: "1.1.1", CitationEnd: "2.1.1"}}
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "spans or misses its constituent book")
}

func TestTranslationRemovalNeedsARevisionBump(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	withoutTranslation := baseOverride()
	withoutTranslation.Translation = nil
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, withoutTranslation), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "content changed without a revision bump")

	revision := 2
	withoutTranslation.Revision = &revision
	updated := generate(t, Options{Layers: layers, Overrides: overrideFile(t, withoutTranslation), Manifest: first.Manifest, UpstreamDir: upstream})
	require.Len(t, updated.Definitions, 1)
	assert.Nil(t, updated.Definitions[0].Translation)
	assert.Equal(t, 2, updated.Definitions[0].Revision)
}

// withLayerClass reclasses one fixture layer and drops any passage credit,
// since layers.json permits credit only on Source content.
func withLayerClass(t *testing.T, layers []byte, key, class, restriction string) []byte {
	t.Helper()
	parsed, err := classical.ParseLayers(layers)
	require.NoError(t, err)
	found := false
	for index := range parsed.Layers {
		if parsed.Layers[index].Key != key {
			continue
		}
		found = true
		parsed.Layers[index].LicenceClass = class
		parsed.Layers[index].Restriction = restriction
		for fileIndex := range parsed.Layers[index].Files {
			parsed.Layers[index].Files[fileIndex].PassageLabel = ""
			parsed.Layers[index].Files[fileIndex].DeckNotice = ""
		}
	}
	require.True(t, found, "fixture layer %q", key)
	encoded, err := json.Marshal(parsed)
	require.NoError(t, err)
	return encoded
}

func TestNonSourceContentLayersAreRefusedForBothRoles(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	cases := []struct {
		name      string
		layer     string
		wantLayer string
	}{
		{"source", "fixture-source", "source layer"},
		{"translation", "fixture-translation", "translation layer"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reclassed := withLayerClass(t, layers, testCase.layer, classical.LicenceClassAnalysisResource, classical.RestrictionUnclear)

			skipped := generate(t, Options{Layers: reclassed, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
			require.Empty(t, skipped.Definitions, "a new candidate whose %s is not Source content is skipped", testCase.name)
			require.Contains(t, skipped.Notes, "Rejected new candidate alpha")
			require.Contains(t, skipped.Notes, testCase.wantLayer)
			require.Contains(t, skipped.Notes, "not Source content")

			first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
			_, err := Generate(Options{Layers: reclassed, Overrides: overrideFile(t, baseOverride()), Manifest: first.Manifest, AllowMissingUpstream: true})
			require.ErrorContains(t, err, "existing alpha failed a check", "an existing entry whose %s is not Source content stops the run", testCase.name)
			require.ErrorContains(t, err, testCase.wantLayer)
			require.ErrorContains(t, err, "not Source content")
		})
	}
}

// withoutPassageCredit removes the fixture translation's card label and deck
// notice from layers.json.
func withoutPassageCredit(t *testing.T, layers []byte) []byte {
	t.Helper()
	parsed, err := classical.ParseLayers(layers)
	require.NoError(t, err)
	for index := range parsed.Layers {
		for fileIndex := range parsed.Layers[index].Files {
			parsed.Layers[index].Files[fileIndex].PassageLabel = ""
			parsed.Layers[index].Files[fileIndex].DeckNotice = ""
		}
	}
	encoded, err := json.Marshal(parsed)
	require.NoError(t, err)
	return encoded
}

func TestTranslationWithoutPassageCreditIsRefused(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	uncredited := withoutPassageCredit(t, layers)

	skipped := generate(t, Options{Layers: uncredited, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	assert.Empty(t, skipped.Definitions, "a new candidate whose translation has no credit is skipped")
	assert.Contains(t, skipped.Notes, "needs a passageLabel and deckNotice")

	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	_, err := Generate(Options{Layers: uncredited, Overrides: overrideFile(t, baseOverride()), Manifest: first.Manifest, AllowMissingUpstream: true})
	require.ErrorContains(t, err, "needs a passageLabel and deckNotice", "verify mode refuses an uncredited translation without the upstream files")
}

func TestShippedTranslationStaysCreditable(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})

	withoutTranslation := baseOverride()
	withoutTranslation.Translation = nil
	revision := 2
	withoutTranslation.Revision = &revision
	_, err := Generate(Options{Layers: withoutPassageCredit(t, layers), Overrides: overrideFile(t, withoutTranslation), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "must stay in layers.json with its passage credit", "a frozen deck still needs the credit of a dropped translation")

	updated := generate(t, Options{Layers: layers, Overrides: overrideFile(t, withoutTranslation), Manifest: first.Manifest, UpstreamDir: upstream})
	assert.Nil(t, updated.Definitions[0].Translation)
}

func TestVerifyDetectsAHandEditedPinWithoutUpstreamFiles(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	tampered := bytes.Replace(first.Manifest, []byte("ea12338efc8169d5f688e7a8d34d6bb93e9b9d1575a31d3fcf276e086efbf2ea"), []byte(strings.Repeat("0", 64)), 1)
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), Manifest: tampered, AllowMissingUpstream: true})
	require.ErrorContains(t, err, "pin does not match layers.json")
}

func TestVerifyDetectsAHandEditedTranslationPinWithoutUpstreamFiles(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	tampered := bytes.Replace(first.Manifest, []byte("ee173f989f4e43bf1654300d50ae970eb8839798bca5c787d36ef120ac126bcd"), []byte(strings.Repeat("0", 64)), 1)
	require.NotEqual(t, first.Manifest, tampered)
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), Manifest: tampered, AllowMissingUpstream: true})
	require.ErrorContains(t, err, "pin does not match layers.json")
}

func TestAWorkWithoutATranslationNeedsNoPassageCredit(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	withoutTranslation := baseOverride()
	withoutTranslation.Translation = nil
	first := generate(t, Options{Layers: withoutPassageCredit(t, layers), Overrides: overrideFile(t, withoutTranslation), UpstreamDir: upstream})
	again := generate(t, Options{Layers: withoutPassageCredit(t, layers), Overrides: overrideFile(t, withoutTranslation), Manifest: first.Manifest, UpstreamDir: upstream})
	assert.Equal(t, first.Manifest, again.Manifest)
}

// appendFile appends text to a file.
func appendFile(t *testing.T, path, text string) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600) //nolint:gosec // test temp path.
	require.NoError(t, err)
	_, err = file.WriteString(text)
	require.NoError(t, err)
	require.NoError(t, file.Close())
}
