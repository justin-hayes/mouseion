// Package epub extracts reading-order text and reproducible source locations
// from EPUB containers.
package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"
)

const mediaType = "application/epub+zip"

var ErrInvalidEPUB = errors.New("epub: invalid EPUB")

type Location struct {
	SourceDocumentID string `json:"source_document_id"`
	Chapter          string `json:"chapter"`
	Section          string `json:"section"`
	StartOffset      uint64 `json:"start_offset"`
	EndOffset        uint64 `json:"end_offset"`
}

type Chapter struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Location Location `json:"location"`
}

type ExtractedBook struct {
	Title            string    `json:"title"`
	SourceIdentifier string    `json:"source_identifier"`
	FullText         string    `json:"full_text"`
	Chapters         []Chapter `json:"chapters"`
}

type container struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}
type packageDoc struct {
	Metadata struct {
		Titles      []string `xml:"title"`
		Identifiers []string `xml:"identifier"`
	} `xml:"metadata"`
	Manifest []struct {
		ID         string `xml:"id,attr"`
		Href       string `xml:"href,attr"`
		MediaType  string `xml:"media-type,attr"`
		Properties string `xml:"properties,attr"`
	} `xml:"manifest>item"`
	Spine []struct {
		IDRef  string `xml:"idref,attr"`
		Linear string `xml:"linear,attr"`
	} `xml:"spine>itemref"`
}

// Extract validates an EPUB byte stream and extracts XHTML documents in spine order.
func Extract(data []byte) (ExtractedBook, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return ExtractedBook{}, invalid("open ZIP container", err)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[path.Clean(f.Name)] = f
	}
	if f := files["mimetype"]; f == nil {
		return ExtractedBook{}, invalid("missing mimetype file", nil)
	} else if value, readErr := readFile(f); readErr != nil {
		return ExtractedBook{}, invalid("read mimetype file", readErr)
	} else if strings.TrimSpace(string(value)) != mediaType {
		return ExtractedBook{}, invalid("mimetype is not "+mediaType, nil)
	}
	var c container
	if err := decodeXMLFile(files, "META-INF/container.xml", &c); err != nil {
		return ExtractedBook{}, invalid("read META-INF/container.xml", err)
	}
	if len(c.Rootfiles) == 0 || strings.TrimSpace(c.Rootfiles[0].FullPath) == "" {
		return ExtractedBook{}, invalid("container.xml has no package rootfile", nil)
	}
	opfPath := path.Clean(c.Rootfiles[0].FullPath)
	var pkg packageDoc
	if err := decodeXMLFile(files, opfPath, &pkg); err != nil {
		return ExtractedBook{}, invalid("read package document "+opfPath, err)
	}
	book := ExtractedBook{Title: firstNonBlank(pkg.Metadata.Titles), SourceIdentifier: firstNonBlank(pkg.Metadata.Identifiers)}
	if book.SourceIdentifier == "" {
		return ExtractedBook{}, invalid("package metadata has no identifier", nil)
	}
	items := make(map[string]struct{ href, mediaType, properties string })
	for _, item := range pkg.Manifest {
		items[item.ID] = struct{ href, mediaType, properties string }{item.Href, item.MediaType, item.Properties}
	}
	base := path.Dir(opfPath)
	var out strings.Builder
	var offset uint64
	for _, ref := range pkg.Spine {
		if strings.EqualFold(ref.Linear, "no") {
			continue
		}
		item, ok := items[ref.IDRef]
		if !ok {
			return ExtractedBook{}, invalid("spine references missing manifest item "+ref.IDRef, nil)
		}
		if item.mediaType != "application/xhtml+xml" || hasWord(item.properties, "nav") {
			continue
		}
		name := path.Clean(path.Join(base, item.href))
		text, title, err := extractXHTML(files[name])
		if err != nil {
			return ExtractedBook{}, invalid("extract spine document "+name, err)
		}
		if text == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
			offset += 2
		}
		start := offset
		out.WriteString(text)
		offset += uint64(len([]rune(text)))
		if title == "" {
			title = ref.IDRef
		}
		book.Chapters = append(book.Chapters, Chapter{ID: ref.IDRef, Title: title, Location: Location{SourceDocumentID: book.SourceIdentifier, Chapter: title, Section: title, StartOffset: start, EndOffset: offset}})
	}
	if len(book.Chapters) == 0 {
		return ExtractedBook{}, invalid("package spine contains no readable XHTML content", nil)
	}
	book.FullText = out.String()
	return book, nil
}

func readFile(f *zip.File) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func decodeXMLFile(files map[string]*zip.File, name string, dst any) error {
	f := files[path.Clean(name)]
	if f == nil {
		return fmt.Errorf("missing %s", name)
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	d := xml.NewDecoder(r)
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("malformed or unsupported encoding: %w", err)
	}
	return nil
}

func extractXHTML(f *zip.File) (string, string, error) {
	if f == nil {
		return "", "", errors.New("manifest resource is missing")
	}
	r, err := f.Open()
	if err != nil {
		return "", "", err
	}
	defer r.Close()
	d := xml.NewDecoder(r)
	var blocks []string
	var current strings.Builder
	var title string
	skip := 0
	flush := func() {
		if s := cleanSpace(current.String()); s != "" {
			blocks = append(blocks, s)
		}
		current.Reset()
	}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", "", fmt.Errorf("malformed XHTML or unsupported encoding: %w", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			n := strings.ToLower(v.Name.Local)
			if skip > 0 {
				skip++
				continue
			}
			if n == "script" || n == "style" || n == "nav" || n == "head" {
				skip = 1
				continue
			}
			if isBlock(n) {
				flush()
			}
		case xml.EndElement:
			if skip > 0 {
				skip--
				continue
			}
			n := strings.ToLower(v.Name.Local)
			if isBlock(n) {
				candidate := cleanSpace(current.String())
				if title == "" && (n == "h1" || n == "h2") {
					title = candidate
				}
				flush()
			}
		case xml.CharData:
			if skip == 0 {
				current.Write([]byte(v))
			}
		}
	}
	flush()
	return strings.Join(blocks, "\n\n"), title, nil
}

func isBlock(s string) bool {
	switch s {
	case "address", "article", "aside", "blockquote", "br", "div", "figcaption", "figure", "footer", "h1", "h2", "h3", "h4", "h5", "h6", "header", "hr", "li", "main", "p", "pre", "section", "td", "th":
		return true
	}
	return false
}
func cleanSpace(s string) string { return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ") }
func firstNonBlank(v []string) string {
	for _, s := range v {
		if s = strings.TrimSpace(s); s != "" {
			return s
		}
	}
	return ""
}
func hasWord(s, word string) bool {
	for _, v := range strings.Fields(s) {
		if v == word {
			return true
		}
	}
	return false
}
func invalid(message string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrInvalidEPUB, message)
	}
	return fmt.Errorf("%w: %s: %v", ErrInvalidEPUB, message, err)
}

func MediaType() string { return mediaType }
