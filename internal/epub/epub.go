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
	"slices"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
)

const mediaType = "application/epub+zip"

var ErrInvalidEPUB = errors.New("epub: invalid EPUB")

// xhtmlEntityReplacer covers the HTML named entities most commonly emitted by
// ebook authoring tools. Numeric references remain XML-compatible, while the
// five XML entities are deliberately absent so valid text such as &amp; is not
// decoded and then mistaken for markup.
var xhtmlEntityReplacer = strings.NewReplacer(
	"&nbsp;", "&#160;", "&iexcl;", "&#161;", "&cent;", "&#162;", "&pound;", "&#163;",
	"&yen;", "&#165;", "&copy;", "&#169;", "&laquo;", "&#171;", "&reg;", "&#174;",
	"&deg;", "&#176;", "&plusmn;", "&#177;", "&middot;", "&#183;", "&raquo;", "&#187;",
	"&iquest;", "&#191;", "&Agrave;", "&#192;", "&Aacute;", "&#193;", "&Acirc;", "&#194;",
	"&Atilde;", "&#195;", "&Auml;", "&#196;", "&Aring;", "&#197;", "&AElig;", "&#198;",
	"&Ccedil;", "&#199;", "&Egrave;", "&#200;", "&Eacute;", "&#201;", "&Ecirc;", "&#202;",
	"&Euml;", "&#203;", "&Igrave;", "&#204;", "&Iacute;", "&#205;", "&Icirc;", "&#206;",
	"&Iuml;", "&#207;", "&Ntilde;", "&#209;", "&Ograve;", "&#210;", "&Oacute;", "&#211;",
	"&Ocirc;", "&#212;", "&Otilde;", "&#213;", "&Ouml;", "&#214;", "&Oslash;", "&#216;",
	"&Ugrave;", "&#217;", "&Uacute;", "&#218;", "&Ucirc;", "&#219;", "&Uuml;", "&#220;",
	"&Yacute;", "&#221;", "&szlig;", "&#223;", "&agrave;", "&#224;", "&aacute;", "&#225;",
	"&acirc;", "&#226;", "&atilde;", "&#227;", "&auml;", "&#228;", "&aring;", "&#229;",
	"&aelig;", "&#230;", "&ccedil;", "&#231;", "&egrave;", "&#232;", "&eacute;", "&#233;",
	"&ecirc;", "&#234;", "&euml;", "&#235;", "&igrave;", "&#236;", "&iacute;", "&#237;",
	"&icirc;", "&#238;", "&iuml;", "&#239;", "&ntilde;", "&#241;", "&ograve;", "&#242;",
	"&oacute;", "&#243;", "&ocirc;", "&#244;", "&otilde;", "&#245;", "&ouml;", "&#246;",
	"&oslash;", "&#248;", "&ugrave;", "&#249;", "&uacute;", "&#250;", "&ucirc;", "&#251;",
	"&uuml;", "&#252;", "&yacute;", "&#253;", "&yuml;", "&#255;", "&shy;", "&#173;",
	"&ndash;", "&#8211;", "&mdash;", "&#8212;", "&lsquo;", "&#8216;", "&rsquo;", "&#8217;",
	"&ldquo;", "&#8220;", "&rdquo;", "&#8221;", "&bull;", "&#8226;", "&hellip;", "&#8230;",
	"&trade;", "&#8482;", "&euro;", "&#8364;",
)

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

// ExtractedBook is the compatibility projection of one extraction: ordered
// readable units, chapter locations and the joined full text.
type ExtractedBook struct {
	Title            string         `json:"title"`
	SourceIdentifier string         `json:"source_identifier"`
	FullText         string         `json:"full_text"`
	Chapters         []Chapter      `json:"chapters"`
	ExtractedUnits   ExtractedUnits `json:"extracted_units"`
}

// Extraction is one EPUB extracted under a specific snapshot contract. The
// default writer remains v1; v2 additionally records source structure.
type Extraction struct {
	Book      ExtractedBook
	Structure domain.SnapshotStructure
}

