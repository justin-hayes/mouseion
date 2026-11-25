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
// Classical catalog from Catalogs through My Books to Reading's chooser.
func TestClassicalCatalogFixtureJourney(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := fixtures.NewClassicalCatalog()
		require.NoError(t, err)
		services.Classical = catalog
		services.AnalysisInsights = fixtures.ClassicalInsights{}
		services.ClassicalPreparation = fixtures.ClassicalPreparation{}
	})
	store.SeedWithdrawnClassicalBooks()

	catalogs := catalogPage(t, h, cookies, "/catalogs")
	require.Equal(t, http.StatusOK, catalogs.Code)
	assert.Contains(t, catalogs.Body.String(), "Classical catalog")

	index := catalogPage(t, h, cookies, "/catalogs/classical")
	require.Equal(t, http.StatusOK, index.Code)
	for _, want := range []string{"Homer", "Xenophon", "Iliad", "Hellenica"} {
		assert.Contains(t, index.Body.String(), want)
	}
	require.Contains(t, index.Body.String(), `action="/catalogs/classical/units/fixture-iliad/add"`)
	require.NotContains(t, index.Body.String(), "In My Books", "leftover per-book ownership is not current Edition ownership")

	retired := catalogPage(t, h, cookies, "/catalogs/classical/authors/Homer")
	require.Equal(t, http.StatusMovedPermanently, retired.Code)
	assert.Equal(t, "/catalogs/classical#author-Homer", retired.Header().Get("Location"))

	work := catalogPage(t, h, cookies, "/catalogs/classical/works/fixture-iliad")
	require.Equal(t, http.StatusOK, work.Code)
	require.Contains(t, work.Body.String(), "Contents of Iliad")
	require.Equal(t, 1, strings.Count(work.Body.String(), "Add and prepare the work"))
	require.NotContains(t, work.Body.String(), "/units/fixture-iliad-1/")

	added := readingTestRequest(t, h, "/catalogs/classical/units/fixture-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 1 Book to My Books in Ancient Greek. Your study language is unchanged. The text is already prepared."))

	owned := catalogPage(t, h, cookies, "/catalogs/classical/works/fixture-iliad")
	require.Equal(t, http.StatusOK, owned.Code)
	assert.Contains(t, owned.Body.String(), "In My Books")

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
	assert.NotContains(t, library.Body.String(), "library-work", "My Books lists each Book as an ordinary row")

	// Add already put the Books in To Read, so the chooser lists them without
	// another move.
	chooser := catalogPage(t, h, cookies, "/reading")
	require.Equal(t, http.StatusOK, chooser.Code)
	for _, want := range []string{
		"Choose a To Read book in Ancient Greek",
		"Iliad, Book 1",
		"Iliad, Book 2",
		"Iliad, Book 3",
		"99% or above",
		"97% to below 99%",
		"Below 95%",
	} {
		assert.Contains(t, chooser.Body.String(), want)
	}
	assert.Equal(t, 4, strings.Count(chooser.Body.String(), `class="reading-chooser-book"`), "each Book is its own chooser entry")
	assert.NotContains(t, chooser.Body.String(), "reading-chooser-work")
}
