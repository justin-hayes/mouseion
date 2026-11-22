package readingintent

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type bookResult struct {
	book domain.MyBook
	err  error
}

// fakeBooks returns the queued results in order and repeats the last one.
type fakeBooks struct {
	results []bookResult
	calls   int
}

func (f *fakeBooks) GetBookDetail(context.Context, string, string) (domain.MyBook, error) {
	f.calls++
	if len(f.results) == 0 {
		return domain.MyBook{}, nil
	}
	index := min(f.calls-1, len(f.results)-1)
	return f.results[index].book, f.results[index].err
}

// fakeCurrentReadings returns one current reading and records the language it
// was asked about.
type fakeCurrentReadings struct {
	current  domain.CurrentReading
	err      error
	calls    int
	language string
}

func (f *fakeCurrentReadings) GetCurrentReading(_ context.Context, _, language string) (domain.CurrentReading, error) {
	f.calls++
	f.language = language
	return f.current, f.err
}

type fakeDispositions struct {
	applied     bool
	err         error
	calls       int
	bookID      string
	revision    int64
	disposition domain.BookDisposition
}

func (f *fakeDispositions) TransitionBookDisposition(_ context.Context, _, bookID string, expectedRevision int64, disposition domain.BookDisposition) (bool, error) {
	f.calls++
	f.bookID = bookID
	f.revision = expectedRevision
	f.disposition = disposition
	return f.applied, f.err
}

// fakePreparation is the one seam Move and Retry depend on. It returns the
// queued submissions in order and repeats the last one.
type fakePreparation struct {
	submissions []Submission
	calls       int
}

func (f *fakePreparation) Prepare(_ context.Context, _ string, book domain.MyBook) Submission {
	f.calls++
	if len(f.submissions) == 0 {
		return Submission{Book: book}
	}
	index := min(f.calls-1, len(f.submissions)-1)
	submission := f.submissions[index]
	if submission.Book.Book.ID == "" {
		submission.Book = book
	}
	return submission
}

// harness wires a module over in-memory fakes.
type harness struct {
	books        *fakeBooks
	current      *fakeCurrentReadings
	dispositions *fakeDispositions
	preparation  *fakePreparation
	module       *Module
}

func newHarness(t *testing.T, book domain.MyBook) *harness {
	t.Helper()
	h := &harness{
		books:        &fakeBooks{results: []bookResult{{book: book}}},
		current:      &fakeCurrentReadings{},
		dispositions: &fakeDispositions{applied: true},
		preparation:  &fakePreparation{},
	}
	module, err := New(Config{
		Books: h.books, CurrentReadings: h.current, Dispositions: h.dispositions,
		Preparation: h.preparation,
	})
	require.NoError(t, err)
	h.module = module
	return h
}

func move(t *testing.T, h *harness) Outcome {
	t.Helper()
	outcome, err := h.module.Move(context.Background(), "owner", "book-1", 3)
	require.NoError(t, err)
	return outcome
}

func retry(t *testing.T, h *harness) Outcome {
	t.Helper()
	outcome, err := h.module.Retry(context.Background(), "owner", "book-1")
	require.NoError(t, err)
	return outcome
}

func classicalBook() domain.MyBook {
	return domain.MyBook{
		Book:                domain.Book{ID: "book-1", Title: "Anabasis"},
		ReadingUnit:         &domain.BookReadingUnit{ReadingUnitKey: "anabasis-1"},
		Disposition:         domain.BookDispositionInbox,
		DispositionRevision: 3,
	}
}

func epubBook(sourceID string) domain.MyBook {
	return domain.MyBook{
		Book:                domain.Book{ID: "book-1", Title: "Der lange Weg"},
		Disposition:         domain.BookDispositionInbox,
		DispositionRevision: 3,
		Acquired: &domain.SourceMaterialSummary{
			Source:  domain.SourceMaterial{ID: sourceID},
			Signals: domain.AnalysisSignals{Content: domain.ContentCurrentEPUB},
		},
	}
}

