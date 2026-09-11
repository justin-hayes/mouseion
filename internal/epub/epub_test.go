package epub

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixtureFile struct {
	name    string
	content string
}

func fixture(t *testing.T) []byte {
	return fixtureDirectory(t, "testdata")
}

func fixtureDirectory(t *testing.T, directory string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func TestPhaseOneEPUBFixturesMatchContract(t *testing.T) {
	t.Run("EPUB 3 nested package and navigation", func(t *testing.T) {
		book, err := Extract(fixtureDirectory(t, "testfixtures/epub3-edge-cases"))
		require.NoError(t, err)
		const wantText = "Shared title\n\nGrüße 👋 aus Köln.\n\nuntitled\n\n東京 café.\n\nShared title\n\nBibliographie."
		assert.Equal(t, "Fixture Three", book.Title)
		assert.Equal(t, "urn:mouseion:phase1", book.SourceIdentifier)
		assert.Equal(t, wantText, book.FullText)
		want := []ExtractedUnit{
			{ID: UnitID(1, "chapter-one"), Order: 0, SpineIndex: 1, Title: "Shared title", TitleSource: UnitTitleHeading, Text: "Shared title\n\nGrüße 👋 aus Köln.", StartOffset: 0, EndOffset: 31, PackagePath: "Books/OPS/package.opf", ManifestID: "chapter-one", SourceHref: "Text/part/one.xhtml", ResolvedHref: "Books/OPS/Text/part/one.xhtml", MediaType: "application/xhtml+xml", Properties: []string{"svg", "mathml"}, Linear: true, NavigationLabels: []string{"Shared label", "Begin"}, LandmarkTypes: []string{"bodymatter", "chapter"}},
			{ID: UnitID(3, "untitled"), Order: 1, SpineIndex: 3, Title: "untitled", TitleSource: UnitTitleManifestID, Text: "untitled\n\n東京 café.", StartOffset: 33, EndOffset: 51, PackagePath: "Books/OPS/package.opf", ManifestID: "untitled", SourceHref: "Text/two.xhtml", ResolvedHref: "Books/OPS/Text/two.xhtml", MediaType: "application/xhtml+xml", Properties: []string{}, Linear: true, NavigationLabels: []string{"No heading"}, LandmarkTypes: []string{}},
			{ID: UnitID(4, "bibliography"), Order: 2, SpineIndex: 4, Title: "Shared title", TitleSource: UnitTitleHeading, Text: "Shared title\n\nBibliographie.", StartOffset: 53, EndOffset: 81, PackagePath: "Books/OPS/package.opf", ManifestID: "bibliography", SourceHref: "Text/back/bibliography.xhtml", ResolvedHref: "Books/OPS/Text/back/bibliography.xhtml", MediaType: "application/xhtml+xml", Properties: []string{}, Linear: true, NavigationLabels: []string{"Works", "Bibliography"}, LandmarkTypes: []string{"bibliography"}},
		}
		assert.Equal(t, ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: want}, book.ExtractedUnits)
		assertCompatibilityProjection(t, book)
	})

	t.Run("EPUB 2 metadata and NCX", func(t *testing.T) {
		book, err := Extract(fixtureDirectory(t, "testfixtures/epub2-clean"))
		require.NoError(t, err)
		assert.Equal(t, "Fixture Two", book.Title)
		assert.Equal(t, "urn:mouseion:epub2", book.SourceIdentifier)
		assert.Equal(t, "First\n\nAlpha.\n\nSecond\n\nOmega.", book.FullText)
		want := ExtractedUnits{SchemaVersion: ExtractedUnitsSchemaVersion, Units: []ExtractedUnit{
			{ID: UnitID(0, "first"), Order: 0, SpineIndex: 0, Title: "First", TitleSource: UnitTitleHeading, Text: "First\n\nAlpha.", StartOffset: 0, EndOffset: 13, PackagePath: "OEBPS/content.opf", ManifestID: "first", SourceHref: "first.xhtml", ResolvedHref: "OEBPS/first.xhtml", MediaType: "application/xhtml+xml", Properties: []string{}, Linear: true, NavigationLabels: []string{}, LandmarkTypes: []string{}},
			{ID: UnitID(2, "second"), Order: 1, SpineIndex: 2, Title: "Second", TitleSource: UnitTitleHeading, Text: "Second\n\nOmega.", StartOffset: 15, EndOffset: 29, PackagePath: "OEBPS/content.opf", ManifestID: "second", SourceHref: "second.xhtml", ResolvedHref: "OEBPS/second.xhtml", MediaType: "application/xhtml+xml", Properties: []string{}, Linear: true, NavigationLabels: []string{}, LandmarkTypes: []string{}},
		}}
		assert.Equal(t, want, book.ExtractedUnits)
		assertCompatibilityProjection(t, book)
	})
}

