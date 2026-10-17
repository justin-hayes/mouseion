package ancientgreek

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recordingReferenceWriter captures the candidate the Brownson seam hands to
// the writer.
type recordingReferenceWriter struct {
	requests []ReferenceTranslationWriteRequest
	err      error
}

func (w *recordingReferenceWriter) WriteReferenceTranslations(_ context.Context, request ReferenceTranslationWriteRequest) (int, error) {
	w.requests = append(w.requests, request)
	return len(request.Translations), w.err
}

// brownsonFixture returns the pinned source and bytes of the synthetic
// perseus-eng2-shaped fixture. The pin is computed from the fixture bytes: the
// real pinned file never enters the repository or CI.
func brownsonFixture(t *testing.T) (BrownsonSource, []byte) {
	t.Helper()
	data, err := os.ReadFile("testdata/perseus-eng2-section.xml")
	require.NoError(t, err)
	return sourceFor(data), data
}

func writeFixture(t *testing.T, data []byte) string {
	t.Helper()
	path := t.TempDir() + "/brownson.xml"
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}

func sourceFor(data []byte) BrownsonSource {
	source := BrownsonAnabasisBook1
	source.FileSize = int64(len(data))
	sum := sha256.Sum256(data)
	source.FileSHA256 = hex.EncodeToString(sum[:])
	return source
}

func TestImportBrownsonExtractsCitedSectionsWithNotesRemoved(t *testing.T) {
	source, data := brownsonFixture(t)
	writer := &recordingReferenceWriter{}
	report, err := ImportBrownson(context.Background(), BrownsonImportRequest{
		OwnerID: "owner-1", SourcePath: writeFixture(t, data), Source: source,
	}, writer)
	require.NoError(t, err)
	assert.Equal(t, source.Pin(), report.SourcePin)
	assert.Equal(t, 4, report.Sections)
	assert.Equal(t, int64(len(data)), report.Bytes)
	require.Len(t, writer.requests, 1)
	request := writer.requests[0]
	assert.Equal(t, "owner-1", request.OwnerID)
	assert.Equal(t, source.Language, request.Language)
	assert.Equal(t, source.Pin(), request.SourcePin)

	byCitation := map[string]string{}
	for _, translation := range request.Translations {
		byCitation[translation.Citation] = translation.Text
		assert.NotContains(t, translation.Text, "editor's note", "a Perseus note is removed")
	}
	assert.Equal(t, []string{"1.1.1", "1.2.16", "1.2.17", "1.2.18"}, citationsInOrder(request.Translations))
	assert.Equal(t, "The men Cyrus led [into the country] were Greeks, and Tarsus was their goal.", byCitation["1.1.1"], "bracketed material is kept")
	assert.Equal(t, "He halted his chariot in front of the phalanx and gave orders.", byCitation["1.2.17"], "the note is removed without merging words")
}

func citationsInOrder(translations []ReferenceTranslation) []string {
	citations := make([]string, 0, len(translations))
	for _, translation := range translations {
		citations = append(citations, translation.Citation)
	}
	return citations
}

func TestImportBrownsonRefusesPinMismatch(t *testing.T) {
	source, data := brownsonFixture(t)
	path := writeFixture(t, data)

	wrongSHA := source
	wrongSHA.FileSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: path, Source: wrongSHA}, &recordingReferenceWriter{})
	assertImportCode(t, err, CodePinMismatch)

	wrongSize := source
	wrongSize.FileSize = source.FileSize + 1
	_, err = ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: path, Source: wrongSize}, &recordingReferenceWriter{})
	assertImportCode(t, err, CodePinMismatch)
}

func TestImportBrownsonRefusesOversize(t *testing.T) {
	_, data := brownsonFixture(t)
	source := sourceFor(data)
	source.FileSize = 16
	_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: writeFixture(t, data), Source: source}, &recordingReferenceWriter{})
	assertImportCode(t, err, CodeOversize)
}

func TestImportBrownsonRefusesUnreadablePath(t *testing.T) {
	source, _ := brownsonFixture(t)
	_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: t.TempDir() + "/missing.xml", Source: source}, &recordingReferenceWriter{})
	assertImportCode(t, err, CodeReadFailed)
}