// epubBookWithoutSource is a Book whose EPUB is missing, so acquisition runs.
func epubBookWithoutSource() domain.MyBook {
	book := epubBook("")
	book.Acquired = nil
	return book
}

func analyzedBook() domain.MyBook {
	book := classicalBook()
	book.Acquired = &domain.SourceMaterialSummary{
		Source:  domain.SourceMaterial{ID: "source-1"},
		Signals: domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunCompleted},
	}
	return book
}

func toRead(book domain.MyBook) domain.MyBook {
	book.Disposition = domain.BookDispositionToRead
	return book
}

func toReadClassicalBook() domain.MyBook {
	return toRead(classicalBook())
}

func toReadAnalyzedBook() domain.MyBook {
	return toRead(analyzedBook())
}

func toReadEPUBBook(sourceID string) domain.MyBook {
	return toRead(epubBook(sourceID))
}

func TestNewRequiresCollaborators(t *testing.T) {
	t.Parallel()
	full := Config{
		Books: &fakeBooks{}, CurrentReadings: &fakeCurrentReadings{},
		Dispositions: &fakeDispositions{}, Preparation: &fakePreparation{},
	}
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "books", change: func(c *Config) { c.Books = nil }},
		{name: "current readings", change: func(c *Config) { c.CurrentReadings = nil }},
		{name: "dispositions", change: func(c *Config) { c.Dispositions = nil }},
		{name: "preparation", change: func(c *Config) { c.Preparation = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := full
			tt.change(&cfg)
			_, err := New(cfg)
			require.Error(t, err)
		})
	}
	_, err := New(full)
	require.NoError(t, err)
}

func TestNewPreparationRequiresCollaborators(t *testing.T) {
	t.Parallel()
	full := PreparationConfig{
		Classical: &fakeClassicalPreparation{}, Analysis: &fakeAnalysis{}, EPUBAcquisition: &fakeEPUBAcquisition{},
	}
	tests := []struct {
		name   string
		change func(*PreparationConfig)
	}{
		{name: "classical preparation", change: func(c *PreparationConfig) { c.Classical = nil }},
		{name: "analysis", change: func(c *PreparationConfig) { c.Analysis = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := full
			tt.change(&cfg)
			_, err := NewPreparation(cfg)
			require.Error(t, err)
		})
	}
	_, err := NewPreparation(full)
	require.NoError(t, err, "acquisition is optional")
}

func TestMoveNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t, domain.MyBook{})
	h.books.results = []bookResult{{err: domain.ErrNotFound}}

	outcome := move(t, h)

	assert.Equal(t, OutcomeNotFound, outcome.Kind)
	assert.Zero(t, h.dispositions.calls, "a missing Book is never transitioned")
}

func TestMoveStaleRevision(t *testing.T) {
	t.Parallel()
	h := newHarness(t, classicalBook())
	h.dispositions.err = domain.ErrStaleBookDisposition

	outcome := move(t, h)

	assert.Equal(t, OutcomeStaleRevision, outcome.Kind)
	assert.Equal(t, "book-1", outcome.Book.Book.ID)
	assert.Zero(t, h.preparation.calls)
}

func TestMoveUnchangedDoesNotSubmit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, classicalBook())
	h.dispositions.applied = false

	outcome := move(t, h)

	assert.Equal(t, OutcomeUnchanged, outcome.Kind)
	assert.Equal(t, "book-1", h.dispositions.bookID)
	assert.EqualValues(t, 3, h.dispositions.revision)
	assert.Equal(t, domain.BookDispositionToRead, h.dispositions.disposition)
	assert.Zero(t, h.preparation.calls, "an unchanged disposition starts no preparation")
}

func TestMoveAlreadyToReadIsUnchangedEvenWhenApplied(t *testing.T) {
	t.Parallel()
	for name, book := range map[string]domain.MyBook{
		"classical": toReadClassicalBook(),
		"epub":      toReadEPUBBook("source-1"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, book)
			h.dispositions.applied = true

			outcome := move(t, h)

			assert.Equal(t, OutcomeUnchanged, outcome.Kind)
			assert.Equal(t, 1, h.dispositions.calls)
			assert.Zero(t, h.preparation.calls, "moving a To Read Book again starts no preparation")
		})
	}
}

