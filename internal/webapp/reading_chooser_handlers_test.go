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

type readingChooserBooks struct {
	*fixtures.Store
	books []domain.MyBook
}

func (b readingChooserBooks) ListMyBooksWithEvidence(context.Context, string) ([]domain.MyBook, error) {
	return b.books, nil
}

type readingChooserInsights struct{ fixtures.Insights }

func (i readingChooserInsights) Coverage(ctx context.Context, owner, corpusID string) (domain.AnalysisCoverage, error) {
	if corpusID == "fixture-route-tie-a-corpus" {
		return domain.AnalysisCoverage{}, nil
	}
	return i.Insights.Coverage(ctx, owner, corpusID)
}

func TestReadingChooserShowsAuthenticatedToReadCandidatesAndRecoveryStates(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.ClearPrimaryGoal(context.Background(), fixtures.OwnerID, "de", fixtures.BookID))
	handler, ok := h.(*Handler)
	require.True(t, ok)
	books, err := store.ListMyBooksWithEvidence(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	for i := range books {
		if books[i].Acquired != nil {
			switch books[i].Acquired.Source.ID {
			case "fixture-route-match":
				books[i].Acquired.AnalysisStatus = "analyzing"
			case "fixture-route-differs":
				books[i].Acquired.AnalysisStatus = "stale"
			}
		}
	}
	dependencies := handler.services.Store
	dependencies.Books = readingChooserBooks{Store: store, books: books}
	handler.services.Store = dependencies
	handler.services.AnalysisInsights = readingChooserInsights{Insights: fixtures.Insights{JourneyStore: store}}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	assert.Contains(t, body, "Choose your next book in German")
	assert.Contains(t, body, "Current coverage:")
	assert.Contains(t, body, "eligible vocabulary identities")
	assert.Contains(t, body, "Start reading")
	assert.Contains(t, body, "Confirm start reading")
	assert.Contains(t, body, "Analysis in progress")
	assert.Contains(t, body, "Review To Read books")
	assert.Contains(t, body, "Needs attention")
	assert.Contains(t, body, "The last analysis did not complete.")
	assert.Contains(t, body, `action="/reading/books/fixture-failed/reanalyze"`)
	assert.Contains(t, body, "The analysis no longer matches the current book content")
	assert.Contains(t, body, "No vocabulary comparison")
	assert.Contains(t, body, "No analyzable tokens")
	assert.NotContains(t, body, "Current coverage: 0.0%")
	assert.NotContains(t, body, "Italian goal")
	assert.NotContains(t, body, "Metadata-only migration book")
	assert.True(t, strings.Contains(body, `action="/reading/books/`) && strings.Contains(body, `/start"`) && strings.Contains(body, `name="csrf_token"`))
	assert.Less(t, strings.Index(body, "Der lange Weg nach Hause"), strings.Index(body, "Route differs: new German"), "same-band candidates should be in neutral title order")
}

func TestReadingChooserRecoveryDoesNotRequireJourneyMembership(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.ClearPrimaryGoal(context.Background(), fixtures.OwnerID, "de", fixtures.BookID))
	journey, err := store.GetReadingJourney(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(context.Background(), fixtures.OwnerID, "de", "fixture-failed", journey.Revision)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-failed", domain.BookDispositionToRead))
	handler, ok := h.(*Handler)
	require.True(t, ok)
	handler.services.Analysis = fixtures.Analysis{}

	response := goalRequest(t, h, "/reading/books/fixture-failed/reanalyze", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "/reading?message=Analysis+")
}

func TestReadingChooserStartConfirmationReturnsToReading(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.ClearPrimaryGoal(context.Background(), fixtures.OwnerID, "de", fixtures.BookID))
	response := goalRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "/reading?message=")
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", current.BookID)
	assert.NotEmpty(t, current.SnapshotID)
	retry := goalRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, retry.Code)
	retried, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, current.SnapshotID, retried.SnapshotID, "retry reuses the frozen snapshot")
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	page := httptest.NewRecorder()
	h.ServeHTTP(page, request)
	require.Equal(t, http.StatusOK, page.Code)
	assert.Contains(t, page.Body.String(), "Route match: familiar German")
	assert.Contains(t, page.Body.String(), "Current coverage")
	assert.Contains(t, page.Body.String(), "After Primary Goal coverage")
}

func TestReadingChooserEmptyStateAndAuthentication(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.ClearPrimaryGoal(context.Background(), fixtures.OwnerID, "de", fixtures.BookID))
	handler, ok := h.(*Handler)
	require.True(t, ok)
	dependencies := handler.services.Store
	dependencies.Books = readingChooserBooks{Store: store}
	handler.services.Store = dependencies

	unauthenticated := httptest.NewRecorder()
	h.ServeHTTP(unauthenticated, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil))
	assert.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "No To Read books yet")
	assert.Contains(t, response.Body.String(), "Browse My Books")
}

func TestReadingChooserKeepsAllPendingCandidatesVisible(t *testing.T) {
	h, cookies, _, store := goalFixtureSession(t)
	require.NoError(t, store.ClearPrimaryGoal(context.Background(), fixtures.OwnerID, "de", fixtures.BookID))
	handler, ok := h.(*Handler)
	require.True(t, ok)
	books, err := store.ListMyBooksWithEvidence(context.Background(), fixtures.OwnerID)
	require.NoError(t, err)
	for i := range books {
		if books[i].Disposition == domain.BookDispositionToRead && books[i].Acquired != nil {
			books[i].Acquired.AnalysisStatus = "analysis queued"
		}
	}
	dependencies := handler.services.Store
	dependencies.Books = readingChooserBooks{Store: store, books: books}
	handler.services.Store = dependencies

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	h.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "Analysis in progress")
	assert.NotContains(t, response.Body.String(), "No To Read books yet")
	assert.NotContains(t, response.Body.String(), "Needs attention")
	assert.NotContains(t, response.Body.String(), "Current coverage:")
}