func TestPhaseOneInvalidFixture(t *testing.T) {
	_, err := Extract(fixtureDirectory(t, "testfixtures/invalid-missing-spine-reference"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidEPUB)
	assert.Contains(t, err.Error(), "spine references missing manifest item absent")
}

func assertCompatibilityProjection(t *testing.T, book ExtractedBook) {
	t.Helper()
	require.Len(t, book.Chapters, len(book.ExtractedUnits.Units))
	for i, unit := range book.ExtractedUnits.Units {
		chapter := book.Chapters[i]
		assert.Equal(t, unit.ManifestID, chapter.ID, "chapter %d", i)
		assert.Equal(t, unit.Title, chapter.Title, "chapter %d", i)
		assert.Equal(t, book.SourceIdentifier, chapter.Location.SourceDocumentID, "chapter %d", i)
		assert.Equal(t, unit.Title, chapter.Location.Chapter, "chapter %d", i)
		assert.Equal(t, unit.Title, chapter.Location.Section, "chapter %d", i)
		assert.Equal(t, unit.StartOffset, chapter.Location.StartOffset, "chapter %d", i)
		assert.Equal(t, unit.EndOffset, chapter.Location.EndOffset, "chapter %d", i)
	}
	require.NoError(t, book.ExtractedUnits.ValidateOffsets(book.FullText))
}

func TestExtractReadingOrderAndLocations(t *testing.T) {
	book, err := Extract(fixture(t))
	require.NoError(t, err)
	want := "Erstes Kapitel\n\nGrüße aus Köln.\n\nZweiter Absatz.\n\nZweites Kapitel\n\nDas Ende."
	assert.Equal(t, want, book.FullText)
	assert.Equal(t, "Die Prüfung", book.Title)
	assert.Equal(t, "urn:isbn:9780000000014", book.SourceIdentifier)
	assert.Len(t, book.Chapters, 2)
	first, second := book.Chapters[0], book.Chapters[1]
	assert.Equal(t, "first", first.ID)
	assert.Equal(t, "Erstes Kapitel", first.Title)
	assert.Equal(t, uint64(0), first.Location.StartOffset)
	assert.Equal(t, uint64(len([]rune("Erstes Kapitel\n\nGrüße aus Köln.\n\nZweiter Absatz."))), first.Location.EndOffset)
	assert.Equal(t, first.Location.EndOffset+2, second.Location.StartOffset)
	assert.Equal(t, uint64(len([]rune(want))), second.Location.EndOffset)
	assert.NotContains(t, book.FullText, "navigation")
	assert.NotContains(t, book.FullText, "steal")
	assert.NotContains(t, book.FullText, "Hidden")
	assert.Equal(t, ExtractedUnitsSchemaVersion, book.ExtractedUnits.SchemaVersion)
	assert.Len(t, book.ExtractedUnits.Units, len(book.Chapters))
	require.NoError(t, book.ExtractedUnits.ValidateOffsets(book.FullText))
}

func TestExtractOrderedUnitsAndNavigationMetadata(t *testing.T) {
	data := epubFixture(t, `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier>book-242</dc:identifier><dc:title>Units</dc:title></metadata><manifest>
<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="scripted nav remote-resources"/>
<item id="first" href="Text/part/first.xhtml" media-type="application/xhtml+xml" properties="svg mathml"/>
<item id="hidden" href="Text/hidden.xhtml" media-type="application/xhtml+xml"/>
<item id="second" href="Text/second.xhtml" media-type="application/xhtml+xml"/>
</manifest><spine><itemref idref="first"/><itemref idref="hidden" linear="NO"/><itemref idref="nav"/><itemref idref="second"/></spine></package>`,
		fixtureFile{"OPS/nav.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc"><ol><li><a href="Text/part/first.xhtml#start">Shared title</a></li><li><a href="Text/second.xhtml">Second label</a></li></ol></nav><nav epub:type="landmarks"><a epub:type="bodymatter chapter" href="Text/part/first.xhtml#start">Begin</a></nav></body></html>`},
		fixtureFile{"OPS/Text/part/first.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Shared title</h1><p>Grüße 👋</p></body></html>`},
		fixtureFile{"OPS/Text/hidden.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Hidden</h1></body></html>`},
		fixtureFile{"OPS/Text/second.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><h2>Shared title</h2><p>東京</p></body></html>`},
	)
	book, err := Extract(data)
	require.NoError(t, err)
	assert.Len(t, book.ExtractedUnits.Units, 2)
	first, second := book.ExtractedUnits.Units[0], book.ExtractedUnits.Units[1]
	assert.Equal(t, UnitID(0, "first"), first.ID)
	assert.Equal(t, uint64(0), first.Order)
	assert.Equal(t, uint64(0), first.SpineIndex)
	assert.Equal(t, "Shared title", first.Title)
	assert.Equal(t, UnitTitleHeading, first.TitleSource)
	assert.Equal(t, "OPS/content.opf", first.PackagePath)
	assert.Equal(t, "Text/part/first.xhtml", first.SourceHref)
	assert.Equal(t, "OPS/Text/part/first.xhtml", first.ResolvedHref)
	assert.Equal(t, "application/xhtml+xml", first.MediaType)
	assert.True(t, first.Linear)
	assert.Equal(t, "svg,mathml", strings.Join(first.Properties, ","))
	assert.Equal(t, "Shared title,Begin", strings.Join(first.NavigationLabels, ","))
	assert.Equal(t, "bodymatter,chapter", strings.Join(first.LandmarkTypes, ","))
	assert.Equal(t, UnitID(3, "second"), second.ID)
	assert.Equal(t, uint64(1), second.Order)
	assert.Equal(t, uint64(3), second.SpineIndex)
	assert.Equal(t, first.Title, second.Title)
	assert.Equal(t, first.EndOffset+2, second.StartOffset)
	assert.Equal(t, uint64(len([]rune(book.FullText))), second.EndOffset)
	require.NoError(t, book.ExtractedUnits.ValidateOffsets(book.FullText))
	assert.Len(t, book.Chapters, 2)
	assert.Equal(t, "first", book.Chapters[0].ID)
	assert.Equal(t, "second", book.Chapters[1].ID)
	assert.Equal(t, first.StartOffset, book.Chapters[0].Location.StartOffset)
	assert.Equal(t, second.EndOffset, book.Chapters[1].Location.EndOffset)
}

func TestExtractUnitTitleFallbackAndMalformedOptionalNavigation(t *testing.T) {
	data := epubFixture(t, validPackage(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="untitled" href="untitled.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="nav"/><itemref idref="untitled"/>`),
		fixtureFile{"OPS/nav.xhtml", `<html><body><nav><a href="untitled.xhtml">broken`},
		fixtureFile{"OPS/untitled.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Readable text.</p></body></html>`},
	)
	book, err := Extract(data)
	require.NoError(t, err)
	unit := book.ExtractedUnits.Units[0]
	assert.Equal(t, "untitled", unit.Title)
	assert.Equal(t, UnitTitleManifestID, unit.TitleSource)
	assert.Equal(t, uint64(1), unit.SpineIndex)
	assert.NotNil(t, unit.NavigationLabels)
	assert.NotNil(t, unit.LandmarkTypes)
}

func TestExtractRejectsMalformedManifestAndSpineReferences(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		spine    string
	}{
		{"blank manifest ID", `<item id="" href="chapter.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="chapter"/>`},
		{"duplicate manifest ID", `<item id="chapter" href="one.xhtml" media-type="application/xhtml+xml"/><item id="chapter" href="two.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="chapter"/>`},
		{"blank spine reference", `<item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref=""/>`},
		{"missing spine reference", `<item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="missing"/>`},
		{"missing resource", `<item id="chapter" href="chapter.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="chapter"/>`},
		{"unsafe resource", `<item id="chapter" href="../../chapter.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="chapter"/>`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Extract(epubFixture(t, validPackage(tt.manifest, tt.spine)))
			assert.ErrorIs(t, err, ErrInvalidEPUB)
		})
	}
}

func TestExtractMalformedEPUB(t *testing.T) {
	_, err := Extract([]byte("not a zip"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidEPUB)
	assert.Contains(t, err.Error(), "open ZIP container")
}

func TestExtractMalformedXHTML(t *testing.T) {
	f := xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>unclosed</body></html>`)
	_, _, err := extractXHTML(f)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "malformed XHTML")
}

func TestExtractXHTMLNamedEntities(t *testing.T) {
	f := xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Rock &amp; Roll</h1><p>Caf&eacute;&nbsp;&mdash;&nbsp;&ldquo;hello&rdquo;&hellip; &copy;</p></body></html>`)
	text, title, err := extractXHTML(f)
	require.NoError(t, err)
	const want = "Rock & Roll\n\nCafé — “hello”… ©"
	assert.Equal(t, want, text)
	assert.Equal(t, "Rock & Roll", title)
}

func xhtmlFile(t *testing.T, content string) *zip.File {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("chapter.xhtml")
	require.NoError(t, err)
	_, err = w.Write([]byte(content))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	return zr.File[0]
}

func validPackage(manifest, spine string) string {
	return `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier>fixture</dc:identifier><dc:title>Fixture</dc:title></metadata><manifest>` + manifest + `</manifest><spine>` + spine + `</spine></package>`
}

func epubFixture(t *testing.T, opf string, extra ...fixtureFile) []byte {
	t.Helper()
	files := append([]fixtureFile{
		{"mimetype", mediaType},
		{"META-INF/container.xml", `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="OPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`},
		{"OPS/content.opf", opf},
	}, extra...)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, file := range files {
		w, err := zw.Create(file.name)
		require.NoError(t, err)
		_, err = w.Write([]byte(file.content))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}
