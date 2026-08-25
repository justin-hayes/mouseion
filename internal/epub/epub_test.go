package epub

import (
	"archive/zip"
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fixtureFile struct {
	name    string
	content string
}

func fixture(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir("testdata", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel("testdata", name)
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
	if err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExtractReadingOrderAndLocations(t *testing.T) {
	book, err := Extract(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	want := "Erstes Kapitel\n\nGrüße aus Köln.\n\nZweiter Absatz.\n\nZweites Kapitel\n\nDas Ende."
	if book.FullText != want {
		t.Fatalf("full text:\n%q\nwant:\n%q", book.FullText, want)
	}
	if book.Title != "Die Prüfung" || book.SourceIdentifier != "urn:isbn:9780000000014" {
		t.Fatalf("metadata: %+v", book)
	}
	if len(book.Chapters) != 2 {
		t.Fatalf("chapters: %+v", book.Chapters)
	}
	first, second := book.Chapters[0], book.Chapters[1]
	if first.ID != "first" || first.Title != "Erstes Kapitel" || first.Location.StartOffset != 0 || first.Location.EndOffset != uint64(len([]rune("Erstes Kapitel\n\nGrüße aus Köln.\n\nZweiter Absatz."))) {
		t.Fatalf("first location: %+v", first)
	}
	if second.Location.StartOffset != first.Location.EndOffset+2 || second.Location.EndOffset != uint64(len([]rune(want))) {
		t.Fatalf("second location: %+v", second)
	}
	if strings.Contains(book.FullText, "navigation") || strings.Contains(book.FullText, "steal") || strings.Contains(book.FullText, "Hidden") {
		t.Fatalf("excluded content leaked: %q", book.FullText)
	}
	if book.ExtractedUnits.SchemaVersion != ExtractedUnitsSchemaVersion || len(book.ExtractedUnits.Units) != len(book.Chapters) {
		t.Fatalf("extracted units: %+v", book.ExtractedUnits)
	}
	if err := book.ExtractedUnits.ValidateOffsets(book.FullText); err != nil {
		t.Fatalf("unit offsets: %v", err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(book.ExtractedUnits.Units), 2; got != want {
		t.Fatalf("unit count = %d, want %d: %+v", got, want, book.ExtractedUnits.Units)
	}
	first, second := book.ExtractedUnits.Units[0], book.ExtractedUnits.Units[1]
	if first.ID != UnitID(0, "first") || first.Order != 0 || first.SpineIndex != 0 || first.Title != "Shared title" || first.TitleSource != UnitTitleHeading {
		t.Fatalf("first identity/title = %+v", first)
	}
	if first.PackagePath != "OPS/content.opf" || first.SourceHref != "Text/part/first.xhtml" || first.ResolvedHref != "OPS/Text/part/first.xhtml" || first.MediaType != "application/xhtml+xml" || !first.Linear {
		t.Fatalf("first provenance = %+v", first)
	}
	if strings.Join(first.Properties, ",") != "svg,mathml" || strings.Join(first.NavigationLabels, ",") != "Shared title,Begin" || strings.Join(first.LandmarkTypes, ",") != "bodymatter,chapter" {
		t.Fatalf("first metadata = %+v", first)
	}
	if second.ID != UnitID(3, "second") || second.Order != 1 || second.SpineIndex != 3 || second.Title != first.Title {
		t.Fatalf("second identity/title = %+v", second)
	}
	if second.StartOffset != first.EndOffset+2 || second.EndOffset != uint64(len([]rune(book.FullText))) {
		t.Fatalf("Unicode offsets = %+v / %+v", first, second)
	}
	if err := book.ExtractedUnits.ValidateOffsets(book.FullText); err != nil {
		t.Fatalf("unit offsets: %v", err)
	}
	if len(book.Chapters) != 2 || book.Chapters[0].ID != "first" || book.Chapters[1].ID != "second" || book.Chapters[0].Location.StartOffset != first.StartOffset || book.Chapters[1].Location.EndOffset != second.EndOffset {
		t.Fatalf("compatibility chapters = %+v", book.Chapters)
	}
}

func TestExtractUnitTitleFallbackAndMalformedOptionalNavigation(t *testing.T) {
	data := epubFixture(t, validPackage(`<item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/><item id="untitled" href="untitled.xhtml" media-type="application/xhtml+xml"/>`, `<itemref idref="nav"/><itemref idref="untitled"/>`),
		fixtureFile{"OPS/nav.xhtml", `<html><body><nav><a href="untitled.xhtml">broken`},
		fixtureFile{"OPS/untitled.xhtml", `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>Readable text.</p></body></html>`},
	)
	book, err := Extract(data)
	if err != nil {
		t.Fatal(err)
	}
	unit := book.ExtractedUnits.Units[0]
	if unit.Title != "untitled" || unit.TitleSource != UnitTitleManifestID || unit.SpineIndex != 1 || unit.NavigationLabels == nil || unit.LandmarkTypes == nil {
		t.Fatalf("fallback unit = %+v", unit)
	}
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
			if !errors.Is(err, ErrInvalidEPUB) {
				t.Fatalf("error = %v, want ErrInvalidEPUB", err)
			}
		})
	}
}

func TestExtractMalformedEPUB(t *testing.T) {
	_, err := Extract([]byte("not a zip"))
	if !errors.Is(err, ErrInvalidEPUB) || !strings.Contains(err.Error(), "open ZIP container") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractMalformedXHTML(t *testing.T) {
	f := xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body><p>unclosed</body></html>`)
	_, _, err := extractXHTML(f)
	if err == nil || !strings.Contains(err.Error(), "malformed XHTML") {
		t.Fatalf("error = %v", err)
	}
}

func TestExtractXHTMLNamedEntities(t *testing.T) {
	f := xhtmlFile(t, `<html xmlns="http://www.w3.org/1999/xhtml"><body><h1>Rock &amp; Roll</h1><p>Caf&eacute;&nbsp;&mdash;&nbsp;&ldquo;hello&rdquo;&hellip; &copy;</p></body></html>`)
	text, title, err := extractXHTML(f)
	if err != nil {
		t.Fatal(err)
	}
	const want = "Rock & Roll\n\nCafé — “hello”… ©"
	if text != want || title != "Rock & Roll" {
		t.Fatalf("text = %q, title = %q; want text = %q", text, title, want)
	}
}

func xhtmlFile(t *testing.T, content string) *zip.File {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("chapter.xhtml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
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
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(file.content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
