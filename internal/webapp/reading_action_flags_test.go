package webapp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/lemmareview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorReadingStore keeps the rest of the Reading store and fails one detail
// read, so a handler's failure response is exercised without a database.
type errorReadingStore struct {
	ReadingStore
	getBookDetailErr error
}

func (s errorReadingStore) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return domain.MyBook{}, s.getBookDetailErr
}

func TestAssessReadingActionFlagsReportsLoadAndAssessmentFailures(t *testing.T) {
	h, _, _, _ := readingFixtureSession(t)
	webHandler, ok := h.(*Handler)
	require.True(t, ok)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/reading/books/"+fixtures.BookID+"/start", nil)

	missing := httptest.NewRecorder()
	require.False(t, webHandler.assessReadingActionFlags(missing, request, fixtures.OwnerID, "missing-book"))
	assert.Equal(t, http.StatusNotFound, missing.Code)

	readingStore := webHandler.services.Store.Reading
	webHandler.services.Store.Reading = errorReadingStore{ReadingStore: readingStore, getBookDetailErr: errors.New("book read failed")}
	loadFailed := httptest.NewRecorder()
	require.False(t, webHandler.assessReadingActionFlags(loadFailed, request, fixtures.OwnerID, fixtures.BookID))
	assert.Equal(t, http.StatusInternalServerError, loadFailed.Code)

	webHandler.services.Store.Reading = readingStore
	webHandler.services.LemmaReview = lemmareview.New(lemmareview.Config{
		Store:       errorReadingStore{ReadingStore: readingStore, getBookDetailErr: errors.New("assessment read failed")},
		Suggestions: enrichment.DisabledLemmaSuggestionProvider{},
	})
	assessmentFailed := httptest.NewRecorder()
	require.False(t, webHandler.assessReadingActionFlags(assessmentFailed, request, fixtures.OwnerID, fixtures.BookID))
	assert.Equal(t, http.StatusInternalServerError, assessmentFailed.Code)
}

func TestLemmaReviewBookReportsLoadFailures(t *testing.T) {
	h, _, _, _ := readingFixtureSession(t)
	webHandler, ok := h.(*Handler)
	require.True(t, ok)
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reading/books/"+fixtures.BookID+"/lemma-review", nil)

	missing := httptest.NewRecorder()
	_, ok = webHandler.lemmaReviewBook(missing, request, domain.User{ID: fixtures.OwnerID}, "missing-book")
	require.False(t, ok)
	assert.Equal(t, http.StatusNotFound, missing.Code)

	webHandler.services.Store.Reading = errorReadingStore{ReadingStore: webHandler.services.Store.Reading, getBookDetailErr: errors.New("book read failed")}
	loadFailed := httptest.NewRecorder()
	_, ok = webHandler.lemmaReviewBook(loadFailed, request, domain.User{ID: fixtures.OwnerID}, fixtures.BookID)
	require.False(t, ok)
	assert.Equal(t, http.StatusInternalServerError, loadFailed.Code)
}