// A Book deleted between the load and the transition is an unexpected
// infrastructure error, exactly as the handlers treated it before the module.
func TestMoveTransitionNotFoundIsReturned(t *testing.T) {
	t.Parallel()
	h := newHarness(t, classicalBook())
	h.dispositions.err = domain.ErrNotFound

	_, err := h.module.Move(context.Background(), "owner", "book-1", 3)

	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Zero(t, h.preparation.calls)
}

// TestMoveClassifiesSubmissions proves reading intent owns the classification
// of every typed submission the seam can return.
func TestMoveClassifiesSubmissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		submission Submission
		want       OutcomeKind
		job        int64
	}{
		{name: "queued", submission: Submission{Kind: SubmissionQueued, Route: RouteClassical, JobNumber: 7}, want: OutcomeJobQueued, job: 7},
		{name: "already current", submission: Submission{Kind: SubmissionAlreadyCurrent, Route: RouteClassical}, want: OutcomeAlreadyCurrent},
		{name: "legacy rows", submission: Submission{Kind: SubmissionRefused, Refusal: RefusalLegacyRows, Route: RouteClassical}, want: OutcomeLegacyRows},
		{name: "withdrawn definition", submission: Submission{Kind: SubmissionRefused, Refusal: RefusalWithdrawnDefinition, Route: RouteClassical}, want: OutcomeWithdrawnDefinition},
		{name: "source refused", submission: Submission{Kind: SubmissionRefused, Refusal: RefusalSourceRefused, Route: RouteClassical}, want: OutcomeSourceRefused},
		{name: "preparation unavailable", submission: Submission{Kind: SubmissionRefused, Refusal: RefusalPreparationUnavailable, Route: RouteClassical}, want: OutcomePreparationUnavailable},
		{name: "no usable units", submission: Submission{Kind: SubmissionRefused, Refusal: RefusalNoUsableEPUBUnits, Route: RouteEPUB}, want: OutcomeNoUsableEPUBUnits},
		{name: "could not start", submission: Submission{Kind: SubmissionCouldNotStart, Route: RouteClassical}, want: OutcomeCouldNotStart},
		{name: "unspecified", submission: Submission{}, want: OutcomeCouldNotStart},
		{name: "unknown refusal", submission: Submission{Kind: SubmissionRefused, Route: RouteEPUB}, want: OutcomeCouldNotStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, classicalBook())
			h.preparation.submissions = []Submission{tt.submission}

			outcome := move(t, h)

			assert.Equal(t, tt.want, outcome.Kind)
			assert.Equal(t, tt.submission.Route, outcome.Route)
			assert.Equal(t, tt.job, outcome.JobNumber)
			assert.Equal(t, 1, h.preparation.calls)
			assert.Equal(t, "book-1", outcome.Book.Book.ID)
		})
	}
}

func TestMoveClassicalWithAnalysisInEffectSubmitsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, analyzedBook())

	outcome := move(t, h)

	assert.Equal(t, OutcomeMoved, outcome.Kind)
	assert.Equal(t, RouteClassical, outcome.Route)
	assert.Zero(t, h.preparation.calls, "settled evidence is not prepared again")
}

func TestMoveReturnsUnexpectedErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("database unavailable")
	tests := []struct {
		name  string
		setup func(*harness)
	}{
		{name: "load", setup: func(h *harness) { h.books.results = []bookResult{{err: boom}} }},
		{name: "transition", setup: func(h *harness) { h.dispositions.err = boom }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, classicalBook())
			tt.setup(h)
			_, err := h.module.Move(context.Background(), "owner", "book-1", 3)
			require.ErrorIs(t, err, boom)
		})
	}
}

func TestRetryNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t, domain.MyBook{})
	h.books.results = []bookResult{{err: domain.ErrNotFound}}

	outcome := retry(t, h)

	assert.Equal(t, OutcomeNotFound, outcome.Kind)
	assert.Zero(t, h.current.calls, "a missing Book is never checked against the current reading")
	assert.Zero(t, h.dispositions.calls)
	assert.Zero(t, h.preparation.calls)
}

func TestRetryReturnsLoadError(t *testing.T) {
	t.Parallel()
	boom := errors.New("database unavailable")
	h := newHarness(t, domain.MyBook{})
	h.books.results = []bookResult{{err: boom}}

	_, err := h.module.Retry(context.Background(), "owner", "book-1")

	require.ErrorIs(t, err, boom)
	assert.Zero(t, h.current.calls)
	assert.Zero(t, h.preparation.calls)
}

func TestRetryReturnsCurrentReadingError(t *testing.T) {
	t.Parallel()
	boom := errors.New("current reading read failed")
	h := newHarness(t, toReadClassicalBook())
	h.current.err = boom

	_, err := h.module.Retry(context.Background(), "owner", "book-1")

	require.ErrorIs(t, err, boom)
	assert.Zero(t, h.dispositions.calls)
	assert.Zero(t, h.preparation.calls)
}

func TestRetryRefusesCurrentBook(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		book domain.MyBook
	}{
		{name: "To Read", book: toReadClassicalBook()},
		{name: "not To Read", book: classicalBook()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, tt.book)
			h.current.current = domain.CurrentReading{BookID: "book-1"}

			outcome := retry(t, h)

			assert.Equal(t, OutcomeCurrentBookRefused, outcome.Kind, "the current Book guard runs before the To Read guard")
			assert.Equal(t, "book-1", outcome.Book.Book.ID)
			assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
			assert.Zero(t, h.preparation.calls, "the current Book is not prepared again")
		})
	}
}

func TestRetryRefusesBookNotToRead(t *testing.T) {
	t.Parallel()
	book := classicalBook()
	book.Book.LanguageTag = "grc"
	h := newHarness(t, book)

	outcome := retry(t, h)

	assert.Equal(t, OutcomeNotToRead, outcome.Kind)
	assert.Equal(t, "grc", h.current.language, "the current reading is read for the Book's language")
	assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
	assert.Zero(t, h.preparation.calls)
}

func TestRetryClassifiesSubmissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		book       domain.MyBook
		submission Submission
		want       OutcomeKind
		job        int64
	}{
		{name: "queued", book: toReadClassicalBook(), submission: Submission{Kind: SubmissionQueued, Route: RouteClassical, JobNumber: 7}, want: OutcomeJobQueued, job: 7},
		{name: "already current", book: toReadAnalyzedBook(), submission: Submission{Kind: SubmissionAlreadyCurrent, Route: RouteClassical}, want: OutcomeAlreadyCurrent},
		{name: "legacy rows", book: toReadClassicalBook(), submission: Submission{Kind: SubmissionRefused, Refusal: RefusalLegacyRows, Route: RouteClassical}, want: OutcomeLegacyRows},
		{name: "withdrawn definition", book: toReadClassicalBook(), submission: Submission{Kind: SubmissionRefused, Refusal: RefusalWithdrawnDefinition, Route: RouteClassical}, want: OutcomeWithdrawnDefinition},
		{name: "source refused", book: toReadClassicalBook(), submission: Submission{Kind: SubmissionRefused, Refusal: RefusalSourceRefused, Route: RouteClassical}, want: OutcomeSourceRefused},
		{name: "preparation unavailable", book: toReadClassicalBook(), submission: Submission{Kind: SubmissionRefused, Refusal: RefusalPreparationUnavailable, Route: RouteClassical}, want: OutcomePreparationUnavailable},
		{name: "could not start", book: toReadClassicalBook(), submission: Submission{Kind: SubmissionCouldNotStart, Route: RouteClassical}, want: OutcomeCouldNotStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, tt.book)
			h.preparation.submissions = []Submission{tt.submission}

			outcome := retry(t, h)

			assert.Equal(t, tt.want, outcome.Kind)
			assert.Equal(t, tt.submission.Route, outcome.Route)
			assert.Equal(t, tt.job, outcome.JobNumber)
			assert.Equal(t, 1, h.preparation.calls)
			assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
			assert.Equal(t, "book-1", outcome.Book.Book.ID)
		})
	}
}

