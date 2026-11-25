package classicalmanifest

import (
	"bytes"
	"encoding/json"
	"errors"
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
		Split:        classical.SplitWholeWork,
		Contents:     classical.ContentsBooks,
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
	assert.Equal(t, "alpha", first.Definitions[0].Key)
	assert.Equal(t, "alpha", first.Definitions[0].WorkKey)
	assert.Equal(t, "alpha-test", first.Definitions[0].EditionKey)
	assert.Nil(t, first.Definitions[0].ConstituentBook)
	assert.Equal(t, 2, first.Definitions[0].ConstituentBooks)
	assert.Equal(t, 10, first.Definitions[0].WordCount)
	assert.Equal(t, []classical.ConstituentBookSpan{
		{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.1.2", WordCount: 8},
		{Book: 2, CitationStart: "2.1.1", CitationEnd: "2.1.1", WordCount: 2},
	}, first.Definitions[0].Contents)
	assert.Equal(t, "2.1.1", first.Definitions[0].Translation.CitationEnd, "the translation covers the whole Edition")
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
	assert.Equal(t, []string{"alpha", "zeta"}, []string{result.Definitions[0].Key, result.Definitions[1].Key})
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

func TestConstituentBooksSplitKindIsRejected(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Split = "constituent-books"
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.ErrorContains(t, err, `unknown split "constituent-books"`)

	_, err = Generate(Options{Layers: layers, Overrides: []byte(`{"workKey":"alpha","books":[1]}` + "\n"), UpstreamDir: upstream})
	require.Error(t, err, "the per-book selection field no longer exists")
}

func TestUnassignedSentenceStopsAnExistingWork(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	first := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	override := baseOverride()
	override.Contents = ""
	override.Units = []classical.UnitSpec{{CitationStart: "1.1.1", CitationEnd: "1.1.2"}}
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
	data = bytes.Replace(data, []byte(`<div type="textpart" subtype="section" n="1"><p>Book two, section one.</p></div>`), nil, 1)
	require.NoError(t, os.WriteFile(filepath.Join(upstream, "work.xml"), data, 0o600)) //nolint:gosec // test temp path.
	result := generate(t, Options{Layers: layersForUpstream(t, layers, upstream), Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "Rejected new candidate alpha", "a translation that cannot cover the whole Edition refuses the unit")
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
	assert.Equal(t, "alpha", tombstone.Key)
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
	unit := classical.Override{Key: "alpha", GreekTitle: "Ἄλφα ἄλλη"}
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride(), unit), Manifest: first.Manifest, UpstreamDir: upstream})
	require.ErrorContains(t, err, "departs from its Work Greek title without a note")
}

func TestBookRangesWithoutContentsOverrideAreRefused(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	override := baseOverride()
	override.Contents = ""
	override.Translation = nil
	_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	require.ErrorContains(t, err, "multiple book ranges but no contents")
}

