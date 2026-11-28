package webapp

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassicalCatalogFixtureJourney walks the fixture server's built-in
// Classical catalog from Catalogs through Reading to Reading's chooser.
func TestClassicalCatalogFixtureJourney(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := fixtures.NewClassicalCatalog()
		require.NoError(t, err)
		services.Classical = catalog
		services.AnalysisInsights = fixtures.ClassicalInsights{}
		services.ClassicalPreparation = fixtures.ClassicalPreparation{}
	})
	store.SeedWithdrawnClassicalBooks()

	catalogs := catalogPage(t, h, cookies, "/catalogs?language=grc")
	require.Equal(t, http.StatusOK, catalogs.Code)
	for _, want := range []string{"Classical catalog", "Iliad", "Hellenica", "Homer", "Xenophon"} {
		assert.Contains(t, catalogs.Body.String(), want)
	}
	require.Contains(t, catalogs.Body.String(), `action="/catalogs/classical/units/fixture-iliad/add"`)
	require.NotContains(t, catalogs.Body.String(), "In Reading", "leftover per-book ownership is not current Edition ownership")

	for _, retiredPath := range []string{"/catalogs/classical", "/catalogs/classical/authors/Homer"} {
		retired := catalogPage(t, h, cookies, retiredPath)
		require.Equal(t, http.StatusMovedPermanently, retired.Code)
		assert.Equal(t, "/catalogs?language=grc", retired.Header().Get("Location"))
	}

	work := catalogPage(t, h, cookies, "/catalogs/classical/works/fixture-iliad")
	require.Equal(t, http.StatusOK, work.Code)
	require.Contains(t, work.Body.String(), "Contents of Iliad")
	require.Equal(t, 1, strings.Count(work.Body.String(), `action="/catalogs/classical/units/fixture-iliad/add"`))
	require.NotContains(t, work.Body.String(), "/units/fixture-iliad-1/")

	added := readingTestRequest(t, h, "/catalogs/classical/units/fixture-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 1 Book to Reading in Ancient Greek. Your study language is unchanged. The text is already prepared."))

	owned := catalogPage(t, h, cookies, "/catalogs/classical/works/fixture-iliad")
	require.Equal(t, http.StatusOK, owned.Code)
	assert.Contains(t, owned.Body.String(), "In Reading")

	landing := catalogPage(t, h, cookies, "/catalogs/classical/books/fixture-classical-fixture-iliad-fixture-learner/in-my-books")
	require.Equal(t, http.StatusSeeOther, landing.Code)
	libraryURL := landing.Header().Get("Location")
	libraryURL = strings.SplitN(libraryURL, "#", 2)[0]
	library := catalogPage(t, h, cookies, libraryURL)
	require.Equal(t, http.StatusOK, library.Code)
	for _, want := range []string{
		"Iliad, Book 1",
		"Iliad, Book 2",
		"Iliad, Book 3",
		"By Homer",
	} {
		assert.Contains(t, library.Body.String(), want)
	}
	assert.NotContains(t, library.Body.String(), "library-work", "Reading lists each Book as an ordinary row")

	// Add already put the Books in To Read, so Reading lists them without
	// another move.
	toRead := catalogPage(t, h, cookies, "/grc/reading?disposition=to_read")
	require.Equal(t, http.StatusOK, toRead.Code)
	for _, want := range []string{
		"Iliad, Book 1",
		"Iliad, Book 2",
		"Iliad, Book 3",
	} {
		assert.Contains(t, toRead.Body.String(), want)
	}
	assert.Equal(t, 4, strings.Count(toRead.Body.String(), `class="library-book"`), "each Book is its own row")
}
