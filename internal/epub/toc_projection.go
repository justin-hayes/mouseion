package epub

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path"
	"sort"
	"strings"
)

const epubOpsNamespace = "http://www.idpf.org/2007/ops"

// TOCChoice is one deterministic scope-review choice. UnitIDs are ordered in
// persisted spine order and First is the order of the first member.
type TOCChoice struct {
	Label   string
	UnitIDs []string
	First   uint64
}

// ProjectTOCChoices projects the EPUB 3 navigation document onto snapshot.
// The snapshot is authoritative for unit identity, readability, and order;
// epubData supplies the ordered navigation structure that is not persisted in
// ExtractedUnits. Any unreliable projection produces one choice per readable
// snapshot unit in ascending persisted order.
//
// Both the navigation entries and every choice are ordered by document/spine
// order. No map iteration contributes to the result, so repeated calls with
// the same EPUB bytes and snapshot are byte-for-byte equivalent in structure.
func ProjectTOCChoices(epubData []byte, snapshot ExtractedUnits) []TOCChoice {
	readable := readableSnapshotUnits(snapshot)
	fallback := flatTOCChoices(readable)

	document, err := parseNavigationDocument(epubData)
	if err != nil {
		return fallback
	}
	if choices, ok := projectNavigationChoices(document, readable); ok {
		return choices
	}
	return fallback
}

type navigationDocument struct {
	entries []navigationEntry
}

type navigationEntry struct {
	label        string
	resolvedHref string
}

type navigationElement struct {
	name     string
	attrs    []navigationAttr
	children []*navigationElement
	text     strings.Builder
}

type navigationAttr struct {
	name, value string
}

func parseNavigationDocument(data []byte) (navigationDocument, error) {
	files, base, pkg, err := openNavigationArchive(data)
	if err != nil {
		return navigationDocument{}, err
	}
	for _, item := range pkg.Manifest {
		if item.MediaType != "application/xhtml+xml" || !hasWord(item.Properties, "nav") {
			continue
		}
		entries, err := navigationDocumentEntries(files, base, item.Href)
		if err != nil {
			return navigationDocument{}, err
		}
		return navigationDocument{entries: entries}, nil
	}
	return navigationDocument{}, errors.New("EPUB has no navigation document")
}

// openNavigationArchive reads the ZIP's file map and the package document its
// container points to, returning the package's base directory alongside them.
func openNavigationArchive(data []byte) (map[string]*zip.File, string, packageDoc, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, "", packageDoc{}, err
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, file := range zr.File {
		files[path.Clean(file.Name)] = file
	}

	var c container
	if err := decodeXMLFile(files, "META-INF/container.xml", &c); err != nil {
		return nil, "", packageDoc{}, err
	}
	if len(c.Rootfiles) == 0 || strings.TrimSpace(c.Rootfiles[0].FullPath) == "" {
		return nil, "", packageDoc{}, errors.New("navigation document has no package rootfile")
	}
	opfPath := path.Clean(c.Rootfiles[0].FullPath)
	var pkg packageDoc
	if err := decodeXMLFile(files, opfPath, &pkg); err != nil {
		return nil, "", packageDoc{}, err
	}
	return files, path.Dir(opfPath), pkg, nil
}

// navigationDocumentEntries parses the package's navigation resource and
// resolves its top-level table-of-contents entries.
func navigationDocumentEntries(files map[string]*zip.File, base, href string) ([]navigationEntry, error) {
	name, err := resolveResourcePath(base, href)
	if err != nil {
		return nil, err
	}
	file := files[name]
	if file == nil {
		return nil, errors.New("navigation resource is missing")
	}
	root, err := parseNavigationXML(file)
	if err != nil {
		return nil, err
	}
	nav := findTOCNavigation(root)
	if nav == nil {
		return nil, errors.New("navigation document has no EPUB 3 toc")
	}
	list := directNavigationList(nav)
	if list == nil {
		return nil, errors.New("EPUB 3 toc has no top-level list")
	}
	return navigationListEntries(list, base)
}

// navigationListEntries walks one navigation list into ordered entries.
func navigationListEntries(list *navigationElement, base string) ([]navigationEntry, error) {
	entries := make([]navigationEntry, 0, len(list.children))
	for _, child := range list.children {
		if child.name != "li" {
			return nil, errors.New("EPUB 3 toc has a non-entry list child")
		}
		label := navigationEntryLabel(child)
		if label == "" {
			return nil, errors.New("EPUB 3 toc entry has no label")
		}
		resolvedHref, ok := navigationEntryStart(child, base)
		if !ok {
			return nil, errors.New("EPUB 3 toc entry has no usable link")
		}
		entries = append(entries, navigationEntry{label: label, resolvedHref: resolvedHref})
	}
	if len(entries) == 0 {
		return nil, errors.New("EPUB 3 toc has no entries")
	}
	return entries, nil
}