type container struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}
type manifestItem struct {
	ID         string `xml:"id,attr"`
	Href       string `xml:"href,attr"`
	MediaType  string `xml:"media-type,attr"`
	Properties string `xml:"properties,attr"`
}

type packageDoc struct {
	Metadata struct {
		Titles      []string `xml:"title"`
		Identifiers []string `xml:"identifier"`
	} `xml:"metadata"`
	Manifest []manifestItem `xml:"manifest>item"`
	Spine    []spineItem    `xml:"spine>itemref"`
}

// spineItem is one package spine reference in reading order.
type spineItem struct {
	IDRef  string `xml:"idref,attr"`
	Linear string `xml:"linear,attr"`
}

// Extract validates an EPUB byte stream and extracts XHTML documents in spine
// order under the current v1 writer contract. v1 records text only.
func Extract(data []byte) (ExtractedBook, error) {
	extraction, err := extractContract(data, ExtractedUnitsSchemaVersion)
	return extraction.Book, err
}

// ExtractV2 extracts under the v2 contract without becoming the default writer.
// It preserves authorial source lines, line groups and markup-declared roles
// while keeping the exact unit text as the only offset basis.
func ExtractV2(data []byte) (Extraction, error) {
	return extractContract(data, domain.ExtractedUnitsSchemaVersionV2)
}

// extractContract validates an EPUB byte stream and extracts it under one
// snapshot contract.
func extractContract(data []byte, schemaVersion int) (Extraction, error) {
	files, err := openEPUB(data)
	if err != nil {
		return Extraction{}, err
	}
	pkg, opfPath, err := readPackage(files)
	if err != nil {
		return Extraction{}, err
	}
	book, err := newExtractedBook(pkg, schemaVersion)
	if err != nil {
		return Extraction{}, err
	}
	items, err := indexManifest(pkg.Manifest)
	if err != nil {
		return Extraction{}, err
	}
	structure, err := extractSpine(&book, files, opfPath, items, pkg, schemaVersion)
	if err != nil {
		return Extraction{}, err
	}
	if len(book.Chapters) == 0 {
		return Extraction{}, invalid("package spine contains no readable XHTML content", nil)
	}
	if err := book.ExtractedUnits.ValidateOffsets(book.FullText); err != nil {
		return Extraction{}, invalid("validate extracted units", err)
	}
	if err := structure.Validate(schemaVersion, book.ExtractedUnits.Units); err != nil {
		return Extraction{}, invalid("validate source structure", err)
	}
	return Extraction{Book: book, Structure: structure}, nil
}

// openEPUB reads the ZIP container and checks its required mimetype entry,
// returning the container's files keyed by cleaned path.
func openEPUB(data []byte) (map[string]*zip.File, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, invalid("open ZIP container", err)
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[path.Clean(f.Name)] = f
	}
	f := files["mimetype"]
	if f == nil {
		return nil, invalid("missing mimetype file", nil)
	}
	value, err := readFile(f)
	if err != nil {
		return nil, invalid("read mimetype file", err)
	}
	if strings.TrimSpace(string(value)) != mediaType {
		return nil, invalid("mimetype is not "+mediaType, nil)
	}
	return files, nil
}

// readPackage locates the package document through META-INF/container.xml and
// decodes it.
func readPackage(files map[string]*zip.File) (packageDoc, string, error) {
	var c container
	if err := decodeXMLFile(files, "META-INF/container.xml", &c); err != nil {
		return packageDoc{}, "", invalid("read META-INF/container.xml", err)
	}
	if len(c.Rootfiles) == 0 || strings.TrimSpace(c.Rootfiles[0].FullPath) == "" {
		return packageDoc{}, "", invalid("container.xml has no package rootfile", nil)
	}
	opfPath := path.Clean(c.Rootfiles[0].FullPath)
	var pkg packageDoc
	if err := decodeXMLFile(files, opfPath, &pkg); err != nil {
		return packageDoc{}, "", invalid("read package document "+opfPath, err)
	}
	return pkg, opfPath, nil
}

