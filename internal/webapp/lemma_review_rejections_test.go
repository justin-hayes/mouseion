package webapp

import (
	"fmt"
	"net/http"
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
