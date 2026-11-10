package classical

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validDefinition() Definition {
	book := 1
	return Definition{
		Key:                   "alpha-1",
		WorkKey:               "alpha",
		WorkTitle:             "Alpha",
		WorkAuthor:            "Author",
		GreekTitle:            "Ἄλφα",
		EditionKey:            "alpha-test",
		EditionLabel:          "Test Edition",
		Language:              Language,
		ConstituentBook:       &book,
		ConstituentBooks:      2,
		CitationStart:         "1.1.1",
		CitationEnd:           "1.1.2",
		WordCount:             8,
		Source:                SourcePin{Layer: "source", FileName: "work.conllu", Size: 10, SHA256: strings.Repeat("a", 64), Format: SourceFormatOGACoNLLU},
		Translation:           &TranslationPin{Layer: "translation", FileName: "work.xml", Size: 10, SHA256: strings.Repeat("b", 64), CitationStart: "1.1.1", CitationEnd: "1.1.2"},
		InterpretationVersion: "oga-agdt-upos/5",
		Revision:              1,
		Note:                  "Reviewed.",
	}
}

func TestDefinitionValidateAcceptsACompleteDefinition(t *testing.T) {
	require.NoError(t, validDefinition().Validate())

	whole := validDefinition()
	whole.ConstituentBook = nil
	whole.ConstituentBooks = 1
	whole.Key = "alpha"
	require.NoError(t, whole.Validate())
}

func TestDefinitionValidateRefusesIncompleteFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Definition)
		want   string
	}{
		{"key", func(d *Definition) { d.Key = "" }, "key is required"},
		{"work key", func(d *Definition) { d.WorkKey = "" }, "Work key is required"},
		{"work title", func(d *Definition) { d.WorkTitle = "" }, "Work title is required"},
		{"work author", func(d *Definition) { d.WorkAuthor = "" }, "Work author is required"},
		{"edition key", func(d *Definition) { d.EditionKey = "" }, "Edition key is required"},
		{"edition label", func(d *Definition) { d.EditionLabel = "" }, "Edition label is required"},
		{"language", func(d *Definition) { d.Language = "de" }, "language must be"},
		{"book count", func(d *Definition) { d.ConstituentBooks = 0 }, "book count must be positive"},
		{"book outside", func(d *Definition) { book := 3; d.ConstituentBook = &book }, "outside its Work"},
		{"whole-work count", func(d *Definition) { d.ConstituentBook = nil }, "whole-work definition has one"},
		{"citation start", func(d *Definition) { d.CitationStart = "1..1" }, "citation start"},
		{"citation end", func(d *Definition) { d.CitationEnd = "1.1.2.3.4/" }, "citation end"},
		{"word count", func(d *Definition) { d.WordCount = 0 }, "word count must be positive"},
		{"source layer", func(d *Definition) { d.Source.Layer = "" }, "source layer is required"},
		{"source file", func(d *Definition) { d.Source.FileName = "" }, "source file name is required"},
		{"source size", func(d *Definition) { d.Source.Size = 0 }, "source file size must be positive"},
		{"source digest", func(d *Definition) { d.Source.SHA256 = strings.ToUpper(d.Source.SHA256) }, "source SHA-256"},
		{"source format", func(d *Definition) { d.Source.Format = "" }, "source format is required"},
		{"interpretation", func(d *Definition) { d.InterpretationVersion = "" }, "interpretation version is required"},
		{"revision", func(d *Definition) { d.Revision = 0 }, "revision must be positive"},
		{"translation layer", func(d *Definition) { d.Translation.Layer = "" }, "translation layer is required"},
		{"translation file", func(d *Definition) { d.Translation.FileName = "" }, "translation file name is required"},
		{"translation size", func(d *Definition) { d.Translation.Size = 0 }, "translation file size must be positive"},
		{"translation digest", func(d *Definition) { d.Translation.SHA256 = "bad" }, "translation SHA-256"},
		{"translation start", func(d *Definition) { d.Translation.CitationStart = "x" }, "translation citation start"},
		{"translation end", func(d *Definition) { d.Translation.CitationEnd = "" }, "translation citation end"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definition := validDefinition()
			test.mutate(&definition)
			require.ErrorContains(t, definition.Validate(), test.want)
		})
	}
}

func validLayer() Layer {
	return Layer{
		Key:                 "source",
		Release:             "v1",
		BaseURL:             "https://example.test/data/",
		LicenceClass:        LicenceClassSourceContent,
		Restriction:         RestrictionShareAlike,
		DeployerRegisterRow: "Fixture row",
		Files:               []LayerFile{{Name: "work.conllu", Size: 10, SHA256: strings.Repeat("a", 64)}},
	}
}

func TestLayerValidateAcceptsAdmissibleLayers(t *testing.T) {
	require.NoError(t, validLayer().Validate())
	analysis := validLayer()
	analysis.LicenceClass = LicenceClassAnalysisResource
	analysis.Restriction = RestrictionUnclear
	analysis.Files = nil
	require.NoError(t, analysis.Validate())
}

