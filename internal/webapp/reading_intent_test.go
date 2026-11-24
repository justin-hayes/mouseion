package webapp

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/readingintent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// noopPreparation stands in for the seam when a Move returns before preparing.
type noopPreparation struct{}

func (noopPreparation) Prepare(context.Context, string, domain.MyBook) readingintent.Submission {
	return readingintent.Submission{}
}

// refusedPreparation returns a stable refusal for the Book.
type refusedPreparation struct{ refusal readingintent.RefusalCode }

func (f refusedPreparation) Prepare(_ context.Context, _ string, book domain.MyBook) readingintent.Submission {
	return readingintent.Submission{Kind: readingintent.SubmissionRefused, Refusal: f.refusal, Route: readingintent.RouteClassical, Book: book}
}

// TestMoveToReadMessages proves the mapping from each reading-intent outcome to
// the learner-facing sentence. The sentences predate the module and must not
// change with it.
func TestMoveToReadMessages(t *testing.T) {
	t.Parallel()
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
		{name: "acquisition job queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued, Route: readingintent.RouteEPUB, JobNumber: 9, Acquisition: true}, want: "Book moved to To Read. Downloading and analyzing the text."},
		{name: "analysis job queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued, Route: readingintent.RouteEPUB, JobNumber: 9}, want: "Book moved to To Read. Analysis job #9 submitted."},
		{name: "legacy rows", outcome: readingintent.Outcome{Kind: readingintent.OutcomeLegacyRows}, want: classical.LegacyRowsReason},
		{name: "withdrawn definition", outcome: readingintent.Outcome{Kind: readingintent.OutcomeWithdrawnDefinition}, want: classicalWithdrawnRefusal},
		{name: "source refused", outcome: readingintent.Outcome{Kind: readingintent.OutcomeSourceRefused}, want: classicalSourceMismatchRefusal},
		{name: "classical could not start", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart, Route: readingintent.RouteClassical}, want: "Book moved to To Read, but preparation could not start. Retry analysis from Reading when ready."},
		{name: "analysis could not start", outcome: readingintent.Outcome{Kind: readingintent.OutcomeCouldNotStart, Route: readingintent.RouteEPUB}, want: "Book moved to To Read, but analysis could not start. Retry analysis from Reading when ready."},
		{name: "no usable EPUB units", outcome: readingintent.Outcome{Kind: readingintent.OutcomeNoUsableEPUBUnits}, want: "Book moved to To Read, but the current source has no usable EPUB units. The To Read status is retained; analysis is unavailable."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, moveToReadMessage(tt.outcome))
		})
	}
}

// TestRetryMessages proves the mapping from each Retry outcome to the
// learner-facing sentence. Every sentence predates the module and must not
// change with it.
func TestRetryMessages(t *testing.T) {
	t.Parallel()
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
		{name: "acquisition job queued", outcome: readingintent.Outcome{Kind: readingintent.OutcomeJobQueued, Route: readingintent.RouteEPUB, JobNumber: 9, Acquisition: true}, want: "Downloading and analyzing the text."},
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
			name:    "unknown outcome",
			outcome: readingintent.Outcome{Kind: readingintent.OutcomeKind("unknown")},
			want:    analysisCouldNotStartMessage,
			isError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			message, isError := retryMessage(tt.outcome)
			assert.Equal(t, tt.want, message)
			assert.Equal(t, tt.isError, isError)
		})
	}
}
