package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalogueScenarios pin the Catalog browse read: one connection's local index,
// filtered by language and the Go search key, paged in title-then-author order,
// with ownership from active catalog-entry aliases. None of them contacts an
// OPDS server.
func catalogueScenarios() []Scenario {
	return []Scenario{{
		Name: "catalogue browse pages in title then author order",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "beta", Title: "Beta", Author: "Anna", LanguageTag: Language, EPUBHref: "https://catalog.example/beta.epub"}},
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "alpha-zed", Title: "Alpha", Author: "Zed", LanguageTag: Language, EPUBHref: "https://catalog.example/alpha-zed.epub"}},
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "gamma", Title: "Gamma", Author: "Anna", LanguageTag: Language, EPUBHref: "https://catalog.example/gamma.epub"}},
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "alpha-anna", Title: "Alpha", Author: "Anna", LanguageTag: Language, EPUBHref: "https://catalog.example/alpha-anna.epub"}},
			})

			first, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{Limit: 2})
			require.NoError(t, err)
			assert.Equal(t, 4, first.Total)
			require.Len(t, first.Entries, 2)
			assert.Equal(t, []string{"alpha-anna", "alpha-zed"}, []string{first.Entries[0].Index.EntryIdentifier, first.Entries[1].Index.EntryIdentifier}, "Alpha orders by author")

			second, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{Offset: 2, Limit: 2})
			require.NoError(t, err)
			assert.Equal(t, 4, second.Total)
			require.Len(t, second.Entries, 2)
			assert.Equal(t, []string{"beta", "gamma"}, []string{second.Entries[0].Index.EntryIdentifier, second.Entries[1].Index.EntryIdentifier}, "paging continues in title-then-author order")

			past, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{Offset: 9, Limit: 2})
			require.NoError(t, err)
			assert.Empty(t, past.Entries)
			assert.Equal(t, 4, past.Total)
		},
	}, {
		Name: "catalogue browse filters by language and folds the search key",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "uber", Title: "Über die Berge", Author: "Anna Berg", LanguageTag: Language, EPUBHref: "https://catalog.example/uber.epub"}},
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "citta", Title: "La città delle meraviglie", Author: "Luca Bianchi", LanguageTag: "it", EPUBHref: "https://catalog.example/citta.epub"}},
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "odipus", Title: "Ödipus auf Kolonos", Author: "Hómēr Nachfolger", LanguageTag: Language, EPUBHref: "https://catalog.example/odipus.epub"}},
			})

			byTitle, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{Search: "UBER"})
			require.NoError(t, err)
			require.Len(t, byTitle.Entries, 1)
			assert.Equal(t, "uber", byTitle.Entries[0].Index.EntryIdentifier, "accented title matched an unaccented, uppercase query")

			byAuthor, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{Search: "homer"})
			require.NoError(t, err)
			require.Len(t, byAuthor.Entries, 1)
			assert.Equal(t, "odipus", byAuthor.Entries[0].Index.EntryIdentifier, "accented author matched an unaccented query")

			italian, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{Language: "IT"})
			require.NoError(t, err)
			require.Len(t, italian.Entries, 1)
			assert.Equal(t, "citta", italian.Entries[0].Index.EntryIdentifier)

			none, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{Language: "it", Search: "uber"})
			require.NoError(t, err)
			assert.Empty(t, none.Entries)
		},
	}, {
		Name: "catalogue browse reports ownership only for an active Book",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			bookID := h.Seeds().SeedBook(t, owner, BookSeed{ToRead: true})
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "owned", Title: "Owned book", Author: "Owner Author", LanguageTag: Language, EPUBHref: "https://catalog.example/owned.epub"}, BookID: bookID},
				{Entry: domain.CatalogueIndexEntry{EntryIdentifier: "unowned", Title: "Unowned book", Author: "Other Author", LanguageTag: Language, EPUBHref: "https://catalog.example/unowned.epub"}},
			})

			page, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
			require.NoError(t, err)
			require.Len(t, page.Entries, 2)
			ownership := map[string]string{}
			for _, entry := range page.Entries {
				ownership[entry.Index.EntryIdentifier] = entry.BookID
			}
			assert.Equal(t, bookID, ownership["owned"], "the linked Book is ownership")
			assert.Empty(t, ownership["unowned"], "an entry without a Book offers Add in a later slice")

			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, bookID))
			removed, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
			require.NoError(t, err)
			require.Len(t, removed.Entries, 2, "the entry stays in the Catalog after its Book leaves My Books")
			removedOwnership := map[string]string{}
			for _, entry := range removed.Entries {
				removedOwnership[entry.Index.EntryIdentifier] = entry.BookID
			}
			assert.Empty(t, removedOwnership["owned"], "a removed Book contributes no ownership")
		},
	}}
}