// newExtractedBook seeds the result with the package metadata and its required
// identifier, initialising the extracted-unit envelope for the contract.
func newExtractedBook(pkg packageDoc, schemaVersion int) (ExtractedBook, error) {
	book := ExtractedBook{
		Title:            firstNonBlank(pkg.Metadata.Titles),
		SourceIdentifier: firstNonBlank(pkg.Metadata.Identifiers),
		ExtractedUnits: ExtractedUnits{
			SchemaVersion: schemaVersion,
			Units:         []ExtractedUnit{},
		},
	}
	if book.SourceIdentifier == "" {
		return ExtractedBook{}, invalid("package metadata has no identifier", nil)
	}
	return book, nil
}

// indexManifest keys the manifest by ID, rejecting blank and duplicate IDs.
func indexManifest(manifest []manifestItem) (map[string]manifestItem, error) {
	items := make(map[string]manifestItem, len(manifest))
	for _, item := range manifest {
		if strings.TrimSpace(item.ID) == "" {
			return nil, invalid("manifest item has no ID", nil)
		}
		if _, exists := items[item.ID]; exists {
			return nil, invalid("duplicate manifest item ID "+item.ID, nil)
		}
		items[item.ID] = item
	}
	return items, nil
}

// extractSpine walks the package spine in order, appending one unit per
// readable XHTML document to book and setting book.FullText. Under v2 it also
// records each unit's role spans and heading resets.
func extractSpine(book *ExtractedBook, files map[string]*zip.File, opfPath string, items map[string]manifestItem, pkg packageDoc, schemaVersion int) (domain.SnapshotStructure, error) {
	base := path.Dir(opfPath)
	navigationLabels, landmarkTypes := navigationMetadata(files, base, pkg.Manifest)
	extractor := spineExtractor{
		files:            files,
		base:             base,
		opfPath:          opfPath,
		items:            items,
		navigationLabels: navigationLabels,
		landmarkTypes:    landmarkTypes,
		schemaVersion:    schemaVersion,
	}
	if schemaVersion >= domain.ExtractedUnitsSchemaVersionV2 {
		extractor.structure = domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet}
	}
	for spineIndex, ref := range pkg.Spine {
		unit, include, err := extractor.readDocument(uint64(spineIndex), ref)
		if err != nil {
			return domain.SnapshotStructure{}, err
		}
		if !include {
			continue
		}
		extractor.appendUnit(book, unit)
	}
	book.FullText = extractor.out.String()
	return extractor.structure, nil
}

// spineExtractor turns spine references into ordered units, keeping the running
// FullText offset so separators and unit spans stay consistent.
type spineExtractor struct {
	files            map[string]*zip.File
	base             string
	opfPath          string
	items            map[string]manifestItem
	navigationLabels map[string][]string
	landmarkTypes    map[string][]string
	schemaVersion    int
	structure        domain.SnapshotStructure
	out              strings.Builder
	offset           uint64
}

