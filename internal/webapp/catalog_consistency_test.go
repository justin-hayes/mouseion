package webapp

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/require"
)

// catalogTwoSourceSession serves a Catalog whose Classical and OPDS sections
// each hold fourteen cards, so both page.
func catalogTwoSourceSession(t *testing.T, opds []domain.CatalogueIndexEntry, definitions []classical.Definition) (http.Handler, []*http.Cookie, string) {
	t.Helper()
	catalog, err := classical.NewCatalog(append(legacyClassicalTombstones(), definitions...))
	require.NoError(t, err)
	h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
		services.Classical = catalog
		store, ok := services.Store.Shell.(*fixtures.Store)
		require.True(t, ok)
		require.NoError(t, store.SeedCatalogueEntries(fixtures.OwnerID, "fixture-connection", opds))
	})
	return h, cookies, csrf
}

func TestCatalogSectionsPageIndependentlyOfEachOther(t *testing.T) {
	var opds []domain.CatalogueIndexEntry
	var definitions []classical.Definition
	for i := range 14 {
		opds = append(opds, domain.CatalogueIndexEntry{ConnectionID: "fixture-connection", EntryIdentifier: fmt.Sprintf("o-%02d", i), Title: fmt.Sprintf("Seite %02d", i), LanguageTag: "de"})
		key := fmt.Sprintf("work-%02d", i)
		definitions = append(definitions, catalogFixtureDefinition(key, key, fmt.Sprintf("Title %02d", i), "Author", "Τίτλος", key+"-edition", "1.1.1", "1.1.10", nil, 1, 500, false, false))
	}
	h, cookies, _ := catalogTwoSourceSession(t, opds, definitions)

	body := catalogPage(t, h, cookies, "/catalogs?language=&page_classical=2").Body.String()
	t.Run("the Classical section moves to its second page", func(t *testing.T) {
		require.Contains(t, body, "Title 13")
		require.NotContains(t, body, "Title 00")
	})
	t.Run("the OPDS section stays on its first page", func(t *testing.T) {
		require.Contains(t, body, "Seite 00")
		require.NotContains(t, body, "Seite 13")
	})
	t.Run("the OPDS paging link keeps the Classical page", func(t *testing.T) {
		require.Contains(t, body, `page_classical=2&amp;page_fixture-connection=2`)
	})

	both := catalogPage(t, h, cookies, "/catalogs?language=&page_classical=2&page_fixture-connection=2").Body.String()
	t.Run("each section shows its own second page together", func(t *testing.T) {
		require.Contains(t, both, "Title 13")
		require.Contains(t, both, "Seite 13")
		require.NotContains(t, both, "Title 00")
		require.NotContains(t, both, "Seite 00")
	})
}

func TestCatalogCardsOrderByTitleThenAuthorInEverySource(t *testing.T) {
	opds := []domain.CatalogueIndexEntry{
		{ConnectionID: "fixture-connection", EntryIdentifier: "o-1", Title: "Gleich", Author: "Zweig", LanguageTag: "de"},
		{ConnectionID: "fixture-connection", EntryIdentifier: "o-2", Title: "Gleich", Author: "Anders", LanguageTag: "de"},
		{ConnectionID: "fixture-connection", EntryIdentifier: "o-3", Title: "Abc", Author: "Zuletzt", LanguageTag: "de"},
	}
	definitions := []classical.Definition{
		catalogFixtureDefinition("same-z", "same-z", "Same", "Zeta", "Τίτλος", "same-z-edition", "1.1.1", "1.1.10", nil, 1, 500, false, false),
		catalogFixtureDefinition("same-a", "same-a", "Same", "Alpha", "Τίτλος", "same-a-edition", "1.1.1", "1.1.10", nil, 1, 500, false, false),
	}
	h, cookies, _ := catalogTwoSourceSession(t, opds, definitions)
	body := catalogPage(t, h, cookies, "/catalogs?language=").Body.String()

	t.Run("OPDS cards with one title order by author", func(t *testing.T) {
		abc, anders, zweig := strings.Index(body, "Abc"), strings.Index(body, "Anders"), strings.Index(body, "Zweig")
		require.Positive(t, abc)
		require.Less(t, abc, anders, "a different title orders first")
		require.Less(t, anders, zweig)
	})
	t.Run("Classical cards with one title order by author", func(t *testing.T) {
		alpha, zeta := strings.Index(body, "Alpha"), strings.Index(body, "Zeta")
		require.Positive(t, alpha)
		require.Less(t, alpha, zeta)
	})
}

func TestCatalogLanguageOptionsNameIndexLanguagesWhenTheServiceKnowsThem(t *testing.T) {
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "/catalogs", nil)
	require.NoError(t, err)
	options := catalogLanguageOptions(request, "", nil, []string{"fr", "xx"}, map[string]string{"fr": "French"})
	labels := map[string]string{}
	for _, option := range options {
		labels[option.Value] = option.Label
	}
	t.Run("a known index language uses its display name", func(t *testing.T) {
		require.Equal(t, "French", labels["fr"])
	})
	t.Run("an unknown index language falls back to its code", func(t *testing.T) {
		require.Equal(t, "xx", labels["xx"])
	})
}

// catalogAddReturn performs one card Add and returns its redirect.
func catalogAddReturn(t *testing.T, h http.Handler, cookies []*http.Cookie, path string, form url.Values) *url.URL {
	t.Helper()
	response := readingTestRequest(t, h, path, form, cookies)
	require.Equal(t, http.StatusSeeOther, response.Code)
	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	return location
}

func TestCatalogAddConfirmationBehavesTheSameForBothSources(t *testing.T) {
	opds := []domain.CatalogueIndexEntry{{ConnectionID: "fixture-connection", EntryIdentifier: "o-1", Title: "Ein Buch", LanguageTag: "de"}}
	h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := fixtures.NewClassicalCatalog()
		require.NoError(t, err)
		services.Classical = catalog
		store, ok := services.Store.Shell.(*fixtures.Store)
		require.True(t, ok)
		require.NoError(t, store.SeedCatalogueEntries(fixtures.OwnerID, "fixture-connection", opds))
	})

	for _, filter := range []string{"de", "grc", ""} {
		opdsLocation := catalogAddReturn(t, h, cookies, "/catalogs/sources/fixture-connection/add", url.Values{
			"csrf_token": {csrf}, "entry_identifier": {"o-1"}, "catalog_language": {filter},
		})
		classicalLocation := catalogAddReturn(t, h, cookies, "/catalogs/classical/units/fixture-iliad/add", url.Values{
			"csrf_token": {csrf}, "return": {"index"}, "catalog_language": {filter},
		})
		name := "filter " + filter
		t.Run(name+" is kept by both", func(t *testing.T) {
			require.Equal(t, "/catalogs", opdsLocation.Path)
			require.Equal(t, "/catalogs", classicalLocation.Path)
			require.True(t, opdsLocation.Query().Has("language"))
			require.Equal(t, filter, opdsLocation.Query().Get("language"))
			require.Equal(t, filter, classicalLocation.Query().Get("language"))
		})
		t.Run(name+" confirms with the same message form", func(t *testing.T) {
			require.True(t, strings.HasPrefix(opdsLocation.Query().Get("message"), "Added") || strings.Contains(opdsLocation.Query().Get("message"), "already"))
			require.NotContains(t, classicalLocation.Query().Get("message"), "study language")
			require.NotEmpty(t, opdsLocation.Query().Get("added_book"))
			require.NotEmpty(t, classicalLocation.Query().Get("added_book"))
		})
	}
}