func TestLayerValidateRefusesInadmissibleLayers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Layer)
		want   string
	}{
		{"key", func(l *Layer) { l.Key = "" }, "layer key is required"},
		{"release", func(l *Layer) { l.Release = ""; l.Commit = "" }, "release or commit"},
		{"url", func(l *Layer) { l.BaseURL = "" }, "base URL is required"},
		{"row", func(l *Layer) { l.DeployerRegisterRow = "" }, "deployer-register row is required"},
		{"bad url", func(l *Layer) { l.BaseURL = "ftp://example.test" }, "not an absolute HTTP URL"},
		{"nc source", func(l *Layer) { l.Restriction = RestrictionNC }, "not redistributable"},
		{"class", func(l *Layer) { l.LicenceClass = "other" }, "unknown licence class"},
		{"analysis restriction", func(l *Layer) { l.LicenceClass = LicenceClassAnalysisResource; l.Restriction = "other" }, "unknown restriction"},
		{"file name", func(l *Layer) { l.Files[0].Name = "" }, "file without a name"},
		{"file pin", func(l *Layer) { l.Files[0].SHA256 = "" }, "needs both size and SHA-256"},
		{"file digest", func(l *Layer) { l.Files[0].SHA256 = "bad" }, "SHA-256 is not 64"},
		{"label without notice", func(l *Layer) { l.Files[0].PassageLabel = "Label" }, "needs both a passage label and a deck notice"},
		{"notice without label", func(l *Layer) { l.Files[0].DeckNotice = "Notice" }, "needs both a passage label and a deck notice"},
		{"credited analysis file", func(l *Layer) {
			l.LicenceClass = LicenceClassAnalysisResource
			l.Restriction = RestrictionUnclear
			l.Files[0].PassageLabel, l.Files[0].DeckNotice = "Label", "Notice"
		}, "not Source content"},
		{"credited unpinned file", func(l *Layer) {
			l.Files[0].Size, l.Files[0].SHA256 = 0, ""
			l.Files[0].PassageLabel, l.Files[0].DeckNotice = "Label", "Notice"
		}, "without a SHA-256 pin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layer := validLayer()
			test.mutate(&layer)
			require.ErrorContains(t, layer.Validate(), test.want)
		})
	}
}

func TestLayersValidateAndLookup(t *testing.T) {
	layers := Layers{Layers: []Layer{validLayer()}}
	require.NoError(t, layers.Validate())
	layer, ok := layers.Layer("source")
	require.True(t, ok)
	file, ok := layer.File("work.conllu")
	require.True(t, ok)
	assert.Equal(t, int64(10), file.Size)
	_, ok = layer.File("missing")
	assert.False(t, ok)
	_, ok = layers.Layer("missing")
	assert.False(t, ok)

	require.ErrorContains(t, (Layers{}).Validate(), "no layers")
	require.ErrorContains(t, (Layers{Layers: []Layer{validLayer(), validLayer()}}).Validate(), "duplicate layer key")
}

func TestOverrideValidateAcceptsWorkAndUnitOverrides(t *testing.T) {
	revision := 1
	work := Override{
		WorkKey: "alpha", WorkTitle: "Alpha", WorkAuthor: "Author", EditionKey: "alpha-test", EditionLabel: "Test",
		Source:      &SourceSpec{Layer: "source", FileName: "work.conllu", Format: SourceFormatOGACoNLLU},
		Translation: &TranslationSpec{Layer: "translation", FileName: "work.xml"},
		Split:       SplitConstituentBooks, Books: []int{1}, Revision: &revision,
	}
	require.NoError(t, work.Validate())
	assert.True(t, work.IsWork())
	assert.Equal(t, "work:alpha", OverrideSortKey(work))

	unit := Override{Key: "alpha-1", WorkTitle: "Alpha", Note: "Unit note", Revision: &revision}
	require.NoError(t, unit.Validate())
	assert.False(t, unit.IsWork())
	assert.Equal(t, "unit:alpha-1", OverrideSortKey(unit))
}

func TestOverrideValidateRefusesIncompleteOverrides(t *testing.T) {
	revision := 1
	zero := 0
	tests := []struct {
		name     string
		override Override
		want     string
	}{
		{"both", Override{WorkKey: "a", Key: "b"}, "either workKey or key"},
		{"neither", Override{}, "needs workKey or key"},
		{"revision", Override{Key: "a", Revision: &zero}, "revision must be positive"},
		{"unit structural", Override{Key: "a", Source: &SourceSpec{}}, "may only change curatorial"},
		{"unit edition", Override{Key: "a", EditionKey: "edition"}, "may only change curatorial"},
		{"no source", Override{WorkKey: "a", Split: SplitWholeWork, EditionKey: "e"}, "needs a source"},
		{"incomplete source", Override{WorkKey: "a", Source: &SourceSpec{Layer: "s"}, Split: SplitWholeWork, EditionKey: "e"}, "incomplete source"},
		{"split", Override{WorkKey: "a", Source: &SourceSpec{Layer: "s", FileName: "f", Format: "x"}, Split: "chapters", EditionKey: "e"}, "unknown split"},
		{"translation", Override{WorkKey: "a", Source: &SourceSpec{Layer: "s", FileName: "f", Format: "x"}, Split: SplitWholeWork, EditionKey: "e", Translation: &TranslationSpec{Layer: "t"}}, "incomplete translation"},
		{"edition", Override{WorkKey: "a", Source: &SourceSpec{Layer: "s", FileName: "f", Format: "x"}, Split: SplitWholeWork}, "needs an Edition key"},
		{"whole books", Override{WorkKey: "a", Source: &SourceSpec{Layer: "s", FileName: "f", Format: "x"}, Split: SplitWholeWork, EditionKey: "e", Books: []int{1}}, "cannot select books"},
		{"unit range", Override{WorkKey: "a", Source: &SourceSpec{Layer: "s", FileName: "f", Format: "x"}, Split: SplitConstituentBooks, EditionKey: "e", Units: []UnitSpec{{Book: -1}}}, "invalid unit range"},
	}
	_ = revision
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorContains(t, test.override.Validate(), test.want)
		})
	}
}

