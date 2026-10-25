package epub

import (
	"archive/zip"
	"bytes"
	"io"
	"math"
	"path"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRetainRemovesNeverReadResourcesAndKeepsTheRest(t *testing.T) {
	catalog := fixtureDirectory(t, "testfixtures/illustrated")
	retained, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.NoError(t, err)
	names := retainedEntryNames(t, retained.Content)

	t.Run("reports the slimming version", func(t *testing.T) {
		require.Equal(t, RetainedSlimmingVersion, retained.SlimmingVersion)
	})
	t.Run("removes image, font, audio and video entries", func(t *testing.T) {
		for _, removed := range []string{
			"OPS/images/cover.jpg",
			"OPS/images/figure.png",
			"OPS/fonts/book.otf",
			"OPS/fonts/legacy.ttf",
			"OPS/media/sample.mp3",
			"OPS/media/sample.mp4",
		} {
			require.NotContains(t, names, removed, "removed resource survived")
		}
	})
	t.Run("keeps exempt and manifest-unlisted entries", func(t *testing.T) {
		for _, kept := range []string{
			"mimetype",
			"META-INF/container.xml",
			"OPS/package.opf",
			"OPS/Navigation/nav.xhtml",
			"OPS/toc.ncx",
			"OPS/styles/book.css",
			"OPS/Text/chapter.xhtml",
			"OPS/Text/plate.svg",
			"OPS/images/unlisted.jpg",
		} {
			require.Contains(t, names, kept, "kept resource disappeared")
		}
	})
	t.Run("reports each removed manifest item", func(t *testing.T) {
		wantRemoved := []RemovedResource{
			{ID: "cover-image", Href: "images/cover.jpg", MediaType: "image/jpeg", Size: int64(len("catalog cover image bytes that Mouseion never reads\n"))},
			{ID: "inline-image", Href: "images/figure.png", MediaType: "image/png", Size: int64(len("inline illustration bytes referenced by the chapter\n"))},
			{ID: "font-otf", Href: "fonts/book.otf", MediaType: "font/otf", Size: int64(len("embedded font bytes\n"))},
			{ID: "font-legacy", Href: "fonts/legacy.ttf", MediaType: "application/x-font-ttf", Size: int64(len("legacy embedded font bytes\n"))},
			{ID: "audio", Href: "media/sample.mp3", MediaType: "audio/mpeg", Size: int64(len("audio bytes\n"))},
			{ID: "video", Href: "media/sample.mp4", MediaType: "video/mp4", Size: int64(len("video bytes\n"))},
		}
		require.Equal(t, wantRemoved, retained.Removed)
	})
	t.Run("extracts the same text and units", func(t *testing.T) {
		original, err := ExtractV2(catalog)
		require.NoError(t, err)
		slimmed, err := ExtractV2(retained.Content)
		require.NoError(t, err)
		require.Equal(t, original.Book.FullText, slimmed.Book.FullText)
		require.Equal(t, original.Book.ExtractedUnits, slimmed.Book.ExtractedUnits)
	})
	t.Run("cuts only the removed manifest items", func(t *testing.T) {
		originalOPF := retainedEntryBytes(t, catalog, "OPS/package.opf")
		wantOPF := string(originalOPF)
		for _, item := range []string{
			`<item id="cover-image" href="images/cover.jpg" media-type="image/jpeg"/>`,
			`<item id="inline-image" href="images/figure.png" media-type="image/png"/>`,
			`<item id="font-otf" href="fonts/book.otf" media-type="font/otf"/>`,
			`<item id="font-legacy" href="fonts/legacy.ttf" media-type="application/x-font-ttf"/>`,
			`<item id="audio" href="media/sample.mp3" media-type="audio/mpeg"/>`,
			`<item id="video" href="media/sample.mp4" media-type="video/mp4"/>`,
		} {
			require.Contains(t, wantOPF, item)
			wantOPF = strings.Replace(wantOPF, item, "", 1)
		}
		gotOPF := string(retainedEntryBytes(t, retained.Content, "OPS/package.opf"))
		require.Equal(t, wantOPF, gotOPF, "slimming changed package-document bytes outside the removed items")
		require.Contains(t, gotOPF, `<meta name="cover" content="cover-image"/>`, "dangling cover metadata was rewritten")
	})
}

func TestRetainIsDeterministicAndKeepsRawEntriesInOrder(t *testing.T) {
	catalog := fixtureDirectory(t, "testfixtures/illustrated")
	first, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.NoError(t, err)
	second, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.NoError(t, err)
	t.Run("repeated runs agree byte-for-byte", func(t *testing.T) {
		require.Equal(t, first.Content, second.Content, "Retained EPUB is not deterministic")
		require.Equal(t, first.Removed, second.Removed)
	})

	catalogEntries := openZip(t, catalog)
	retainedEntries := openZip(t, first.Content)
	removed := make(map[string]struct{}, len(first.Removed))
	for _, item := range first.Removed {
		removed[path.Join("OPS", item.Href)] = struct{}{}
	}
	var wantNames []string
	for _, file := range catalogEntries.File {
		if _, drop := removed[path.Clean(file.Name)]; drop {
			continue
		}
		wantNames = append(wantNames, file.Name)
	}
	t.Run("kept entries keep their order", func(t *testing.T) {
		var gotNames []string
		for _, file := range retainedEntries.File {
			gotNames = append(gotNames, file.Name)
		}
		require.Equal(t, wantNames, gotNames, "kept entries changed order")
	})

	for _, want := range catalogEntries.File {
		name := path.Clean(want.Name)
		if _, drop := removed[name]; drop {
			continue
		}
		got := zipEntry(t, retainedEntries, name)
		require.NotNil(t, got, "kept entry %s is missing", name)
		t.Run("kept entry "+name+" is unchanged", func(t *testing.T) {
			if name == "OPS/package.opf" {
				require.Equal(t, want.Method, got.Method, "package document compression method changed")
				require.True(t, got.Modified.Equal(want.Modified), "package document modification time changed: got %s want %s", got.Modified, want.Modified)
				return
			}
			require.Equal(t, want.Method, got.Method, "compression method changed")
			require.Equal(t, want.Modified, got.Modified, "modification time changed")
			require.Equal(t, want.CRC32, got.CRC32, "raw checksum changed")
			require.Equal(t, rawEntryBytes(t, want), rawEntryBytes(t, got), "raw bytes changed")
		})
	}
}

func TestRetainReturnsTheCatalogFileWhenNothingMatches(t *testing.T) {
	catalog := fixtureDirectory(t, "testfixtures/epub2-clean")
	retained, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.NoError(t, err)
	t.Run("the bytes are byte-identical", func(t *testing.T) {
		require.Equal(t, catalog, retained.Content)
	})
	t.Run("nothing is reported as removed", func(t *testing.T) {
		require.Empty(t, retained.Removed)
		require.Equal(t, RetainedSlimmingVersion, retained.SlimmingVersion)
	})
}

func TestRetainRemovesEveryFontMediaTypeSpelling(t *testing.T) {
	for _, media := range []string{
		"font/otf",
		"font/woff2",
		"application/font-woff",
		"application/font-sfnt",
		"application/x-font-ttf",
		"application/x-font-otf",
		"application/vnd.ms-opentype",
		"FONT/OTF; charset=binary",
	} {
		t.Run(media, func(t *testing.T) {
			catalog := minimalEPUB(t, validPackage(`<item id="font" href="font.bin" media-type="`+media+`"/>`, ""))
			retained, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
			require.NoError(t, err)
			t.Run("reports the font item", func(t *testing.T) {
				require.Len(t, retained.Removed, 1)
				require.Equal(t, "font", retained.Removed[0].ID)
			})
			t.Run("removes the font entry", func(t *testing.T) {
				require.NotContains(t, retainedEntryNames(t, retained.Content), "OPS/font.bin")
			})
		})
	}
}

func TestRetainHonoursEveryExemption(t *testing.T) {
	opf := validPackage(`
    <item id="mime" href="../mimetype" media-type="image/png"/>
    <item id="meta" href="../META-INF/custom.xml" media-type="image/png"/>
    <item id="package" href="other.opf" media-type="image/jpeg"/>
    <item id="spine-image" href="page.png" media-type="image/png"/>
    <item id="nav" href="nav.svg" media-type="image/svg+xml" properties="nav"/>
    <item id="ncx" href="toc.ncx" media-type="application/x-dtbncx+xml"/>
    <item id="style" href="style.css" media-type="text/css"/>
    <item id="remove" href="remove.png" media-type="image/png"/>`, `<itemref idref="spine-image"/>`)
	catalog := epubFixture(t, opf,
		fixtureFile{name: "META-INF/custom.xml", content: "meta"},
		fixtureFile{name: "OPS/other.opf", content: "package"},
		fixtureFile{name: "OPS/page.png", content: "spine image"},
		fixtureFile{name: "OPS/nav.svg", content: "navigation"},
		fixtureFile{name: "OPS/toc.ncx", content: "ncx"},
		fixtureFile{name: "OPS/style.css", content: "css"},
		fixtureFile{name: "OPS/remove.png", content: "remove me"},
	)
	retained, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.NoError(t, err)
	t.Run("removes only the unexempt resource", func(t *testing.T) {
		require.Len(t, retained.Removed, 1)
		require.Equal(t, "remove", retained.Removed[0].ID)
	})
	t.Run("keeps every exempt resource", func(t *testing.T) {
		names := retainedEntryNames(t, retained.Content)
		for _, kept := range []string{"mimetype", "META-INF/custom.xml", "OPS/other.opf", "OPS/page.png", "OPS/nav.svg", "OPS/toc.ncx", "OPS/style.css"} {
			require.Contains(t, names, kept)
		}
		require.NotContains(t, names, "OPS/remove.png")
	})
}

func TestRetainRefusesAnInvalidContainer(t *testing.T) {
	tests := map[string][]byte{
		"not a ZIP file": []byte("not a ZIP file"),
		"missing mimetype": zipFixture(t,
			fixtureFile{name: "META-INF/container.xml", content: containerXML},
		),
		"wrong mimetype": zipFixture(t,
			fixtureFile{name: "mimetype", content: "text/plain"},
			fixtureFile{name: "META-INF/container.xml", content: containerXML},
		),
		"missing container.xml": zipFixture(t,
			fixtureFile{name: "mimetype", content: mediaType},
		),
		"several package rootfiles": zipFixture(t,
			fixtureFile{name: "mimetype", content: mediaType},
			fixtureFile{name: "META-INF/container.xml", content: `<?xml version="1.0"?><container xmlns="urn:oasis:names:tc:opendocument:xmlns:container" version="1.0"><rootfiles><rootfile full-path="OPS/one.opf" media-type="application/oebps-package+xml"/><rootfile full-path="OPS/two.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`},
			fixtureFile{name: "OPS/one.opf", content: validPackage("", "")},
		),
		"duplicate container entry": zipFixture(t,
			fixtureFile{name: "mimetype", content: mediaType},
			fixtureFile{name: "mimetype", content: mediaType},
		),
		"empty package document":          epubFixture(t, ""),
		"removable item escapes the root": epubFixture(t, validPackage(`<item id="image" href="../../outside.png" media-type="image/png"/>`, "")),
		"nested manifest item":            epubFixture(t, validPackage(`<item id="outer" href="outer.png" media-type="image/png"><item id="inner" href="inner.png" media-type="image/png"/></item>`, "")),
	}
	for name, catalog := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
			require.ErrorIs(t, err, ErrInvalidEPUB)
		})
	}
}

