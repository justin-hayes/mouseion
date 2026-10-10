package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lemmaSuggestionFixture struct {
	request enrichment.LemmaSuggestionRequest
	err     error
}

func (*lemmaSuggestionFixture) Name() string    { return "fake-provider" }
func (*lemmaSuggestionFixture) Version() string { return "fake-v1" }
func (p *lemmaSuggestionFixture) SuggestLemma(_ context.Context, request enrichment.LemmaSuggestionRequest) (enrichment.LemmaSuggestion, error) {
	p.request = request
	return enrichment.LemmaSuggestion{Lemma: "Pfad"}, p.err
}

func TestOptionalLemmaSuggestionIsBoundedAndNeverApplied(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	provider := &lemmaSuggestionFixture{}
	webHandler, ok := h.(*Handler)
	require.True(t, ok)
	webHandler.services.LemmaReview = lemmaReviewWithSuggestions(webHandler, provider)
	response := readingTestRequest(t, h, "/reading/books/fixture-route-match/lemma-suggestion", url.Values{
		"csrf_token": {csrf}, "form": {"Weg"}, "target": {lemmaOccurrenceIDs(t, store, "fixture-route-match", "Weg")[0]},
	}, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "LLM lemma suggestion — not applied")
	assert.Contains(t, response.Body.String(), "fake-provider")
	assert.Contains(t, provider.request.Sentence, "Weg")
	assert.Equal(t, "de", provider.request.Language)
	assert.Empty(t, provider.request.LexicalAlternative, "no unrelated evidence is added")
	occurrences, err := store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, "fixture-route-match", "Weg")
	require.NoError(t, err)
	assert.Empty(t, occurrences[0].CorrectedLemma, "suggestion does not update effective identity")
	assert.Contains(t, response.Body.String(), `action="/reading/books/fixture-route-match/lemma-review"`, "manual review form remains available without JavaScript")
}

func TestFailedOptionalLemmaSuggestionKeepsManualReviewAvailable(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	webHandler, ok := h.(*Handler)
	require.True(t, ok)
	webHandler.services.LemmaReview = lemmaReviewWithSuggestions(webHandler, &lemmaSuggestionFixture{err: context.DeadlineExceeded})
	response := readingTestRequest(t, h, "/reading/books/fixture-route-match/lemma-suggestion", url.Values{
		"csrf_token": {csrf}, "form": {"Weg"}, "target": {lemmaOccurrenceIDs(t, store, "fixture-route-match", "Weg")[0]},
	}, cookies)
	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "A lemma suggestion is unavailable right now.")
	assert.Contains(t, response.Body.String(), "Preview correction")
}

func TestLearnerCorrectsOneExactOccurrenceAndStartReadingRemainsAvailable(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	activeRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading/books/"+fixtures.BookID+"/lemma-review?form=Weg", nil)
	for _, cookie := range cookies {
		activeRequest.AddCookie(cookie)
	}
	activePage := httptest.NewRecorder()
	h.ServeHTTP(activePage, activeRequest)
	assert.Contains(t, activePage.Body.String(), "Stop before changing this Book's vocabulary")
	assert.Contains(t, activePage.Body.String(), "A ready deck is a historical artifact")
	endGermanFixtureReading(t, store)
	reviewBookID := "fixture-route-match"
	weg := lemmaOccurrenceIDs(t, store, reviewBookID, "Weg")
	path := "/reading/books/" + reviewBookID + "/lemma-review?form=Weg"
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
		return readingTestRequest(t, h, "/reading/books/"+reviewBookID+"/lemma-review", values, cookies)
	}
	confirm := func(response *httptest.ResponseRecorder, lemma string, indices ...string) *httptest.ResponseRecorder {
		match := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(response.Body.String())
		require.Len(t, match, 2, "preview includes its stale-state fingerprint")
		values := url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {lemma}, "fingerprint": {match[1]}, "selected": indices}
		return readingTestRequest(t, h, "/reading/books/"+reviewBookID+"/lemma-review", values, cookies)
	}
	proposal := preview(weg[0], "correct", "Pfad")
	require.Equal(t, http.StatusOK, proposal.Code)
	assert.Contains(t, proposal.Body.String(), "This is a preview only")
	fingerprint := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(proposal.Body.String())
	require.Len(t, fingerprint, 2)
	tampered := readingTestRequest(t, h, "/reading/books/"+reviewBookID+"/lemma-review", url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {"Pfad"}, "fingerprint": {fingerprint[1]}, "selected": {weg[0], weg[1]}}, cookies)
	assert.Equal(t, http.StatusConflict, tampered.Code, "the confirmed selection must match the reviewed proposal")
	occurrences, err := store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, reviewBookID, "Weg")
	require.NoError(t, err)
	require.Len(t, occurrences, 2)
	assert.Empty(t, occurrences[0].CorrectedLemma, "preview is not a decision")
	require.Equal(t, http.StatusSeeOther, confirm(proposal, "Pfad", weg[0]).Code)
	occurrences, err = store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, reviewBookID, "Weg")
	require.NoError(t, err)
	assert.Equal(t, "pfad", occurrences[0].CorrectedLemma)
	assert.Empty(t, occurrences[1].CorrectedLemma, "same-surface occurrence must remain untouched")
	multi := preview(weg[0], "correct", "Pfad", weg[1])
	require.Contains(t, multi.Body.String(), `value="`+weg[0]+`"`)
	assert.Contains(t, multi.Body.String(), `value="`+weg[1]+`"`)
	require.Equal(t, http.StatusSeeOther, confirm(multi, "Pfad", weg[0], weg[1]).Code)
	occurrences, err = store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, reviewBookID, "Weg")
	require.NoError(t, err)
	assert.Equal(t, "pfad", occurrences[1].CorrectedLemma, "explicit multi-occurrence selection applies the reviewed decision to the selected peer")
	stalePreview := preview(weg[0], "correct", "Wand")
	require.Equal(t, http.StatusOK, stalePreview.Code)
	updated := preview(weg[0], "correct", "Wand")
	require.Equal(t, http.StatusSeeOther, confirm(updated, "Wand", weg[0]).Code)
	staleFingerprint := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(stalePreview.Body.String())
	stale := readingTestRequest(t, h, "/reading/books/"+reviewBookID+"/lemma-review", url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {"Wand"}, "fingerprint": {staleFingerprint[1]}, "selected": {weg[0]}}, cookies)
	assert.Equal(t, http.StatusConflict, stale.Code, "a changed learner decision invalidates its preview")
	exclusion := preview(weg[1], "exclude", "")
	exclusionConfirm := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(exclusion.Body.String())
	require.Len(t, exclusionConfirm, 2)
	exclude := readingTestRequest(t, h, "/reading/books/"+reviewBookID+"/lemma-review", url.Values{"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"exclude"}, "fingerprint": {exclusionConfirm[1]}, "selected": {weg[1]}}, cookies)
	require.Equal(t, http.StatusSeeOther, exclude.Code)
	occurrences, err = store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, reviewBookID, "Weg")
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
	start := readingTestRequest(t, h, "/reading/books/"+reviewBookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "is+now+your+current+reading")
}

