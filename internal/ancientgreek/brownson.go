package ancientgreek

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// BrownsonSource is the compiled, reviewed pin of an operator-supplied English
// reference translation: the Perseus perseus-eng2 digitisation of Brownson's
// 1921-22 translation of Xenophon's Anabasis. It is checked by size and SHA-256
// exactly like the pinned oga_src annotation, and its sections are stored as
// source material keyed by citation.
type BrownsonSource struct {
	Language      string
	EditionLabel  string
	WorkLabel     string
	FileName      string
	FileSize      int64
	FileSHA256    string
	CitationStart string
	CitationEnd   string
}

// BrownsonAnabasisBook1 is the pinned Brownson translation of the Anabasis in
// the Perseus perseus-eng2 digitisation (canonical-greekLit
// 0189b9c8d8d1f148133f2f0af309b4ee90ea4332). Size and SHA-256 are the reviewed
// commitments the import checks.
var BrownsonAnabasisBook1 = BrownsonSource{
	Language:      Language,
	EditionLabel:  "Brownson (1922)",
	WorkLabel:     "Xen. Anab.",
	FileName:      "tlg0032.tlg006.perseus-eng2.xml",
	FileSize:      1101217,
	FileSHA256:    "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7",
	CitationStart: "1.1.1",
	CitationEnd:   "1.10.19",
}

// Pin returns the stable source identity frozen on a prepared deck item. It is
// the same digest the import checks, never any source text.
func (s BrownsonSource) Pin() string { return "sha256:" + s.FileSHA256 }

// validate refuses a source whose reviewed pin is not a positive size and a
// lowercase SHA-256, mirroring the reading-unit definition contract.
func (s BrownsonSource) validate() error {
	if s.FileSize <= 0 || len(s.FileSHA256) != 64 {
		return errors.New("ancientgreek: brownson source pin is invalid")
	}
	for _, r := range s.FileSHA256 {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return errors.New("ancientgreek: brownson source pin is invalid")
		}
	}
	if _, ok := parseCitation(s.CitationStart); !ok {
		return errors.New("ancientgreek: brownson source citation start is invalid")
	}
	if _, ok := parseCitation(s.CitationEnd); !ok {
		return errors.New("ancientgreek: brownson source citation end is invalid")
	}
	return nil
}

// ReferenceTranslation is one cited section's English text, keyed by citation
// such as "1.2.17".
type ReferenceTranslation struct {
	Citation string
	Text     string
}

// BrownsonImportRequest names the learner and the local path of the pinned
// Brownson file.
type BrownsonImportRequest struct {
	OwnerID    string
	SourcePath string
	Source     BrownsonSource
}

// ReferenceTranslationWriteRequest is the source-neutral candidate the writer
// persists: the cited sections and the pin they were read from.
type ReferenceTranslationWriteRequest struct {
	OwnerID      string
	Language     string
	SourcePin    string
	Translations []ReferenceTranslation
}

// BrownsonImportWriter persists one imported reference translation. It is the
// only part of the Brownson seam that touches the installation.
type BrownsonImportWriter interface {
	WriteReferenceTranslations(context.Context, ReferenceTranslationWriteRequest) (int, error)
}

// BrownsonImportReport is the outcome of one Brownson import. It names counts
// and the pin only, never any source text.
type BrownsonImportReport struct {
	SourcePin string
	Sections  int
	Bytes     int64
}

// ImportBrownson reads the pinned Brownson file, checks its pin, extracts the
// cited sections of the reading unit and hands them to the writer. It never
// logs or returns source text.
func ImportBrownson(ctx context.Context, req BrownsonImportRequest, writer BrownsonImportWriter) (BrownsonImportReport, error) {
	if writer == nil {
		return BrownsonImportReport{}, errors.New("ancientgreek: brownson import writer is required")
	}
	if err := req.Source.validate(); err != nil {
		return BrownsonImportReport{}, err
	}
	data, err := readPinnedBytes(req.SourcePath, req.Source.FileSize, req.Source.FileSHA256)
	if err != nil {
		return BrownsonImportReport{}, err
	}
	sections, err := parsePerseusEng2(data, req.Source)
	if err != nil {
		return BrownsonImportReport{}, err
	}
	if _, err := writer.WriteReferenceTranslations(ctx, ReferenceTranslationWriteRequest{
		OwnerID: req.OwnerID, Language: req.Source.Language, SourcePin: req.Source.Pin(), Translations: sections,
	}); err != nil {
		return BrownsonImportReport{}, err
	}
	return BrownsonImportReport{SourcePin: req.Source.Pin(), Sections: len(sections), Bytes: int64(len(data))}, nil
}

// parsePerseusEng2 extracts every cited section of the reading unit from a
// Perseus perseus-eng2 TEI file. Perseus notes are removed and the remaining
// text is shown as is, including bracketed material; whitespace is collapsed.
// Sections are returned in citation order.
func parsePerseusEng2(data []byte, source BrownsonSource) ([]ReferenceTranslation, error) {
	start, ok := parseCitation(source.CitationStart)
	if !ok {
		return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "citation start is not a citation"}
	}
	end, ok := parseCitation(source.CitationEnd)
	if !ok {
		return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "citation end is not a citation"}
	}
	parser := &eng2Parser{start: start, end: end, seen: make(map[string]struct{})}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, xmlImportError(err)
		}
		if err := parser.accept(token); err != nil {
			return nil, err
		}
	}
	if err := parser.finish(); err != nil {
		return nil, err
	}
	return parser.sections, nil
}