func TestRetainStopsAtTheSizeLimit(t *testing.T) {
	clean := fixtureDirectory(t, "testfixtures/epub2-clean")
	illustrated := fixtureDirectory(t, "testfixtures/illustrated")
	tests := []struct {
		name    string
		catalog []byte
		limit   int64
	}{
		{name: "a no-op Catalog file is not read", catalog: clean, limit: int64(len(clean)) - 1},
		{name: "a rewritten Retained EPUB is not materialised in full", catalog: illustrated, limit: 100},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Retain(bytes.NewReader(test.catalog), int64(len(test.catalog)), test.limit)
			require.ErrorIs(t, err, ErrRetainedSizeLimit)
		})
	}
}

func TestCutManifestItemsRejectsAnInvalidRange(t *testing.T) {
	_, err := cutManifestItems([]byte("<package/>"), []plannedRemoval{{item: packageManifestItem{start: 5, end: 4}}})
	require.ErrorIs(t, err, ErrInvalidEPUB)
}

func TestRetainRefusesAmbiguousPackageDocuments(t *testing.T) {
	tests := map[string]string{
		"malformed XML":         `<package><manifest></package>`,
		"missing package root":  `<book><manifest><item id="image" href="image.jpg" media-type="image/jpeg"/></manifest><spine/></book>`,
		"missing manifest":      `<package><spine/></package>`,
		"missing spine":         `<package><manifest><item id="image" href="image.jpg" media-type="image/jpeg"/></manifest></package>`,
		"two manifests":         `<package><manifest><item id="one" href="one.css" media-type="text/css"/></manifest><manifest><item id="two" href="two.css" media-type="text/css"/></manifest><spine/></package>`,
		"two spines":            `<package><manifest><item id="one" href="one.css" media-type="text/css"/></manifest><spine/><spine/></package>`,
		"item without an ID":    `<package><manifest><item href="image.jpg" media-type="image/jpeg"/></manifest><spine/></package>`,
		"duplicate manifest ID": `<package><manifest><item id="image" href="one.jpg" media-type="image/jpeg"/><item id="image" href="two.jpg" media-type="image/jpeg"/></manifest><spine/></package>`,
	}
	for name, opf := range tests {
		t.Run(name, func(t *testing.T) {
			catalog := minimalEPUB(t, opf)
			_, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
			require.ErrorIs(t, err, ErrInvalidEPUB)
		})
	}
}

