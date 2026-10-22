package epub

import (
	"archive/zip"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// groupSeparator joins source groups in the unit text. A block boundary, a
// blank line and <hr> all contribute the same separator.
const groupSeparator = "\n\n"

// relativeRoleSpan is one role span relative to the text of the group that
// records it. Extraction converts it into a unit-relative domain.UnitRoleSpan.
type relativeRoleSpan struct {
	role  domain.SourceStructureRole
	start int
	end   int
}

// v2Group is one source group: a block's exact text, its nested role spans and
// whether the group is a heading.
type v2Group struct {
	text    string
	spans   []relativeRoleSpan
	heading bool
}

// xhtmlV2Document is one extracted document under the v2 contract.
type xhtmlV2Document struct {
	groups []v2Group
	title  string
}

// v2Block is one block context on the extractor's open-element stack. The root
// context has no tag and no role.
type v2Block struct {
	tag          string
	heading      bool
	titleHeading bool
	role         domain.SourceStructureRole
}

// openSpan is one open span relative to the current group. A non-role span
// carries an empty role so its end tag never closes an enclosing role span.
type openSpan struct {
	role  domain.SourceStructureRole
	start int
}

// xhtmlV2Extractor applies the versioned break and role conventions to one
// XHTML document. Text, breaks and role offsets share one code-point basis.
type xhtmlV2Extractor struct {
	blocks    []v2Block
	text      []rune
	spans     []relativeRoleSpan
	openSpans []openSpan
	skip      int
	groups    []v2Group
	title     string
}

func newXHTMLV2Extractor() *xhtmlV2Extractor {
	return &xhtmlV2Extractor{blocks: []v2Block{{}}}
}

// extractXHTMLV2 extracts one XHTML document under the v2 contract.
func extractXHTMLV2(f *zip.File) (document xhtmlV2Document, err error) {
	if f == nil {
		return xhtmlV2Document{}, errors.New("manifest resource is missing")
	}
	r, err := f.Open()
	if err != nil {
		return xhtmlV2Document{}, err
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	data, err := io.ReadAll(r)
	if err != nil {
		return xhtmlV2Document{}, err
	}
	d := xml.NewDecoder(strings.NewReader(xhtmlEntityReplacer.Replace(string(data))))
	extractor := newXHTMLV2Extractor()
	for {
		tok, tokenErr := d.Token()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return xhtmlV2Document{}, fmt.Errorf("malformed XHTML or unsupported encoding: %w", tokenErr)
		}
		extractor.visit(tok)
	}
	extractor.flush()
	return xhtmlV2Document{groups: extractor.groups, title: extractor.title}, nil
}

func (x *xhtmlV2Extractor) visit(tok xml.Token) {
	switch value := tok.(type) {
	case xml.StartElement:
		x.start(value)
	case xml.EndElement:
		x.end(strings.ToLower(value.Name.Local))
	case xml.CharData:
		if x.skip == 0 {
			x.appendText(string(value))
		}
	}
}

func (x *xhtmlV2Extractor) start(element xml.StartElement) {
	name := strings.ToLower(element.Name.Local)
	if x.skip > 0 {
		x.skip++
		return
	}
	if isSkippedElement(name) {
		x.skip = 1
		return
	}
	classes := classNames(element.Attr)
	switch name {
	case "br":
		x.lineBreak()
		return
	case "hr":
		x.flush()
		return
	}
	if isBlock(name) {
		x.beginBlock(name, classes)
		return
	}
	if name == "span" {
		x.openSpans = append(x.openSpans, openSpan{role: inlineRoleFor(classes), start: len(x.text)})
	}
}

func (x *xhtmlV2Extractor) end(name string) {
	if x.skip > 0 {
		x.skip--
		return
	}
	if name == "br" || name == "hr" {
		return
	}
	if isBlock(name) {
		x.endBlock(name)
		return
	}
	if name == "span" {
		x.closeSpan()
	}
}

// beginBlock starts a new group. The previous group is flushed first so a
// nested block never shares text with its parent.
func (x *xhtmlV2Extractor) beginBlock(name string, classes []string) {
	x.flush()
	x.blocks = append(x.blocks, v2Block{
		tag:          name,
		heading:      isHeadingElement(name),
		titleHeading: isHeading(name),
		role:         blockRoleFor(name, classes),
	})
}

// endBlock flushes the group and returns to the enclosing block context.
func (x *xhtmlV2Extractor) endBlock(name string) {
	x.flush()
	for index := len(x.blocks) - 1; index >= 1; index-- {
		if x.blocks[index].tag == name {
			x.blocks = x.blocks[:index]
			return
		}
	}
}

// closeSpan closes the innermost open span. Only a role-bearing span records a
// role span.
func (x *xhtmlV2Extractor) closeSpan() {
	if len(x.openSpans) == 0 {
		return
	}
	open := x.openSpans[len(x.openSpans)-1]
	x.openSpans = x.openSpans[:len(x.openSpans)-1]
	x.recordSpan(open)
}

// recordSpan records one open span that ends at the current text length.
func (x *xhtmlV2Extractor) recordSpan(open openSpan) {
	if open.role != "" {
		x.spans = append(x.spans, relativeRoleSpan{role: open.role, start: open.start, end: len(x.text)})
	}
}

// lineBreak turns one authorial <br/> into a single line break. A second
// adjacent break is a blank line, which ends the group: groups are exactly the
// text between the \n\n separators. Incidental whitespace before a break is not
// part of the line.
func (x *xhtmlV2Extractor) lineBreak() {
	x.trimTrailingSpaces()
	if len(x.text) > 0 && x.text[len(x.text)-1] == '\n' {
		x.flush()
		return
	}
	x.text = append(x.text, '\n')
}

// appendText collapses incidental whitespace to one space, except for the line
// breaks that <br/> already recorded.
func (x *xhtmlV2Extractor) appendText(data string) {
	for _, r := range data {
		if !isSourceSpace(r) {
			x.text = append(x.text, r)
			continue
		}
		if len(x.text) == 0 || x.text[len(x.text)-1] == '\n' || x.text[len(x.text)-1] == ' ' {
			continue
		}
		x.text = append(x.text, ' ')
	}
}

func (x *xhtmlV2Extractor) trimTrailingSpaces() {
	for len(x.text) > 0 && x.text[len(x.text)-1] == ' ' {
		x.text = x.text[:len(x.text)-1]
	}
}

// flush finalizes the current group. Empty groups are dropped: a block boundary
// and a blank line both contribute the same group break, and <hr> adds no text.
// An inline role span that crosses a group boundary is truncated there, because
// valid phrasing markup cannot cross a block.
func (x *xhtmlV2Extractor) flush() {
	if len(x.openSpans) > 0 {
		for _, open := range x.openSpans {
			x.recordSpan(open)
		}
		x.openSpans = nil
	}
	trimmed, leading := trimGroupText(x.text)
	if len(trimmed) == 0 {
		x.text = nil
		x.spans = nil
		return
	}
	spans := trimSpanOffsets(x.spans, leading, len(trimmed))
	if block := x.block(); block.role != "" {
		spans = append(spans, relativeRoleSpan{role: block.role, start: 0, end: len(trimmed)})
	}
	x.groups = append(x.groups, v2Group{text: string(trimmed), spans: normalizeSpans(spans), heading: x.block().heading})
	if x.title == "" && x.block().titleHeading {
		x.title = cleanSpace(string(trimmed))
	}
	x.text = nil
	x.spans = nil
}

func (x *xhtmlV2Extractor) block() v2Block {
	return x.blocks[len(x.blocks)-1]
}

// trimGroupText trims incidental whitespace at both group edges and reports how
// many leading runes were removed so span offsets can be rebased.
func trimGroupText(text []rune) ([]rune, int) {
	start, end := 0, len(text)
	for start < end && isSourceSpace(text[start]) {
		start++
	}
	for end > start && isSourceSpace(text[end-1]) {
		end--
	}
	return text[start:end], start
}

// trimSpanOffsets rebases spans into a trimmed group and drops spans that become
// empty. Spans partly outside the group are clamped.
func trimSpanOffsets(spans []relativeRoleSpan, leading, length int) []relativeRoleSpan {
	trimmed := make([]relativeRoleSpan, 0, len(spans))
	for _, span := range spans {
		start := clampInt(span.start-leading, 0, length)
		end := clampInt(span.end-leading, 0, length)
		if end > start {
			trimmed = append(trimmed, relativeRoleSpan{role: span.role, start: start, end: end})
		}
	}
	return trimmed
}

// normalizeSpans resolves contradictory markup and returns a stable outer-first
// order. A direction that contains or crosses a speaker label is contradictory,
// so the inline speaker label survives and the enclosing direction does not.
// A direction nested inside a speaker label remains valid.
func normalizeSpans(spans []relativeRoleSpan) []relativeRoleSpan {
	speakers := speakerSpans(spans)
	out := make([]relativeRoleSpan, 0, len(spans))
	seen := make(map[relativeRoleSpan]struct{}, len(spans))
	for _, span := range spans {
		if !keepSpan(span, speakers, seen) {
			continue
		}
		seen[span] = struct{}{}
		out = append(out, span)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].start != out[j].start {
			return out[i].start < out[j].start
		}
		if out[i].end != out[j].end {
			return out[i].end > out[j].end
		}
		return out[i].role < out[j].role
	})
	return out
}