func TestGeneratePreservesTheSourceCitationScheme(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	whole := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})

	t.Run("hand-reviewed bounds cannot hide derived books", func(t *testing.T) {
		override := baseOverride()
		override.Contents = ""
		override.Units = []classical.UnitSpec{{CitationStart: "1.1.1", CitationEnd: "2.1.1"}}
		opts := Options{Layers: layers, Overrides: overrideFile(t, override), Manifest: whole.Manifest, UpstreamDir: upstream}
		_, err := Generate(opts)
		require.ErrorContains(t, err, "cannot replace derived book contents")
		opts.UpstreamDir, opts.AllowMissingUpstream = "", true
		_, err = Generate(opts)
		require.ErrorContains(t, err, "cannot replace derived book contents")
	})
	t.Run("backwards hand-reviewed bounds are refused", func(t *testing.T) {
		override := baseOverride()
		override.Contents = ""
		override.Units = []classical.UnitSpec{{CitationStart: "2.1.1", CitationEnd: "1.1.1"}}
		_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), Manifest: whole.Manifest, UpstreamDir: upstream})
		require.ErrorContains(t, err, "backwards citation range")
	})
	t.Run("flat citations are not constituent books", func(t *testing.T) {
		root := copyUpstream(t)
		path := filepath.Join(root, "work.conllu")
		data, err := os.ReadFile(path) //nolint:gosec // test temp path.
		require.NoError(t, err)
		replacer := strings.NewReplacer("_1.1.1", "_17", "_1.1.2", "_18", "_2.1.1", "_19")
		require.NoError(t, os.WriteFile(path, []byte(replacer.Replace(string(data))), 0o600)) //nolint:gosec // test temp path.
		pins := layersForUpstream(t, layers, root)
		override := baseOverride()
		override.Contents, override.Translation = "", nil
		flat := generate(t, Options{Layers: pins, Overrides: overrideFile(t, override), UpstreamDir: root})
		require.Len(t, flat.Definitions, 1)
		require.Equal(t, 1, flat.Definitions[0].ConstituentBooks)
		require.Empty(t, flat.Definitions[0].Contents)
		override.Units = []classical.UnitSpec{{CitationStart: "17", CitationEnd: "19"}}
		reviewed := overrideFile(t, override)
		for name, opts := range map[string]Options{
			"with upstream":    {Layers: pins, Overrides: reviewed, Manifest: flat.Manifest, UpstreamDir: root},
			"without upstream": {Layers: pins, Overrides: reviewed, Manifest: flat.Manifest, AllowMissingUpstream: true},
		} {
			t.Run(name, func(t *testing.T) {
				require.Equal(t, flat.Manifest, generate(t, opts).Manifest)
			})
		}
		override.Units = nil
		override.Contents = classical.ContentsBooks
		_, err = Generate(Options{Layers: pins, Overrides: overrideFile(t, override), Manifest: flat.Manifest, UpstreamDir: root})
		require.ErrorContains(t, err, "source has no multiple book ranges")
		_, err = Generate(Options{Layers: pins, Overrides: overrideFile(t, override), Manifest: flat.Manifest, AllowMissingUpstream: true})
		require.ErrorContains(t, err, "committed manifest has no book ranges")
	})
	t.Run("a different whole-work key is not an offline source", func(t *testing.T) {
		previous := whole.Definitions[0]
		previous.Key = "alpha-old"
		manifest, err := classical.RenderDefinitions([]classical.Definition{previous})
		require.NoError(t, err)
		_, err = Generate(Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), Manifest: manifest, AllowMissingUpstream: true})
		require.ErrorContains(t, err, "new work alpha needs the upstream source file")
	})
	t.Run("book zero is refused", func(t *testing.T) {
		root := copyUpstream(t)
		path := filepath.Join(root, "work.conllu")
		data, err := os.ReadFile(path) //nolint:gosec // test temp path.
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path, bytes.ReplaceAll(data, []byte("_1.1."), []byte("_0.1.")), 0o600)) //nolint:gosec // test temp path.
		_, err = Generate(Options{Layers: layersForUpstream(t, layers, root), Overrides: overrideFile(t, baseOverride()), Manifest: whole.Manifest, UpstreamDir: root})
		require.ErrorContains(t, err, `citation "0.1.1" has no constituent book number`)
	})
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
	override.Contents = ""
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

func TestWholeWorkWithBooksRecordsEachConstituentBook(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, baseOverride()), UpstreamDir: upstream})
	require.Len(t, result.Definitions, 1)
	definition := result.Definitions[0]
	assert.Equal(t, "alpha", definition.Key)
	assert.Equal(t, 2, definition.ConstituentBooks)
	require.Len(t, definition.Contents, 2)
	words := 0
	for _, book := range definition.Contents {
		words += book.WordCount
	}
	assert.Equal(t, definition.WordCount, words, "the contents sum to the whole Work")
	assert.Contains(t, result.Notes, "whole work (2 books)")
}

// perBookManifest renders the ADR 0116 shape of a generated whole-work
// definition: one definition per constituent book.
func perBookManifest(t *testing.T, whole classical.Definition) []byte {
	t.Helper()
	definitions := make([]classical.Definition, 0, len(whole.Contents))
	for _, book := range whole.Contents {
		number := book.Book
		definition := whole
		definition.Key = fmt.Sprintf("%s-%d", whole.WorkKey, number)
		definition.ConstituentBook = &number
		definition.Withdrawn = true
		definition.CitationStart, definition.CitationEnd, definition.WordCount = book.CitationStart, book.CitationEnd, book.WordCount
		definition.Contents = nil
		translation := *whole.Translation
		translation.CitationStart, translation.CitationEnd = book.CitationStart, book.CitationEnd
		definition.Translation = &translation
		definitions = append(definitions, definition)
	}
	manifest, err := classical.RenderDefinitions(definitions)
	require.NoError(t, err)
	return manifest
}

