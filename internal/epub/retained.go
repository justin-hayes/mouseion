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

	"github.com/justin-hayes/mouseion/internal/checked"
)

// RetainedSlimmingVersion identifies the rules that turn a Catalog file into a
// Retained EPUB. A change to what is removed gets a new version.
const RetainedSlimmingVersion = 1

// ErrRetainedSizeLimit reports that the Retained EPUB would exceed the
// acquisition's in-memory and storage bound. Slimming stops as soon as the
// output crosses the limit instead of materialising the whole file.
var ErrRetainedSizeLimit = errors.New("epub: Retained EPUB exceeds the size limit")

// RemovedResource is one manifest item and container entry that slimming
// removed from a Catalog file.
type RemovedResource struct {
	ID        string `json:"id"`
	Href      string `json:"href"`
	MediaType string `json:"media_type"`
	Size      int64  `json:"size"`
}

// RetainedEPUB is what Mouseion stores for a content revision: a Catalog file
// without the images, fonts, audio and video that extraction never reads.
type RetainedEPUB struct {
	Content         []byte
	SlimmingVersion int
	Removed         []RemovedResource
}

// Retain derives the Retained EPUB from a Catalog file available through
// random access. The result is a pure function of the Catalog file's bytes and
// the current slimming rules: a Catalog file with nothing to remove is
// returned byte-for-byte. A positive limit bounds the output and its peak
// memory; a Retained EPUB over it fails with ErrRetainedSizeLimit.
func Retain(catalog io.ReaderAt, size, limit int64) (RetainedEPUB, error) {
	retained := RetainedEPUB{SlimmingVersion: RetainedSlimmingVersion}
	container, err := zip.NewReader(catalog, size)
	if err != nil {
		return retained, invalid("open ZIP container", err)
	}
	files, err := catalogFiles(container)
	if err != nil {
		return retained, err
	}
	if err := validateMimetype(files); err != nil {
		return retained, err
	}
	opfPath, opfFile, err := locatePackageDocument(files)
	if err != nil {
		return retained, err
	}
	opfData, err := readFile(opfFile)
	if err != nil {
		return retained, invalid("read package document "+opfPath, err)
	}
	layout, err := parsePackageLayout(opfData)
	if err != nil {
		return retained, err
	}
	removals, report, err := planRemovals(files, opfPath, layout)
	if err != nil {
		return retained, err
	}
	if len(report) == 0 {
		if limit > 0 && size > limit {
			return retained, ErrRetainedSizeLimit
		}
		content, err := readCatalogBytes(catalog, size)
		if err != nil {
			return retained, err
		}
		retained.Content = content
		return retained, nil
	}
	rewritten, err := cutManifestItems(opfData, removals)
	if err != nil {
		return retained, err
	}
	content, err := writeRetained(container, opfPath, rewritten, removals, limit)
	if err != nil {
		return retained, err
	}
	retained.Content = content
	retained.Removed = report
	return retained, nil
}

// catalogFiles indexes the container by cleaned name. Duplicate names would
// make a manifest-driven removal ambiguous, so they are refused.
func catalogFiles(container *zip.Reader) (map[string]*zip.File, error) {
	files := make(map[string]*zip.File, len(container.File))
	for _, file := range container.File {
		name := path.Clean(file.Name)
		if _, exists := files[name]; exists {
			return nil, invalid("duplicate container entry "+name, nil)
		}
		files[name] = file
	}
	return files, nil
}

func validateMimetype(files map[string]*zip.File) error {
	file := files["mimetype"]
	if file == nil {
		return invalid("missing mimetype file", nil)
	}
	value, err := readFile(file)
	if err != nil {
		return invalid("read mimetype file", err)
	}
	if strings.TrimSpace(string(value)) != mediaType {
		return invalid("mimetype is not "+mediaType, nil)
	}
	return nil
}

