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

func TestLearnerCorrectsOneExactOccurrenceAndStartReadingRemainsAvailable(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.ClearPrimaryGoal(context.Background(), fixtures.OwnerID, "de", fixtures.BookID))
	path := "/reading/books/" + fixtures.BookID + "/lemma-review?form=Weg"
	get := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	for _, cookie := range cookies {
		get.AddCookie(cookie)
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, get)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Der Weg führt zum Haus.")
	assert.Contains(t, page.Body.String(), "Analyzer lemma:")
	assert.Equal(t, 2, strings.Count(page.Body.String(), "Save correction for this occurrence"))

	correct := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{
		"csrf_token": {csrf}, "form": {"Weg"}, "analysis_run_id": {fixtures.ResultRunID},
		"source_document_id": {"fixture-unit"}, "start_offset": {"4"}, "end_offset": {"7"},
		"surface": {"Weg"}, "raw_lemma": {"Weg"}, "canonical_lemma": {"weg"}, "upos": {"NOUN"},
		"expected_corrected_lemma": {""}, "lemma": {"Pfad"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, correct.Code)
	occurrences, err := store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, fixtures.BookID, "Weg")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	assert.Equal(t, "pfad", occurrences[0].CorrectedLemma)
	assert.Empty(t, occurrences[1].CorrectedLemma, "same-surface occurrence must remain untouched")
	stale := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{
		"csrf_token": {csrf}, "form": {"Weg"}, "analysis_run_id": {fixtures.ResultRunID},
		"source_document_id": {"fixture-unit"}, "start_offset": {"4"}, "end_offset": {"7"},
		"surface": {"Weg"}, "raw_lemma": {"Weg"}, "canonical_lemma": {"weg"}, "upos": {"NOUN"},
		"expected_corrected_lemma": {""}, "lemma": {"Wand"},
	}, cookies)
	assert.Equal(t, http.StatusConflict, stale.Code, "a stale review form cannot overwrite a later correction")

	start := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "is+now+your+current+reading")
}
