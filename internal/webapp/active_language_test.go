package webapp

import (
	"bytes"
	"context"
	"mime/multipart"
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
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, ""))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.True(t, strings.Contains(response.Body.String(), `<option value="it" selected`), "shell did not render the most recently activated language: %s", response.Body.String())
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "", stored)
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

func TestDeliberateLanguageChangeLandsAtMyBooksAndResetsReturnState(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	response := goalRequest(t, h, "/active-study-language", url.Values{
		"csrf_token": {csrf + ".de"}, "language": {"it"}, "return_to": {"/vocabulary/concordance?term=haus&page=3"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library", response.Header().Get("Location"), "old screen, query, and return state must not survive a language change")
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "it", stored)
}

func TestLanguageChangeFromStaleTabStillTakesEffect(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	response := goalRequest(t, h, "/active-study-language", url.Values{"csrf_token": {csrf + ".de"}, "language": {"de"}}, cookies)
	assert.Equal(t, "/library", response.Header().Get("Location"))
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "de", stored, "the deliberate switch is language-agnostic and must not be rejected as stale")
}

func TestActiveStudyLanguageEmptySubmissionIsNoOpAtMyBooks(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	response := goalRequest(t, h, "/active-study-language", url.Values{
		"csrf_token": {csrf}, "language": {""}, "return_to": {"/reading"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/library", response.Header().Get("Location"))
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "de", stored)
}

func TestSupportedOldLanguageRequestsRecoverToMyBooksWithoutSwitchingBack(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	for _, path := range []string{
		"/reading?language=de",
		"/library?language=de&q=Dampf&page=2",
		"/vocabulary?language=de&q=ha&reading=fixture-book&page=2",
		"/vocabulary/concordance?language=de&mode=effective&term=haus&upos=NOUN",
		"/vocabulary/concordance/sentence?language=de&book=fixture-book",
		"/vocabulary/import?language=de",
		"/reading/books/fixture-book/lemma-review?language=de",
	} {
		t.Run(path, func(t *testing.T) {
			response := getAs(t, h, path, cookies)
			assert.Equal(t, http.StatusSeeOther, response.Code)
			assert.Equal(t, languageChangedPath(), response.Header().Get("Location"))
		})
	}
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "it", stored, "recovery never switches the selection back")

	notice := getAs(t, h, languageChangedPath(), cookies)
	assert.Equal(t, http.StatusOK, notice.Code)
	assert.Contains(t, notice.Body.String(), "Your study language changed")
}

func TestRequestNamingTheActiveLanguageIsNotStale(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	response := getAs(t, h, "/reading?language=it", cookies)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "fixture-italian-goal")
}

func TestRenderedFormsCarryTheActiveLanguage(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	body := getAs(t, h, "/library", cookies).Body.String()
	assert.Contains(t, body, `name="csrf_token" value="`+csrf+`.de"`)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	body = getAs(t, h, "/library", cookies).Body.String()
	assert.Contains(t, body, `name="csrf_token" value="`+csrf+`.it"`)
}

func TestStaleLanguageSubmissionRecoversWithoutMutation(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	before, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	require.NotEmpty(t, before.BookID)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	for _, path := range []string{
		"/reading/stop", "/reading/finish", "/reading/books/" + fixtures.BookID + "/start",
		"/reading/books/" + fixtures.BookID + "/switch", "/library/books/" + fixtures.BookID + "/to-read",
		"/library/books/" + fixtures.BookID + "/previously-read",
	} {
		t.Run(path, func(t *testing.T) {
			response := goalRequest(t, h, path, url.Values{
				"csrf_token":                   {csrf + ".de"},
				"expected_current_book_id":     {before.BookID},
				"expected_current_snapshot_id": {before.SnapshotID},
			}, cookies)
			assert.Equal(t, http.StatusSeeOther, response.Code)
			assert.Equal(t, languageChangedPath(), response.Header().Get("Location"))
		})
	}
	after, err := store.GetPrimaryGoal(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, before, after, "a stale old-language submission must not change the reading")
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "it", stored)
}

func TestStaleLanguageImportRecoversWithoutImporting(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	require.NoError(t, form.WriteField("csrf_token", csrf+".de"))
	file, err := form.CreateFormFile("vocabulary_file", "known.txt")
	require.NoError(t, err)
	_, err = file.Write([]byte("haus\nwort\n"))
	require.NoError(t, err)
	require.NoError(t, form.Close())
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/vocabulary/import", &body)
	request.Header.Set("Content-Type", form.FormDataContentType())
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, languageChangedPath(), response.Header().Get("Location"))
}

func TestSubmissionsWithoutLanguageIdentityKeepWorking(t *testing.T) {
	h, cookies, csrf, _ := goalFixtureSession(t)
	response := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.NotEqual(t, languageChangedPath(), response.Header().Get("Location"))
	forged := goalRequest(t, h, "/reading/stop", url.Values{"csrf_token": {"forged.de"}}, cookies)
	assert.Equal(t, http.StatusForbidden, forged.Code, "the language suffix never replaces the CSRF secret")
}

func TestSupportedConcordanceAndBrowseURLsCarryLanguage(t *testing.T) {
	lookup := domain.ConcordanceLookup{Language: "de", Mode: "effective", Term: "haus", UPOS: "NOUN"}
	assert.Contains(t, vocabularyConcordancePageURL(2, lookup), "language=de")
	assert.Contains(t, vocabularyConcordanceURL("de", "haus", "NOUN", "book-1"), "language=de")
	assert.Contains(t, vocabularyBrowsePageURL(2, domain.VocabularyBrowsePage{Language: "de"}, "ha"), "language=de")
	assert.Equal(t, "/vocabulary/concordance?mode=surface&page=1&term=x", vocabularyConcordancePageURL(1, domain.ConcordanceLookup{Mode: "surface", Term: "x"}))
}
