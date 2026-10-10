package webapp

import (
	"context"
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rejectingReading answers Start and Switch with a fixed persistence error, so
// a handler test shows the message each rejection reason produces without
// arranging the store state behind it.
type rejectingReading struct {
	ReadingStore
	err error
}

func (r rejectingReading) StartCurrentReading(context.Context, string, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{}, r.err
}

func (r rejectingReading) StartCurrentReadingResult(context.Context, string, string, string) (persistence.StartResult, error) {
	return persistence.StartResult{}, r.err
}

func (r rejectingReading) SwitchCurrentReading(context.Context, string, string, string, string, string) (domain.CurrentReading, error) {
	return domain.CurrentReading{}, r.err
}

const (
	notEligibleCandidateMessage = "This book is no longer an eligible To Read candidate. Review Reading before trying again."
	notEligibleChoiceMessage    = "This book is no longer an eligible To Read choice. No changes were made; refresh Reading and try again."
	startNoAnalysisMessage      = "This book no longer has trustworthy current analysis or is no longer To Read. No changes were made; refresh Reading and try again."
	switchNoAnalysisMessage     = "This book no longer has current analyzed content or is no longer To Read. No changes were made; refresh Reading and try again."
	switchStaleMessage          = "The current book changed while you were choosing. No changes were made; review Reading before trying again."
)

func rejectionLocation(t *testing.T, mutate func(*Handler), path string, form func(domain.CurrentReading) url.Values) (int, string) {
	t.Helper()
	h, cookies, csrf, store := readingFixtureSession(t)
	current, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	mutate(requireHandler(t, h))
	values := form(current)
	values.Set("csrf_token", csrf)
	response := readingTestRequest(t, h, path, values, cookies)
	location, err := url.QueryUnescape(response.Header().Get("Location"))
	require.NoError(t, err)
	return response.Code, location
}

func rejectWith(err error) func(*Handler) {
	return func(h *Handler) {
		h.services.Store.Reading = rejectingReading{ReadingStore: h.services.Store.Reading, err: err}
	}
}

func TestStartShowsTheMessageForEachPersistenceRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not to read", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingNotToRead}, "/reading?error=" + notEligibleCandidateMessage},
		{"other language", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingOtherLanguage}, "/reading?error=" + notEligibleCandidateMessage},
		{"no completed analysis", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingNoCompletedAnalysis}, "/reading?error=" + startNoAnalysisMessage},
		{"another book current", persistence.ErrCurrentReadingExists, "/reading?error=A current book is already set for this language. Review it in Reading before starting another."},
		{"unresolved flags", persistence.ErrUnresolvedLemmaReviewFlags, "/reading/books/fixture-route-match/lemma-review"},
		{"counts pending", persistence.ErrVocabularyBrowseCountsPending, "/reading?error=Across-book vocabulary counts are updating. No reading was changed; try starting again when the counts are ready."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, location := rejectionLocation(t, rejectWith(tt.err), "/reading/books/fixture-route-match/start", func(domain.CurrentReading) url.Values { return url.Values{} })
			assert.Equal(t, 303, code)
			assert.Equal(t, tt.want, location)
		})
	}
}

func TestStartOfUnknownBookIsNotFound(t *testing.T) {
	code, _ := rejectionLocation(t, rejectWith(persistence.ErrNotFound), "/reading/books/fixture-route-match/start", func(domain.CurrentReading) url.Values { return url.Values{} })
	assert.Equal(t, 404, code)
}

func TestSwitchShowsTheMessageForEachPersistenceRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not to read", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingNotToRead}, "/reading?error=" + notEligibleChoiceMessage},
		{"other language", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingOtherLanguage}, "/reading?error=" + notEligibleChoiceMessage},
		{"no completed analysis", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingNoCompletedAnalysis}, "/reading?error=" + switchNoAnalysisMessage},
		{"stale", persistence.ErrCurrentReadingStale, "/reading?error=" + switchStaleMessage},
		{"no current reading", persistence.ErrNotFound, "/reading?error=" + switchStaleMessage},
		{"unresolved flags", persistence.ErrUnresolvedLemmaReviewFlags, "/reading/books/fixture-route-match/lemma-review"},
		{"counts unavailable", persistence.ErrVocabularyBrowseCountsUnavailable, "/reading?error=Across-book vocabulary counts are unavailable after repeated rebuild failures. No reading was changed; ask the server operator to restart Mouseion, then retry."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, location := rejectionLocation(t, rejectWith(tt.err), "/reading/books/fixture-route-match/switch", func(current domain.CurrentReading) url.Values {
				return url.Values{"expected_current_book_id": {current.BookID}, "expected_current_snapshot_id": {current.SnapshotID}}
			})
			assert.Equal(t, 303, code)
			assert.Equal(t, tt.want, location)
		})
	}
}

func TestStartOfTheCurrentBookIsIdempotent(t *testing.T) {
	code, location := rejectionLocation(t, func(*Handler) {}, "/reading/books/"+fixtures.BookID+"/start", func(domain.CurrentReading) url.Values { return url.Values{} })
	assert.Equal(t, 303, code)
	assert.Contains(t, location, "is already your current reading")
	assert.NotContains(t, location, "error=", "a repeat Start is not a conflict")
}

func TestStartOfTheCurrentBookReplaysWithoutChangingIt(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	current, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	response := readingTestRequest(t, h, "/reading/books/"+current.BookID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, 303, response.Code)
	after, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, current, after, "replaying the current Start must not refreeze or rewrite the reading")
}

func TestSwitchToTheCurrentBookChangesNothing(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	current, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	response := readingTestRequest(t, h, "/reading/books/"+current.BookID+"/switch", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {current.BookID}, "expected_current_snapshot_id": {current.SnapshotID},
	}, cookies)
	assert.Equal(t, 303, response.Code)
	after, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, current.SnapshotID, after.SnapshotID, "switching to the current Book must not refreeze its snapshot")
}
