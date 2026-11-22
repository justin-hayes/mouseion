package storecontract

import (
	"strings"
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
		Name: "catalogue Add creates a To Read Book with its catalog-entry alias",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry: domain.CatalogueIndexEntry{EntryIdentifier: "new-entry", Title: "New Entry", Author: "Entry Author", LanguageTag: Language, EPUBHref: "https://catalog.example/new-entry.epub"},
			}})

			added, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "new-entry")
			require.NoError(t, err)
			assert.True(t, added.Created, "the unowned entry has no Book yet")
			assert.True(t, added.Changed, "creating the Book changes the learner's choice")
			require.NotEmpty(t, added.Book.ID)

			detail, err := h.BookState().GetBookDetail(owner, added.Book.ID)
			require.NoError(t, err)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
			assert.Equal(t, "New Entry", detail.Book.Title)
			assert.Equal(t, "Entry Author", detail.Book.Author)
			assert.Equal(t, Language, detail.Book.LanguageTag)

			page, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
			require.NoError(t, err)
			require.Len(t, page.Entries, 1)
			assert.Equal(t, added.Book.ID, page.Entries[0].BookID, "Add links the Book through its catalog-entry alias")

			languages, err := h.Classical().ListStudyLanguages(owner)
			require.NoError(t, err)
			assert.True(t, catalogueHasStudyLanguage(languages, Language), "adding the first Book in a language makes it a study language: %+v", languages)
		},
	}, {
		Name: "catalogue Add restores the same Book after it leaves My Books",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			bookID := h.Seeds().SeedBook(t, owner, BookSeed{ToRead: true, MetadataOnly: true})
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry:  domain.CatalogueIndexEntry{EntryIdentifier: "owned-entry", Title: "Owned Entry", Author: "Entry Author", LanguageTag: Language, EPUBHref: "https://catalog.example/owned-entry.epub"},
				BookID: bookID,
			}})
			require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, bookID))

			added, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "owned-entry")
			require.NoError(t, err)
			assert.False(t, added.Created, "the alias still names the learner's Book")
			assert.True(t, added.Changed, "restoring membership changes the learner's choice")
			assert.Equal(t, bookID, added.Book.ID, "Add restores the same Book ID")

			detail, err := h.BookState().GetBookDetail(owner, bookID)
			require.NoError(t, err)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
		},
	}, {
		Name: "catalogue Add replay creates no duplicate Book and changes nothing",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry: domain.CatalogueIndexEntry{EntryIdentifier: "replay-entry", Title: "Replay Entry", Author: "Entry Author", LanguageTag: Language, EPUBHref: "https://catalog.example/replay-entry.epub"},
			}})

			first, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "replay-entry")
			require.NoError(t, err)
			require.True(t, first.Created)

			again, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "replay-entry")
			require.NoError(t, err)
			assert.False(t, again.Created)
			assert.False(t, again.Changed, "a replayed Add starts no preparation")
			assert.Equal(t, first.Book.ID, again.Book.ID)

			page, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
			require.NoError(t, err)
			assert.Equal(t, 1, page.Total, "Add never duplicates the entry's Book")
			require.Len(t, page.Entries, 1)
			assert.Equal(t, first.Book.ID, page.Entries[0].BookID)
		},
	}, {
		Name: "catalogue Add refuses an entry missing from the index",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")

			_, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "missing-entry")
			require.ErrorIs(t, err, domain.ErrNotFound)

			page, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
			require.NoError(t, err)
			assert.Zero(t, page.Total, "a refused Add creates no Book")
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
			assert.Empty(t, ownership["unowned"], "an entry without a Book offers Add and reports no ownership")

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

// catalogueHasStudyLanguage reports whether the derived study-language set contains
// language, ignoring case in the stored tag as the surfaces do.
func catalogueHasStudyLanguage(languages []domain.StudyLanguage, language string) bool {
	for _, candidate := range languages {
		if strings.EqualFold(candidate.Language, language) {
			return true
		}
	}
	return false
}