func TestRetryClassifiesCurrentBookRaceAsRefusal(t *testing.T) {
	t.Parallel()
	h := newHarness(t, toReadEPUBBook("source-1"))
	h.preparation.submissions = []Submission{{
		Kind: SubmissionCouldNotStart, Route: RouteEPUB, Book: toReadEPUBBook("source-1"), Err: analysis.ErrCurrentBook,
	}}

	outcome := retry(t, h)

	assert.Equal(t, OutcomeCurrentBookRefused, outcome.Kind)
}

// recordingPreparation records which implementation the seam chose.
type recordingPreparation struct {
	calls int
	route Route
}

func (r *recordingPreparation) Prepare(_ context.Context, _ string, book domain.MyBook) Submission {
	r.calls++
	return Submission{Kind: SubmissionQueued, Route: r.route, Book: book}
}

func TestPreparationsChoosesImplementationByReadingUnit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		book           domain.MyBook
		want           Route
		classicalCalls int
		epubCalls      int
	}{
		{name: "classical", book: classicalBook(), want: RouteClassical, classicalCalls: 1},
		{name: "epub", book: epubBook("source-1"), want: RouteEPUB, epubCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			classicalSide := &recordingPreparation{route: RouteClassical}
			epubSide := &recordingPreparation{route: RouteEPUB}
			seam := preparations{classical: classicalSide, epub: epubSide}

			submission := seam.Prepare(context.Background(), "owner", tt.book)

			assert.Equal(t, tt.want, submission.Route)
			assert.Equal(t, tt.classicalCalls, classicalSide.calls)
			assert.Equal(t, tt.epubCalls, epubSide.calls)
		})
	}
}

// fakeClassicalPreparation records the existing Classical preparation
// submission the implementation wraps.
type fakeClassicalPreparation struct {
	handle analysis.ClassicalPreparationHandle
	err    error
	calls  int
	owner  string
	bookID string
}

func (f *fakeClassicalPreparation) SubmitClassicalPreparation(_ context.Context, owner, bookID string) (analysis.ClassicalPreparationHandle, error) {
	f.calls++
	f.owner, f.bookID = owner, bookID
	return f.handle, f.err
}

func TestClassicalPreparationSubmissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		handle  analysis.ClassicalPreparationHandle
		err     error
		kind    SubmissionKind
		refusal RefusalCode
		job     int64
	}{
		{name: "already current", handle: analysis.ClassicalPreparationHandle{Complete: true}, kind: SubmissionAlreadyCurrent},
		{name: "queued", handle: analysis.ClassicalPreparationHandle{JobID: 7, Queued: true}, kind: SubmissionQueued, job: 7},
		{name: "legacy rows", err: classical.ErrLegacyRows, kind: SubmissionRefused, refusal: RefusalLegacyRows},
		{name: "withdrawn definition", err: analysis.ErrWithdrawnClassicalDefinition, kind: SubmissionRefused, refusal: RefusalWithdrawnDefinition},
		{name: "source refused", handle: analysis.ClassicalPreparationHandle{Terminal: true}, kind: SubmissionRefused, refusal: RefusalSourceRefused},
		{name: "preparation unavailable", err: ErrPreparationUnavailable, kind: SubmissionRefused, refusal: RefusalPreparationUnavailable},
		{name: "could not start", err: errors.New("preparation failed"), kind: SubmissionCouldNotStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			submission := &fakeClassicalPreparation{handle: tt.handle, err: tt.err}
			preparation := classicalPreparation{submission: submission}

			got := preparation.Prepare(context.Background(), "owner", classicalBook())

			assert.Equal(t, tt.kind, got.Kind)
			assert.Equal(t, tt.refusal, got.Refusal)
			assert.Equal(t, tt.job, got.JobNumber)
			assert.Equal(t, RouteClassical, got.Route)
			assert.Equal(t, "book-1", got.Book.Book.ID)
			assert.Equal(t, 1, submission.calls)
			assert.Equal(t, "owner", submission.owner)
			assert.Equal(t, "book-1", submission.bookID)
		})
	}
}

