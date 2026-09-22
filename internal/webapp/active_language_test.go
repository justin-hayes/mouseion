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

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/journey", nil)
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

func TestActiveStudyLanguageEmptySubmissionIsNoOp(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	response := goalRequest(t, h, "/active-study-language", url.Values{
		"csrf_token": {csrf}, "language": {""}, "return_to": {"/journey"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/journey", response.Header().Get("Location"))
	stored, err := store.GetStoredActiveStudyLanguage(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	assert.Equal(t, "de", stored)
}

func TestActiveStudyLanguageReturnPathPreservesJourneyAnchor(t *testing.T) {
	assert.Equal(t, "/journey#journey-book-book-1", activeStudyLanguageReturnPath("/journey#journey-book-book-1", "de"))
}
