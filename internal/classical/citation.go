package classical

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Named Citation sections form a closed, reviewed list. A prefatory section
// (pr, the preface) orders before every numbered component; a closing section
// (ep, the epilogue) orders after every numbered component. A name outside the
// list is refused by parsing and by manifest validation.
var (
	prefatorySections = map[string]struct{}{"pr": {}}
	closingSections   = map[string]struct{}{"ep": {}}
)

// CitationComponent is one dotted component of a Citation: a number with an
// optional lowercase letter suffix (331a), or a named section (pr).
type CitationComponent struct {
	Number int
	Suffix string
	Name   string
}

// Citation is a parsed Citation, outermost component first.
type Citation []CitationComponent

// String renders the component as written.
func (c CitationComponent) String() string {
	if c.Name != "" {
		return c.Name
	}
	return strconv.Itoa(c.Number) + c.Suffix
}

// String renders the Citation as written.
func (c Citation) String() string {
	parts := make([]string, len(c))
	for i, component := range c {
		parts[i] = component.String()
	}
	return strings.Join(parts, ".")
}

// ParseCitation parses dotted components such as "1.10.19", "331a" or "1.pr.1".
// It refuses an empty component, a leading zero, a negative number, an
// uppercase or repeated suffix and a named section outside the reviewed list.
func ParseCitation(value string) (Citation, bool) {
	if value == "" {
		return nil, false
	}
	parts := strings.Split(value, ".")
	citation := make(Citation, 0, len(parts))
	for _, part := range parts {
		component, ok := parseCitationComponent(part)
		if !ok {
			return nil, false
		}
		citation = append(citation, component)
	}
	return citation, true
}

func parseCitationComponent(part string) (CitationComponent, bool) {
	if _, ok := prefatorySections[part]; ok {
		return CitationComponent{Name: part}, true
	}
	if _, ok := closingSections[part]; ok {
		return CitationComponent{Name: part}, true
	}
	digits := len(part)
	for i, r := range part {
		if r < '0' || r > '9' {
			digits = i
			break
		}
	}
	number, suffix := part[:digits], part[digits:]
	if number == "" || (len(number) > 1 && number[0] == '0') {
		return CitationComponent{}, false
	}
	if len(suffix) > 1 || (suffix != "" && (suffix[0] < 'a' || suffix[0] > 'z')) {
		return CitationComponent{}, false
	}
	parsed, err := strconv.Atoi(number)
	if err != nil {
		return CitationComponent{}, false
	}
	return CitationComponent{Number: parsed, Suffix: suffix}, true
}

// IsCitation reports whether value is a valid Citation.
func IsCitation(value string) bool {
	_, ok := ParseCitation(value)
	return ok
}

// CitationContains reports whether outer equals inner or is a whole-component
// ancestor of it ("1.2" contains "1.2.3" but not "1.20" or "1.2a"). It is the one
// containment rule Citation consumers share: passage context and the manifest's
// translation join gate.
func CitationContains(outer, inner string) bool {
	return outer != "" && (outer == inner || strings.HasPrefix(inner, outer+"."))
}

// rank orders prefatory sections before numbers and closing sections
// after them.
func (c CitationComponent) rank() int {
	if _, ok := prefatorySections[c.Name]; ok {
		return 0
	}
	if c.Name != "" {
		return 2
	}
	return 1
}

func compareComponents(a, b CitationComponent) int {
	if a.rank() != b.rank() {
		return cmpInt(a.rank(), b.rank())
	}
	if a.Number != b.Number {
		return cmpInt(a.Number, b.Number)
	}
	return strings.Compare(a.Suffix, b.Suffix)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	return 1
}

// Compare orders two Citations component-wise, treating a missing component as
// the number zero. A suffixed number follows its bare number (331 < 331a).
func (c Citation) Compare(other Citation) int {
	for i := range max(len(c), len(other)) {
		left, right := CitationComponent{}, CitationComponent{}
		if i < len(c) {
			left = c[i]
		}
		if i < len(other) {
			right = other[i]
		}
		if order := compareComponents(left, right); order != 0 {
			return order
		}
	}
	return 0
}

// Within reports whether c lies inside the inclusive range start to end.
func (c Citation) Within(start, end Citation) bool {
	return c.Compare(start) >= 0 && c.Compare(end) <= 0
}

// FormatCitationRange renders the distinct cited sections as one label, joining
// consecutive sections into a range: 1.2.17 and 1.2.18 become "1.2.17-18".
func FormatCitationRange(citations []string) string {
	type parsedCitation struct {
		parts Citation
		raw   string
	}
	items := make([]parsedCitation, 0, len(citations))
	for _, citation := range citations {
		parts, ok := ParseCitation(citation)
		if !ok {
			return strings.Join(citations, ", ")
		}
		items = append(items, parsedCitation{parts: parts, raw: citation})
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].parts.Compare(items[j].parts) < 0 })
	groups := make([]string, 0, len(items))
	for start := 0; start < len(items); {
		end := start
		for end+1 < len(items) && consecutiveCitation(items[end].parts, items[end+1].parts) {
			end++
		}
		if end == start {
			groups = append(groups, items[start].raw)
		} else {
			first, last := items[start].parts, items[end].parts
			prefix := first[:len(first)-1].String()
			if prefix != "" {
				prefix += "."
			}
			groups = append(groups, prefix+first[len(first)-1].String()+"\u2013"+last[len(last)-1].String())
		}
		start = end + 1
	}
	return strings.Join(groups, ", ")
}

// consecutiveCitation reports whether b is the section immediately after a:
// the same components except a final plain number exactly one greater. A
// suffixed or named final component never joins a range.
func consecutiveCitation(a, b Citation) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range len(a) - 1 {
		if compareComponents(a[i], b[i]) != 0 {
			return false
		}
	}
	last, next := a[len(a)-1], b[len(b)-1]
	return last.Name == "" && last.Suffix == "" && next.Name == "" && next.Suffix == "" && next.Number == last.Number+1
}

// CompareCitations orders two dotted citations component-wise, treating
// missing components as zero. It refuses a value that is not a dotted
// citation instead of ordering it as zero.
func CompareCitations(a, b string) (int, error) {
	left, ok := ParseCitation(a)
	if !ok {
		return 0, fmt.Errorf("classical: %q is not a dotted citation", a)
	}
	right, ok := ParseCitation(b)
	if !ok {
		return 0, fmt.Errorf("classical: %q is not a dotted citation", b)
	}
	return left.Compare(right), nil
}
