package webapp

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every workflow rejection in the domain needs a learner-facing entry in
// lemmaReviewRejections. A new sentinel without one fails here instead of
// falling through to the generic infrastructure-failure response.
func TestEveryLemmaReviewRejectionHasLearnerMapping(t *testing.T) {
	sentinels := []error{
		domain.ErrLemmaReviewCurrentReading,
		domain.ErrLemmaReviewStale,
		domain.ErrLemmaReviewCountsRefreshing,
		domain.ErrLemmaReviewRepreparationRequired,
		domain.ErrLemmaReviewBlockedByReading,
		domain.ErrLemmaReviewNotToRead,
		domain.ErrLemmaReviewOccurrenceUnavailable,
		domain.ErrLemmaReviewOccurrenceMissing,
		domain.ErrLemmaReviewNoSelection,
		domain.ErrLemmaReviewDecision,
		domain.ErrLemmaReviewLemma,
		domain.ErrLemmaReviewUnsupportedLanguage,
	}
	for _, sentinel := range sentinels {
		rejection, ok := lemmaReviewRejectionFor(fmt.Errorf("wrapped: %w", sentinel))
		require.True(t, ok, "%v needs a learner-facing mapping", sentinel)
		assert.NotEmpty(t, rejection.message, "%v needs learner copy", sentinel)
		assert.GreaterOrEqual(t, rejection.status, http.StatusBadRequest, "%v is a client rejection", sentinel)
		assert.Less(t, rejection.status, http.StatusInternalServerError, "%v is a client rejection", sentinel)
	}
	assert.Len(t, lemmaReviewRejections, len(sentinels), "each mapping must correspond to one domain sentinel")
}

func TestWriteLemmaReviewErrorAnswersRejectionsWithLearnerCopy(t *testing.T) {
	response := httptest.NewRecorder()
	writeLemmaReviewError(response, fmt.Errorf("wrapped: %w", domain.ErrLemmaReviewStale))
	assert.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "Learner vocabulary state or this proposal changed after preview.")
}

func TestWriteLemmaReviewErrorTreatsOtherFailuresAsInfrastructure(t *testing.T) {
	response := httptest.NewRecorder()
	writeLemmaReviewError(response, errors.New("database unavailable"))
	assert.Equal(t, http.StatusInternalServerError, response.Code, "unmapped errors take the generic failure path")
}

func TestLemmaSuggestionForMissingOccurrenceIsConflict(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	endGermanFixtureReading(t, store)
	webHandler, ok := h.(*Handler)
	require.True(t, ok)
	webHandler.services.LemmaReview = lemmaReviewWithSuggestions(webHandler, &lemmaSuggestionFixture{})
	response := readingTestRequest(t, h, "/reading/books/fixture-route-match/lemma-suggestion", url.Values{
		"csrf_token": {csrf}, "form": {"Weg"}, "target": {"no-such-occurrence"},
	}, cookies)
	require.Equal(t, http.StatusConflict, response.Code)
	assert.Contains(t, response.Body.String(), "That occurrence is no longer available in this analysis.")
}