func TestClassicalFailureLogCarriesStageAndCodeOnly(t *testing.T) {
	submission := &fakeClassicalPreparation{err: errors.New("cannot import Ἀνάβασις from source")}
	preparation := classicalPreparation{submission: submission}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", classicalBook())
	line := logs.String()

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	t.Run("names the stable stage and code", func(t *testing.T) {
		require.Contains(t, line, "mouseion: classical preparation submission failed")
		require.Contains(t, line, `stage="import"`)
		require.Contains(t, line, `code="classical_preparation_failed"`)
	})
	t.Run("quotes the identifiers", func(t *testing.T) {
		require.Contains(t, line, `owner "owner"`)
		require.Contains(t, line, `book "book-1"`)
	})
	t.Run("omits raw error text", func(t *testing.T) {
		require.NotContains(t, line, "Ἀνάβασις")
		require.NotContains(t, line, "cannot import")
	})
}

func TestPreparationUnavailableIsNotLoggedAsFailure(t *testing.T) {
	submission := &fakeClassicalPreparation{err: ErrPreparationUnavailable}
	preparation := classicalPreparation{submission: submission}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", classicalBook())

	require.Equal(t, SubmissionRefused, got.Kind)
	assert.Empty(t, logs.String(), "an absent preparation seam is not a failed submission")
}

// fakeAnalysis records the analysis submission the EPUB implementation makes.
type fakeAnalysis struct {
	handle   analysis.Handle
	err      error
	calls    int
	sourceID string
}

func (f *fakeAnalysis) SubmitToReadBookAnalysis(_ context.Context, _, _, sourceID string) (analysis.Handle, error) {
	f.calls++
	f.sourceID = sourceID
	return f.handle, f.err
}

// fakeEPUBAcquisition records the acquisition submission the EPUB
// implementation makes. River itself enforces one live job per Book.
type fakeEPUBAcquisition struct {
	handle analysis.EPUBAcquisitionHandle
	err    error
	calls  int
	owner  string
	bookID string
}

func (f *fakeEPUBAcquisition) SubmitEPUBAcquisition(_ context.Context, owner, bookID string) (analysis.EPUBAcquisitionHandle, error) {
	f.calls++
	f.owner, f.bookID = owner, bookID
	return f.handle, f.err
}

func TestEPUBPreparationEnqueuesAcquisitionForAMissingSource(t *testing.T) {
	t.Parallel()
	acquisition := &fakeEPUBAcquisition{handle: analysis.EPUBAcquisitionHandle{JobID: 91}}
	analysisSubmission := &fakeAnalysis{}
	preparation := epubPreparation{analysis: analysisSubmission, acquisition: acquisition}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	assert.Equal(t, SubmissionQueued, got.Kind)
	assert.Equal(t, RouteEPUB, got.Route)
	assert.EqualValues(t, 91, got.JobNumber)
	assert.Equal(t, 1, acquisition.calls, "the Book without an EPUB enqueues one acquisition")
	assert.Equal(t, "owner", acquisition.owner)
	assert.Equal(t, "book-1", acquisition.bookID)
	assert.Zero(t, analysisSubmission.calls, "acquisition queues no analysis until the EPUB exists")
}

func TestEPUBPreparationSubmitsAnalysisForABookThatAlreadyHasItsEPUB(t *testing.T) {
	t.Parallel()
	acquisition := &fakeEPUBAcquisition{}
	analysisSubmission := &fakeAnalysis{handle: analysis.Handle{ID: 4, DisplayNumber: 12, RunID: "run-1"}}
	preparation := epubPreparation{analysis: analysisSubmission, acquisition: acquisition}

	got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

	assert.Equal(t, SubmissionQueued, got.Kind)
	assert.Equal(t, RouteEPUB, got.Route)
	assert.EqualValues(t, 12, got.JobNumber)
	assert.Equal(t, "source-1", analysisSubmission.sourceID)
	assert.Zero(t, acquisition.calls, "a current source is not acquired again")
}

