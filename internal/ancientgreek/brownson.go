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

// BrownsonSource pins an operator-supplied English reference translation by
// size and SHA-256, exactly like a pinned oga_src annotation. Its sections are
// stored as source material keyed by citation.
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

// BrownsonImportRequest names the local path of the pinned Brownson file. The
// parsed rows are installation-wide, so no learner is named.
type BrownsonImportRequest struct {
	SourcePath string
	Source     BrownsonSource
}

// ReferenceTranslationWriteRequest is the source-neutral candidate the writer
// persists: the cited sections and the reviewed pin they were read from. The
// pin is a 64-character lowercase SHA-256 digest and keys installation-wide
// rows, so the request carries no owner.
type ReferenceTranslationWriteRequest struct {
	Language             string
	TranslationPinSHA256 string
	Translations         []ReferenceTranslation
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
	sections, err := ParseReferenceTranslation(data, req.Source.CitationStart, req.Source.CitationEnd)
	if err != nil {
		return BrownsonImportReport{}, err
	}
	if _, err := writer.WriteReferenceTranslations(ctx, ReferenceTranslationWriteRequest{
		Language: req.Source.Language, TranslationPinSHA256: req.Source.FileSHA256, Translations: sections,
	}); err != nil {
		return BrownsonImportReport{}, err
	}
	return BrownsonImportReport{SourcePin: req.Source.Pin(), Sections: len(sections), Bytes: int64(len(data))}, nil
}

// ParseReferenceTranslation extracts every cited section of the citation range
// from a Perseus perseus-eng2 TEI file. It is the generalised parser used for
// any pinned reference translation, not just Brownson: Perseus notes are
// removed, remaining text is kept as is, whitespace is collapsed, and the
// citation depth comes from the range so section-only and book/chapter/section
// work files are read the same way. Sections are returned in citation order.
func ParseReferenceTranslation(data []byte, citationStart, citationEnd string) ([]ReferenceTranslation, error) {
	return parsePerseusEng2(data, BrownsonSource{CitationStart: citationStart, CitationEnd: citationEnd})
}

// parsePerseusEng2 extracts every cited section of the reading unit from a
// Perseus perseus-eng2 TEI file. Perseus notes are removed and the remaining
// text is shown as is, including bracketed material; whitespace is collapsed.
// The citation depth comes from the reading unit's range, so a section-only
// work and a book/chapter/section/subsection work are read the same way.
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
	if len(start) != len(end) || compareCitation(start, end) > 0 {
		return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "citation range does not align"}
	}
	parser := &eng2Parser{start: start, end: end, depth: len(start), seen: make(map[string]struct{})}
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

// InspectTranslation parses a pinned translation file over the citation range
// and returns the number of aligned sections. It refuses a missing or extra
// section inside the range rather than filling a gap.
func InspectTranslation(data []byte, start, end string) (int, error) {
	sections, err := ParseReferenceTranslation(data, start, end)
	if err != nil {
		return 0, err
	}
	return len(sections), nil
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
	start, end []int
	depth      int
	divs       []string
	textparts  []textpart
	body       strings.Builder
	noteDepth  int
	sections   []ReferenceTranslation
	seen       map[string]struct{}
}

// textpart is one open textpart div, whose numbers join into a citation.
type textpart struct {
	number        string
	hasCitedChild bool
}

// accept feeds one XML token to the state machine.
func (p *eng2Parser) accept(token xml.Token) error {
	switch typed := token.(type) {
	case xml.StartElement:
		return p.open(typed)
	case xml.EndElement:
		return p.close(typed.Name.Local)
	case xml.CharData:
		if p.noteDepth == 0 && len(p.textparts) == p.depth {
			p.body.Write(typed)
		}
	}
	return nil
}

