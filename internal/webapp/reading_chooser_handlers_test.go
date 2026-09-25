package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
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
				books[i].Acquired.AnalysisStatus = "analysis running"
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
	assert.Contains(t, body, "Analysis in progress")
	assert.Contains(t, body, "Needs attention")
	assert.Contains(t, body, "The analysis no longer matches the current book content")
	assert.Contains(t, body, "No vocabulary comparison")
	assert.Contains(t, body, "No analyzable tokens")
	assert.NotContains(t, body, "Current coverage: 0.0%")
	assert.NotContains(t, body, "Italian goal")
	assert.NotContains(t, body, "Metadata-only migration book")
	assert.True(t, strings.Contains(body, `action="/goal/books/`) && strings.Contains(body, `name="csrf_token"`))
	assert.Less(t, strings.Index(body, "Der lange Weg nach Hause"), strings.Index(body, "Route differs: new German"), "same-band candidates should be in neutral title order")
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
