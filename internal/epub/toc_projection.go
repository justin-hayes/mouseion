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
	name  string
	attrs []struct {
		name, value string
	}
	children []*navigationElement
	text     strings.Builder
}

func parseNavigationDocument(data []byte) (navigationDocument, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return navigationDocument{}, err
	}
	files := make(map[string]*zip.File, len(zr.File))
	for _, file := range zr.File {
		files[path.Clean(file.Name)] = file
	}

	var c container
	if err = decodeXMLFile(files, "META-INF/container.xml", &c); err != nil {
		return navigationDocument{}, err
	}
	if len(c.Rootfiles) == 0 || strings.TrimSpace(c.Rootfiles[0].FullPath) == "" {
		return navigationDocument{}, errors.New("navigation document has no package rootfile")
	}
	opfPath := path.Clean(c.Rootfiles[0].FullPath)
	var pkg packageDoc
	if err = decodeXMLFile(files, opfPath, &pkg); err != nil {
		return navigationDocument{}, err
	}
	base := path.Dir(opfPath)

	for _, item := range pkg.Manifest {
		if item.MediaType != "application/xhtml+xml" || !hasWord(item.Properties, "nav") {
			continue
		}
		name, resolveErr := resolveResourcePath(base, item.Href)
		if resolveErr != nil {
			return navigationDocument{}, resolveErr
		}
		file := files[name]
		if file == nil {
			return navigationDocument{}, errors.New("navigation resource is missing")
		}
		root, parseErr := parseNavigationXML(file)
		if parseErr != nil {
			return navigationDocument{}, parseErr
		}
		nav := findTOCNavigation(root)
		if nav == nil {
			return navigationDocument{}, errors.New("navigation document has no EPUB 3 toc")
		}
		list := directNavigationList(nav)
		if list == nil {
			return navigationDocument{}, errors.New("EPUB 3 toc has no top-level list")
		}

		entries := make([]navigationEntry, 0, len(list.children))
		for _, child := range list.children {
			if child.name != "li" {
				return navigationDocument{}, errors.New("EPUB 3 toc has a non-entry list child")
			}
			label := navigationEntryLabel(child)
			if label == "" {
				return navigationDocument{}, errors.New("EPUB 3 toc entry has no label")
			}
			resolvedHref, ok := navigationEntryStart(child, base)
			if !ok {
				return navigationDocument{}, errors.New("EPUB 3 toc entry has no usable link")
			}
			entries = append(entries, navigationEntry{label: label, resolvedHref: resolvedHref})
		}
		if len(entries) == 0 {
			return navigationDocument{}, errors.New("EPUB 3 toc has no entries")
		}
		return navigationDocument{entries: entries}, nil
	}
	return navigationDocument{}, errors.New("EPUB has no navigation document")
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
		if tokenErr == io.EOF {
			if root == nil || len(stack) != 0 {
				return nil, errors.New("malformed navigation document")
			}
			return root, nil
		}
		if tokenErr != nil {
			return nil, tokenErr
		}
		switch token := token.(type) {
		case xml.StartElement:
			element := &navigationElement{name: strings.ToLower(token.Name.Local)}
			for _, attr := range token.Attr {
				name := attr.Name.Local
				if attr.Name.Space != "" {
					name = attr.Name.Space + " " + name
				}
				element.attrs = append(element.attrs, struct {
					name, value string
				}{name: name, value: attr.Value})
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("navigation document has multiple roots")
				}
				root = element
			} else {
				stack[len(stack)-1].children = append(stack[len(stack)-1].children, element)
			}
			stack = append(stack, element)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write([]byte(token))
			}
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].name != strings.ToLower(token.Name.Local) {
				return nil, errors.New("malformed navigation document")
			}
			stack = stack[:len(stack)-1]
		}
	}
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