// open tracks div nesting, enters a Perseus note, or opens a textpart. A
// textpart deeper than the reading unit's citation depth is an extra division
// the parser refuses rather than silently folding into its parent.
func (p *eng2Parser) open(element xml.StartElement) error {
	switch element.Name.Local {
	case "note":
		if p.noteDepth == 0 && len(p.textparts) == p.depth && !endsWithSpace(p.body.String()) {
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
	if kind == "" {
		return nil
	}
	if len(p.textparts) >= p.depth {
		return &ImportError{Code: CodeMalformed, Detail: "textpart deeper than citation depth"}
	}
	if len(p.textparts)+1 == p.depth {
		for index := range p.textparts {
			p.textparts[index].hasCitedChild = true
		}
	}
	p.textparts = append(p.textparts, textpart{number: number})
	if len(p.textparts) == p.depth {
		p.body.Reset()
	}
	return nil
}

// close leaves a Perseus note or closes a div. Closing the textpart that sits
// at the reading unit's citation depth emits its section.
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
	if kind == "" {
		return nil
	}
	if len(p.textparts) == 0 {
		return &ImportError{Code: CodeMalformed, Detail: "textpart nesting is inconsistent"}
	}
	current := p.textparts[len(p.textparts)-1]
	if len(p.textparts) == p.depth {
		if err := p.emit(); err != nil {
			return err
		}
	} else if !current.hasCitedChild {
		return &ImportError{Code: CodeMalformed, Detail: "textpart has no cited section"}
	}
	p.textparts = p.textparts[:len(p.textparts)-1]
	return nil
}

// emit records the closed section when it falls inside the reading unit.
func (p *eng2Parser) emit() error {
	citation, ok := citationFromTextparts(p.textparts)
	if !ok {
		return &ImportError{Code: CodeMalformed, Detail: "section citation is not a citation"}
	}
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

// finish validates the completed document, refuses a gap in the cited
// numbering, and orders its sections.
func (p *eng2Parser) finish() error {
	if p.noteDepth != 0 {
		return &ImportError{Code: CodeMalformed, Detail: "unterminated note"}
	}
	if len(p.sections) == 0 {
		return &ImportError{Code: CodeMalformed, Detail: "no cited sections"}
	}
	if err := p.checkContiguous(); err != nil {
		return err
	}
	// checkContiguous validated every citation, so the sort comparator cannot
	// see an unparseable key.
	sort.SliceStable(p.sections, func(i, j int) bool {
		left, _ := parseCitation(p.sections[i].Citation)
		right, _ := parseCitation(p.sections[j].Citation)
		return compareCitation(left, right) < 0
	})
	return nil
}

// checkContiguous refuses a missing section rather than letting the range fill
// the gap. Under each parent prefix the observed child numbers must be
// consecutive; a translation that skips 1.2.17 is misaligned even if its keys
// otherwise compare inside the range. When the start or end parent is present,
// its first or last number must match the range boundary.
func (p *eng2Parser) checkContiguous() error {
	groups, err := citationGroups(p.sections)
	if err != nil {
		return err
	}
	for prefix, values := range groups {
		if err := checkContiguousGroup(values); err != nil {
			return err
		}
		if err := checkRangeBoundary(prefix, values, p.start, p.end); err != nil {
			return err
		}
	}
	return nil
}

// citationGroups groups cited section numbers by every ancestor prefix, so a
// missing whole chapter shows up as a gap in the book's chapter numbers.
func citationGroups(sections []ReferenceTranslation) (map[string][]int, error) {
	groups := map[string][]int{}
	for _, section := range sections {
		parsed, ok := parseCitation(section.Citation)
		if !ok || len(parsed) == 0 {
			return nil, &ImportError{Code: CodeMalformed, Detail: "section citation is not a citation"}
		}
		for length := 1; length <= len(parsed); length++ {
			prefix := citationPrefix(parsed[:length])
			groups[prefix] = append(groups[prefix], parsed[length-1])
		}
	}
	return groups, nil
}

// checkContiguousGroup refuses a gap in one parent's child numbers. Repeated
// ancestor entries (a book seen once per section) collapse before the check.
func checkContiguousGroup(values []int) error {
	sort.Ints(values)
	last := values[0]
	for _, value := range values[1:] {
		if value == last {
			continue
		}
		if value != last+1 {
			return &ImportError{Code: CodeMalformed, Detail: "cited sections are not contiguous"}
		}
		last = value
	}
	return nil
}

// checkRangeBoundary refuses a group that begins after the range start or ends
// before the range end.
func checkRangeBoundary(prefix string, values []int, start, end []int) error {
	if prefix == citationPrefix(start) && values[0] != start[len(start)-1] {
		return &ImportError{Code: CodeMalformed, Detail: "cited range starts at a missing section"}
	}
	if prefix == citationPrefix(end) && values[len(values)-1] != end[len(end)-1] {
		return &ImportError{Code: CodeMalformed, Detail: "cited range ends at a missing section"}
	}
	return nil
}

// citationFromTextparts joins an open textpart stack into a dotted citation.
func citationFromTextparts(parts []textpart) (string, bool) {
	if len(parts) == 0 {
		return "", false
	}
	numbers := make([]string, len(parts))
	for index, part := range parts {
		numbers[index] = part.number
	}
	return strings.Join(numbers, "."), true
}

// citationPrefix returns every citation component except the last as a dotted
// string.
func citationPrefix(citation []int) string {
	if len(citation) <= 1 {
		return ""
	}
	parts := make([]string, len(citation)-1)
	for index, number := range citation[:len(citation)-1] {
		parts[index] = strconv.Itoa(number)
	}
	return strings.Join(parts, ".")
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
	case "book", "chapter", "section", "subsection":
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