func TestReadyReadingDeckCorrectionRestartsAndPreparesNewSnapshot(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	oldReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	oldSnapshotID := oldReading.SnapshotID
	endGermanFixtureReading(t, store)
	weg := lemmaOccurrenceIDs(t, store, fixtures.BookID, "Weg")

	preview := readingTestRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{
		"csrf_token": {csrf}, "stage": {"preview"}, "form": {"Weg"}, "target": {weg[0]}, "decision": {"correct"}, "lemma": {"Pfad"},
	}, cookies)
	require.Equal(t, http.StatusOK, preview.Code)
	assert.Contains(t, preview.Body.String(), "Start this Book again with a new snapshot and prepare a new deck")
	fingerprint := regexp.MustCompile(`name="fingerprint" value="([a-f0-9]+)"`).FindStringSubmatch(preview.Body.String())
	require.Len(t, fingerprint, 2)

	withoutConsent := readingTestRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{
		"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {"pfad"}, "fingerprint": {fingerprint[1]}, "selected": {weg[0]},
	}, cookies)
	assert.Equal(t, http.StatusBadRequest, withoutConsent.Code)
	stillStopped, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, stillStopped.IsActive(), "missing restart/re-preparation consent cannot start a new reading")

	confirmed := readingTestRequest(t, h, "/reading/books/"+fixtures.BookID+"/lemma-review", url.Values{
		"csrf_token": {csrf}, "stage": {"confirm"}, "form": {"Weg"}, "decision": {"correct"}, "lemma": {"pfad"}, "fingerprint": {fingerprint[1]}, "selected": {weg[0]}, "reprepare_ready_deck": {"yes"},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, confirmed.Code)
	assert.Contains(t, confirmed.Header().Get("Location"), "/deck-preparations/fixture-goal-preparation-")
	newReading, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, fixtures.BookID, newReading.BookID)
	assert.NotEqual(t, oldSnapshotID, newReading.SnapshotID)
	occurrences, err := store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, fixtures.BookID, "Weg")
	require.NoError(t, err)
	assert.Equal(t, "pfad", occurrences[0].CorrectedLemma)
}

// lemmaOccurrenceIDs lists one form's occurrence IDs in review order. Review
// forms address occurrences by these stable IDs, not by list position.
func lemmaOccurrenceIDs(t *testing.T, store lemmareview.Store, bookID, form string) []string {
	t.Helper()
	occurrences, err := store.ListLemmaReviewOccurrences(context.Background(), fixtures.OwnerID, bookID, form)
	require.NoError(t, err)
	ids := make([]string, 0, len(occurrences))
	for _, occurrence := range occurrences {
		ids = append(ids, occurrence.ID())
	}
	return ids
}

// lemmaReviewWithSuggestions rebuilds the handler's lemma review workflow over
// the same Reading store, with the given suggestion provider.
func lemmaReviewWithSuggestions(h *Handler, provider enrichment.LemmaSuggestionProvider) *lemmareview.Service {
	return lemmareview.New(lemmareview.Config{Store: h.services.Store.Reading, Suggestions: provider, Preparer: h.services.PreparedDeck, BrowseCounts: h.services.Analysis})
}
