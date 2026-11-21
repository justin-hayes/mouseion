package webapp

import (
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func catalogAuthorsWorkCount(authors []classical.CatalogAuthor) int {
	total := 0
	for _, author := range authors {
		total += len(author.Works)
	}
	return total
}

// filterClassicalCatalog is the Classical side of the Catalog search. It
// matches author, English title and Greek title through the Go search key, and
// a trailing number narrows the match to a Work that carries that constituent
// book, as My Books does. It returns the matching author sections and the one
// Book each narrowed Work displays.
func filterClassicalCatalog(authors []classical.CatalogAuthor, query string) ([]classical.CatalogAuthor, map[string]int) {
	text, number, hasNumber := splitCatalogTrailingBookNumber(query)
	key := canonicalization.SearchKey(text)
	filtered := make([]classical.CatalogAuthor, 0, len(authors))
	narrowed := map[string]int{}
	for _, author := range authors {
		works := make([]classical.CatalogWork, 0, len(author.Works))
		for _, work := range author.Works {
			if !catalogWorkMatches(work, key, number, hasNumber) {
				continue
			}
			if hasNumber {
				narrowed[work.Key] = number
			}
			works = append(works, work)
		}
		if len(works) > 0 {
			filtered = append(filtered, classical.CatalogAuthor{Name: author.Name, Works: works})
		}
	}
	return filtered, narrowed
}

func catalogWorkMatches(work classical.CatalogWork, key string, number int, hasNumber bool) bool {
	if hasNumber && !catalogWorkHasConstituentBook(work, number) {
		return false
	}
	if key == "" {
		return true
	}
	return domain.CatalogueTextMatches(work.Title, key) ||
		domain.CatalogueTextMatches(work.Author, key) ||
		domain.CatalogueTextMatches(work.GreekTitle, key)
}

func catalogWorkHasConstituentBook(work classical.CatalogWork, number int) bool {
	for _, unit := range work.Units {
		if unit.ConstituentBook == number {
			return true
		}
	}
	return false
}

// classicalDisplayUnits keeps the existing Work presentation while a trailing
// search number narrows the visible Book cells and length segment to that one
// constituent Book.
func classicalDisplayUnits(work classical.CatalogWork, narrowed int) []classical.CatalogUnit {
	if narrowed == 0 {
		return work.Units
	}
	units := make([]classical.CatalogUnit, 0, 1)
	for _, unit := range work.Units {
		if unit.ConstituentBook == narrowed {
			units = append(units, unit)
		}
	}
	return units
}

// splitCatalogTrailingBookNumber separates a search such as "iliad 6" into its
// text and constituent Book number. A lone number is ordinary search text.
func splitCatalogTrailingBookNumber(query string) (text string, number int, ok bool) {
	fields := strings.Fields(query)
	if len(fields) < 2 {
		return query, 0, false
	}
	parsed, err := strconv.Atoi(fields[len(fields)-1])
	if err != nil || parsed < 1 {
		return query, 0, false
	}
	return strings.Join(fields[:len(fields)-1], " "), parsed, true
}