// locatePackageDocument resolves the single rootfile from
// META-INF/container.xml.
func locatePackageDocument(files map[string]*zip.File) (string, *zip.File, error) {
	var containerDoc container
	if err := decodeXMLFile(files, "META-INF/container.xml", &containerDoc); err != nil {
		return "", nil, invalid("read META-INF/container.xml", err)
	}
	if len(containerDoc.Rootfiles) != 1 || strings.TrimSpace(containerDoc.Rootfiles[0].FullPath) == "" {
		return "", nil, invalid("container.xml must name exactly one package rootfile", nil)
	}
	opfPath := path.Clean(containerDoc.Rootfiles[0].FullPath)
	file := files[opfPath]
	if file == nil {
		return "", nil, invalid("missing package document "+opfPath, nil)
	}
	return opfPath, file, nil
}

func readCatalogBytes(catalog io.ReaderAt, size int64) ([]byte, error) {
	content, err := io.ReadAll(io.NewSectionReader(catalog, 0, size))
	if err != nil {
		return nil, fmt.Errorf("epub: read Catalog file: %w", err)
	}
	return content, nil
}

// packageManifestItem is one manifest entry together with the exact byte range
// of its <item> element in the package document.
type packageManifestItem struct {
	ID         string
	Href       string
	MediaType  string
	Properties string
	start      int64
	end        int64
}

type packageLayout struct {
	items     []packageManifestItem
	spineRefs map[string]struct{}
	spineTOC  string
}

// packageParser accumulates the package document's manifest items and spine
// references while tracking element depth and exact token offsets.
type packageParser struct {
	layout        packageLayout
	depth         int
	manifests     int
	spines        int
	rootSeen      bool
	manifestDepth int
	spineDepth    int
	item          *packageManifestItem
	itemDepth     int
}

// parsePackageLayout reads the package document once, recording manifest item
// byte ranges and spine references. It refuses a document whose shape does not
// identify one manifest and one spine unambiguously.
func parsePackageLayout(data []byte) (packageLayout, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	parser := packageParser{layout: packageLayout{spineRefs: make(map[string]struct{})}}
	for {
		start := decoder.InputOffset()
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return packageLayout{}, invalid("parse package document", err)
		}
		if err := parser.visit(token, start, decoder.InputOffset()); err != nil {
			return packageLayout{}, err
		}
	}
	return parser.finish()
}

func (p *packageParser) visit(token xml.Token, start, end int64) error {
	switch value := token.(type) {
	case xml.StartElement:
		return p.start(value, start)
	case xml.EndElement:
		p.end(value, end)
	}
	return nil
}

func (p *packageParser) start(element xml.StartElement, offset int64) error {
	p.depth++
	local := strings.ToLower(element.Name.Local)
	if p.depth == 1 {
		if local != "package" {
			return invalid("package document root is not package", nil)
		}
		p.rootSeen = true
	}
	switch {
	case local == "manifest":
		p.manifests++
		p.manifestDepth = p.depth
	case local == "spine":
		p.spines++
		p.spineDepth = p.depth
		p.layout.spineTOC = strings.TrimSpace(attribute(element.Attr, "toc"))
	case local == "itemref" && p.spineDepth > 0 && p.depth == p.spineDepth+1:
		if idref := strings.TrimSpace(attribute(element.Attr, "idref")); idref != "" {
			p.layout.spineRefs[idref] = struct{}{}
		}
	case local == "item" && p.item != nil:
		return invalid("nested package manifest item", nil)
	case local == "item" && p.manifestDepth > 0 && p.depth == p.manifestDepth+1:
		p.item = &packageManifestItem{
			ID:         attribute(element.Attr, "id"),
			Href:       attribute(element.Attr, "href"),
			MediaType:  attribute(element.Attr, "media-type"),
			Properties: attribute(element.Attr, "properties"),
			start:      offset,
		}
		p.itemDepth = p.depth
	}
	return nil
}