func parseNavigationXML(file *zip.File) (*navigationElement, error) {
	data, err := readFile(file)
	if err != nil {
		return nil, err
	}
	decoder := xml.NewDecoder(strings.NewReader(xhtmlEntityReplacer.Replace(string(data))))
	var root *navigationElement
	var stack []*navigationElement
	for {
		token, tokenErr := decoder.Token()
		if errors.Is(tokenErr, io.EOF) {
			return finishNavigationElement(root, stack)
		}
		if tokenErr != nil {
			return nil, tokenErr
		}
		root, stack, err = applyNavigationToken(token, root, stack)
		if err != nil {
			return nil, err
		}
	}
}

// applyNavigationToken folds one XML token into the element tree under
// construction and returns the updated root and open-element stack.
func applyNavigationToken(token xml.Token, root *navigationElement, stack []*navigationElement) (*navigationElement, []*navigationElement, error) {
	switch token := token.(type) {
	case xml.StartElement:
		element := &navigationElement{name: strings.ToLower(token.Name.Local), attrs: navigationAttrs(token.Attr)}
		switch {
		case len(stack) == 0 && root != nil:
			return nil, nil, errors.New("navigation document has multiple roots")
		case len(stack) == 0:
			root = element
		default:
			stack[len(stack)-1].children = append(stack[len(stack)-1].children, element)
		}
		return root, append(stack, element), nil
	case xml.CharData:
		if len(stack) > 0 {
			stack[len(stack)-1].text.Write([]byte(token))
		}
	case xml.EndElement:
		if len(stack) == 0 || stack[len(stack)-1].name != strings.ToLower(token.Name.Local) {
			return nil, nil, errors.New("malformed navigation document")
		}
		stack = stack[:len(stack)-1]
	}
	return root, stack, nil
}

// finishNavigationElement rejects a document that never opened a root or ended
// with unclosed elements.
func finishNavigationElement(root *navigationElement, stack []*navigationElement) (*navigationElement, error) {
	if root == nil || len(stack) != 0 {
		return nil, errors.New("malformed navigation document")
	}
	return root, nil
}

// navigationAttrs converts XML attributes to the element tree's attribute
// representation, qualifying namespaced names the same way. Elements without
// attributes keep a nil slice, as the parser built them before.
func navigationAttrs(attrs []xml.Attr) []navigationAttr {
	if len(attrs) == 0 {
		return nil
	}
	out := make([]navigationAttr, 0, len(attrs))
	for _, attr := range attrs {
		name := attr.Name.Local
		if attr.Name.Space != "" {
			name = attr.Name.Space + " " + name
		}
		out = append(out, navigationAttr{name: name, value: attr.Value})
	}
	return out
}

func findTOCNavigation(node *navigationElement) *navigationElement {
	if node == nil {
		return nil
	}
	if node.name == "nav" && hasEPUBType(node, "toc") {
		return node
	}
	for _, child := range node.children {
		if found := findTOCNavigation(child); found != nil {
			return found
		}
	}
	return nil
}

func directNavigationList(nav *navigationElement) *navigationElement {
	for _, child := range nav.children {
		if child.name == "ol" {
			return child
		}
	}
	return nil
}

func hasEPUBType(node *navigationElement, token string) bool {
	for _, attr := range node.attrs {
		if attr.name == epubOpsNamespace+" type" && containsWord(strings.Fields(attr.value), token) {
			return true
		}
	}
	return false
}

func navigationEntryLabel(entry *navigationElement) string {
	for _, child := range entry.children {
		if child.name == "a" {
			return cleanSpace(navigationText(child))
		}
	}
	return cleanSpace(navigationTextWithoutLists(entry))
}

func navigationText(node *navigationElement) string {
	if node == nil {
		return ""
	}
	var out strings.Builder
	out.WriteString(node.text.String())
	for _, child := range node.children {
		out.WriteString(navigationText(child))
	}
	return out.String()
}

