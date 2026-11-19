package webapp

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingBookReader makes the module return an unexpected load error.
type failingBookReader struct{}

func (failingBookReader) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return domain.MyBook{}, errors.New("book read failed")
}

func TestReadingIntentModulePrefersTheComposedModule(t *testing.T) {
	t.Parallel()
	composed := &readingintent.Module{}
	h := &Handler{services: Services{ReadingIntent: composed}}
	got, err := h.readingIntentModule()
	require.NoError(t, err)
	assert.Same(t, composed, got)
}

func TestUnavailableAnalysisReportsItself(t *testing.T) {
	t.Parallel()
	_, err := (unavailableAnalysis{}).SubmitToReadBookAnalysis(context.Background(), "owner", "book-1", "source-1")
	require.ErrorIs(t, err, errReadingIntentAnalysisUnavailable)
}

// staticBookReader returns one Book and leaves the transition to the writer.
type staticBookReader struct{ book domain.MyBook }

func (s staticBookReader) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	return s.book, nil
}

// missingDispositions reports a Book that left the collection between the
// Work list and its move.
type missingDispositions struct{}

func (missingDispositions) TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error) {
	return false, domain.ErrNotFound
}

func TestMoveWorkBookToReadCountsModuleOutcomes(t *testing.T) {
	t.Parallel()
	eligible := domain.MyBook{Book: domain.Book{ID: "book-1"}}
	t.Run("module cannot be composed", func(t *testing.T) {
		t.Parallel()
		h := &Handler{}
		assert.Equal(t, moveWorkToReadOutcome{failed: 1}, h.moveWorkBookToRead(context.Background(), "owner", eligible))
	})
	t.Run("move reports an unexpected error", func(t *testing.T) {
		t.Parallel()
		module, err := readingintent.New(readingintent.Config{
			Books: failingBookReader{}, Dispositions: fixtures.NewStore(),
			Preparation: fixtures.ClassicalPreparation{}, Analysis: fixtures.Analysis{},
		})
		require.NoError(t, err)
		h := &Handler{services: Services{ReadingIntent: module}}
		assert.Equal(t, moveWorkToReadOutcome{failed: 1}, h.moveWorkBookToRead(context.Background(), "owner", eligible))
	})
	t.Run("missing Book is unchanged", func(t *testing.T) {
		t.Parallel()
		module, err := readingintent.New(readingintent.Config{
			Books: fixtures.NewStore(), Dispositions: fixtures.NewStore(),
			Preparation: fixtures.ClassicalPreparation{}, Analysis: fixtures.Analysis{},
		})
		require.NoError(t, err)
		h := &Handler{services: Services{ReadingIntent: module}}
		assert.Equal(t, moveWorkToReadOutcome{unchanged: 1}, h.moveWorkBookToRead(context.Background(), "owner", eligible))
	})
	t.Run("vanished Book is unchanged", func(t *testing.T) {
		t.Parallel()
		module, err := readingintent.New(readingintent.Config{
			Books: staticBookReader{book: eligible}, Dispositions: missingDispositions{},
			Preparation: fixtures.ClassicalPreparation{}, Analysis: fixtures.Analysis{},
		})
		require.NoError(t, err)
		h := &Handler{services: Services{ReadingIntent: module}}
		assert.Equal(t, moveWorkToReadOutcome{unchanged: 1}, h.moveWorkBookToRead(context.Background(), "owner", eligible))
	})
}

// TestMoveToReadMessages proves the mapping from each reading-intent outcome to
// the learner-facing sentence. The sentences predate the module and must not
// change with it.
func TestMoveToReadMessages(t *testing.T) {
	t.Parallel()
	catalog := fixtures.NewStore()
	tests := []struct {
		name    string
		outcome readingintent.Outcome
		want    string
	}{
		{name: "unchanged", outcome: readingintent.Outcome{Kind: readingintent.OutcomeUnchanged}, want: "Book moved to To Read."},
		{name: "moved with nothing to submit", outcome: readingintent.Outcome{Kind: readingintent.OutcomeMoved}, want: "Book moved to To Read."},
		{name: "no preparation seam", outcome: readingintent.Outcome{Kind: readingintent.OutcomePreparationUnavailable}, want: "Book moved to To Read."},
		{name: "already current", outcome: readingintent.Outcome{Kind: readingintent.OutcomeAlreadyCurrent}, want: "Book moved to To Read. Analysis is already current."},
		{name: "classical job queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued, Route: readingintent.RouteClassical, JobNumber: 7}, want: "Book moved to To Read. Preparation job #7 submitted."},
		{name: "analysis job queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued, Route: readingintent.RouteEPUB, JobNumber: 9}, want: "Book moved to To Read. Analysis job #9 submitted."},
		{name: "legacy rows", outcome: readingintent.Outcome{Kind: readingintent.OutcomeLegacyRows}, want: classical.LegacyRowsReason},
		{name: "withdrawn definition", outcome: readingintent.Outcome{Kind: readingintent.OutcomeWithdrawnDefinition}, want: classicalWithdrawnRefusal},
		{name: "source refused", outcome: readingintent.Outcome{Kind: readingintent.OutcomeSourceRefused}, want: classicalSourceMismatchRefusal},
		{name: "classical could not start", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart, Route: readingintent.RouteClassical}, want: "Book moved to To Read, but preparation could not start. Retry analysis from Reading when ready."},
		{name: "analysis could not start", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart, Route: readingintent.RouteEPUB}, want: "Book moved to To Read, but analysis could not start. Retry analysis from Reading when ready."},
		{name: "no usable EPUB units", outcome: readingintent.Outcome{Kind: readingintent.OutcomeNoUsableEPUBUnits}, want: "Book moved to To Read, but the current source has no usable EPUB units. The To Read status is retained; analysis is unavailable."},
		{
			name: "acquisition failed",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Title: "Der lange Weg", Err: opds.ErrNoEPUB, AcquisitionFailed: true,
			},
			want: `Book moved to To Read, but Could not acquire "Der lange Weg" from connection catalog connection: This book is not available as an EPUB. Choose another edition or format. The To Read status is retained; analysis is unavailable until the current EPUB can be acquired.`,
		},
		{
			name: "acquisition names the connection and entry",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Target: cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Entry: opds.Entry{Title: "Der Weg"}},
				Title:  "Book", Err: opds.ErrNoEPUB, AcquisitionFailed: true,
			},
			want: `Book moved to To Read, but Could not acquire "Der Weg" from connection connection-1: This book is not available as an EPUB. Choose another edition or format. The To Read status is retained; analysis is unavailable until the current EPUB can be acquired.`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, moveToReadMessage(context.Background(), catalog, "owner", "book-1", tt.outcome))
		})
	}
}
