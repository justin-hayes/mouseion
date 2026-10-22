package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAnalysisUpdates struct {
	offers    map[string]domain.AnalysisUpdateCandidate
	offersErr error
	submitErr error
	submitted []string
}

func (f *fakeAnalysisUpdates) AnalysisUpdateCandidates(context.Context, string, []string) (map[string]domain.AnalysisUpdateCandidate, error) {
	return f.offers, f.offersErr
}

func (f *fakeAnalysisUpdates) SubmitAnalysisUpdate(_ context.Context, _, bookID, sourceID, expectedRunID, expectedTarget string) (analysis.Handle, error) {
	f.submitted = append(f.submitted, strings.Join([]string{bookID, sourceID, expectedRunID, expectedTarget}, "|"))
	if f.submitErr != nil {
		return analysis.Handle{}, f.submitErr
	}
	return analysis.Handle{ID: 41, DisplayNumber: 7, RunID: "update-run"}, nil
}

var testUpdateTarget = domain.AnalysisUpdateTarget{AnalyzerName: "mouseion-snapshot-analyzer", AnalyzerVersion: "3", NormalizationProfile: "german-standard-post-1996", NormalizationVersion: "7"}

func fixtureSourceID(t *testing.T, store *fixtures.Store, bookID string) string {
	t.Helper()
	books, err := store.ListMyBooksWithEvidence(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.ID == bookID && book.Acquired != nil {
			return book.Acquired.Source.ID
		}
	}
	t.Fatalf("fixture Book %s has no source", bookID)
	return ""
}

func readingGet(t *testing.T, h http.Handler, path string, cookies []*http.Cookie) string {
	t.Helper()
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
	return response.Body.String()
}

func TestReadingChooserOffersAConfirmedAnalysisUpdate(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	sourceID := fixtureSourceID(t, store, "fixture-route-match")
	updates := &fakeAnalysisUpdates{offers: map[string]domain.AnalysisUpdateCandidate{sourceID: {PublishedRunID: "published-run", Target: testUpdateTarget, LemmaCorrections: 1, OccurrenceExclusions: 2}}}
	handler.services.AnalysisUpdates = updates
	handler.services.AnalysisInsights = readingChooserInsights{Insights: fixtures.Insights{}}

	body := readingGet(t, h, "/reading", cookies)
	assert.Equal(t, 1, strings.Count(body, "A newer analysis is available."), "only the outdated Book shows the notice")
	assert.Contains(t, body, `<details class="confirmation`, "the confirmation is a native disclosure that works without JavaScript")
	assert.Contains(t, body, "Update analysis")
	assert.Contains(t, body, "The published analysis stays in effect until the update publishes.")
	assert.Contains(t, body, "Your 1 confirmed lemma correction and 2 occurrence exclusions on it do not carry over to the new analysis.")
	assert.Contains(t, body, "Prepared decks and earlier Readings keep the evidence they froze.")
	assert.Contains(t, body, `action="/reading/books/fixture-route-match/analysis-update"`)
	assert.Contains(t, body, `name="expected_run_id" value="published-run"`)
	assert.Contains(t, body, `name="expected_target" value="`+testUpdateTarget.Key()+`"`)
	assert.Contains(t, body, "Confirm analysis update")
	assert.Contains(t, body, "Confirm start reading", "the offer does not change readiness")
}

func TestReadingChooserTreatsUnavailableOffersAsCurrent(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	handler.services.AnalysisUpdates = &fakeAnalysisUpdates{offersErr: errors.New("NLP unreachable")}
	handler.services.AnalysisInsights = readingChooserInsights{Insights: fixtures.Insights{}}

	body := readingGet(t, h, "/reading", cookies)
	assert.NotContains(t, body, "A newer analysis is available.")
	assert.Contains(t, body, "Confirm start reading")
}

func TestMyBooksTellsTheCurrentReadingToEndFirst(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	sourceID := fixtureSourceID(t, store, fixtures.BookID)
	handler.services.AnalysisUpdates = &fakeAnalysisUpdates{offers: map[string]domain.AnalysisUpdateCandidate{sourceID: {PublishedRunID: "published-run", Target: testUpdateTarget}}}

	body := readingGet(t, h, "/library", cookies)
	assert.Contains(t, body, "A newer analysis is available.")
	assert.Contains(t, body, "End the Reading first")
	assert.NotContains(t, body, "Confirm analysis update", "the current Reading's Book is offered no submit")
}

