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
			owner, connectionID, bookID := seedRemovedCatalogueBook(t, h, "owned-entry", "Owned Entry")

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
			requireMissingCatalogueEntryRefused(t, h, func(owner, connectionID string) error {
				_, err := h.Catalogue().AddCatalogueEntry(owner, connectionID, "missing-entry")
				return err
			})
		},
	}, {
		Name: "catalogue I've read this creates a Read Book with a Previously read record",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry: domain.CatalogueIndexEntry{EntryIdentifier: "read-entry", Title: "Read Entry", Author: "Entry Author", LanguageTag: Language, EPUBHref: "https://catalog.example/read-entry.epub"},
			}})
			read, err := h.Catalogue().MarkCatalogueEntryPreviouslyRead(owner, connectionID, "read-entry")
			require.NoError(t, err)
			assert.True(t, read.Created, "the unowned entry has no Book yet")
			assert.True(t, read.Changed, "recording the Book changes the learner's history")
			require.NotEmpty(t, read.Book.ID)

			detail, err := h.BookState().GetBookDetail(owner, read.Book.ID)
			require.NoError(t, err)
			assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "a Read Book is not To Read")
			assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
			assert.Equal(t, 1, detail.CompletionCount)
			assert.Equal(t, domain.ReadingCompletionPreviouslyRead, detail.LatestCompletionSource)
			assert.False(t, detail.EverCurrentReading, "the assertion never starts a reading")
			assert.Equal(t, "Read Entry", detail.Book.Title)
			assert.Equal(t, Language, detail.Book.LanguageTag)

			page, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
			require.NoError(t, err)
			require.Len(t, page.Entries, 1)
			assert.Equal(t, read.Book.ID, page.Entries[0].BookID, "the Book is linked through its catalog-entry alias")
		},
	}, {
		Name: "catalogue I've read this restores the same Book after it leaves My Books",
		Run: func(t *testing.T, h Harness) {
			owner, connectionID, bookID := seedRemovedCatalogueBook(t, h, "removed-read-entry", "Removed Read Entry")
			_, err := h.BookState().GetBookDetail(owner, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
			read, err := h.Catalogue().MarkCatalogueEntryPreviouslyRead(owner, connectionID, "removed-read-entry")
			require.NoError(t, err)
			assert.False(t, read.Created, "the alias still names the learner's Book")
			assert.True(t, read.Changed, "restoring membership changes the learner's state")
			assert.Equal(t, bookID, read.Book.ID, "I've read this restores the same Book ID")

			detail, err := h.BookState().GetBookDetail(owner, bookID)
			require.NoError(t, err)
			assert.Equal(t, domain.BookDispositionInbox, detail.Disposition)
			assert.Equal(t, domain.MyBookBucketRead, detail.WorkflowBucket())
			assert.Equal(t, 1, detail.CompletionCount)
			assert.Equal(t, domain.ReadingCompletionPreviouslyRead, detail.LatestCompletionSource)
		},
	}, {
		Name: "catalogue I've read this replay keeps one Book and one Previously read record",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry: domain.CatalogueIndexEntry{EntryIdentifier: "read-replay", Title: "Read Replay", Author: "Entry Author", LanguageTag: Language, EPUBHref: "https://catalog.example/read-replay.epub"},
			}})

			first, err := h.Catalogue().MarkCatalogueEntryPreviouslyRead(owner, connectionID, "read-replay")
			require.NoError(t, err)
			require.True(t, first.Created)

			again, err := h.Catalogue().MarkCatalogueEntryPreviouslyRead(owner, connectionID, "read-replay")
			require.NoError(t, err)
			assert.False(t, again.Created)
			assert.False(t, again.Changed, "a replayed assertion writes nothing")
			assert.Equal(t, first.Book.ID, again.Book.ID)

			detail, err := h.BookState().GetBookDetail(owner, first.Book.ID)
			require.NoError(t, err)
			assert.Equal(t, 1, detail.CompletionCount, "a replay adds no second history row")
		},
	}, {
		Name: "catalogue I've read this refuses the current reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startReading(t, h, defaultVocabulary)
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
				Entry:  domain.CatalogueIndexEntry{EntryIdentifier: "current-reading-entry", Title: "Current Reading Entry", Author: "Entry Author", LanguageTag: Language, EPUBHref: "https://catalog.example/current-reading-entry.epub"},
				BookID: bookID,
			}})

			_, err := h.Catalogue().MarkCatalogueEntryPreviouslyRead(owner, connectionID, "current-reading-entry")
			require.ErrorIs(t, err, domain.ErrBookIsCurrentReading)
			current, err := h.Store().GetCurrentReading(owner, Language)
			require.NoError(t, err)
			assert.Equal(t, bookID, current.BookID, "the refused assertion leaves the reading current")
			assert.Zero(t, requireDetail(t, h, owner, bookID).CompletionCount, "the refused assertion records no history")
		},
	}, {
		Name: "catalogue I've read this refuses an entry missing from the index",
		Run: func(t *testing.T, h Harness) {
			requireMissingCatalogueEntryRefused(t, h, func(owner, connectionID string) error {
				_, err := h.Catalogue().MarkCatalogueEntryPreviouslyRead(owner, connectionID, "missing-read-entry")
				return err
			})
		},
	}, {
		Name: "catalogue browse reports ownership only for an active Book",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
			bookID := h.Seeds().SeedBook(t, owner, BookSeed{})
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

// seedRemovedCatalogueBook seeds one unowned index entry whose Book has left My
// Books, so Add and I've read this can both prove they restore the same Book.
func seedRemovedCatalogueBook(t *testing.T, h Harness, entryIdentifier, title string) (string, string, string) {
	t.Helper()
	owner := h.Seeds().Owner()
	connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
	bookID := h.Seeds().SeedBook(t, owner, BookSeed{MetadataOnly: true})
	h.Seeds().SeedCatalogueEntries(t, owner, connectionID, []CatalogueEntrySeed{{
		Entry: domain.CatalogueIndexEntry{
			EntryIdentifier: entryIdentifier, Title: title, Author: "Entry Author",
			LanguageTag: Language, EPUBHref: "https://catalog.example/" + entryIdentifier + ".epub",
		},
		BookID: bookID,
	}})
	require.NoError(t, h.BookState().RemoveBookFromMyBooks(owner, bookID))
	return owner, connectionID, bookID
}

// requireMissingCatalogueEntryRefused asserts that a Catalog action refuses an
// entry absent from the local index and creates no Book.
func requireMissingCatalogueEntryRefused(t *testing.T, h Harness, mark func(owner, connectionID string) error) {
	t.Helper()
	owner := h.Seeds().Owner()
	connectionID := h.Seeds().SeedCatalogueConnection(t, owner, "Contract catalog")
	require.ErrorIs(t, mark(owner, connectionID), domain.ErrNotFound)
	page, err := h.Catalogue().BrowseCatalogueEntries(owner, connectionID, domain.CatalogueQuery{})
	require.NoError(t, err)
	assert.Zero(t, page.Total, "a refused action creates no Book")
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
