package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
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
	assert.Equal(t, 2, strings.Count(page.Body.String(), "Preview correction"))

	preview := func(target, decision, lemma string, additional ...string) *httptest.ResponseRecorder {
		values := url.Values{"csrf_token": {csrf}, "stage": {"preview"}, "form": {"Weg"}, "target": {target}, "decision": {decision}, "lemma": {lemma}}
		if len(additional) > 0 {
			values["also"] = additional
		}
		return goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", values, cookies)
	}
	confirm := func(response *httptest.ResponseRecorder, lemma string, indices ...string) *httptest.ResponseRecorder {
		match := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(response.Body.String())
		require.Len(t, match, 2, "preview includes its stale-state fingerprint")
		values := url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {lemma}, "fingerprint": {match[1]}, "selected": indices}
		return goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", values, cookies)
	}
	proposal := preview("0", "correct", "Pfad")
	require.Equal(t, http.StatusOK, proposal.Code)
	assert.Contains(t, proposal.Body.String(), "This is a preview only")
	fingerprint := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(proposal.Body.String())
	require.Len(t, fingerprint, 2)
	tampered := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {"Pfad"}, "fingerprint": {fingerprint[1]}, "selected": {"0", "1"}}, cookies)
	assert.Equal(t, http.StatusConflict, tampered.Code, "the confirmed selection must match the reviewed proposal")
	occurrences, err := store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, fixtures.BookID, "Weg")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	assert.Empty(t, occurrences[0].CorrectedLemma, "preview is not a decision")
	require.Equal(t, http.StatusSeeOther, confirm(proposal, "Pfad", "0").Code)
	occurrences, err = store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, fixtures.BookID, "Weg")
	require.NoError(t, err)
	assert.Equal(t, "pfad", occurrences[0].CorrectedLemma)
	assert.Empty(t, occurrences[1].CorrectedLemma, "same-surface occurrence must remain untouched")
	multi := preview("0", "correct", "Pfad", "1")
	require.Contains(t, multi.Body.String(), `value="0"`)
	assert.Contains(t, multi.Body.String(), `value="1"`)
	require.Equal(t, http.StatusSeeOther, confirm(multi, "Pfad", "0", "1").Code)
	occurrences, err = store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, fixtures.BookID, "Weg")
	require.NoError(t, err)
	assert.Equal(t, "pfad", occurrences[1].CorrectedLemma, "explicit multi-occurrence selection applies the reviewed decision to the selected peer")
	stalePreview := preview("0", "correct", "Wand")
	require.Equal(t, http.StatusOK, stalePreview.Code)
	updated := preview("0", "correct", "Wand")
	require.Equal(t, http.StatusSeeOther, confirm(updated, "Wand", "0").Code)
	staleFingerprint := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(stalePreview.Body.String())
	stale := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {"Wand"}, "fingerprint": {staleFingerprint[1]}, "selected": {"0"}}, cookies)
	assert.Equal(t, http.StatusConflict, stale.Code, "a changed learner decision invalidates its preview")
	exclusion := preview("1", "exclude", "")
	exclusionConfirm := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(exclusion.Body.String())
	require.Len(t, exclusionConfirm, 2)
	exclude := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"exclude"}, "fingerprint": {exclusionConfirm[1]}, "selected": {"1"}}, cookies)
	require.Equal(t, http.StatusSeeOther, exclude.Code)
	occurrences, err = store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, fixtures.BookID, "Weg")
	require.NoError(t, err)
	assert.True(t, occurrences[1].Excluded)
	assert.Contains(t, func() string {
		get := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		for _, cookie := range cookies {
			get.AddCookie(cookie)
		}
		page := httptest.NewRecorder()
		h.ServeHTTP(page, get)
		return page.Body.String()
	}(), "excluded for this occurrence.")
	start := goalRequest(t, h, "/reading/books/"+fixtures.BookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "is+now+your+current+reading")
}