func TestEPUBPreparationWithoutAnAcquisitionSeamCouldNotStart(t *testing.T) {
	t.Parallel()
	analysisSubmission := &fakeAnalysis{}
	preparation := epubPreparation{analysis: analysisSubmission}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	assert.Equal(t, SubmissionCouldNotStart, got.Kind)
	assert.Equal(t, RouteEPUB, got.Route)
	require.ErrorIs(t, got.Err, ErrAcquisitionUnavailable)
	assert.Zero(t, analysisSubmission.calls)
}

func TestEPUBPreparationAcquisitionSubmissionFailureCouldNotStart(t *testing.T) {
	t.Parallel()
	boom := errors.New("queue unavailable")
	analysisSubmission := &fakeAnalysis{}
	preparation := epubPreparation{
		analysis:    analysisSubmission,
		acquisition: &fakeEPUBAcquisition{err: boom},
	}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	assert.Equal(t, SubmissionCouldNotStart, got.Kind)
	assert.Equal(t, RouteEPUB, got.Route)
	require.ErrorIs(t, got.Err, boom)
	assert.Zero(t, analysisSubmission.calls)
}

func TestEPUBAnalysisSubmissions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		err     error
		kind    SubmissionKind
		refusal RefusalCode
	}{
		{name: "no usable units", err: domain.ErrExtractedUnitsUnavailable, kind: SubmissionRefused, refusal: RefusalNoUsableEPUBUnits},
		{name: "EPUB required", err: analysis.ErrEPUBRequired, kind: SubmissionRefused, refusal: RefusalNoUsableEPUBUnits},
		{name: "could not start", err: errors.New("analysis failed"), kind: SubmissionCouldNotStart},
		{name: "current book raced", err: analysis.ErrCurrentBook, kind: SubmissionCouldNotStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			analysisSubmission := &fakeAnalysis{err: tt.err}
			preparation := epubPreparation{analysis: analysisSubmission}

			got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

			assert.Equal(t, tt.kind, got.Kind)
			assert.Equal(t, tt.refusal, got.Refusal)
			assert.Equal(t, RouteEPUB, got.Route)
			assert.Equal(t, 1, analysisSubmission.calls)
		})
	}
}

func TestEPUBAcquisitionSubmissionFailureLogQuotesTheErrorAndStep(t *testing.T) {
	preparation := epubPreparation{
		analysis: &fakeAnalysis{},
		acquisition: &fakeEPUBAcquisition{
			err: errors.New("queue unavailable\nmouseion: injected"),
		},
	}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())
	line := logs.String()

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	t.Run("keeps one log line", func(t *testing.T) {
		require.Equal(t, 1, strings.Count(line, "\n"))
	})
	t.Run("names the step and quotes the error", func(t *testing.T) {
		require.Contains(t, line, "mouseion: To Read acquisition submission failed")
		require.Contains(t, line, `book "book-1": "queue unavailable\nmouseion: injected"`)
	})
}

func TestEPUBAnalysisFailureLogNamesTheStep(t *testing.T) {
	preparation := epubPreparation{
		analysis: &fakeAnalysis{err: errors.New("submission failed")},
	}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	assert.Contains(t, logs.String(), `mouseion: To Read analysis submission failed for owner "owner" book "book-1": "submission failed"`, "identifiers are quoted")
}

func TestCurrentBookRefusalIsNotLoggedAsFailure(t *testing.T) {
	preparation := epubPreparation{
		analysis: &fakeAnalysis{err: analysis.ErrCurrentBook},
	}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	assert.Empty(t, logs.String())
}

// captureLog redirects the standard logger for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &output
}
