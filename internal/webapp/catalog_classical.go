package webapp

import (
	"sort"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// classicalCatalogCards is the Classical side of the Catalog: one card per
// current Reading unit whose Work matches the search, ordered by title, then
// author, then unit key, like every other source. It matches author, English
// title and Greek title through the Go search key.
func classicalCatalogCards(authors []classical.CatalogAuthor, query string, view classicalCatalogView) []catalogCard {
	key := canonicalization.SearchKey(query)
	var cards []catalogCard
	for _, author := range authors {
		for _, work := range author.Works {
			if !catalogWorkMatches(work, key) {
				continue
			}
			for _, unit := range work.Units {
				cards = append(cards, classicalCatalogCard(work, unit, view))
			}
		}
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if left, right := canonicalization.SearchKey(cards[i].Title), canonicalization.SearchKey(cards[j].Title); left != right {
			return left < right
		}
		if left, right := canonicalization.SearchKey(cards[i].Author), canonicalization.SearchKey(cards[j].Author); left != right {
			return left < right
		}
		return cards[i].ID < cards[j].ID
	})
	return cards
}

// classicalCatalogCard is one Reading unit, a Work in one Edition, as a card.
// Its title opens the Work page; its detail line is the Edition label.
func classicalCatalogCard(work classical.CatalogWork, unit classical.CatalogUnit, view classicalCatalogView) catalogCard {
	card := catalogCard{
		ID:         unit.Key,
		Title:      work.Title,
		GreekTitle: work.GreekTitle,
		Author:     work.Author,
		Detail:     unit.EditionLabel,
		TitleURL:   classicalWorkURL(work.Key),
		Plate:      newTitlePlate(work.Title, work.GreekTitle, work.Author),
	}
	if bookID := view.OwnedUnits[unit.Key]; bookID != "" {
		card.OwnedURL = classicalBookInMyBooksURL(bookID)
		return card
	}
	if view.AddAvailable {
		card.AddURL = classicalUnitAddURL(unit.Key)
		card.ReadURL = classicalUnitReadURL(unit.Key)
		card.ReturnTo = classicalAddReturnIndex
	}
	return card
}

func catalogWorkMatches(work classical.CatalogWork, key string) bool {
	if key == "" {
		return true
	}
	return domain.CatalogueTextMatches(work.Title, key) ||
		domain.CatalogueTextMatches(work.Author, key) ||
		domain.CatalogueTextMatches(work.GreekTitle, key)
}