func (p *packageParser) end(element xml.EndElement, offset int64) {
	if p.item != nil && p.depth == p.itemDepth {
		p.item.end = offset
		p.layout.items = append(p.layout.items, *p.item)
		p.item = nil
	}
	if p.manifests > 0 && p.depth == p.manifestDepth && strings.EqualFold(element.Name.Local, "manifest") {
		p.manifestDepth = 0
	}
	if p.spines > 0 && p.depth == p.spineDepth && strings.EqualFold(element.Name.Local, "spine") {
		p.spineDepth = 0
	}
	p.depth--
}

func (p *packageParser) finish() (packageLayout, error) {
	if !p.rootSeen {
		return packageLayout{}, invalid("package document has no package element", nil)
	}
	if p.manifests != 1 || p.spines != 1 {
		return packageLayout{}, invalid("package document must have exactly one manifest and one spine", nil)
	}
	seen := make(map[string]struct{}, len(p.layout.items))
	for _, entry := range p.layout.items {
		if strings.TrimSpace(entry.ID) == "" {
			return packageLayout{}, invalid("manifest item has no ID", nil)
		}
		if _, duplicate := seen[entry.ID]; duplicate {
			return packageLayout{}, invalid("duplicate manifest item ID "+entry.ID, nil)
		}
		seen[entry.ID] = struct{}{}
	}
	return p.layout, nil
}

// removableMediaType reports whether the media type names a resource Mouseion
// never reads.
func removableMediaType(value string) bool {
	media := normalizeMediaType(value)
	switch {
	case strings.HasPrefix(media, "image/"),
		strings.HasPrefix(media, "audio/"),
		strings.HasPrefix(media, "video/"),
		strings.HasPrefix(media, "font/"),
		strings.HasPrefix(media, "application/font-"),
		strings.HasPrefix(media, "application/x-font-"),
		media == "application/vnd.ms-opentype":
		return true
	}
	return false
}

func normalizeMediaType(value string) string {
	media, _, _ := strings.Cut(value, ";")
	return strings.ToLower(strings.TrimSpace(media))
}

// exemptFromSlimming applies the resources that stay whatever their media
// type: mimetype, META-INF, package documents, spine items, navigation and
// NCX, and stylesheets.
func exemptFromSlimming(item packageManifestItem, resolved, opfPath string, layout packageLayout) bool {
	media := normalizeMediaType(item.MediaType)
	if resolved == "mimetype" || resolved == opfPath || strings.HasPrefix(resolved, "META-INF/") {
		return true
	}
	if media == "application/oebps-package+xml" || strings.EqualFold(path.Ext(resolved), ".opf") {
		return true
	}
	if _, inSpine := layout.spineRefs[item.ID]; inSpine {
		return true
	}
	if hasNavProperty(item.Properties) {
		return true
	}
	if media == "application/x-dtbncx+xml" || (layout.spineTOC != "" && item.ID == layout.spineTOC) {
		return true
	}
	return media == "text/css"
}

// plannedRemoval is one removable manifest item and the container entry it
// names.
type plannedRemoval struct {
	item     packageManifestItem
	resolved string
}

// planRemovals resolves removable manifest entries and records the exact
// container entries they name.
func planRemovals(files map[string]*zip.File, opfPath string, layout packageLayout) ([]plannedRemoval, []RemovedResource, error) {
	base := path.Dir(opfPath)
	var removals []plannedRemoval
	var report []RemovedResource
	for _, item := range layout.items {
		if !removableMediaType(item.MediaType) {
			continue
		}
		resolved, err := resolveResourcePath(base, item.Href)
		if err != nil {
			return nil, nil, invalid("resolve removable manifest item "+item.ID, err)
		}
		if exemptFromSlimming(item, resolved, opfPath, layout) {
			continue
		}
		var size int64
		if file := files[resolved]; file != nil {
			size, err = checked.Int64FromUint64(file.UncompressedSize64)
			if err != nil {
				return nil, nil, invalid("manifest resource size is out of range", err)
			}
		}
		removals = append(removals, plannedRemoval{item: item, resolved: resolved})
		report = append(report, RemovedResource{ID: item.ID, Href: item.Href, MediaType: item.MediaType, Size: size})
	}
	return removals, report, nil
}

