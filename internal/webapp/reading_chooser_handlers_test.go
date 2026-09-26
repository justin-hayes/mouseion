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
	handler.services.AnalysisInsights = readingChooserInsights{Insights: fixtures.Insights{}}

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

func TestReadingChooserRecoveryDoesNotRequireIsToReadship(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	require.NoError(t, store.ClearPrimaryGoal(context.Background(), fixtures.OwnerID, "de", fixtures.BookID))
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
	assert.NotContains(t, page.Body.String(), "On arrival")
	assert.NotContains(t, page.Body.String(), "Lower bound")
}

func TestAuthenticatedCurrentReadingCanSwitchStopAndSetAside(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	handler, ok := h.(*Handler)
	require.True(t, ok)
	handler.services.AnalysisInsights = fixtures.Insights{}
	readingRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading", nil)
	for _, cookie := range cookies {
		readingRequest.AddCookie(cookie)
	}
	readingPage := httptest.NewRecorder()
	h.ServeHTTP(readingPage, readingRequest)
	require.Equal(t, http.StatusOK, readingPage.Code)
	assert.Contains(t, readingPage.Body.String(), "Stop reading for now")
	assert.Contains(t, readingPage.Body.String(), "Set aside this Book")
	assert.Contains(t, readingPage.Body.String(), `name="expected_current_snapshot_id"`)
	assert.Contains(t, readingPage.Body.String(), `action="/reading/finish"`)
	assert.NotContains(t, readingPage.Body.String(), `action="/goal/finish"`)

	switchRequest := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading/switch", nil)
	for _, cookie := range cookies {
		switchRequest.AddCookie(cookie)
	}
	switchPage := httptest.NewRecorder()
	h.ServeHTTP(switchPage, switchRequest)
	require.Equal(t, http.StatusOK, switchPage.Code)
	assert.Contains(t, switchPage.Body.String(), "Switch current reading")
	assert.Contains(t, switchPage.Body.String(), `action="/reading/books/fixture-route-match/switch"`)
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	initialSnapshotID := current.SnapshotID

	switched := goalRequest(t, h, "/reading/books/fixture-route-match/switch", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {fixtures.BookID},
		"expected_current_snapshot_id": {initialSnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, switched.Code)
	assert.Contains(t, switched.Header().Get("Location"), "is+now+your+current+reading")
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, "fixture-route-match", current.BookID)
	snapshotID := current.SnapshotID
	retriedSwitch := goalRequest(t, h, "/reading/books/fixture-route-match/switch", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {fixtures.BookID},
		"expected_current_snapshot_id": {initialSnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, retriedSwitch.Code)
	assert.NotContains(t, retriedSwitch.Header().Get("Location"), "error=")
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, snapshotID, current.SnapshotID, "retry leaves the switched reading unchanged")

	stopped := goalRequest(t, h, "/reading/stop", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {current.BookID},
		"expected_current_snapshot_id": {snapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, stopped.Code)
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	disposition, err := store.GetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-route-match")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)

	retriedStop := goalRequest(t, h, "/reading/stop", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {"fixture-route-match"},
		"expected_current_snapshot_id": {snapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, retriedStop.Code)
	assert.NotContains(t, retriedStop.Header().Get("Location"), "error=")

	started := goalRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	setAsideSnapshotID := current.SnapshotID
	setAside := goalRequest(t, h, "/reading/set-aside", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {"fixture-route-match"},
		"expected_current_snapshot_id": {setAsideSnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, setAside.Code)
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
	disposition, err = store.GetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-route-match")
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, disposition)
	retriedSetAside := goalRequest(t, h, "/reading/set-aside", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {"fixture-route-match"},
		"expected_current_snapshot_id": {setAsideSnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, retriedSetAside.Code)
	assert.NotContains(t, retriedSetAside.Header().Get("Location"), "error=")

	require.NoError(t, store.SetBookDisposition(context.Background(), fixtures.OwnerID, "fixture-route-match", domain.BookDispositionToRead))
	started = goalRequest(t, h, "/reading/books/fixture-route-match/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	finished := goalRequest(t, h, "/reading/finish", url.Values{
		"csrf_token":                   {csrf},
		"expected_current_book_id":     {current.BookID},
		"expected_current_snapshot_id": {current.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusOK, finished.Code)
	assert.Contains(t, finished.Body.String(), "Reading finished")
	assert.Contains(t, finished.Body.String(), `href="/reading">Choose what to read next</a>`)
	current, err = store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, current.IsActive())
}

func TestLegacyGoalMutationsAreRejectedWithoutChangingCurrentReading(t *testing.T) {
	h, cookies, csrf, store := goalFixtureSession(t)
	currentBefore, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)

	legacyRequests := []struct {
		path string
		form url.Values
	}{
		{"/goal/books/fixture-route-match", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}}},
		{"/goal/clear", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}}},
		{"/goal/finish", url.Values{"csrf_token": {csrf}, "expected_goal_book_id": {fixtures.BookID}, "expected_goal_snapshot_id": {currentBefore.SnapshotID}}},
	}
	for _, test := range legacyRequests {
		response := goalRequest(t, h, test.path, test.form, cookies)
		assert.Equalf(t, http.StatusNotFound, response.Code, "legacy mutation %s should be retired", test.path)
	}

	currentAfter, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, currentBefore, currentAfter)
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