// speakerSpans returns the non-empty speaker spans used to resolve
// contradictory direction markup.
func speakerSpans(spans []relativeRoleSpan) []relativeRoleSpan {
	var speakers []relativeRoleSpan
	for _, span := range spans {
		if span.role == domain.SourceStructureRoleSpeaker && span.end > span.start {
			speakers = append(speakers, span)
		}
	}
	return speakers
}

// keepSpan drops empty spans, duplicate spans and directions that contradict a
// speaker label.
func keepSpan(span relativeRoleSpan, speakers []relativeRoleSpan, seen map[relativeRoleSpan]struct{}) bool {
	if span.end <= span.start {
		return false
	}
	if span.role == domain.SourceStructureRoleDirection && contradictsSpeaker(span, speakers) {
		return false
	}
	_, exists := seen[span]
	return !exists
}

// contradictsSpeaker reports whether a direction conflicts with a speaker label.
// A direction is valid only when every overlapping speaker label strictly
// contains it.
func contradictsSpeaker(direction relativeRoleSpan, speakers []relativeRoleSpan) bool {
	for _, speaker := range speakers {
		if speaker.start >= direction.end || direction.start >= speaker.end {
			continue
		}
		contains := speaker.start <= direction.start && speaker.end >= direction.end && (speaker.start < direction.start || speaker.end > direction.end)
		if !contains {
			return true
		}
	}
	return false
}

