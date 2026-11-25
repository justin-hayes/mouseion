package webapp

import (
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// classicalCatalogCards is the Classical side of the Catalog: one card per
// current Reading unit whose Work matches the search, ordered by title, then
// author, then unit key, like every other source. It matches author, English
// title and Greek title through the Go search key.
func classicalCatalogCards(authors []classical.CatalogAuthor, query string, view classicalCatalogView) []catalogCard {
	return classicalCatalogCardsIn(authors, query, "", view)
}

// classicalCatalogCardsIn is classicalCatalogCards narrowed to the Works of one
// classical language; an empty language keeps every Work.
func classicalCatalogCardsIn(authors []classical.CatalogAuthor, query, language string, view classicalCatalogView) []catalogCard {
	key := canonicalization.SearchKey(query)
	var cards []catalogCard
	for _, author := range authors {
		for _, work := range author.Works {
			if language != "" && work.Language != language {
				continue
			}
			if !catalogWorkMatches(work, key) {
				continue
			}
			for _, unit := range work.Units {
				cards = append(cards, classicalCatalogCard(work, unit, view))
			}
		}
	}
	sortCatalogCards(cards)
	return cards
}

// distinctOriginalTitle is the original title to show: empty when it is the
// English title again (ADR 0132), as for a Latin Work whose title is the same.
func distinctOriginalTitle(title, original string) string {
	if strings.TrimSpace(original) == strings.TrimSpace(title) {
		return ""
	}
	return original
}

// sortCatalogCards orders cards by title, then author, then ID.
func sortCatalogCards(cards []catalogCard) {
	sort.SliceStable(cards, func(i, j int) bool {
		if left, right := canonicalization.SearchKey(cards[i].Title), canonicalization.SearchKey(cards[j].Title); left != right {
			return left < right
		}
		if left, right := canonicalization.SearchKey(cards[i].Author), canonicalization.SearchKey(cards[j].Author); left != right {
			return left < right
		}
		return cards[i].ID < cards[j].ID
	})
}

// classicalCatalogCard is one Reading unit, a Work in one Edition, as a card.
// Its title opens the Work page; its detail line is the Edition label.
func classicalCatalogCard(work classical.CatalogWork, unit classical.CatalogUnit, view classicalCatalogView) catalogCard {
	original := distinctOriginalTitle(work.Title, work.OriginalTitle)
	card := catalogCard{
		ID:            "classical-" + unit.Key,
		Title:         work.Title,
		OriginalTitle: original,
		OriginalLang:  work.Language,
		Author:        work.Author,
		Detail:        unit.EditionLabel,
		TitleURL:      classicalWorkURL(work.Key),
		Plate:         newTitlePlate(work.Title, original, work.Language, work.Author),
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
		domain.CatalogueTextMatches(work.OriginalTitle, key)
}