func TestPerBookDefinitionsBecomeTombstonesUnderTheWholeWorkUnit(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	overrides := overrideFile(t, baseOverride())
	whole := generate(t, Options{Layers: layers, Overrides: overrides, UpstreamDir: upstream}).Definitions[0]
	legacy := perBookManifest(t, whole)

	for name, options := range map[string]Options{
		"with upstream":    {Layers: layers, Overrides: overrides, Manifest: legacy, UpstreamDir: upstream},
		"without upstream": {Layers: layers, Overrides: overrides, Manifest: legacy, AllowMissingUpstream: true},
	} {
		t.Run(name, func(t *testing.T) {
			result := generate(t, options)
			byKey := map[string]classical.Definition{}
			for _, definition := range result.Definitions {
				byKey[definition.Key] = definition
			}
			require.Len(t, byKey, 3)
			assert.Equal(t, whole, byKey["alpha"], "the whole-work definition is the same offline and with upstream")
			assert.False(t, byKey["alpha"].Withdrawn)
			assert.True(t, byKey["alpha-1"].Withdrawn)
			assert.True(t, byKey["alpha-2"].Withdrawn)

			again := generate(t, Options{Layers: layers, Overrides: overrides, Manifest: result.Manifest, AllowMissingUpstream: true})
			assert.Equal(t, result.Manifest, again.Manifest, "the committed manifest reproduces offline")
		})
	}
}

func TestNewWholeWorkUnitOfAWorkWithCommittedBooksNeverFailsQuietly(t *testing.T) {
	failing := &builder{previousByKey: map[string]classical.Definition{"alpha-1": {Key: "alpha-1", WorkKey: "alpha"}}}
	err := failing.candidateFailure("alpha", "alpha", errors.New("boom"))
	require.ErrorContains(t, err, "existing alpha failed a check", "skipping would withdraw the Books learners own")

	fresh := &builder{previousByKey: map[string]classical.Definition{}}
	require.NoError(t, fresh.candidateFailure("alpha", "alpha", errors.New("boom")), "a brand-new Work is only rejected")
	assert.Contains(t, fresh.notes.String(), "Rejected new candidate alpha")
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
	unit := classical.Override{Key: "alpha", Revision: &revision}
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
	override.Contents = ""
	override.Units = []classical.UnitSpec{{CitationStart: "3.1.1", CitationEnd: "3.1.2"}}
	override.Translation = nil
	result := generate(t, Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
	assert.Empty(t, result.Definitions)
	assert.Contains(t, result.Notes, "Rejected new candidate")
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

func TestGeneratorAcceptsNamedSectionsAndRefusesUnknownNamesAndLeadingZeros(t *testing.T) {
	layers, _, upstream := fixtureInputs(t)
	for _, test := range []struct {
		start, end string
		wantErr    string
	}{
		{start: "1.1.1", end: "1.1.2"},
		{start: "1.pr.1", end: "1.1.2"},
		{start: "331a", end: "331b"},
		{start: "1.1", end: "1.ep"},
		{start: "1.1.1", end: "1.praef.2", wantErr: "invalid unit range"},
		{start: "1.01.1", end: "1.1.2", wantErr: "invalid unit range"},
	} {
		override := baseOverride()
		override.Contents = ""
		override.Units = []classical.UnitSpec{{CitationStart: test.start, CitationEnd: test.end}}
		override.Translation = nil
		if test.wantErr == "" {
			require.NoError(t, override.Validate(), "%s-%s", test.start, test.end)
		}
		_, err := Generate(Options{Layers: layers, Overrides: overrideFile(t, override), UpstreamDir: upstream})
		if test.wantErr != "" {
			require.ErrorContains(t, err, test.wantErr, "%s-%s", test.start, test.end)
		} else if err != nil {
			assert.NotContains(t, err.Error(), "invalid unit range", "%s-%s", test.start, test.end)
		}
	}
}