func TestSortOverridesIsStableByKindAndKey(t *testing.T) {
	overrides := []Override{{Key: "b"}, {WorkKey: "z"}, {Key: "a"}, {WorkKey: "a"}}
	SortOverrides(overrides)
	assert.Equal(t, []Override{{Key: "a"}, {Key: "b"}, {WorkKey: "a"}, {WorkKey: "z"}}, overrides)
}

func TestParseDefinitionsAndRenderRoundTrip(t *testing.T) {
	definition := validDefinition()
	definition.Note = "a < b & c"
	definitions := []Definition{definition}
	data, err := RenderDefinitions(definitions)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(string(data), "\n"))
	assert.Contains(t, string(data), "a < b & c")
	assert.NotContains(t, string(data), "\\u003c")
	parsed, err := ParseDefinitions(data)
	require.NoError(t, err)
	assert.Equal(t, definitions, parsed)
	assert.Empty(t, mustParseDefinitions(t, nil))
}

func TestParseDefinitionsRefusesInvalidLines(t *testing.T) {
	_, err := ParseDefinitions([]byte("{\n"))
	require.Error(t, err)
	_, err = ParseDefinitions([]byte(`{"key":"x","unknown":true}` + "\n"))
	require.Error(t, err)
}

func TestParseOverridesAndLayers(t *testing.T) {
	overrideLine := []byte(`{"key":"alpha-1","note":"Reviewed"}` + "\n")
	overrides, err := ParseOverrides(overrideLine)
	require.NoError(t, err)
	require.Len(t, overrides, 1)
	_, err = ParseOverrides([]byte("{\n"))
	require.Error(t, err)

	layerData := []byte(`{"layers":[{"key":"source","release":"v1","baseURL":"https://example.test/","licenceClass":"source-content","restriction":"share-alike","deployerRegisterRow":"row"}]}`)
	layers, err := ParseLayers(layerData)
	require.NoError(t, err)
	require.Len(t, layers.Layers, 1)
	_, err = ParseLayers([]byte(`{"layers":[]}`))
	require.Error(t, err)
	_, err = ParseLayers(append(layerData, layerData...))
	require.Error(t, err)
}

func TestCitationAndDigestHelpers(t *testing.T) {
	assert.True(t, isCitation("1.2.3"))
	assert.True(t, isCitation("0.1"))
	assert.False(t, isCitation("1.02.3"))
	assert.False(t, isCitation("1..3"))
	assert.False(t, isCitation("x"))
	assert.True(t, isLowerHexSHA256(strings.Repeat("a", 64)))
	assert.False(t, isLowerHexSHA256(strings.Repeat("A", 64)))
	assert.False(t, isLowerHexSHA256("a"))
}

func mustParseDefinitions(t *testing.T, data []byte) []Definition {
	t.Helper()
	definitions, err := ParseDefinitions(data)
	require.NoError(t, err)
	return definitions
}

func TestLayersPassageAttribution(t *testing.T) {
	credited := validLayer()
	credited.Files[0].PassageLabel, credited.Files[0].DeckNotice = "Fixture (2026), Alph.", "Fixture notice."
	require.NoError(t, credited.Validate())
	uncredited := validLayer()
	uncredited.Key = "other"
	uncredited.Files[0].SHA256 = strings.Repeat("b", 64)
	layers := Layers{Layers: []Layer{uncredited, credited}}

	for _, pin := range []string{strings.Repeat("a", 64), "sha256:" + strings.Repeat("a", 64), " sha256:" + strings.Repeat("a", 64) + " "} {
		got, ok := layers.PassageAttribution(pin)
		require.True(t, ok, pin)
		assert.Equal(t, PassageAttribution{Label: "Fixture (2026), Alph.", Notice: "Fixture notice."}, got)
	}
	for _, pin := range []string{"", "sha256:", "sha256:" + strings.Repeat("b", 64), "sha256:" + strings.Repeat("c", 64)} {
		_, ok := layers.PassageAttribution(pin)
		assert.False(t, ok, pin)
	}
}
