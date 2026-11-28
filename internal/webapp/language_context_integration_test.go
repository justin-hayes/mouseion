//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// languageContextHandler builds a learner with a German Book and an Italian
// Catalog Book, so two study languages exist, and logs them in.
func languageContextHandler(t *testing.T) (*Handler, []*http.Cookie, string, *persistence.PostgresStore, domain.User, domain.Book) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "language-context-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	owner := createAccount(t, ctx, store, "language-context", "learner-password")
	book, _, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "language-context", "Language context", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3}})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Italian catalog", URL: "https://italian.example/opds"})
	require.NoError(t, err)
	err = seedWebappCatalogueBook(t, ctx, store, owner.ID, connection.ID, "it-entry", "Un libro", "Autrice", "it")
	require.NoError(t, err)
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "de"))

	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), Analysis: analysis.NewService(store.Pool(), nil), SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	return h, cookies, csrf, store, owner, book
}

func TestMyBooksResolvesItsLanguageFromTheURL(t *testing.T) {
	h, cookies, _, store, owner, _ := languageContextHandler(t)
	ctx := context.Background()
	stored := func() string {
		t.Helper()
		language, err := store.GetStoredActiveStudyLanguage(ctx, owner.ID)
		require.NoError(t, err)
		return language
	}

	german := getAs(t, h, "/de/library", cookies)
	require.Equal(t, http.StatusOK, german.Code)
	assert.Contains(t, german.Body.String(), "Deutsch")
	assert.Contains(t, german.Body.String(), `<option value="de" selected>`)
	assert.Contains(t, german.Body.String(), "Language context")
	assert.Equal(t, "de", stored())

	// A second prefix works beside the first and becomes the Last study language.
	italian := getAs(t, h, "/it/library", cookies)
	require.Equal(t, http.StatusOK, italian.Code)
	assert.Contains(t, italian.Body.String(), `<option value="it" selected>`)
	assert.NotContains(t, italian.Body.String(), "Language context")
	assert.Equal(t, "it", stored())
	assert.Contains(t, italian.Body.String(), `href="/it/library"`, "navigation links use the Last study language")

	// Back in German: the earlier prefix renders German whatever is stored.
	again := getAs(t, h, "/de/library", cookies)
	assert.Contains(t, again.Body.String(), "Language context")
	assert.Equal(t, "de", stored())

	// An unprefixed address redirects to the prefixed form; the one-request
	// ?language= override no longer exists.
	redirects := []struct{ name, path, want string }{
		{"unprefixed", "/library?q=haus", "/de/library?q=haus"},
		{"a valid old language link keeps its language", "/library?language=it", "/it/library"},
		{"an invalid old language link uses the Last study language", "/library?language=fr&q=haus", "/de/library?q=haus"},
		{"an unknown prefix recovers under the Last study language", "/fr/library?q=haus", "/de/library?q=haus"},
		{"a different case is canonical", "/DE/library", "/de/library"},
		{"a locale suffix de-DE", "/de-DE/library?q=haus", "/de/library?q=haus"},
		{"a locale suffix de_DE", "/de_DE/library?q=haus", "/de/library?q=haus"},
		{"a locale suffix DE-de", "/DE-de/library?q=haus", "/de/library?q=haus"},
	}
	for _, tc := range redirects {
		t.Run(tc.name, func(t *testing.T) {
			response := getAs(t, h, tc.path, cookies)
			require.Equal(t, http.StatusSeeOther, response.Code)
			require.Equal(t, tc.want, response.Header().Get("Location"))
		})
	}
	assert.Equal(t, "de", stored(), "a redirect alone never switches the language")

	// The picker is a plain GET navigation that opens My Books in the chosen
	// language and drops search and paging.
	switched := getAs(t, h, "/study-language?to=my-books&lang=it&q=haus&page=2", cookies)
	assert.Equal(t, http.StatusSeeOther, switched.Code)
	assert.Equal(t, "/it/library", switched.Header().Get("Location"))
	invalid := getAs(t, h, "/study-language?to=my-books&lang=fr", cookies)
	assert.Equal(t, http.StatusBadRequest, invalid.Code)
}

func TestBookActionsReturnToTheBooksLanguage(t *testing.T) {
	h, cookies, csrf, store, owner, book := languageContextHandler(t)
	ctx := context.Background()
	// The learner is last in Italian, but the German Book's action returns to German.
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "it"))
	response := readingTestRequest(t, h, "/library/books/"+book.ID+"/previously-read", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "/de/library")
}

func TestMyBooksWithoutAStudyLanguageIsTheUnprefixedEmptyState(t *testing.T) {
	h, _, _, store, _, _ := languageContextHandler(t)
	ctx := context.Background()
	empty := createAccount(t, ctx, store, "no-language", "empty-learner-password")
	cookies, _ := loginCookies(t, h, empty.Username, "empty-learner-password")

	page := getAs(t, h, "/library", cookies)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Browse the Catalog")
	assert.NotContains(t, page.Body.String(), `id="active-study-language"`)

	prefixed := getAs(t, h, "/de/library", cookies)
	assert.Equal(t, http.StatusSeeOther, prefixed.Code)
	assert.Equal(t, "/library", prefixed.Header().Get("Location"))
}

func TestPerLanguageSurfacesWithoutAStudyLanguageRedirectToMyBooks(t *testing.T) {
	h, _, _, store, _, _ := languageContextHandler(t)
	ctx := context.Background()
	empty := createAccount(t, ctx, store, "no-language-surfaces", "empty-learner-password")
	cookies, _ := loginCookies(t, h, empty.Username, "empty-learner-password")

	for _, path := range []string{"/reading", "/reading/switch", "/vocabulary", "/vocabulary/concordance?term=haus"} {
		t.Run(path, func(t *testing.T) {
			response := getAs(t, h, path, cookies)
			require.Equal(t, http.StatusSeeOther, response.Code)
			require.Equal(t, "/library", response.Header().Get("Location"))
		})
	}
	myBooks := getAs(t, h, "/library", cookies)
	assert.Equal(t, http.StatusOK, myBooks.Code)
}
