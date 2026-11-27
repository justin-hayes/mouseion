package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedShellLazilyDefaultsWithoutWritingStoredLanguage(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, ""))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/jobs", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), `id="active-study-language"`, "job status has no page language and no picker")
	assert.Contains(t, response.Body.String(), `href="/it/library"`, "navigation uses the most recently activated language")
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Empty(t, stored)
}

func getAs(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	return response
}

// prefixedMyBooksRedirect reports where an unprefixed GET of My Books was sent:
// the same page under the Last study language. Test helpers follow it so a test
// that does not care about the prefix still reads the page.
func prefixedMyBooksRedirect(path string, response *httptest.ResponseRecorder) (string, bool) {
	if response.Code != http.StatusSeeOther || (path != "/library" && !strings.HasPrefix(path, "/library?")) {
		return "", false
	}
	location := response.Header().Get("Location")
	_, rest, ok := strings.Cut(strings.TrimPrefix(location, "/"), "/")
	return location, ok && (rest == "library" || strings.HasPrefix(rest, "library?"))
}

func TestRequestNamingTheActiveLanguageIsNotStale(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	response := getAs(t, h, "/it/reading", cookies)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "fixture-italian-goal")
}

func TestRenderedFormsCarryTheBareCSRFToken(t *testing.T) {
	h, cookies, csrf, _ := readingFixtureSession(t)
	body := getAs(t, h, "/de/library", cookies).Body.String()
	assert.Contains(t, body, `name="csrf_token" value="`+csrf+`"`)
}

// The URL, or the Book a Book-addressed action names, is the language
// (ADR 0131): the Last study language never refuses a submission.
func TestSubmissionsIgnoreTheLastStudyLanguage(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	for _, path := range []string{
		"/reading/end", "/reading/finish", "/reading/books/" + fixtures.BookID + "/start",
		"/reading/books/" + fixtures.BookID + "/switch", "/library/books/" + fixtures.BookID + "/to-read",
		"/library/books/" + fixtures.BookID + "/previously-read",
	} {
		t.Run(path, func(t *testing.T) {
			response := readingTestRequest(t, h, path, url.Values{
				"csrf_token": {csrf},
			}, cookies)
			assert.NotEqual(t, http.StatusForbidden, response.Code)
			assert.NotContains(t, response.Header().Get("Location"), "study language changed")
		})
	}
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "it", stored)
}

func TestForgedCSRFTokenIsRefused(t *testing.T) {
	h, cookies, _, _ := readingFixtureSession(t)
	forged := readingTestRequest(t, h, "/reading/end", url.Values{"csrf_token": {"forged"}}, cookies)
	assert.Equal(t, http.StatusForbidden, forged.Code)
}

func TestSupportedConcordanceAndBrowseURLsCarryLanguage(t *testing.T) {
	lookup := domain.ConcordanceLookup{Language: "de", Term: "haus", Match: domain.ConcordanceMatchLemma, UPOS: "NOUN"}
	assert.Equal(t, "/de/vocabulary/concordance?as=lemma&page=2&term=haus&upos=NOUN", vocabularyConcordancePageURL(2, lookup))
	assert.Equal(t, "/de/vocabulary/concordance?as=lemma&page=1&term=haus&upos=NOUN", vocabularyConcordanceURL("de", "haus", "NOUN"))
	assert.Equal(t, "/de/reading?page=2&q=ha", vocabularyBrowsePageURL(2, domain.VocabularyBrowsePage{Language: "de"}, "ha"))
	assert.Equal(t, "/vocabulary/concordance?page=1&term=x", vocabularyConcordancePageURL(1, domain.ConcordanceLookup{Term: "x"}))
}