func TestImportBrownsonRefusesMalformedInput(t *testing.T) {
	cases := map[string]string{
		"invalid XML":        `<?xml version="1.0"?><TEI><div`,
		"section not number": `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2"><div type="textpart" subtype="section" n="x"><p>text</p></div></div></div></TEI>`,
		"no sections":        `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2"></div></div></TEI>`,
		"empty section":      `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2"><div type="textpart" subtype="section" n="1"></div></div></div></TEI>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			data := []byte(body)
			_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: writeFixture(t, data), Source: sourceFor(data)}, &recordingReferenceWriter{})
			assertImportCode(t, err, CodeMalformed)
			assert.NotContains(t, err.Error(), "chariot", "a refusal never carries source text")
		})
	}
}

func TestImportBrownsonReimportIsIdempotent(t *testing.T) {
	source, data := brownsonFixture(t)
	path := writeFixture(t, data)
	writer := &recordingReferenceWriter{}
	for range 2 {
		_, err := ImportBrownson(context.Background(), BrownsonImportRequest{OwnerID: "owner-1", SourcePath: path, Source: source}, writer)
		require.NoError(t, err)
	}
	require.Len(t, writer.requests, 2)
	assert.Equal(t, writer.requests[0], writer.requests[1], "a re-import writes the same cited sections")
}

func TestImportBrownsonRequiresWriter(t *testing.T) {
	source, data := brownsonFixture(t)
	_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: writeFixture(t, data), Source: source}, nil)
	require.Error(t, err)
}

func TestImportBrownsonPropagatesWriterFailure(t *testing.T) {
	source, data := brownsonFixture(t)
	writer := &recordingReferenceWriter{err: errors.New("store unavailable")}
	_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: writeFixture(t, data), Source: source}, writer)
	require.Error(t, err)
}

func TestImportBrownsonValidatesSource(t *testing.T) {
	source, data := brownsonFixture(t)
	path := writeFixture(t, data)
	for _, test := range []struct {
		name   string
		mutate func(*BrownsonSource)
	}{
		{name: "zero size", mutate: func(s *BrownsonSource) { s.FileSize = 0 }},
		{name: "short sha", mutate: func(s *BrownsonSource) { s.FileSHA256 = "abc" }},
		{name: "non-hex sha", mutate: func(s *BrownsonSource) { s.FileSHA256 = strings.Repeat("z", 64) }},
		{name: "bad citation start", mutate: func(s *BrownsonSource) { s.CitationStart = "x" }},
		{name: "bad citation end", mutate: func(s *BrownsonSource) { s.CitationEnd = "y" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			broken := source
			test.mutate(&broken)
			_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: path, Source: broken}, &recordingReferenceWriter{})
			require.Error(t, err)
		})
	}
}

func TestImportBrownsonRefusesStructuralNoise(t *testing.T) {
	cases := map[string]string{
		"section without book or chapter": `<TEI><div type="textpart" subtype="section" n="17"><p>text</p></div></TEI>`,
		"duplicate cited section": `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
			`<div type="textpart" subtype="section" n="17"><p>first</p></div>` +
			`<div type="textpart" subtype="section" n="17"><p>second</p></div></div></div></TEI>`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			data := []byte(body)
			_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: writeFixture(t, data), Source: sourceFor(data)}, &recordingReferenceWriter{})
			assertImportCode(t, err, CodeMalformed)
		})
	}
}

func TestImportBrownsonDropsALeadingNoteWithoutMergingWords(t *testing.T) {
	body := `<TEI><div type="textpart" subtype="book" n="1"><div type="textpart" subtype="chapter" n="2">` +
		`<div type="textpart" subtype="section" n="17"><note anchored="true">opening note</note>Words remain.</div></div></div></TEI>`
	data := []byte(body)
	writer := &recordingReferenceWriter{}
	_, err := ImportBrownson(context.Background(), BrownsonImportRequest{SourcePath: writeFixture(t, data), Source: sourceFor(data)}, writer)
	require.NoError(t, err)
	require.Len(t, writer.requests, 1)
	require.Len(t, writer.requests[0].Translations, 1)
	assert.Equal(t, "Words remain.", writer.requests[0].Translations[0].Text, "a leading note leaves the text unchanged")
}

func TestBrownsonAnabasisBook1Pin(t *testing.T) {
	assert.Equal(t, int64(1101217), BrownsonAnabasisBook1.FileSize)
	assert.Equal(t, "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7", BrownsonAnabasisBook1.FileSHA256)
	assert.Equal(t, "sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7", BrownsonAnabasisBook1.Pin())
	assert.Equal(t, "1.1.1", BrownsonAnabasisBook1.CitationStart)
	assert.Equal(t, "1.10.19", BrownsonAnabasisBook1.CitationEnd)
}
