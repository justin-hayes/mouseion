//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingAddressesResolveTheLanguageThenRedirectToMyBooks(t *testing.T) {
	h, cookies, _, store, owner, _ := languageContextHandler(t)
	ctx := context.Background()

	german := getAs(t, h, "/de/library", cookies)
	require.Equal(t, http.StatusOK, german.Code)
	assert.Contains(t, german.Body.String(), `<option value="de" selected>`)
	assert.NotContains(t, german.Body.String(), `href="/de/reading"`, "Reading left the shell navigation")

	italian := getAs(t, h, "/it/library", cookies)
	require.Equal(t, http.StatusOK, italian.Code)
	assert.Contains(t, italian.Body.String(), `<option value="it" selected>`)
	stored, err := store.GetStoredActiveStudyLanguage(ctx, owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "it", stored, "a prefixed view records the Last study language")

	// Unprefixed addresses redirect to the prefixed form; a valid legacy
	// ?language= names the language and is stripped. The prefixed Reading
	// address then redirects to My Books in the same language (ADR 0131).
	redirects := []struct{ name, path, want string }{
		{"reading", "/reading", "/it/reading"},
		{"reading with a valid language", "/reading?language=de", "/de/reading"},
		{"reading with an invalid language", "/reading?language=fr", "/it/reading"},
		{"switch", "/reading/switch", "/it/reading/switch"},
		{"switch with a valid language", "/reading/switch?language=de", "/de/reading/switch"},
		{"upper-case prefix", "/DE/reading", "/de/reading"},
		{"German reading", "/de/reading", "/de/library"},
		{"Italian reading", "/it/reading", "/it/library"},
		{"German switch", "/de/reading/switch", "/de/library"},
		{"Italian switch", "/it/reading/switch", "/it/library"},
	}
	for _, tc := range redirects {
		t.Run(tc.name, func(t *testing.T) {
			response := getAs(t, h, tc.path, cookies)
			require.Equal(t, http.StatusSeeOther, response.Code)
			require.Equal(t, tc.want, response.Header().Get("Location"))
		})
	}

	// The picker still resolves the retired kinds, and they land on My Books.
	reading := getAs(t, h, "/study-language?to=reading&lang=de&q=haus", cookies)
	assert.Equal(t, "/de/reading", reading.Header().Get("Location"))
	followed := getAs(t, h, reading.Header().Get("Location"), cookies)
	assert.Equal(t, "/de/library", followed.Header().Get("Location"))
}

func TestReadingActionsReturnToTheBooksLanguage(t *testing.T) {
	h, cookies, csrf, store, owner, book := languageContextHandler(t)
	ctx := context.Background()
	// The learner is last in Italian; the German Book's actions return to German.
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "it"))

	start := readingTestRequest(t, h, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "/de/library?message=")
	current, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Equal(t, book.ID, current.BookID, "the Book's language, not the Last study language, is started")

	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "it"))
	stale := readingTestRequest(t, h, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Contains(t, stale.Header().Get("Location"), "/de/library?message=")

	// End and Finish act on the active study language.
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "de"))
	end := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, end.Code)
	assert.Contains(t, end.Header().Get("Location"), "/de/library?message=")

	reanalyze := readingTestRequest(t, h, "/reading/books/"+book.ID+"/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Contains(t, reanalyze.Header().Get("Location"), "/de/library?")
}

func TestReadingRefusesAStaleBrowseOriginInAnotherLanguage(t *testing.T) {
	h, cookies, csrf, store, owner, book := languageContextHandler(t)
	ctx := context.Background()
	start := readingTestRequest(t, h, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	_, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)

	origin := "?reading=" + book.ID
	ok := getAs(t, h, "/de/reading"+origin, cookies)
	require.Equal(t, http.StatusOK, ok.Code)

	// The same origin under another language's URL restores nothing.
	other := getAs(t, h, "/it/reading"+origin, cookies)
	assert.Equal(t, http.StatusConflict, other.Code)
	assert.NotContains(t, other.Body.String(), "Language context")
}