// blockRoleFor resolves the block role from the versioned convention set.
func blockRoleFor(tag string, classes []string) domain.SourceStructureRole {
	if tag != "p" {
		return ""
	}
	return exclusiveRole(hasClass(classes, "speaker"), hasDirectionClass(classes))
}

// inlineRoleFor resolves an inline role from the versioned convention set.
func inlineRoleFor(classes []string) domain.SourceStructureRole {
	return exclusiveRole(hasClass(classes, "speaker"), hasClass(classes, "sd"))
}

// exclusiveRole resolves exactly one of two contradictory role signals. Both or
// neither establish no role.
func exclusiveRole(speaker, direction bool) domain.SourceStructureRole {
	if speaker == direction {
		return ""
	}
	if speaker {
		return domain.SourceStructureRoleSpeaker
	}
	return domain.SourceStructureRoleDirection
}

func hasDirectionClass(classes []string) bool {
	for _, class := range classes {
		switch class {
		case "entrance", "sdcenter", "sdsetting", "dirlong", "dirlongind":
			return true
		}
	}
	return false
}

func hasClass(classes []string, want string) bool {
	return slices.Contains(classes, want)
}

func classNames(attrs []xml.Attr) []string {
	for _, attr := range attrs {
		if attr.Name.Local == "class" {
			return strings.Fields(attr.Value)
		}
	}
	return nil
}

// isHeadingElement reports whether the block is any heading element. Every
// heading group records a heading-reset offset.
func isHeadingElement(name string) bool {
	switch name {
	case "h1", "h2", "h3", "h4", "h5", "h6":
		return true
	}
	return false
}

func isSourceSpace(r rune) bool {
	return unicode.IsSpace(r)
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
