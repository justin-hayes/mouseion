package cardexport

import (
	"sort"
	"strconv"
	"strings"
)

// The Ancient Greek default card shows the pinned Brownson passage context of
// the cited sections a representative sentence touches (ADR 0101). It is never
// a translation of that sentence, so it is labelled and highlights nothing.
const (
	passageContextPrefix = "Passage context: "
	passageEditionLabel  = "Brownson (1922)"                      //nolint:gosec // G101 false positive: a user-facing card label, not a credential.
	passageWorkLabel     = "Xen. Anab."                           //nolint:gosec // G101 false positive: a user-facing card label, not a credential.
	passageContextSuffix = ". Not a translation of this sentence" //nolint:gosec // G101 false positive: a user-facing card label, not a credential.
)

// passageContextLabel renders the frozen passage citation as the labelled,
// unhighlighted card heading.
func passageContextLabel(citation string) string {
	return passageContextPrefix + passageEditionLabel + ", " + passageWorkLabel + " " + citation + passageContextSuffix
}

// resolvePassage returns the passage text, its citation label and the pin for a
// representative sentence, or empty values when the card must show no English.
// Every cited section must have a translation; a single missing key withholds
// the whole passage, never a partial one.
func resolvePassage(citations []string, translations map[string]string, pin string) (text, citation, sourcePin string) {
	if strings.TrimSpace(pin) == "" || len(citations) == 0 || len(translations) == 0 {
		return "", "", ""
	}
	sections := make([]string, 0, len(citations))
	for _, section := range citations {
		translation := strings.TrimSpace(translations[section])
		if translation == "" {
			return "", "", ""
		}
		sections = append(sections, translation)
	}
	return strings.Join(sections, " "), formatCitationRange(citations), pin
}

// formatCitationRange renders the distinct cited sections as one label, joining
// consecutive sections into a range: 1.2.17 and 1.2.18 become "1.2.17-18".
func formatCitationRange(citations []string) string {
	type parsedCitation struct {
		parts []int
		raw   string
	}
	items := make([]parsedCitation, 0, len(citations))
	for _, citation := range citations {
		parts, ok := parseDottedCitation(citation)
		if !ok {
			return strings.Join(citations, ", ")
		}
		items = append(items, parsedCitation{parts: parts, raw: citation})
	}
	sort.SliceStable(items, func(i, j int) bool { return compareDotted(items[i].parts, items[j].parts) < 0 })
	groups := make([]string, 0, len(items))
	for start := 0; start < len(items); {
		end := start
		for end+1 < len(items) && consecutiveCitation(items[end].parts, items[end+1].parts) {
			end++
		}
		if end == start {
			groups = append(groups, items[start].raw)
		} else {
			prefix := strings.Join(intStrings(items[start].parts[:len(items[start].parts)-1]), ".")
			groups = append(groups, prefix+"."+strconv.Itoa(items[start].parts[len(items[start].parts)-1])+"\u2013"+strconv.Itoa(items[end].parts[len(items[end].parts)-1]))
		}
		start = end + 1
	}
	return strings.Join(groups, ", ")
}

// consecutiveCitation reports whether b is the section immediately after a:
// the same components except a final component exactly one greater.
func consecutiveCitation(a, b []int) bool {
	if len(a) != len(b) || len(a) == 0 {
		return false
	}
	for i := range len(a) - 1 {
		if a[i] != b[i] {
			return false
		}
	}
	return b[len(b)-1] == a[len(a)-1]+1
}

// compareDotted compares two dotted citations component-wise, treating missing
// components as zero.
func compareDotted(a, b []int) int {
	length := max(len(a), len(b))
	for i := range length {
		left, right := 0, 0
		if i < len(a) {
			left = a[i]
		}
		if i < len(b) {
			right = b[i]
		}
		if left != right {
			if left < right {
				return -1
			}
			return 1
		}
	}
	return 0
}

// parseDottedCitation splits "1.10.19" into non-negative numbers.
func parseDottedCitation(value string) ([]int, bool) {
	if strings.TrimSpace(value) == "" {
		return nil, false
	}
	parts := strings.Split(value, ".")
	numbers := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return nil, false
		}
		numbers = append(numbers, number)
	}
	return numbers, true
}

func intStrings(values []int) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = strconv.Itoa(value)
	}
	return result
}
