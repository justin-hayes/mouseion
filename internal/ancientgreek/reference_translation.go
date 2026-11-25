package ancientgreek

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/classical"
)

// ReferenceTranslation is one cited section's English text, keyed by citation
// such as "1.2.17".
type ReferenceTranslation struct {
	Citation string
	Text     string
}

// ParseReferenceTranslation extracts every cited section of the citation range
// from a Perseus perseus-eng2 TEI file. Perseus notes and editorial textparts
// (an argument, introduction, chronology or note) are removed, remaining text
// is kept as is, whitespace is collapsed, and the citation depth comes from the
// range so section-only and book/chapter/section work files are read the same
// way. A file with no cited textpart is read by its section milestones when
// the range has a single level. Sections are returned in citation order.
func ParseReferenceTranslation(data []byte, citationStart, citationEnd string) ([]ReferenceTranslation, error) {
	return parseReferenceTranslation(data, citationStart, citationEnd, false, true)
}

// ParseReferenceTranslationMeasured reads a translation for measurement rather
// than admission. It accepts what the strict parser refuses as a misalignment:
// gaps in the cited numbering are kept as gaps, and a translation keyed at a
// shallower level than the range (a chapter-level file against section
// Citations) is read at its own depth, so a join rate can be computed over it.
// A section repeated in the file is read as one section. It still refuses a
// file that is not well-formed or has no cited section.
func ParseReferenceTranslationMeasured(data []byte, citationStart, citationEnd string) ([]ReferenceTranslation, error) {
	sections, err := parseReferenceTranslation(data, citationStart, citationEnd, true, true)
	if err == nil {
		return sections, nil
	}
	start, ok := parseCitation(citationStart)
	end, endOK := parseCitation(citationEnd)
	if !ok || !endOK || len(start) != len(end) {
		return nil, err
	}
	for depth := len(start) - 1; depth >= 1; depth-- {
		// Section milestones key the Latin's own sections; a shallower read
		// never takes them for a coarser level.
		shallow, shallowErr := parseReferenceTranslation(data, start[:depth].String(), end[:depth].String(), true, false)
		if shallowErr == nil {
			return shallow, nil
		}
	}
	return nil, err
}

// detailNoCitedSections is the refusal of a file with no cited section.
const detailNoCitedSections = "no cited sections"

// parseReferenceTranslation is the shared parser; lenient skips the
// contiguity check and folds a repeated section into its first occurrence.
// When milestones is set and a single-level range finds no cited textpart, the
// file is read again by its section milestones.
func parseReferenceTranslation(data []byte, citationStart, citationEnd string, lenient, milestones bool) ([]ReferenceTranslation, error) {
	start, ok := parseCitation(citationStart)
	if !ok {
		return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "citation start is not a citation"}
	}
	end, ok := parseCitation(citationEnd)
	if !ok {
		return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "citation end is not a citation"}
	}
	if len(start) != len(end) || compareCitation(start, end) > 0 {
		return nil, &ImportError{Code: CodeUnknownInterpret, Detail: "citation range does not align"}
	}
	sections, err := runReferenceParser(data, &eng2Parser{start: start, end: end, depth: len(start), seen: make(map[string]int), lenient: lenient})
	if importErr, ok := errors.AsType[*ImportError](err); ok && importErr.Detail == detailNoCitedSections && milestones && len(start) == 1 {
		return runReferenceParser(data, &eng2Parser{start: start, end: end, depth: len(start), seen: make(map[string]int), lenient: lenient, milestones: true})
	}
	return sections, err
}

// runReferenceParser streams the file through one parser.
func runReferenceParser(data []byte, parser *eng2Parser) ([]ReferenceTranslation, error) {
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

// eng2Parser is the streaming state machine of ParseReferenceTranslation.
type eng2Parser struct {
	start, end classical.Citation
	depth      int
	divs       []string
	textparts  []textpart
	body       strings.Builder
	noteDepth  int
	sections   []ReferenceTranslation
	// seen maps an emitted citation to its index in sections.
	seen map[string]int
	// lenient skips the contiguity check and folds a repeated section into
	// its first occurrence, for measurement.
	lenient bool
	// editorialDepth counts the open divs inside an editorial textpart, whose
	// text and divisions are skipped.
	editorialDepth int
	// milestones reads sections from section milestones instead of textparts.
	milestones bool
	// milestone is the number of the open milestone section, if any, and
	// milestoneDivs the div depth it began at: it ends at the next section
	// milestone or when that div closes.
	milestone     string
	milestoneDivs int
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
		if p.collecting() {
			p.body.Write(typed)
		}
	}
	return nil
}

// collecting reports whether character data belongs to the open section.
func (p *eng2Parser) collecting() bool {
	return p.noteDepth == 0 && p.editorialDepth == 0 && (len(p.textparts) == p.depth || p.milestone != "")
}