// readDocument extracts a single spine reference. It reports include=false for
// references the reader must skip, including non-linear, navigation, and empty
// documents.
func (x *spineExtractor) readDocument(spineIndex uint64, ref spineItem) (ExtractedUnit, bool, error) {
	if strings.TrimSpace(ref.IDRef) == "" {
		return ExtractedUnit{}, false, invalid("spine item has no idref", nil)
	}
	item, ok := x.items[ref.IDRef]
	if !ok {
		return ExtractedUnit{}, false, invalid("spine references missing manifest item "+ref.IDRef, nil)
	}
	if strings.EqualFold(ref.Linear, "no") {
		return ExtractedUnit{}, false, nil
	}
	if item.MediaType != "application/xhtml+xml" || hasWord(item.Properties, "nav") {
		return ExtractedUnit{}, false, nil
	}
	name, err := resolveResourcePath(x.base, item.Href)
	if err != nil {
		return ExtractedUnit{}, false, invalid("resolve manifest resource "+item.Href, err)
	}
	text, title, groups, err := x.extractDocument(name)
	if err != nil {
		return ExtractedUnit{}, false, invalid("extract spine document "+name, err)
	}
	if text == "" {
		return ExtractedUnit{}, false, nil
	}
	titleSource := UnitTitleHeading
	if title == "" {
		title = ref.IDRef
		titleSource = UnitTitleManifestID
	}
	unit := ExtractedUnit{
		ID:               UnitID(spineIndex, ref.IDRef),
		SpineIndex:       spineIndex,
		Title:            title,
		TitleSource:      titleSource,
		Text:             text,
		PackagePath:      x.opfPath,
		ManifestID:       ref.IDRef,
		SourceHref:       item.Href,
		ResolvedHref:     name,
		MediaType:        item.MediaType,
		Properties:       append([]string{}, strings.Fields(item.Properties)...),
		Linear:           true,
		NavigationLabels: append([]string{}, x.navigationLabels[name]...),
		LandmarkTypes:    append([]string{}, x.landmarkTypes[name]...),
	}
	if x.schemaVersion >= domain.ExtractedUnitsSchemaVersionV2 {
		if err := x.recordUnitStructure(unit.ID, groups); err != nil {
			return ExtractedUnit{}, false, invalid("record source structure", err)
		}
	}
	return unit, true, nil
}

// extractDocument extracts one XHTML resource under the active contract. v2
// also returns the groups that carry structure offsets relative to the unit.
func (x *spineExtractor) extractDocument(name string) (string, string, []v2Group, error) {
	if x.schemaVersion < domain.ExtractedUnitsSchemaVersionV2 {
		text, title, err := extractXHTML(x.files[name])
		return text, title, nil, err
	}
	document, err := extractXHTMLV2(x.files[name])
	if err != nil {
		return "", "", nil, err
	}
	return documentText(document.groups), document.title, document.groups, nil
}

// recordUnitStructure maps group-relative offsets into the exact unit text.
func (x *spineExtractor) recordUnitStructure(unitID string, groups []v2Group) error {
	offset := uint64(0)
	for index, group := range groups {
		if index > 0 {
			offset += uint64(len([]rune(groupSeparator)))
		}
		if group.heading {
			x.structure.HeadingResets = append(x.structure.HeadingResets, domain.UnitHeadingReset{UnitID: unitID, Offset: offset})
		}
		for _, span := range group.spans {
			start, err := checked.Uint64FromInt(span.start)
			if err != nil {
				return err
			}
			end, err := checked.Uint64FromInt(span.end)
			if err != nil {
				return err
			}
			x.structure.RoleSpans = append(x.structure.RoleSpans, domain.UnitRoleSpan{
				UnitID: unitID, Role: span.role,
				StartOffset: offset + start, EndOffset: offset + end,
			})
		}
		offset += uint64(len([]rune(group.text)))
	}
	return nil
}

// documentText joins extracted groups with the established group separator.
func documentText(groups []v2Group) string {
	var text strings.Builder
	for index, group := range groups {
		if index > 0 {
			text.WriteString(groupSeparator)
		}
		text.WriteString(group.text)
	}
	return text.String()
}

// appendUnit writes one included document to the FullText builder, assigning
// its span and its compatibility chapter.
func (x *spineExtractor) appendUnit(book *ExtractedBook, unit ExtractedUnit) {
	if x.out.Len() > 0 {
		x.out.WriteString("\n\n")
		x.offset += 2
	}
	unit.Order = uint64(len(book.ExtractedUnits.Units))
	unit.StartOffset = x.offset
	x.out.WriteString(unit.Text)
	x.offset += uint64(len([]rune(unit.Text)))
	unit.EndOffset = x.offset
	book.ExtractedUnits.Units = append(book.ExtractedUnits.Units, unit)
	book.Chapters = append(book.Chapters, Chapter{ID: unit.ManifestID, Title: unit.Title, Location: Location{SourceDocumentID: book.SourceIdentifier, Chapter: unit.Title, Section: unit.Title, StartOffset: unit.StartOffset, EndOffset: unit.EndOffset}})
}

