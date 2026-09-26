package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

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

func TestReadingLanguageQueryIsExplicitAndDoesNotChangeStoredMode(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading?language=de", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "fixture-book")
	assert.NotContains(t, response.Body.String(), "fixture-italian-goal")
	assert.Contains(t, response.Body.String(), `<option value="it" selected`)
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "it", stored)
}

func TestReadingInvalidLanguageQueryFallsBackToActiveMode(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.SetActiveStudyLanguage(context.Background(), fixtures.OwnerID, "it"))

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading?language=fr", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "fixture-italian-goal")
}

func TestActiveStudyLanguageEmptySubmissionIsNoOp(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	response := goalRequest(t, h, "/active-study-language", url.Values{
		"csrf_token": {csrf}, "language": {""}, "return_to": {"/reading"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/reading", response.Header().Get("Location"))
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "de", stored)
}

func TestActiveStudyLanguageReturnPathPreservesJourneyAnchor(t *testing.T) {
	assert.Equal(t, "/reading#journey-book-book-1", activeStudyLanguageReturnPath("/reading#journey-book-book-1", "de"))
}

func TestActiveStudyLanguageChangeOverridesOneRequestReadingLanguage(t *testing.T) {
	assert.Equal(t, "/reading", activeStudyLanguageReturnPath("/reading?language=de", "it"))
}