// xmlImportError turns a decoder failure into a source-free refusal. The
// decoder's own message may quote source text, so only its line is kept.
func xmlImportError(err error) error {
	if syntax, ok := errors.AsType[*xml.SyntaxError](err); ok {
		return &ImportError{Code: CodeMalformed, Line: syntax.Line, Detail: "invalid XML"}
	}
	return &ImportError{Code: CodeMalformed, Detail: "invalid XML"}
}

// eng2Parser is the streaming state machine of parsePerseusEng2.
type eng2Parser struct {
	start, end             []int
	divs                   []string
	book, chapter, section string
	body                   strings.Builder
	noteDepth              int
	sections               []ReferenceTranslation
	seen                   map[string]struct{}
}

// accept feeds one XML token to the state machine.
func (p *eng2Parser) accept(token xml.Token) error {
	switch typed := token.(type) {
	case xml.StartElement:
		return p.open(typed)
	case xml.EndElement:
		return p.close(typed.Name.Local)
	case xml.CharData:
		if p.noteDepth == 0 && p.section != "" {
			p.body.Write(typed)
		}
	}
	return nil
}

// start opens a textpart div or enters a Perseus note.
func (p *eng2Parser) open(element xml.StartElement) error {
	switch element.Name.Local {
	case "note":
		if p.noteDepth == 0 && p.section != "" && !endsWithSpace(p.body.String()) {
			p.body.WriteByte(' ')
		}
		p.noteDepth++
		return nil
	case "div":
	default:
		return nil
	}
	kind, number, err := textpartKind(element)
	if err != nil {
		return err
	}
	p.divs = append(p.divs, kind)
	switch kind {
	case "book":
		p.book, p.chapter, p.section = number, "", ""
	case "chapter":
		p.chapter, p.section = number, ""
	case "section":
		p.section = number
		p.body.Reset()
	}
	return nil
}

// end closes a textpart div or a Perseus note.
func (p *eng2Parser) close(local string) error {
	switch local {
	case "note":
		if p.noteDepth > 0 {
			p.noteDepth--
		}
		return nil
	case "div":
	default:
		return nil
	}
	if len(p.divs) == 0 {
		return nil
	}
	kind := p.divs[len(p.divs)-1]
	p.divs = p.divs[:len(p.divs)-1]
	switch kind {
	case "book":
		p.book, p.chapter, p.section = "", "", ""
	case "chapter":
		p.chapter, p.section = "", ""
	case "section":
		err := p.emit()
		p.section = ""
		return err
	}
	return nil
}

// emit records the closed section when it falls inside the reading unit.
func (p *eng2Parser) emit() error {
	if p.section == "" {
		return nil
	}
	if p.book == "" || p.chapter == "" {
		return &ImportError{Code: CodeMalformed, Detail: "section without book or chapter"}
	}
	citation := p.book + "." + p.chapter + "." + p.section
	parsed, ok := parseCitation(citation)
	if !ok {
		return &ImportError{Code: CodeMalformed, Detail: "section citation is not a citation"}
	}
	if compareCitation(parsed, p.start) < 0 || compareCitation(parsed, p.end) > 0 {
		return nil
	}
	text := normalizeSectionText(p.body.String())
	if text == "" {
		return &ImportError{Code: CodeMalformed, Detail: "cited section has no text"}
	}
	if _, duplicate := p.seen[citation]; duplicate {
		return &ImportError{Code: CodeMalformed, Detail: "duplicate cited section"}
	}
	p.seen[citation] = struct{}{}
	p.sections = append(p.sections, ReferenceTranslation{Citation: citation, Text: text})
	return nil
}

// finish validates the completed document and orders its sections.
func (p *eng2Parser) finish() error {
	if p.noteDepth != 0 {
		return &ImportError{Code: CodeMalformed, Detail: "unterminated note"}
	}
	if len(p.sections) == 0 {
		return &ImportError{Code: CodeMalformed, Detail: "no cited sections"}
	}
	sort.SliceStable(p.sections, func(i, j int) bool {
		left, _ := parseCitation(p.sections[i].Citation)
		right, _ := parseCitation(p.sections[j].Citation)
		return compareCitation(left, right) < 0
	})
	return nil
}

// textpartKind reports the reading-unit div kind a TEI textpart carries, and
// its number. A non-textpart div reports an empty kind.
func textpartKind(element xml.StartElement) (kind, number string, err error) {
	var subtype, name string
	for _, attribute := range element.Attr {
		switch attribute.Name.Local {
		case "subtype":
			subtype = attribute.Value
		case "n":
			name = attribute.Value
		}
	}
	switch subtype {
	case "book", "chapter", "section":
	default:
		return "", "", nil
	}
	if _, parseErr := strconv.Atoi(name); parseErr != nil {
		return "", "", &ImportError{Code: CodeMalformed, Detail: "textpart number is not a number"}
	}
	return subtype, name, nil
}

// normalizeSectionText collapses the file's formatting whitespace and trims it.
// It never edits the translation itself.
func normalizeSectionText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// endsWithSpace reports whether the accumulated section text already ends with
// whitespace, so a removed note does not merge the words around it.
func endsWithSpace(text string) bool {
	if text == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(text)
	return unicode.IsSpace(last)
}
