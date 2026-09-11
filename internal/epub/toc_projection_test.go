package epub

import (
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
