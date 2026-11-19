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
	h, cookies, csrf, _ := readingFixtureSessionWith(t, func(services *Services) {
		catalog, err := fixtures.NewClassicalCatalog()
		require.NoError(t, err)
		services.Classical = catalog
		services.AnalysisInsights = fixtures.ClassicalInsights{}
		services.ClassicalPreparation = fixtures.ClassicalPreparation{}
	})

	catalogs := catalogPage(t, h, cookies, "/catalogs")
	require.Equal(t, http.StatusOK, catalogs.Code)
	assert.Contains(t, catalogs.Body.String(), "Classical catalog")

	index := catalogPage(t, h, cookies, "/catalogs/classical")
	require.Equal(t, http.StatusOK, index.Code)
	for _, want := range []string{"Homer", "Xenophon", "Iliad", "Hellenica"} {
		assert.Contains(t, index.Body.String(), want)
	}

	retired := catalogPage(t, h, cookies, "/catalogs/classical/authors/Homer")
	require.Equal(t, http.StatusMovedPermanently, retired.Code)
	assert.Equal(t, "/catalogs/classical#author-Homer", retired.Header().Get("Location"))

	work := catalogPage(t, h, cookies, "/catalogs/classical/works/fixture-iliad")
	require.Equal(t, http.StatusOK, work.Code)
	for _, want := range []string{"Add the whole work", "Book 1", "Book 2", "Book 3"} {
		assert.Contains(t, work.Body.String(), want)
	}

	added := readingTestRequest(t, h, "/catalogs/classical/works/fixture-iliad/add", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, added.Code)
	assert.Contains(t, added.Header().Get("Location"), url.QueryEscape("Added 3 Books to My Books in Ancient Greek. Your study language is unchanged."))

	owned := catalogPage(t, h, cookies, "/catalogs/classical/works/fixture-iliad")
	require.Equal(t, http.StatusOK, owned.Code)
	assert.Contains(t, owned.Body.String(), "In My Books")

	landing := catalogPage(t, h, cookies, "/catalogs/classical/works/fixture-iliad/in-my-books")
	require.Equal(t, http.StatusSeeOther, landing.Code)
	libraryURL := landing.Header().Get("Location")
	libraryURL = strings.SplitN(libraryURL, "#", 2)[0]
	library := catalogPage(t, h, cookies, libraryURL)
	require.Equal(t, http.StatusOK, library.Code)
	for _, want := range []string{
		`id="work-row-fixture-iliad"`,
		`aria-label="Books of Iliad"`,
		"all 3 books",
		"library-work__cell-number",
		"By Homer",
	} {
		assert.Contains(t, library.Body.String(), want)
	}

	moved := readingTestRequest(t, h, "/library/works/fixture-iliad/to-read", url.Values{
		"csrf_token": {csrf},
		"return_to":  {"/library"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, moved.Code)

	chooser := catalogPage(t, h, cookies, "/reading")
	require.Equal(t, http.StatusOK, chooser.Code)
	for _, want := range []string{
		"Choose a To Read book in Ancient Greek",
		"reading-chooser-work",
		"99% or above",
		"97% to below 99%",
		"Below 95%",
		"Iliad, Book 1",
	} {
		assert.Contains(t, chooser.Body.String(), want)
	}
	assert.Equal(t, 3, strings.Count(chooser.Body.String(), `class="reading-chooser-band-book"`))
}