func navigationTextWithoutLists(node *navigationElement) string {
	if node == nil {
		return ""
	}
	var out strings.Builder
	out.WriteString(node.text.String())
	for _, child := range node.children {
		if child.name != "ol" && child.name != "ul" {
			out.WriteString(navigationTextWithoutLists(child))
		}
	}
	return out.String()
}

func navigationEntryStart(entry *navigationElement, base string) (string, bool) {
	for _, child := range entry.children {
		if child.name == "a" {
			if resolved, ok := resolveNavigationHref(child, base); ok {
				return resolved, true
			}
			break
		}
	}
	var descendants []*navigationElement
	collectNavigationLinks(entry, &descendants)
	for _, link := range descendants {
		if resolved, ok := resolveNavigationHref(link, base); ok {
			return resolved, true
		}
	}
	return "", false
}

func collectNavigationLinks(node *navigationElement, links *[]*navigationElement) {
	for _, child := range node.children {
		if child.name == "a" {
			*links = append(*links, child)
		}
		collectNavigationLinks(child, links)
	}
}

func resolveNavigationHref(link *navigationElement, base string) (string, bool) {
	href := navigationAttribute(link, "href")
	if strings.TrimSpace(href) == "" {
		return "", false
	}
	resolved, err := resolveResourcePath(base, href)
	return resolved, err == nil
}

func navigationAttribute(node *navigationElement, name string) string {
	for _, attr := range node.attrs {
		parts := strings.SplitN(attr.name, " ", 2)
		if len(parts) == 1 && parts[0] == name {
			return attr.value
		}
	}
	return ""
}

func readableSnapshotUnits(snapshot ExtractedUnits) []ExtractedUnit {
	units := make([]ExtractedUnit, 0, len(snapshot.Units))
	for _, unit := range snapshot.Units {
		if strings.TrimSpace(unit.Text) != "" {
			units = append(units, unit)
		}
	}
	sort.SliceStable(units, func(i, j int) bool {
		if units[i].Order != units[j].Order {
			return units[i].Order < units[j].Order
		}
		return units[i].ID < units[j].ID
	})
	return units
}

func flatTOCChoices(units []ExtractedUnit) []TOCChoice {
	choices := make([]TOCChoice, 0, len(units))
	for _, unit := range units {
		label := cleanSpace(unit.Title)
		if label == "" {
			label = strings.TrimSpace(unit.ManifestID)
		}
		choices = append(choices, TOCChoice{Label: label, UnitIDs: []string{unit.ID}, First: unit.Order})
	}
	return choices
}

func projectNavigationChoices(document navigationDocument, units []ExtractedUnit) ([]TOCChoice, bool) {
	if len(document.entries) == 0 || len(units) == 0 {
		return nil, false
	}
	starts, ok := navigationEntryStarts(document, units)
	if !ok {
		return nil, false
	}
	return navigationChoices(document, units, starts)
}

// navigationEntryStarts resolves the index of the first readable unit each
// navigation entry targets. It rejects a projection where unit order regresses,
// an entry's href does not match exactly one unit, entry targets do not
// advance, or the first entry does not target the first readable unit.
func navigationEntryStarts(document navigationDocument, units []ExtractedUnit) ([]int, bool) {
	byHref := make(map[string][]int, len(units))
	for i, unit := range units {
		if i > 0 && unit.Order <= units[i-1].Order {
			return nil, false
		}
		byHref[unit.ResolvedHref] = append(byHref[unit.ResolvedHref], i)
	}
	starts := make([]int, len(document.entries))
	previous := -1
	for i, entry := range document.entries {
		matches := byHref[entry.resolvedHref]
		if len(matches) != 1 || matches[0] <= previous {
			return nil, false
		}
		starts[i] = matches[0]
		previous = matches[0]
	}
	if starts[0] != 0 {
		return nil, false
	}
	return starts, true
}

// navigationChoices builds one choice per entry, each owning the units between
// its start and the next entry's start. It rejects an entry that would own no
// units.
func navigationChoices(document navigationDocument, units []ExtractedUnit, starts []int) ([]TOCChoice, bool) {
	choices := make([]TOCChoice, 0, len(document.entries))
	for i, entry := range document.entries {
		end := len(units)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if end <= starts[i] {
			return nil, false
		}
		members := make([]string, 0, end-starts[i])
		for _, unit := range units[starts[i]:end] {
			members = append(members, unit.ID)
		}
		choices = append(choices, TOCChoice{Label: entry.label, UnitIDs: members, First: units[starts[i]].Order})
	}
	return choices, true
}