// cutManifestItems removes only the recorded <item> byte ranges from the
// package document, leaving every other byte untouched.
func cutManifestItems(opf []byte, removals []plannedRemoval) ([]byte, error) {
	var rewritten bytes.Buffer
	last := int64(0)
	for _, removal := range removals {
		item := removal.item
		if item.start < last || item.end < item.start || item.end > int64(len(opf)) {
			return nil, invalid("package manifest item has an invalid byte range", nil)
		}
		rewritten.Write(opf[last:item.start])
		last = item.end
	}
	rewritten.Write(opf[last:])
	return rewritten.Bytes(), nil
}

// writeRetained copies every kept entry raw and in its original order, except
// directory entries, which are written empty. The rewritten package document
// keeps its entry's position, name, compression method and modification time.
// A positive limit stops the output as soon as it would cross the bound, so
// peak memory follows the limit rather than the Catalog file.
func writeRetained(container *zip.Reader, opfPath string, opf []byte, removals []plannedRemoval, limit int64) ([]byte, error) {
	removed := make(map[string]struct{}, len(removals))
	for _, removal := range removals {
		removed[removal.resolved] = struct{}{}
	}
	out := &limitedBuffer{limit: limit}
	writer := zip.NewWriter(out)
	for _, file := range container.File {
		name := path.Clean(file.Name)
		if _, drop := removed[name]; drop {
			continue
		}
		if err := writeRetainedEntry(writer, file, name, opfPath, opf); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, retainedWriteError(err, "close Retained EPUB")
	}
	return out.Bytes(), nil
}

// writeRetainedEntry writes one kept entry: a directory as an empty entry, the
// rewritten package document under its original header, and anything else raw.
func writeRetainedEntry(writer *zip.Writer, file *zip.File, name, opfPath string, opf []byte) error {
	if strings.HasSuffix(file.Name, "/") {
		return writeRetainedDirectory(writer, file, name)
	}
	if name != opfPath {
		if err := writer.Copy(file); err != nil {
			return retainedWriteError(err, "copy retained container entry "+name)
		}
		return nil
	}
	header := file.FileHeader
	header.Extra = nil
	entry, err := writer.CreateHeader(&header)
	if err != nil {
		return retainedWriteError(err, "rewrite package document")
	}
	if _, err := entry.Write(opf); err != nil {
		return retainedWriteError(err, "write package document")
	}
	return nil
}

// writeRetainedDirectory writes a directory entry empty rather than raw. Some
// tools deflate directories, leaving a few bytes of empty stream that the zip
// writer refuses for a directory. A directory that claims content is invalid.
func writeRetainedDirectory(writer *zip.Writer, file *zip.File, name string) error {
	if file.UncompressedSize64 != 0 {
		return invalid("directory entry "+name+" holds data", nil)
	}
	header := file.FileHeader
	header.Extra = nil
	if _, err := writer.CreateHeader(&header); err != nil {
		return retainedWriteError(err, "write retained directory entry "+name)
	}
	return nil
}

// limitedBuffer refuses output past a positive byte limit with
// ErrRetainedSizeLimit.
type limitedBuffer struct {
	bytes.Buffer
	limit int64
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.limit > 0 && int64(b.Len())+int64(len(p)) > b.limit {
		return 0, ErrRetainedSizeLimit
	}
	return b.Buffer.Write(p)
}

// retainedWriteError keeps the size-limit sentinel intact so acquisition can
// classify it, while malformed container failures stay invalid EPUBs.
func retainedWriteError(err error, message string) error {
	if errors.Is(err, ErrRetainedSizeLimit) {
		return err
	}
	return invalid(message, err)
}