func TestRetainRefusesACatalogWithNoPackageDocument(t *testing.T) {
	catalog := zipFixture(t,
		fixtureFile{name: "mimetype", content: mediaType},
		fixtureFile{name: "META-INF/container.xml", content: containerXML},
	)
	_, err := Retain(bytes.NewReader(catalog), int64(len(catalog)), math.MaxInt64)
	require.ErrorIs(t, err, ErrInvalidEPUB)
}

func minimalEPUB(t *testing.T, opf string) []byte {
	t.Helper()
	return epubFixture(t, opf, fixtureFile{name: "OPS/font.bin", content: "font bytes"})
}

func openZip(t *testing.T, data []byte) *zip.Reader {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	return reader
}

func zipEntry(t *testing.T, reader *zip.Reader, name string) *zip.File {
	t.Helper()
	for _, file := range reader.File {
		if path.Clean(file.Name) == name {
			return file
		}
	}
	return nil
}

func rawEntryBytes(t *testing.T, file *zip.File) []byte {
	t.Helper()
	reader, err := file.OpenRaw()
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	return data
}

func retainedEntryNames(t *testing.T, data []byte) []string {
	t.Helper()
	reader := openZip(t, data)
	names := make([]string, 0, len(reader.File))
	for _, file := range reader.File {
		names = append(names, path.Clean(file.Name))
	}
	return names
}

func retainedEntryBytes(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	reader := openZip(t, data)
	file := zipEntry(t, reader, name)
	require.NotNil(t, file, "missing entry %s", name)
	content, err := readFile(file)
	require.NoError(t, err)
	return content
}
