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

// rejectingReading answers Read next with a fixed persistence error, so
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

const (
	notEligibleCandidateMessage = "This book is no longer an eligible To Read candidate. Review My Books before trying again."
	startNoAnalysisMessage      = "This book no longer has trustworthy current analysis or is no longer To Read. No changes were made; refresh My Books and try again."
)

func rejectionLocation(t *testing.T, mutate func(*Handler), path string) (int, string) {
	t.Helper()
	h, cookies, csrf, _ := readingFixtureSession(t)
	mutate(requireHandler(t, h))
	response := readingTestRequest(t, h, path, url.Values{"csrf_token": {csrf}}, cookies)
	location, err := url.QueryUnescape(response.Header().Get("Location"))
	require.NoError(t, err)
	return response.Code, location
}

func rejectWith(err error) func(*Handler) {
	return func(h *Handler) {
		h.services.Store.Reading = rejectingReading{ReadingStore: h.services.Store.Reading, err: err}
	}
}

func TestReadNextShowsTheMessageForEachPersistenceRejection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"not to read", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingNotToRead}, "/de/library?error=" + notEligibleCandidateMessage},
		{"other language", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingOtherLanguage}, "/de/library?error=" + notEligibleCandidateMessage},
		{"no completed analysis", persistence.CurrentReadingIneligibleError{Reason: domain.CurrentReadingNoCompletedAnalysis}, "/de/library?error=" + startNoAnalysisMessage},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, location := rejectionLocation(t, rejectWith(tt.err), "/reading/books/fixture-route-match/start")
			assert.Equal(t, 303, code)
			assert.Equal(t, tt.want, location)
		})
	}
}

func TestStartOfUnknownBookIsNotFound(t *testing.T) {
	code, _ := rejectionLocation(t, rejectWith(persistence.ErrNotFound), "/reading/books/fixture-route-match/start")
	assert.Equal(t, 404, code)
}

func TestStartOfTheCurrentBookIsIdempotent(t *testing.T) {
	code, location := rejectionLocation(t, func(*Handler) {}, "/reading/books/"+fixtures.BookID+"/start")
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

func TestSwitchRouteIsReadNext(t *testing.T) {
	h, cookies, csrf, store := readingFixtureSession(t)
	current, err := store.GetCurrentReading(t.Context(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	response := readingTestRequest(t, h, "/reading/books/"+current.BookID+"/switch", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, 303, response.Code)
	assert.Contains(t, response.Header().Get("Location"), "is+already+your+current+reading", "the switch path is the same Read next, and the current Book is a replay")
}
