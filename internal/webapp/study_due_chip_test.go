package webapp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dueCountFailure is a Study store whose due count fails for one language.
type dueCountFailure struct {
	StudyStore
	language string
}

func (s dueCountFailure) CountStudyReviewsDue(ctx context.Context, owner, language string, now time.Time) (int, error) {
	if language == s.language {
		return 0, errors.New("due count unavailable")
	}
	return s.StudyStore.CountStudyReviewsDue(ctx, owner, language, now)
}

func TestShellShowsTheDueCountOfTheLastStudyLanguage(t *testing.T) {
	h, cookies, _, store := readingFixtureSessionWith(t, func(services *Services) { services.Clock = func() time.Time { return time.Now() } })
	store.SeedDueStudyReviews(fixtures.OwnerID, 2, time.Now())
	page := getAs(t, h, "/de/library", cookies).Body.String()
	assert.Contains(t, page, `aria-label="Study, 2 reviews due"`)
}

func TestShellOmitsTheDueChipWithoutAStudyStore(t *testing.T) {
	h, cookies, _, _ := readingFixtureSessionWith(t, func(services *Services) { services.Store.Study = nil })
	response := getAs(t, h, "/de/library", cookies)
	require.Equal(t, http.StatusOK, response.Code)
	assert.NotContains(t, response.Body.String(), "site-nav__count")
}

func TestPagesFailWhenTheDueCountCannotBeRead(t *testing.T) {
	failing := func(language string) func(*Services) {
		return func(services *Services) {
			services.Store.Study = dueCountFailure{StudyStore: services.Store.Study, language: language}
		}
	}
	t.Run("for the stored language", func(t *testing.T) {
		h, cookies, _, _ := readingFixtureSessionWith(t, failing("de"))
		assert.Equal(t, http.StatusInternalServerError, getAs(t, h, "/de/library", cookies).Code)
	})
	t.Run("for the language of the page", func(t *testing.T) {
		h, cookies, _, store := readingFixtureSessionWith(t, failing("de"))
		require.NoError(t, store.SetActiveStudyLanguage(t.Context(), fixtures.OwnerID, "it"))
		assert.Equal(t, http.StatusInternalServerError, getAs(t, h, "/de/library", cookies).Code)
	})
}
