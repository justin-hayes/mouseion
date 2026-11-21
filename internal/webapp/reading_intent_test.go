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

// noopPreparation stands in for the seam when a Move returns before preparing.
type noopPreparation struct{}

func (noopPreparation) Prepare(context.Context, string, domain.MyBook) readingintent.Submission {
	return readingintent.Submission{}
}

// missingDispositions reports a Book that left the collection between the
// Work list and its move.
type missingDispositions struct{}

func (missingDispositions) TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error) {
	return false, domain.ErrNotFound
}

// acceptingDispositions accepts a real move so the route is exercised.
type acceptingDispositions struct{}

func (acceptingDispositions) TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error) {
	return true, nil
}

// refusedPreparation returns a stable refusal for the Book.
type refusedPreparation struct{ refusal readingintent.RefusalCode }

func (f refusedPreparation) Prepare(_ context.Context, _ string, book domain.MyBook) readingintent.Submission {
	return readingintent.Submission{Kind: readingintent.SubmissionRefused, Refusal: f.refusal, Route: readingintent.RouteClassical, Book: book}
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
			Books: failingBookReader{}, CurrentReadings: fixtures.NewStore(), Dispositions: fixtures.NewStore(),
			Preparation: noopPreparation{},
		})
		require.NoError(t, err)
		h := &Handler{services: Services{ReadingIntent: module}}
		assert.Equal(t, moveWorkToReadOutcome{failed: 1}, h.moveWorkBookToRead(context.Background(), "owner", eligible))
	})
	t.Run("missing Book is unchanged", func(t *testing.T) {
		t.Parallel()
		module, err := readingintent.New(readingintent.Config{
			Books: fixtures.NewStore(), CurrentReadings: fixtures.NewStore(), Dispositions: fixtures.NewStore(),
			Preparation: noopPreparation{},
		})
		require.NoError(t, err)
		h := &Handler{services: Services{ReadingIntent: module}}
		assert.Equal(t, moveWorkToReadOutcome{unchanged: 1}, h.moveWorkBookToRead(context.Background(), "owner", eligible))
	})
	t.Run("vanished Book is unchanged", func(t *testing.T) {
		t.Parallel()
		module, err := readingintent.New(readingintent.Config{
			Books: staticBookReader{book: eligible}, CurrentReadings: fixtures.NewStore(), Dispositions: missingDispositions{},
			Preparation: noopPreparation{},
		})
		require.NoError(t, err)
		h := &Handler{services: Services{ReadingIntent: module}}
		assert.Equal(t, moveWorkToReadOutcome{unchanged: 1}, h.moveWorkBookToRead(context.Background(), "owner", eligible))
	})
}

func TestMoveWorkBookToReadCountsNamedPreparationCauses(t *testing.T) {
	t.Parallel()
	eligible := domain.MyBook{
		Book:        domain.Book{ID: "book-1", LanguageState: domain.LanguageChosen, LanguageTag: classical.Language},
		ReadingUnit: &domain.BookReadingUnit{ReadingUnitKey: "unit-1"},
	}
	tests := []struct {
		name    string
		refusal readingintent.RefusalCode
		want    moveWorkToReadOutcome
	}{
		{name: "legacy rows", refusal: readingintent.RefusalLegacyRows, want: moveWorkToReadOutcome{moved: 1, legacyRows: 1}},
		{name: "withdrawn definition", refusal: readingintent.RefusalWithdrawnDefinition, want: moveWorkToReadOutcome{moved: 1, withdrawn: 1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			module, err := readingintent.New(readingintent.Config{
				Books: staticBookReader{book: eligible}, CurrentReadings: fixtures.NewStore(),
				Dispositions: acceptingDispositions{},
				Preparation:  refusedPreparation{refusal: tc.refusal},
			})
			require.NoError(t, err)
			h := &Handler{services: Services{ReadingIntent: module}}
			require.Equal(t, tc.want, h.moveWorkBookToRead(context.Background(), "owner", eligible))
		})
	}
}

func TestWorkToReadNoticeNamesPreparationCausesSeparately(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		outcome moveWorkToReadOutcome
		want    string
	}{
		{
			name: "every part",
			outcome: moveWorkToReadOutcome{
				moved: 6, unchanged: 1, failed: 1, prepFailed: 1, legacyRows: 2, withdrawn: 1,
			},
			want: "Moved 6 Books to To Read. Left 1 Book unchanged. Could not update 1 Book. " +
				"Preparation could not start for 1 Book. " +
				"2 Books cannot be prepared while this installation converts its legacy Ancient Greek rows. " +
				"1 Book has a withdrawn definition and cannot be prepared.",
		},
		{
			name:    "singular legacy rows",
			outcome: moveWorkToReadOutcome{moved: 1, legacyRows: 1},
			want: "Moved 1 Book to To Read. " +
				"1 Book cannot be prepared while this installation converts its legacy Ancient Greek rows.",
		},
		{
			name:    "plural withdrawn definitions",
			outcome: moveWorkToReadOutcome{moved: 3, withdrawn: 3},
			want: "Moved 3 Books to To Read. " +
				"3 Books have a withdrawn definition and cannot be prepared.",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, workToReadNotice(tc.outcome))
		})
	}
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
		{name: "stale revision", outcome: readingintent.Outcome{Kind: readingintent.OutcomeStaleRevision}, want: "This Book's decision changed in another tab. No changes were made; refresh My Books and try again."},
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
		{
			name: "acquisition download limit",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Title: "Der lange Weg", Err: opds.ErrDownloadTooLarge, AcquisitionFailed: true,
			},
			want: `Book moved to To Read, but Could not acquire "Der lange Weg" from connection catalog connection: This EPUB is larger than the 512 MiB download limit. Choose a smaller edition. The To Read status is retained; analysis is unavailable until the current EPUB can be acquired.`,
		},
		{
			name: "acquisition stored-content limit",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Title: "Der lange Weg", Err: opds.ErrRetainedEPUBTooLarge, AcquisitionFailed: true,
			},
			want: `Book moved to To Read, but Could not acquire "Der lange Weg" from connection catalog connection: This EPUB is larger than the 100 MiB limit for stored book content. Choose a less illustrated edition. The To Read status is retained; analysis is unavailable until the current EPUB can be acquired.`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, moveToReadMessage(context.Background(), catalog, "owner", "book-1", tt.outcome))
		})
	}
}

