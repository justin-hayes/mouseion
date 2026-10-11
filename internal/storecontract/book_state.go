package storecontract

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bookStateScenarios cover Book visibility, workflow disposition, and the
// previously read import. Revisions are read back from the store rather than
// assumed, so a scenario holds whatever revision an adapter starts from.
func bookStateScenarios() []Scenario {
	scenarios := visibilityScenarios()
	scenarios = append(scenarios, dispositionScenarios()...)
	return append(scenarios, importScenarios()...)
}

// seedBook seeds an analyzed Book for the learner under test, in the Inbox
// unless the seed says otherwise.
func seedBook(t *testing.T, h Harness, seed BookSeed) string {
	t.Helper()
	return h.Seeds().SeedBook(t, h.Seeds().Owner(), seed)
}

// requireVisibility asserts the learner's Hidden choice and revision for bookID.
func requireVisibility(t *testing.T, h Harness, owner, bookID string, hidden bool, revision int64) {
	t.Helper()
	gotHidden, gotRevision, err := h.BookState().GetBookVisibility(owner, bookID)
	require.NoError(t, err)
	assert.Equal(t, hidden, gotHidden)
	assert.Equal(t, revision, gotRevision)
}

// requireDetail returns the Book as the learner sees it, failing on error.
func requireDetail(t *testing.T, h Harness, owner, bookID string) domain.MyBook {
	t.Helper()
	detail, err := h.BookState().GetBookDetail(owner, bookID)
	require.NoError(t, err)
	return detail
}

func visibilityScenarios() []Scenario {
	return []Scenario{{
		Name: "hide applies at the expected revision and a verified replay is a no-op",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			requireVisibility(t, h, owner, bookID, false, 0)
			applied, err := h.BookState().SetBookHidden(owner, bookID, 0, true)
			require.NoError(t, err)
			assert.True(t, applied)
			applied, err = h.BookState().SetBookHidden(owner, bookID, 0, true)
			require.NoError(t, err)
			assert.False(t, applied)
			requireVisibility(t, h, owner, bookID, true, 1)
		},
	}, {
		Name: "a stale visibility choice cannot reverse a newer one",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			applied, err := h.BookState().SetBookHidden(owner, bookID, 0, true)
			require.NoError(t, err)
			require.True(t, applied)
			applied, err = h.BookState().SetBookHidden(owner, bookID, 1, false)
			require.NoError(t, err)
			require.True(t, applied)
			_, err = h.BookState().SetBookHidden(owner, bookID, 0, true)
			require.ErrorIs(t, err, domain.ErrStaleBookVisibility)
			requireVisibility(t, h, owner, bookID, false, 2)
		},
	}, {
		Name: "visibility is owner-scoped",
		Run: func(t *testing.T, h Harness) {
			owner, other := h.Seeds().Owner(), h.Seeds().OtherOwner()
			bookID := seedBook(t, h, BookSeed{})
			_, err := h.BookState().SetBookHidden(other, bookID, 0, true)
			require.ErrorIs(t, err, domain.ErrNotFound)
			_, _, err = h.BookState().GetBookVisibility(other, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
			requireVisibility(t, h, owner, bookID, false, 0)
		},
	}, {
		Name: "visibility is independent of disposition",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			applied, err := h.BookState().SetBookHidden(owner, bookID, 0, true)
			require.NoError(t, err)
			require.True(t, applied)
			detail := requireDetail(t, h, owner, bookID)
			applied, err = h.BookState().TransitionBookDisposition(owner, bookID, detail.DispositionRevision, domain.BookDispositionToRead)
			require.NoError(t, err)
			require.True(t, applied)
			detail = requireDetail(t, h, owner, bookID)
			assert.True(t, detail.Hidden)
			assert.EqualValues(t, 1, detail.VisibilityRevision)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)

			_, err = h.BookState().SetBookHidden(owner, bookID, 1, false)
			require.NoError(t, err)
			detail = requireDetail(t, h, owner, bookID)
			assert.False(t, detail.Hidden)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
		},
	}}
}

func dispositionScenarios() []Scenario {
	return []Scenario{{
		Name: "a disposition change applies at the expected revision and a verified replay is a no-op",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			initial := requireDetail(t, h, owner, bookID).DispositionRevision
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.NoError(t, err)
			assert.True(t, applied)
			applied, err = h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.NoError(t, err)
			assert.False(t, applied)
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
			assert.Equal(t, initial+1, detail.DispositionRevision)
		},
	}, {
		Name: "a stale disposition choice cannot reverse a newer one",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			initial := requireDetail(t, h, owner, bookID).DispositionRevision
			applied, err := h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.NoError(t, err)
			require.True(t, applied)
			applied, err = h.BookState().TransitionBookDisposition(owner, bookID, initial+1, domain.BookDispositionInbox)
			require.NoError(t, err)
			require.True(t, applied)
			_, err = h.BookState().TransitionBookDisposition(owner, bookID, initial, domain.BookDispositionToRead)
			require.ErrorIs(t, err, domain.ErrStaleBookDisposition)
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionInbox, detail.Disposition)
			assert.Equal(t, initial+2, detail.DispositionRevision)
		},
	}, {
		Name: "disposition is owner-scoped",
		Run: func(t *testing.T, h Harness) {
			owner, other := h.Seeds().Owner(), h.Seeds().OtherOwner()
			bookID := seedBook(t, h, BookSeed{})
			initial := requireDetail(t, h, owner, bookID).DispositionRevision
			_, err := h.BookState().TransitionBookDisposition(other, bookID, initial, domain.BookDispositionToRead)
			require.ErrorIs(t, err, domain.ErrNotFound)
			detail := requireDetail(t, h, owner, bookID)
			assert.Equal(t, domain.BookDispositionInbox, detail.Disposition)
			assert.Equal(t, initial, detail.DispositionRevision)
		},
	}}
}

func importScenarios() []Scenario {
	return []Scenario{{
		Name: "previously-read import records a chosen-language Book that is not the current reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			completion, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			assert.Equal(t, domain.ReadingCompletionPreviouslyRead, completion.Source)
			assert.Equal(t, bookID, completion.BookID)
			assert.Equal(t, Language, completion.Language)
		},
	}, {
		Name: "previously-read import replays the same completion",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{})
			first, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			second, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.NoError(t, err)
			assert.True(t, second.CompletedAt.Equal(first.CompletedAt))
			assert.Equal(t, first.Source, second.Source)
		},
	}, {
		Name: "previously-read import refuses the current reading",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID, _ := startReading(t, h, defaultVocabulary)
			_, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}, {
		Name: "previously-read import refuses a Book whose language is not chosen",
		Run: func(t *testing.T, h Harness) {
			owner := h.Seeds().Owner()
			bookID := seedBook(t, h, BookSeed{LanguageUnknown: true})
			_, err := h.BookState().ImportPreviouslyRead(owner, bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}, {
		Name: "previously-read import is owner-scoped",
		Run: func(t *testing.T, h Harness) {
			bookID := seedBook(t, h, BookSeed{})
			_, err := h.BookState().ImportPreviouslyRead(h.Seeds().OtherOwner(), bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		},
	}}
}