func resolveResourcePath(base, href string) (string, error) {
	resource := href
	if i := strings.IndexByte(resource, '#'); i >= 0 {
		resource = resource[:i]
	}
	if resource == "" || path.IsAbs(resource) {
		return "", errors.New("unsafe or blank resource path")
	}
	name := path.Clean(path.Join(base, resource))
	if name == "." || name == ".." || strings.HasPrefix(name, "../") {
		return "", errors.New("resource escapes EPUB root")
	}
	return name, nil
}

func navigationMetadata(files map[string]*zip.File, base string, manifest []manifestItem) (map[string][]string, map[string][]string) {
	labels := make(map[string][]string)
	landmarks := make(map[string][]string)
	for _, item := range manifest {
		if item.MediaType != "application/xhtml+xml" || !hasWord(item.Properties, "nav") {
			continue
		}
		name, err := resolveResourcePath(base, item.Href)
		if err != nil || files[name] == nil {
			continue
		}
		documentLabels := make(map[string][]string)
		documentLandmarks := make(map[string][]string)
		if err := extractNavigation(files[name], path.Dir(name), documentLabels, documentLandmarks); err != nil {
			continue
		}
		for resource, values := range documentLabels {
			labels[resource] = append(labels[resource], values...)
		}
		for resource, values := range documentLandmarks {
			landmarks[resource] = append(landmarks[resource], values...)
		}
	}
	return labels, landmarks
}

func extractNavigation(f *zip.File, base string, labels, landmarks map[string][]string) error {
	data, err := readFile(f)
	if err != nil {
		return err
	}
	collector := navigationCollector{
		base:      base,
		labels:    labels,
		landmarks: landmarks,
	}
	return traverseNavigation(data, &collector)
}

type navigationCollector struct {
	base      string
	labels    map[string][]string
	landmarks map[string][]string
	navs      []navigationScope
	anchor    *navigationAnchor
}

type navigationScope struct {
	types []string
}

type navigationAnchor struct {
	href     string
	types    []string
	navTypes []string
	depth    int
	label    strings.Builder
}

func traverseNavigation(data []byte, collector *navigationCollector) error {
	d := xml.NewDecoder(strings.NewReader(xhtmlEntityReplacer.Replace(string(data))))
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		collector.visit(tok)
	}
}

func (c *navigationCollector) visit(tok xml.Token) {
	switch value := tok.(type) {
	case xml.StartElement:
		c.start(value)
	case xml.CharData:
		if c.anchor != nil {
			c.anchor.label.WriteString(string(value))
		}
	case xml.EndElement:
		c.end(value)
	}
}

func (c *navigationCollector) start(element xml.StartElement) {
	name := strings.ToLower(element.Name.Local)
	if name == "nav" {
		c.navs = append(c.navs, navigationScope{types: strings.Fields(attribute(element.Attr, "type"))})
	}
	if c.anchor != nil {
		c.anchor.depth++
		return
	}
	if name != "a" || len(c.navs) == 0 {
		return
	}
	navTypes := c.navs[len(c.navs)-1].types
	c.anchor = &navigationAnchor{
		href:     attribute(element.Attr, "href"),
		types:    strings.Fields(attribute(element.Attr, "type")),
		navTypes: append([]string{}, navTypes...),
		depth:    1,
	}
}

func (c *navigationCollector) end(element xml.EndElement) {
	if c.anchor != nil {
		c.anchor.depth--
		if c.anchor.depth == 0 {
			c.collectAnchor()
			c.anchor = nil
		}
		return
	}
	if strings.EqualFold(element.Name.Local, "nav") && len(c.navs) > 0 {
		c.navs = c.navs[:len(c.navs)-1]
	}
}