// open tracks div nesting, enters a Perseus note, or opens a textpart. A
// textpart deeper than the reading unit's citation depth is an extra division
// the parser refuses rather than silently folding into its parent.
func (p *eng2Parser) open(element xml.StartElement) error {
	switch element.Name.Local {
	case "note":
		if p.collecting() && !endsWithSpace(p.body.String()) {
			p.body.WriteByte(' ')
		}
		p.noteDepth++
		return nil
	case "milestone":
		return p.openMilestone(element)
	case "div":
	default:
		return nil
	}
	if p.editorialDepth > 0 {
		p.editorialDepth++
		p.divs = append(p.divs, "")
		return nil
	}
	kind, number, err := textpartKind(element)
	if err != nil {
		return err
	}
	if kind == kindEditorial {
		p.editorialDepth = 1
		p.divs = append(p.divs, "")
		return nil
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
	if p.editorialDepth > 0 {
		p.editorialDepth--
		return nil
	}
	if p.milestone != "" && len(p.divs) < p.milestoneDivs {
		if err := p.closeMilestone(); err != nil {
			return err
		}
	}
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

// openMilestone starts a section at a section milestone, in milestone mode,
// outside notes, editorial textparts and textparts. Any other milestone is
// not a section boundary.
func (p *eng2Parser) openMilestone(element xml.StartElement) error {
	if !p.milestones || p.noteDepth > 0 || p.editorialDepth > 0 || len(p.textparts) > 0 {
		return nil
	}
	var unit, number string
	for _, attribute := range element.Attr {
		switch attribute.Name.Local {
		case "unit":
			unit = attribute.Value
		case "n":
			number = attribute.Value
		}
	}
	if unit != "section" {
		return nil
	}
	if _, ok := classical.ParseCitation(number); !ok || strings.Contains(number, ".") {
		return &ImportError{Code: CodeMalformed, Detail: "milestone number is not a Citation component"}
	}
	if err := p.closeMilestone(); err != nil {
		return err
	}
	p.milestone, p.milestoneDivs = number, len(p.divs)
	p.body.Reset()
	return nil
}

// closeMilestone emits the open milestone section, if any.
func (p *eng2Parser) closeMilestone() error {
	if p.milestone == "" {
		return nil
	}
	citation := p.milestone
	p.milestone = ""
	return p.emitCitation(citation)
}

// emit records the closed textpart section when it falls inside the reading
// unit.
func (p *eng2Parser) emit() error {
	citation, ok := citationFromTextparts(p.textparts)
	if !ok {
		return &ImportError{Code: CodeMalformed, Detail: "section citation is not a citation"}
	}
	return p.emitCitation(citation)
}

// emitCitation records the collected text as the section citation when it
// falls inside the reading unit.
func (p *eng2Parser) emitCitation(citation string) error {
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
	if index, duplicate := p.seen[citation]; duplicate {
		if !p.lenient {
			return &ImportError{Code: CodeMalformed, Detail: "duplicate cited section"}
		}
		p.sections[index].Text += " " + text
		return nil
	}
	p.seen[citation] = len(p.sections)
	p.sections = append(p.sections, ReferenceTranslation{Citation: citation, Text: text})
	return nil
}

// finish validates the completed document, refuses a gap in the cited
// numbering, and orders its sections.
func (p *eng2Parser) finish() error {
	if p.noteDepth != 0 {
		return &ImportError{Code: CodeMalformed, Detail: "unterminated note"}
	}
	if err := p.closeMilestone(); err != nil {
		return err
	}
	if len(p.sections) == 0 {
		return &ImportError{Code: CodeMalformed, Detail: detailNoCitedSections}
	}
	if !p.lenient {
		if err := p.checkContiguous(); err != nil {
			return err
		}
	}
	// Every emitted citation parsed in emit, so the sort comparator cannot see
	// an unparseable key.
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
			component := parsed[length-1]
			if component.Name != "" {
				// A named section (pr, ep) has no number to order, so it
				// takes no part in the contiguity check.
				continue
			}
			prefix := citationPrefix(parsed[:length])
			groups[prefix] = append(groups[prefix], component.Number)
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
func checkRangeBoundary(prefix string, values []int, start, end classical.Citation) error {
	if len(values) == 0 {
		return nil
	}
	if prefix == citationPrefix(start) && start[len(start)-1].Name == "" && values[0] != start[len(start)-1].Number {
		return &ImportError{Code: CodeMalformed, Detail: "cited range starts at a missing section"}
	}
	if prefix == citationPrefix(end) && end[len(end)-1].Name == "" && values[len(values)-1] != end[len(end)-1].Number {
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
func citationPrefix(citation classical.Citation) string {
	if len(citation) <= 1 {
		return ""
	}
	return citation[:len(citation)-1].String()
}

// kindEditorial marks an editorial textpart, such as an argument, an
// introduction, a chronology or a note.
const kindEditorial = "editorial"

// textpartKind reports the reading-unit div kind a TEI textpart carries, and
// its number. A non-textpart div reports an empty kind. A textpart named by a
// lowercase word of two or more letters that is not a Citation component is
// editorial matter, not a cited division.
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
	case "book", "speech", "chapter", "section", "subsection":
	default:
		return "", "", nil
	}
	if _, ok := classical.ParseCitation(name); !ok || strings.Contains(name, ".") {
		if editorialName(name) {
			return kindEditorial, "", nil
		}
		return "", "", &ImportError{Code: CodeMalformed, Detail: "textpart number is not a Citation component"}
	}
	return subtype, name, nil
}

// editorialName reports whether a textpart name is a lowercase word of two or
// more letters.
func editorialName(name string) bool {
	if len(name) < 2 {
		return false
	}
	for _, letter := range name {
		if letter < 'a' || letter > 'z' {
			return false
		}
	}
	return true
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
