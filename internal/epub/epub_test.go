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
