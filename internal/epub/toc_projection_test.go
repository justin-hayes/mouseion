package epub

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectTOCChoicesReliableProjectionAndDeterminism(t *testing.T) {
	data, units := tocProjectionFixture(t, `<nav epub:type="toc"><ol>
<li><a href="Text/one.xhtml#start">  First   Part </a></li>
<li><a href="Text/two.xhtml">Second Part</a></li>
<li><a href="Text/three.xhtml#end">Third Part</a></li>
</ol></nav>`)

	want := []TOCChoice{
		{Label: "First Part", UnitIDs: []string{units.Units[0].ID}, First: 0},
		{Label: "Second Part", UnitIDs: []string{units.Units[1].ID}, First: 1},
		{Label: "Third Part", UnitIDs: []string{units.Units[2].ID}, First: 2},
	}
	got := ProjectTOCChoices(data, units)
	assert.Equal(t, want, got)
	assert.Equal(t, got, ProjectTOCChoices(data, units), "projection is not deterministic")
}

func TestProjectTOCChoicesNestedTargetsBelongToParent(t *testing.T) {
	data, units := tocProjectionFixture(t, `<nav epub:type="toc"><ol>
<li><span>Part One</span><ol>
  <li><a href="Text/one.xhtml">Chapter One</a></li>
  <li><a href="Text/two.xhtml">Chapter Two</a></li>
</ol></li>
<li><a href="Text/three.xhtml">Part Two</a></li>
</ol></nav>`)

	want := []TOCChoice{
		{Label: "Part One", UnitIDs: []string{units.Units[0].ID, units.Units[1].ID}, First: 0},
		{Label: "Part Two", UnitIDs: []string{units.Units[2].ID}, First: 2},
	}
	assert.Equal(t, want, ProjectTOCChoices(data, units))
}

func TestProjectTOCChoicesFlatFallback(t *testing.T) {
	tests := []struct {
		name string
		nav  string
	}{
		{name: "no navigation document"},
		{name: "navigation without toc", nav: `<nav epub:type="landmarks"><a href="Text/one.xhtml">Landmark</a></nav>`},
		{name: "malformed navigation", nav: `<nav epub:type="toc"><ol><li><a href="Text/one.xhtml">First</a></ol>`},
		{name: "empty label", nav: `<nav epub:type="toc"><ol><li><a href="Text/one.xhtml">  </a></li><li><a href="Text/two.xhtml">Second</a></li></ol></nav>`},
		{name: "unresolved link", nav: `<nav epub:type="toc"><ol><li><a href="Text/missing.xhtml">First</a></li><li><a href="Text/two.xhtml">Second</a></li></ol></nav>`},
		{name: "duplicate starts", nav: `<nav epub:type="toc"><ol><li><a href="Text/one.xhtml">First</a></li><li><a href="Text/one.xhtml#again">Again</a></li></ol></nav>`},
		{name: "out of order starts", nav: `<nav epub:type="toc"><ol><li><a href="Text/two.xhtml">Second</a></li><li><a href="Text/one.xhtml">First</a></li></ol></nav>`},
		{name: "first start is not first readable unit", nav: `<nav epub:type="toc"><ol><li><a href="Text/two.xhtml">Second</a></li><li><a href="Text/three.xhtml">Third</a></li></ol></nav>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, units := tocProjectionFixture(t, test.nav)
			assertFlatTOCChoices(t, ProjectTOCChoices(data, units), units)
		})
	}
}

func TestProjectTOCChoicesEPUB2UsesFlatFallback(t *testing.T) {
	data := fixtureDirectory(t, "testfixtures/epub2-clean")
	book, err := Extract(data)
	require.NoError(t, err)
	assertFlatTOCChoices(t, ProjectTOCChoices(data, book.ExtractedUnits), book.ExtractedUnits)
}

func TestProjectTOCChoicesEPUB3FixtureProjection(t *testing.T) {
	data := fixtureDirectory(t, "testfixtures/epub3-main-text")
	book, err := Extract(data)
	require.NoError(t, err)
	require.Len(t, book.ExtractedUnits.Units, 4)

	want := []TOCChoice{
		{Label: "Vorwort", UnitIDs: []string{book.ExtractedUnits.Units[0].ID}, First: 0},
		{Label: "Kapitel eins", UnitIDs: []string{book.ExtractedUnits.Units[1].ID}, First: 1},
		{Label: "Kapitel zwei", UnitIDs: []string{book.ExtractedUnits.Units[2].ID}, First: 2},
		{Label: "Bibliographie", UnitIDs: []string{book.ExtractedUnits.Units[3].ID}, First: 3},
	}
	assert.Equal(t, want, ProjectTOCChoices(data, book.ExtractedUnits))
}

func TestProjectTOCChoicesFlatFallbackNormalizesPersistedTitle(t *testing.T) {
	units := ExtractedUnits{Units: []ExtractedUnit{
		{ID: "first", Order: 4, ManifestID: "first", Title: "  First\n Chapter  ", Text: "readable"},
		{ID: "second", Order: 9, ManifestID: "second", Title: "\t", Text: "also readable"},
	}}
	got := ProjectTOCChoices(nil, units)
	want := []TOCChoice{
		{Label: "First Chapter", UnitIDs: []string{"first"}, First: 4},
		{Label: "second", UnitIDs: []string{"second"}, First: 9},
	}
	assert.Equal(t, want, got)
}

func tocProjectionFixture(t *testing.T, nav string) ([]byte, ExtractedUnits) {
	t.Helper()
	manifest := `<item id="one" href="Text/one.xhtml" media-type="application/xhtml+xml"/><item id="two" href="Text/two.xhtml" media-type="application/xhtml+xml"/><item id="three" href="Text/three.xhtml" media-type="application/xhtml+xml"/>`
	spine := `<itemref idref="one"/><itemref idref="two"/><itemref idref="three"/>`
	files := []fixtureFile{
		{"OPS/Text/one.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>One</h1><p>First text.</p></body></html>`},
		{"OPS/Text/two.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Two</h1><p>Second text.</p></body></html>`},
		{"OPS/Text/three.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Three</h1><p>Third text.</p></body></html>`},
	}
	if nav != "" {
		manifest = `<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + manifest
		files = append(files, fixtureFile{"OPS/nav.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>` + nav + `</body></html>`})
		spine = `<itemref idref="nav"/>` + spine
	}
	data := epubFixture(t, validPackage(manifest, spine), files...)
	book, err := Extract(data)
	require.NoError(t, err)
	return data, book.ExtractedUnits
}