func TestMyBooksOffersTheUpdateOnTheToReadLifecycleCard(t *testing.T) {
	h, cookies, _, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	sourceID := fixtureSourceID(t, store, fixtures.BookID)
	handler.services.AnalysisUpdates = &fakeAnalysisUpdates{offers: map[string]domain.AnalysisUpdateCandidate{sourceID: {PublishedRunID: "published-run", Target: testUpdateTarget}}}

	body := readingGet(t, h, "/library", cookies)
	assert.Contains(t, body, "A newer analysis is available.")
	assert.Contains(t, body, "You have no confirmed lemma corrections or occurrence exclusions on it.")
	assert.Contains(t, body, `action="/reading/books/`+fixtures.BookID+`/analysis-update"`)
	assert.NotContains(t, body, "End the Reading first")
}

func TestAnalysisUpdateConfirmationSubmitsTheConfirmedOffer(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	updates := &fakeAnalysisUpdates{}
	handler.services.AnalysisUpdates = updates
	sourceID := fixtureSourceID(t, store, "fixture-route-match")

	response := readingTestRequest(t, h, "/reading/books/fixture-route-match/analysis-update", url.Values{"csrf_token": {csrf}, "expected_run_id": {"published-run"}, "expected_target": {testUpdateTarget.Key()}}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	location, err := url.Parse(response.Header().Get("Location"))
	require.NoError(t, err)
	assert.Equal(t, "/reading", location.Path)
	assert.Contains(t, location.Query().Get("message"), "submitted as analysis job #7")
	assert.Contains(t, location.Query().Get("message"), "The published analysis stays in effect until the update publishes.")
	assert.Equal(t, []string{"fixture-route-match|" + sourceID + "|published-run|" + testUpdateTarget.Key()}, updates.submitted)
}

func TestAnalysisUpdateConfirmationReportsRefusals(t *testing.T) {
	for _, tt := range []struct {
		name    string
		err     error
		status  int
		message string
	}{
		{"offer changed", analysis.ErrAnalysisUpdateChanged, http.StatusSeeOther, analysisUpdateChangedMessage},
		{"current reading", analysis.ErrCurrentBook, http.StatusSeeOther, "End the Reading first."},
		{"not To Read", analysis.ErrNotFound, http.StatusNotFound, ""},
		{"no EPUB", analysis.ErrEPUBRequired, http.StatusSeeOther, analysisUpdateChangedMessage},
		{"operational", errors.New("database down"), http.StatusInternalServerError, ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			h, cookies, csrf, store := readingFixtureSession(t)
			endGermanFixtureReading(t, store)
			handler, ok := h.(*Handler)
			require.True(t, ok)
			handler.services.AnalysisUpdates = &fakeAnalysisUpdates{submitErr: tt.err}

			response := readingTestRequest(t, h, "/reading/books/fixture-route-match/analysis-update", url.Values{"csrf_token": {csrf}}, cookies)
			assert.Equal(t, tt.status, response.Code)
			if tt.message != "" {
				location, err := url.Parse(response.Header().Get("Location"))
				require.NoError(t, err)
				assert.Contains(t, location.Query().Get("error"), tt.message)
			}
		})
	}
}

//nolint:dupl // the Source structure refusal contract mirrors this one with its own route and copy
func TestAnalysisUpdateConfirmationWithoutAnUpdateSeamSubmitsNothing(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)

	t.Run("no update seam", func(t *testing.T) {
		response := readingTestRequest(t, h, "/reading/books/fixture-route-match/analysis-update", url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Contains(t, response.Header().Get("Location"), url.QueryEscape(analysisUpdateChangedMessage))
	})
	t.Run("unknown Book", func(t *testing.T) {
		missing := readingTestRequest(t, h, "/reading/books/no-such-book/analysis-update", url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equal(t, http.StatusNotFound, missing.Code)
	})
	t.Run("missing CSRF token", func(t *testing.T) {
		forged := readingTestRequest(t, h, "/reading/books/fixture-route-match/analysis-update", url.Values{}, cookies)
		assert.Equal(t, http.StatusForbidden, forged.Code)
	})
}

func TestAnalysisUpdateDecisionsText(t *testing.T) {
	for _, tt := range []struct {
		name                    string
		corrections, exclusions int
		want                    string
	}{
		{"none", 0, 0, "You have no confirmed lemma corrections or occurrence exclusions on it."},
		{"both", 2, 1, "Your 2 confirmed lemma corrections and 1 occurrence exclusion on it do not carry over to the new analysis."},
		{"exclusions only", 0, 3, "Your 3 occurrence exclusions on it do not carry over to the new analysis."},
		{"one correction", 1, 0, "Your 1 confirmed lemma correction on it does not carry over to the new analysis."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, analysisUpdateDecisionsText(tt.corrections, tt.exclusions))
		})
	}
}

func TestMyBookAnalysisUpdateNeedsAnOfferedCandidate(t *testing.T) {
	offer, candidate := myBookAnalysisUpdate(domain.MyBook{})
	assert.Equal(t, domain.AnalysisUpdateNone, offer)
	assert.Nil(t, candidate)
}