func (c *navigationCollector) collectAnchor() {
	anchor := c.anchor
	resource, err := resolveResourcePath(c.base, anchor.href)
	if err != nil {
		return
	}
	if label := cleanSpace(anchor.label.String()); label != "" {
		c.labels[resource] = append(c.labels[resource], label)
	}
	if containsWord(anchor.navTypes, "landmarks") {
		c.landmarks[resource] = append(c.landmarks[resource], anchor.types...)
	}
}

func attribute(attrs []xml.Attr, name string) string {
	for _, attr := range attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

func containsWord(words []string, word string) bool {
	return slices.Contains(words, word)
}

func readFile(f *zip.File) (data []byte, err error) {
	r, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	return io.ReadAll(r)
}

func decodeXMLFile(files map[string]*zip.File, name string, dst any) (err error) {
	f := files[path.Clean(name)]
	if f == nil {
		return fmt.Errorf("missing %s", name)
	}
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	d := xml.NewDecoder(r)
	if err := d.Decode(dst); err != nil {
		return fmt.Errorf("malformed or unsupported encoding: %w", err)
	}
	return nil
}

func extractXHTML(f *zip.File) (text string, heading string, err error) {
	if f == nil {
		return "", "", errors.New("manifest resource is missing")
	}
	r, err := f.Open()
	if err != nil {
		return "", "", err
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	data, err := io.ReadAll(r)
	if err != nil {
		return "", "", err
	}
	d := xml.NewDecoder(strings.NewReader(xhtmlEntityReplacer.Replace(string(data))))
	extractor := xhtmlExtractor{}
	for {
		tok, tokenErr := d.Token()
		if errors.Is(tokenErr, io.EOF) {
			break
		}
		if tokenErr != nil {
			return "", "", fmt.Errorf("malformed XHTML or unsupported encoding: %w", tokenErr)
		}
		extractor.visit(tok)
	}
	extractor.flush()
	return strings.Join(extractor.blocks, "\n\n"), extractor.title, nil
}

// xhtmlExtractor accumulates the block text and first heading of one document
// while tracking elements whose content must be skipped.
type xhtmlExtractor struct {
	blocks  []string
	current strings.Builder
	title   string
	skip    int
}

func (x *xhtmlExtractor) visit(tok xml.Token) {
	switch value := tok.(type) {
	case xml.StartElement:
		x.start(strings.ToLower(value.Name.Local))
	case xml.EndElement:
		x.end(strings.ToLower(value.Name.Local))
	case xml.CharData:
		if x.skip == 0 {
			x.current.Write([]byte(value))
		}
	}
}

func (x *xhtmlExtractor) start(name string) {
	if x.skip > 0 {
		x.skip++
		return
	}
	if isSkippedElement(name) {
		x.skip = 1
		return
	}
	if isBlock(name) {
		x.flush()
	}
}

func (x *xhtmlExtractor) end(name string) {
	if x.skip > 0 {
		x.skip--
		return
	}
	if !isBlock(name) {
		return
	}
	candidate := cleanSpace(x.current.String())
	if x.title == "" && isHeading(name) {
		x.title = candidate
	}
	x.flush()
}

func (x *xhtmlExtractor) flush() {
	if block := cleanSpace(x.current.String()); block != "" {
		x.blocks = append(x.blocks, block)
	}
	x.current.Reset()
}

func isSkippedElement(s string) bool {
	switch s {
	case "script", "style", "nav", "head":
		return true
	}
	return false
}

func isHeading(s string) bool { return s == "h1" || s == "h2" }

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
	return slices.Contains(strings.Fields(s), word)
}
func invalid(message string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", ErrInvalidEPUB, message)
	}
	return fmt.Errorf("%w: %s: %w", ErrInvalidEPUB, message, err)
}

func MediaType() string { return mediaType }