func assertFlatTOCChoices(t *testing.T, got []TOCChoice, units ExtractedUnits) {
	t.Helper()
	readable := readableSnapshotUnits(units)
	require.Len(t, got, len(readable))
	for i, choice := range got {
		unit := readable[i]
		wantLabel := cleanSpace(unit.Title)
		if wantLabel == "" {
			wantLabel = unit.ManifestID
		}
		want := TOCChoice{Label: wantLabel, UnitIDs: []string{unit.ID}, First: unit.Order}
		assert.Equal(t, want, choice, "choice %d", i)
	}
}

// TestProjectTOCChoicesNavigationDocumentErrorsFallBack drives every rejecting
// branch of navigation-document parsing through ProjectTOCChoices. Each fixture
// would project successfully if its step accepted the document, so the flat
// fallback proves the step rejected it.
func TestProjectTOCChoicesNavigationDocumentErrorsFallBack(t *testing.T) {
	withoutRootfilePath := `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path=" " media-type="application/oebps-package+xml"/></rootfiles></container>`
	nav := `<nav epub:type="toc"><ol><li><a href="Text/one.xhtml">One</a></li><li><a href="Text/two.xhtml">Two</a></li></ol></nav>`

	// withNav supplies an otherwise valid container, package, and navigation
	// document around one navigation body, so only the body can reject.
	withNav := func(body string) []fixtureFile {
		return []fixtureFile{
			{"mimetype", mediaType},
			{"META-INF/container.xml", tocNavigationContainer},
			{"OPS/content.opf", tocNavigationPackage("nav.xhtml")},
			tocNavigationDocument(body),
		}
	}

	tests := []struct {
		name  string
		files []fixtureFile
	}{
		{
			name: "missing container document",
			files: []fixtureFile{
				{"mimetype", mediaType},
				{"OPS/content.opf", tocNavigationPackage("nav.xhtml")},
				tocNavigationDocument(nav),
			},
		},
		{
			name: "container without package rootfile",
			files: []fixtureFile{
				{"mimetype", mediaType},
				{"META-INF/container.xml", withoutRootfilePath},
				{"OPS/content.opf", tocNavigationPackage("nav.xhtml")},
				tocNavigationDocument(nav),
			},
		},
		{
			name: "missing package document",
			files: []fixtureFile{
				{"mimetype", mediaType},
				{"META-INF/container.xml", tocNavigationContainer},
				tocNavigationDocument(nav),
			},
		},
		{
			name: "unsafe navigation href",
			files: []fixtureFile{
				{"mimetype", mediaType},
				{"META-INF/container.xml", tocNavigationContainer},
				{"OPS/content.opf", tocNavigationPackage("/absolute.xhtml")},
			},
		},
		{
			name: "missing navigation resource",
			files: []fixtureFile{
				{"mimetype", mediaType},
				{"META-INF/container.xml", tocNavigationContainer},
				{"OPS/content.opf", tocNavigationPackage("nav.xhtml")},
			},
		},
		{name: "toc without top-level list", files: withNav(`<nav epub:type="toc"><p>Not a list</p></nav>`)},
		{name: "non-entry list child", files: withNav(`<nav epub:type="toc"><ol><p>Bad child</p></ol></nav>`)},
		{name: "entry without usable link", files: withNav(`<nav epub:type="toc"><ol><li><span>Label</span></li></ol></nav>`)},
		{name: "toc without entries", files: withNav(`<nav epub:type="toc"><ol></ol></nav>`)},
		{
			name: "empty navigation document",
			files: []fixtureFile{
				{"mimetype", mediaType},
				{"META-INF/container.xml", tocNavigationContainer},
				{"OPS/content.opf", tocNavigationPackage("nav.xhtml")},
				{"OPS/nav.xhtml", ""},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := tocProjectionSnapshot()
			data := tocProjectionEpub(t, test.files...)
			assertFlatTOCChoices(t, ProjectTOCChoices(data, snapshot), snapshot)
		})
	}
}