// TestRetryMessages proves the mapping from each Retry outcome to the
// learner-facing sentence. Every sentence predates the module and must not
// change with it.
func TestRetryMessages(t *testing.T) {
	t.Parallel()
	catalog := fixtures.NewStore()
	tests := []struct {
		name    string
		outcome readingintent.Outcome
		want    string
		isError bool
	}{
		{name: "current book refused", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCurrentBookRefused}, want: "The current book cannot be refreshed. Its frozen evidence remains unchanged.", isError: true},
		{name: "no preparation seam", outcome: readingintent.Outcome{Kind: readingintent.OutcomePreparationUnavailable}, want: "Classical preparation is unavailable.", isError: true},
		{name: "already current", outcome: readingintent.Outcome{Kind: readingintent.OutcomeAlreadyCurrent}, want: "The analysis is already current."},
		{name: "classical job queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued, Route: readingintent.RouteClassical, JobNumber: 7}, want: "Preparation job #7 submitted."},
		{name: "analysis job queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued, Route: readingintent.RouteEPUB, JobNumber: 9}, want: "Analysis job #9 submitted."},
		{name: "legacy rows", outcome: readingintent.Outcome{Kind: readingintent.OutcomeLegacyRows}, want: classical.LegacyRowsReason, isError: true},
		{name: "withdrawn definition", outcome: readingintent.Outcome{Kind: readingintent.OutcomeWithdrawnDefinition}, want: classicalWithdrawnRefusal, isError: true},
		{name: "source refused", outcome: readingintent.Outcome{Kind: readingintent.OutcomeSourceRefused}, want: classicalSourceMismatchRefusal, isError: true},
		{name: "classical could not start", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart, Route: readingintent.RouteClassical}, want: "Preparation could not be started. Retry when ready.", isError: true},
		{
			name:    "analysis could not start",
			outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart, Route: readingintent.RouteEPUB, Err: assert.AnError},
			want:    "Analysis could not be started. The To Read choice is retained. Review current book content in My Books and try again.",
			isError: true,
		},
		{name: "no usable EPUB units", outcome: readingintent.Outcome{Kind: readingintent.OutcomeNoUsableEPUBUnits}, want: "The current source has no usable EPUB units. The To Read choice is retained; analysis is unavailable.", isError: true},
		{
			name: "acquisition failed",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Title: "Der lange Weg", Err: opds.ErrNoEPUB, AcquisitionFailed: true,
			},
			want:    `Could not acquire "Der lange Weg" from connection catalog connection: This book is not available as an EPUB. Choose another edition or format. The To Read choice is retained; analysis is unavailable until the current EPUB can be acquired.`,
			isError: true,
		},
		{
			name: "acquisition names the connection and entry",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Target: cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Entry: opds.Entry{Title: "Der Weg"}},
				Title:  "Book", Err: opds.ErrNoEPUB, AcquisitionFailed: true,
			},
			want:    `Could not acquire "Der Weg" from connection connection-1: This book is not available as an EPUB. Choose another edition or format. The To Read choice is retained; analysis is unavailable until the current EPUB can be acquired.`,
			isError: true,
		},
		{
			name: "acquisition download limit",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Title: "Der lange Weg", Err: opds.ErrDownloadTooLarge, AcquisitionFailed: true,
			},
			want:    `Could not acquire "Der lange Weg" from connection catalog connection: This EPUB is larger than the 512 MiB download limit. Choose a smaller edition. The To Read choice is retained; analysis is unavailable until the current EPUB can be acquired.`,
			isError: true,
		},
		{
			name: "acquisition stored-content limit",
			outcome: readingintent.Outcome{
				Kind: readingintent.OutcomeAcquisitionFailed, Route: readingintent.RouteEPUB,
				Title: "Der lange Weg", Err: opds.ErrRetainedEPUBTooLarge, AcquisitionFailed: true,
			},
			want:    `Could not acquire "Der lange Weg" from connection catalog connection: This EPUB is larger than the 100 MiB limit for stored book content. Choose a less illustrated edition. The To Read choice is retained; analysis is unavailable until the current EPUB can be acquired.`,
			isError: true,
		},
		{
			name:    "unknown outcome",
			outcome: readingintent.Outcome{Kind: readingintent.OutcomeKind("unknown")},
			want:    analysisCouldNotStartMessage,
			isError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			message, isError := retryMessage(context.Background(), catalog, "owner", tt.outcome)
			assert.Equal(t, tt.want, message)
			assert.Equal(t, tt.isError, isError)
		})
	}
}
