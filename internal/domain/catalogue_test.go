package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowseCataloguePageFiltersFoldsAndPages(t *testing.T) {
	entries := []CatalogueIndexEntry{
		{EntryIdentifier: "z", Title: "Zulu", Author: "Anna", LanguageTag: "de"},
		{EntryIdentifier: "odyssee", Title: "Odyssée", Author: "Hómēr", LanguageTag: "fr"},
		{EntryIdentifier: "anna", Title: "Atlas", Author: "Anna", LanguageTag: "de"},
		{EntryIdentifier: "beta", Title: "Atlas", Author: "Bernd", LanguageTag: "de"},
	}
	owned := map[string]string{"odyssee": "book-odyssee"}

	t.Run("title then author then identity order with paging", func(t *testing.T) {
		first := BrowseCataloguePage(entries, nil, CatalogueQuery{Limit: 2})
		require.Len(t, first.Entries, 2)
		assert.Equal(t, 4, first.Total)
		assert.Equal(t, []string{"Atlas", "Atlas"}, []string{first.Entries[0].Index.Title, first.Entries[1].Index.Title})
		assert.Equal(t, []string{"Anna", "Bernd"}, []string{first.Entries[0].Index.Author, first.Entries[1].Index.Author})

		second := BrowseCataloguePage(entries, nil, CatalogueQuery{Offset: 2, Limit: 2})
		require.Len(t, second.Entries, 2)
		assert.Equal(t, 4, second.Total)
		assert.Equal(t, []string{"Odyssée", "Zulu"}, []string{second.Entries[0].Index.Title, second.Entries[1].Index.Title})

		past := BrowseCataloguePage(entries, nil, CatalogueQuery{Offset: 9, Limit: 2})
		assert.Empty(t, past.Entries)
		assert.Equal(t, 4, past.Total)
	})

	t.Run("language filter uses normalized tags", func(t *testing.T) {
		page := BrowseCataloguePage(entries, nil, CatalogueQuery{Language: "DE", Limit: 10})
		require.Len(t, page.Entries, 3)
		for _, entry := range page.Entries {
			assert.Equal(t, "de", entry.Index.LanguageTag)
		}
	})

	t.Run("search matches the Go key without accents or case", func(t *testing.T) {
		page := BrowseCataloguePage(entries, owned, CatalogueQuery{Search: "ODYSSEE", Limit: 10})
		require.Len(t, page.Entries, 1)
		assert.Equal(t, "odyssee", page.Entries[0].Index.EntryIdentifier)
		assert.Equal(t, "book-odyssee", page.Entries[0].BookID)

		byAuthor := BrowseCataloguePage(entries, nil, CatalogueQuery{Search: "homer", Limit: 10})
		require.Len(t, byAuthor.Entries, 1)
		assert.Equal(t, "odyssee", byAuthor.Entries[0].Index.EntryIdentifier)
	})

	t.Run("search and language narrow together", func(t *testing.T) {
		page := BrowseCataloguePage(entries, nil, CatalogueQuery{Search: "atlas", Language: "de", Limit: 10})
		require.Len(t, page.Entries, 2)

		none := BrowseCataloguePage(entries, nil, CatalogueQuery{Search: "atlas", Language: "fr", Limit: 10})
		assert.Empty(t, none.Entries)
		assert.Zero(t, none.Total)
	})

	t.Run("ownership is reported only through the supplied map", func(t *testing.T) {
		page := BrowseCataloguePage(entries, owned, CatalogueQuery{Search: "odyssee", Limit: 10})
		require.Len(t, page.Entries, 1)
		assert.Equal(t, "book-odyssee", page.Entries[0].BookID)

		unowned := BrowseCataloguePage(entries, nil, CatalogueQuery{Search: "odyssee", Limit: 10})
		require.Len(t, unowned.Entries, 1)
		assert.Empty(t, unowned.Entries[0].BookID)
	})
}

func TestBrowseCataloguePageOrdersByFoldedTitleThenAuthor(t *testing.T) {
	page := BrowseCataloguePage([]CatalogueIndexEntry{
		{EntryIdentifier: "zulu", Title: "Zulu", Author: "Anna", LanguageTag: "fr"},
		{EntryIdentifier: "emile", Title: "Émile", Author: "Rousseau", LanguageTag: "fr"},
		{EntryIdentifier: "atlas-zed", Title: "Atlas", Author: "Zed", LanguageTag: "fr"},
		{EntryIdentifier: "atlas-anna", Title: "Atlas", Author: "Anna", LanguageTag: "fr"},
	}, nil, CatalogueQuery{Limit: 10})
	require.Len(t, page.Entries, 4)
	assert.Equal(t, []string{"atlas-anna", "atlas-zed", "emile", "zulu"}, []string{
		page.Entries[0].Index.EntryIdentifier,
		page.Entries[1].Index.EntryIdentifier,
		page.Entries[2].Index.EntryIdentifier,
		page.Entries[3].Index.EntryIdentifier,
	}, "title order ignores accents and still uses author as the tie-break")
}