// TestProjectTOCChoicesFallsBackWhenNavigationHasMultipleRoots places the only
// valid table of contents in a second root element. A parser that keeps the
// last root would project it, so the flat fallback proves the first root is
// retained and the second is rejected.
func TestProjectTOCChoicesFallsBackWhenNavigationHasMultipleRoots(t *testing.T) {
	nav := `<html/><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body><nav epub:type="toc"><ol><li><a href="Text/one.xhtml">One</a></li><li><a href="Text/two.xhtml">Two</a></li></ol></nav></body></html>`
	files := []fixtureFile{
		{"mimetype", mediaType},
		{"META-INF/container.xml", tocNavigationContainer},
		{"OPS/content.opf", tocNavigationPackage("nav.xhtml")},
		{"OPS/nav.xhtml", nav},
	}
	snapshot := tocProjectionSnapshot()
	assertFlatTOCChoices(t, ProjectTOCChoices(tocProjectionEpub(t, files...), snapshot), snapshot)
}

// TestProjectTOCChoicesResolvesDescendantLinkWhenDirectLinkIsUnusable
// exercises the fall from a direct, unusable anchor to a resolvable descendant
// anchor for both a blank and an absent href.
func TestProjectTOCChoicesResolvesDescendantLinkWhenDirectLinkIsUnusable(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "blank href", body: `<a href="">One</a><a href="Text/one.xhtml">Fallback</a>`},
		{name: "missing href", body: `<a>One</a><a href="Text/one.xhtml">Fallback</a>`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data, units := tocProjectionFixture(t, `<nav epub:type="toc"><ol><li>`+test.body+`</li></ol></nav>`)

			want := []TOCChoice{{Label: "One", UnitIDs: []string{units.Units[0].ID, units.Units[1].ID, units.Units[2].ID}, First: 0}}
			assert.Equal(t, want, ProjectTOCChoices(data, units))
		})
	}
}

// tocNavigationContainer is a valid EPUB container describing tocNavigationPackage.
const tocNavigationContainer = `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="OPS/content.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`

// tocNavigationPackage declares a navigation item and the two content
// documents that tocProjectionSnapshot describes.
func tocNavigationPackage(navHref string) string {
	manifest := `<item id="nav" href="` + navHref + `" media-type="application/xhtml+xml" properties="nav"/>` +
		`<item id="one" href="Text/one.xhtml" media-type="application/xhtml+xml"/>` +
		`<item id="two" href="Text/two.xhtml" media-type="application/xhtml+xml"/>`
	return validPackage(manifest, `<itemref idref="one"/><itemref idref="two"/>`)
}

// tocNavigationDocument wraps a navigation body in the XHTML document the
// package declares.
func tocNavigationDocument(body string) fixtureFile {
	return fixtureFile{"OPS/nav.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><body>` + body + `</body></html>`}
}

// tocProjectionSnapshot is the persisted snapshot matching tocNavigationPackage.
func tocProjectionSnapshot() ExtractedUnits {
	return ExtractedUnits{Units: []ExtractedUnit{
		{ID: UnitID(1, "one"), Order: 0, SpineIndex: 1, ManifestID: "one", Title: "One", Text: "First text.", ResolvedHref: "OPS/Text/one.xhtml"},
		{ID: UnitID(2, "two"), Order: 1, SpineIndex: 2, ManifestID: "two", Title: "Two", Text: "Second text.", ResolvedHref: "OPS/Text/two.xhtml"},
	}}
}

// tocProjectionEpub assembles raw EPUB bytes from files without validating the
// container, so tests can reach parseNavigationDocument's error paths.
func tocProjectionEpub(t *testing.T, files ...fixtureFile) []byte {
	t.Helper()
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
