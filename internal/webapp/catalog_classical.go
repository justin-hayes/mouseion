package webapp

import (
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
// matches author, English title and Greek title through the Go search key and
// returns the matching author sections.
func filterClassicalCatalog(authors []classical.CatalogAuthor, query string) []classical.CatalogAuthor {
	key := canonicalization.SearchKey(query)
	filtered := make([]classical.CatalogAuthor, 0, len(authors))
	for _, author := range authors {
		works := make([]classical.CatalogWork, 0, len(author.Works))
		for _, work := range author.Works {
			if catalogWorkMatches(work, key) {
				works = append(works, work)
			}
		}
		if len(works) > 0 {
			filtered = append(filtered, classical.CatalogAuthor{Name: author.Name, Works: works})
		}
	}
	return filtered
}

func catalogWorkMatches(work classical.CatalogWork, key string) bool {
	if key == "" {
		return true
	}
	return domain.CatalogueTextMatches(work.Title, key) ||
		domain.CatalogueTextMatches(work.Author, key) ||
		domain.CatalogueTextMatches(work.GreekTitle, key)
}
