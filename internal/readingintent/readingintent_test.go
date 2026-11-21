package readingintent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/testutil"
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

func toReadEPUBBookWithoutSource() domain.MyBook {
	return toRead(epubBookWithoutSource())
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
		Books: &fakeBooks{}, Classical: &fakeClassicalPreparation{}, Analysis: &fakeAnalysis{},
	}
	tests := []struct {
		name   string
		change func(*PreparationConfig)
	}{
		{name: "books", change: func(c *PreparationConfig) { c.Books = nil }},
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
		{name: "acquisition failed", submission: Submission{Kind: SubmissionCouldNotStart, Route: RouteEPUB, AcquisitionFailed: true}, want: OutcomeAcquisitionFailed},
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
		{name: "acquisition failed", book: toReadEPUBBookWithoutSource(), submission: Submission{Kind: SubmissionCouldNotStart, Route: RouteEPUB, AcquisitionFailed: true}, want: OutcomeAcquisitionFailed},
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

// fakeAcquisitionTargets returns one catalogue target or an error.
type fakeAcquisitionTargets struct {
	target cataloguesync.AcquisitionTarget
	err    error
	calls  int
}

func (f *fakeAcquisitionTargets) find(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
	f.calls++
	return f.target, f.err
}

// fakeOPDSConnectionStore serves one connection to the real OPDS service.
type fakeOPDSConnectionStore struct{ connection domain.OpdsConnection }

func (s fakeOPDSConnectionStore) GetOpdsConnection(context.Context, string, string) (domain.OpdsConnection, error) {
	return s.connection, nil
}

// fakeAcquisitionImporter records the private import and returns a source.
type fakeAcquisitionImporter struct {
	source   string
	err      error
	owner    string
	language string
	bookID   string
	content  epub.AcquisitionContent
}

func (*fakeAcquisitionImporter) Import(context.Context, string, string, []byte) (epub.ImportResult, error) {
	return epub.ImportResult{}, errors.New("the acquisition path must use the private import")
}

func (i *fakeAcquisitionImporter) ImportForAcquisitionForBook(_ context.Context, owner, language, bookID string, content epub.AcquisitionContent) (epub.ImportResult, error) {
	i.owner, i.language, i.bookID, i.content = owner, language, bookID, content
	if i.err != nil {
		return epub.ImportResult{}, i.err
	}
	return epub.ImportResult{Source: domain.SourceMaterial{ID: i.source}}, nil
}

func TestEPUBPreparationQueuesAnalysisWithCurrentSource(t *testing.T) {
	t.Parallel()
	analysisSubmission := &fakeAnalysis{handle: analysis.Handle{ID: 4, DisplayNumber: 12, RunID: "run-1"}}
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBook("source-1")}}},
		analysis: analysisSubmission,
	}

	got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

	assert.Equal(t, SubmissionQueued, got.Kind)
	assert.Equal(t, RouteEPUB, got.Route)
	assert.EqualValues(t, 12, got.JobNumber)
	assert.Equal(t, "source-1", analysisSubmission.sourceID)
	assert.Empty(t, got.Target.ConnectionID, "a current source is not acquired again")
}

// fakeOPDS serves the EPUB acquisition the EPUB implementation performs.
type fakeOPDS struct {
	server   *httptest.Server
	importer *fakeAcquisitionImporter
	service  *opds.Service
}

func newFakeOPDS(t *testing.T, status int) *fakeOPDS {
	t.Helper()
	content := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "epub2-clean"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		w.Header().Set("Content-Type", opds.EPUBMediaType)
		if _, err := w.Write(content); err != nil {
			http.Error(w, "fixture EPUB write failed", http.StatusInternalServerError)
		}
	}))
	t.Cleanup(server.Close)
	importer := &fakeAcquisitionImporter{source: "source-9"}
	store := fakeOPDSConnectionStore{connection: domain.OpdsConnection{URL: server.URL}}
	return &fakeOPDS{
		server:   server,
		importer: importer,
		service:  opds.NewService(store, importer, server.Client()),
	}
}

func (f *fakeOPDS) target() cataloguesync.AcquisitionTarget {
	return cataloguesync.AcquisitionTarget{
		ConnectionID: "connection-1",
		Language:     "de",
		Entry:        opds.Entry{Title: "Der lange Weg", Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: f.server.URL + "/book.epub"}}},
	}
}

func TestEPUBPreparationAcquiresThroughTheOPDSServerThenQueuesAnalysis(t *testing.T) {
	t.Parallel()
	catalogue := &fakeAcquisitionTargets{}
	opdsServer := newFakeOPDS(t, http.StatusOK)
	catalogue.target = opdsServer.target()
	reloading := &fakeBooks{results: []bookResult{{book: epubBook("source-9")}}}
	analysisSubmission := &fakeAnalysis{handle: analysis.Handle{DisplayNumber: 12}}
	preparation := epubPreparation{
		books:      reloading,
		analysis:   analysisSubmission,
		findTarget: catalogue.find,
		acquire:    opdsServer.service.AcquireForBook,
	}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	t.Run("queues analysis", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, SubmissionQueued, got.Kind)
		require.Equal(t, RouteEPUB, got.Route)
		require.EqualValues(t, 12, got.JobNumber)
	})
	t.Run("analysis uses the source acquired and reloaded", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "source-9", analysisSubmission.sourceID)
		require.Equal(t, 1, reloading.calls)
	})
	t.Run("reports the acquisition target", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, catalogue.target, got.Target)
	})
	t.Run("acquires through the catalogue with the Book's identity", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, 1, catalogue.calls)
		require.Equal(t, "owner", opdsServer.importer.owner)
		require.Equal(t, "book-1", opdsServer.importer.bookID)
		require.Equal(t, "de", opdsServer.importer.language)
	})
}

type acquisitionFailureFixture struct {
	findTarget func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
	acquire    func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
	target     cataloguesync.AcquisitionTarget
	wantErr    error
	wantAny    bool
}

func TestEPUBAcquisitionFailures(t *testing.T) {
	t.Parallel()
	acquireErr := errors.New("catalog refused the download")

	tests := []struct {
		name  string
		setup func(t *testing.T) acquisitionFailureFixture
	}{
		{
			name: "no acquisition seam",
			setup: func(*testing.T) acquisitionFailureFixture {
				return acquisitionFailureFixture{wantErr: ErrAcquisitionUnavailable}
			},
		},
		{
			name: "no acquirer",
			setup: func(*testing.T) acquisitionFailureFixture {
				target := cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Entry: opds.Entry{Title: "Der lange Weg"}}
				return acquisitionFailureFixture{
					findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
						return target, nil
					},
					target:  target,
					wantErr: ErrAcquisitionUnavailable,
				}
			},
		},
		{
			name: "lookup failed",
			setup: func(t *testing.T) acquisitionFailureFixture {
				t.Helper()
				opdsServer := newFakeOPDS(t, http.StatusOK)
				return acquisitionFailureFixture{
					findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
						return cataloguesync.AcquisitionTarget{}, acquireErr
					},
					acquire: opdsServer.service.AcquireForBook,
					wantErr: acquireErr,
				}
			},
		},
		{
			name: "download failed",
			setup: func(t *testing.T) acquisitionFailureFixture {
				t.Helper()
				opdsServer := newFakeOPDS(t, http.StatusInternalServerError)
				target := opdsServer.target()
				return acquisitionFailureFixture{
					findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
						return target, nil
					},
					acquire: opdsServer.service.AcquireForBook,
					target:  target,
					wantAny: true,
				}
			},
		},
		{
			name: "import failed",
			setup: func(t *testing.T) acquisitionFailureFixture {
				t.Helper()
				opdsServer := newFakeOPDS(t, http.StatusOK)
				opdsServer.importer.err = acquireErr
				target := opdsServer.target()
				return acquisitionFailureFixture{
					findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
						return target, nil
					},
					acquire: opdsServer.service.AcquireForBook,
					target:  target,
					wantErr: acquireErr,
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fixture := tt.setup(t)
			analysisSubmission := &fakeAnalysis{}
			preparation := epubPreparation{
				books:      &fakeBooks{results: []bookResult{{book: epubBookWithoutSource()}}},
				analysis:   analysisSubmission,
				findTarget: fixture.findTarget,
				acquire:    fixture.acquire,
			}

			got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

			assert.Equal(t, SubmissionCouldNotStart, got.Kind)
			assert.Equal(t, RouteEPUB, got.Route)
			assert.True(t, got.AcquisitionFailed, "the failing step is acquisition")
			assert.Equal(t, fixture.target, got.Target)
			assert.Equal(t, "Der lange Weg", got.Title)
			if fixture.wantAny {
				require.Error(t, got.Err)
			} else {
				require.ErrorIs(t, got.Err, fixture.wantErr)
			}
			assert.Zero(t, analysisSubmission.calls, "a failed acquisition submits nothing")
		})
	}
}

func TestEPUBAcquisitionReloadFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("detail read failed")
	opdsServer := newFakeOPDS(t, http.StatusOK)
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{err: boom}}},
		analysis: &fakeAnalysis{},
		findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			return opdsServer.target(), nil
		},
		acquire: opdsServer.service.AcquireForBook,
	}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	assert.Equal(t, SubmissionCouldNotStart, got.Kind)
	require.ErrorIs(t, got.Err, boom)
	assert.Empty(t, got.Title, "a failed reload has no Book title to name")
	assert.True(t, got.AcquisitionFailed)
}

func TestEPUBAcquisitionWithoutReloadedSourceFails(t *testing.T) {
	t.Parallel()
	opdsServer := newFakeOPDS(t, http.StatusOK)
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBookWithoutSource()}}},
		analysis: &fakeAnalysis{},
		findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			return opdsServer.target(), nil
		},
		acquire: opdsServer.service.AcquireForBook,
	}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	assert.Equal(t, SubmissionCouldNotStart, got.Kind)
	require.ErrorContains(t, got.Err, "could not be loaded after acquisition")
	assert.True(t, got.AcquisitionFailed)
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
			preparation := epubPreparation{
				books:    &fakeBooks{results: []bookResult{{book: epubBook("source-1")}}},
				analysis: analysisSubmission,
			}

			got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

			assert.Equal(t, tt.kind, got.Kind)
			assert.Equal(t, tt.refusal, got.Refusal)
			assert.Equal(t, RouteEPUB, got.Route)
			assert.False(t, got.AcquisitionFailed)
			assert.Equal(t, 1, analysisSubmission.calls)
		})
	}
}

func TestEPUBAcquisitionFailureLogQuotesTheErrorAndStep(t *testing.T) {
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBookWithoutSource()}}},
		analysis: &fakeAnalysis{},
		findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			return cataloguesync.AcquisitionTarget{}, errors.New("download failed\nmouseion: injected")
		},
		acquire: func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
			return epub.ImportResult{}, nil
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
		require.Contains(t, line, "mouseion: To Read acquisition failed")
		require.Contains(t, line, `book "book-1": "download failed\nmouseion: injected"`)
	})
}

func TestEPUBAnalysisFailureLogNamesTheStep(t *testing.T) {
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBook("source-1")}}},
		analysis: &fakeAnalysis{err: errors.New("submission failed")},
	}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	assert.Contains(t, logs.String(), `mouseion: To Read analysis submission failed for owner "owner" book "book-1": "submission failed"`, "identifiers are quoted")
}

func TestCurrentBookRefusalIsNotLoggedAsFailure(t *testing.T) {
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBook("source-1")}}},
		analysis: &fakeAnalysis{err: analysis.ErrCurrentBook},
	}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", epubBook("source-1"))

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	assert.Empty(t, logs.String())
}

type fakeAcquisitionFailures struct {
	records   []domain.EPUBAcquisitionFailureRecord
	cleared   []string
	recordErr error
	clearErr  error
}

func (f *fakeAcquisitionFailures) RecordEPUBAcquisitionFailure(_ context.Context, record domain.EPUBAcquisitionFailureRecord) error {
	f.records = append(f.records, record)
	return f.recordErr
}

func (f *fakeAcquisitionFailures) ClearEPUBAcquisitionFailure(_ context.Context, owner, bookID string) error {
	f.cleared = append(f.cleared, owner+"/"+bookID)
	return f.clearErr
}

func TestEPUBAcquisitionFailureRecordingClassifiesTheStage(t *testing.T) {
	t.Parallel()
	target := cataloguesync.AcquisitionTarget{
		ConnectionID: "connection-1",
		Language:     "de",
		Entry:        opds.Entry{ID: "entry-1", Title: "Der lange Weg"},
	}
	tests := []struct {
		name       string
		err        error
		stage      domain.EPUBAcquisitionFailureStage
		code       string
		wantRecord bool
	}{
		{
			name:       "download refused",
			err:        errors.New("catalog refused the download"),
			stage:      domain.EPUBAcquisitionFailureFetch,
			code:       domain.EPUBAcquisitionCodeSourceDownloadFailed,
			wantRecord: true,
		},
		{
			name:       "download too large",
			err:        fmt.Errorf("opds: download EPUB: %w", opds.ErrDownloadTooLarge),
			stage:      domain.EPUBAcquisitionFailureSource,
			code:       domain.EPUBAcquisitionCodeSourceOversize,
			wantRecord: true,
		},
		{
			name:       "retained EPUB too large",
			err:        fmt.Errorf("opds: ingest downloaded EPUB: %w", fmt.Errorf("%w: %w", opds.ErrImportFailed, opds.ErrRetainedEPUBTooLarge)),
			stage:      domain.EPUBAcquisitionFailureSource,
			code:       domain.EPUBAcquisitionCodeSourceOversize,
			wantRecord: true,
		},
		{
			name:       "no EPUB link",
			err:        opds.ErrNoEPUB,
			stage:      domain.EPUBAcquisitionFailureSource,
			code:       domain.EPUBAcquisitionCodeSourceFormatUnknown,
			wantRecord: true,
		},
		{
			name:       "invalid EPUB",
			err:        fmt.Errorf("opds: retain downloaded EPUB: %w", epub.ErrInvalidEPUB),
			stage:      domain.EPUBAcquisitionFailureSource,
			code:       domain.EPUBAcquisitionCodeSourceFormatUnknown,
			wantRecord: true,
		},
		{
			name:       "incompatible media type",
			err:        fmt.Errorf("opds: download EPUB: %w", opds.ErrIncompatibleMediaType),
			stage:      domain.EPUBAcquisitionFailureSource,
			code:       domain.EPUBAcquisitionCodeSourceFormatUnknown,
			wantRecord: true,
		},
		{
			name:       "import failed",
			err:        fmt.Errorf("opds: ingest downloaded EPUB: %w", fmt.Errorf("%w: database unavailable", opds.ErrImportFailed)),
			stage:      domain.EPUBAcquisitionFailureImport,
			code:       domain.EPUBAcquisitionCodeEPUBImportFailed,
			wantRecord: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			acquisitions := &fakeAcquisitionFailures{}
			preparation := epubPreparation{
				books:    &fakeBooks{results: []bookResult{{book: epubBookWithoutSource()}}},
				analysis: &fakeAnalysis{},
				findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
					return target, nil
				},
				acquire: func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
					return epub.ImportResult{}, tt.err
				},
				acquisitions: acquisitions,
			}

			got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

			require.Equal(t, SubmissionCouldNotStart, got.Kind)
			if !tt.wantRecord {
				assert.Empty(t, acquisitions.records)
				assert.Empty(t, acquisitions.cleared)
				return
			}
			require.Len(t, acquisitions.records, 1)
			record := acquisitions.records[0]
			assert.Equal(t, "owner", record.OwnerID)
			assert.Equal(t, "book-1", record.BookID)
			assert.Equal(t, "connection-1", record.ConnectionID)
			assert.Equal(t, "entry-1", record.EntryIdentifier)
			assert.Equal(t, tt.stage, record.Stage)
			assert.Equal(t, tt.code, record.Code)
			assert.Empty(t, acquisitions.cleared)
		})
	}
}

func TestEPUBAcquisitionMissingCatalogEntryRecordsEntryFailure(t *testing.T) {
	t.Parallel()
	acquisitions := &fakeAcquisitionFailures{}
	target := cataloguesync.AcquisitionTarget{
		ConnectionID: "connection-1",
		Language:     "de",
		Entry:        opds.Entry{ID: "entry-1", Title: "Der lange Weg"},
	}
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBookWithoutSource()}}},
		analysis: &fakeAnalysis{},
		findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			return target, cataloguesync.ErrCatalogEntryMissing
		},
		acquire: func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
			t.Fatal("a missing index entry is never downloaded")
			return epub.ImportResult{}, nil
		},
		acquisitions: acquisitions,
	}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	require.True(t, got.AcquisitionFailed)
	require.ErrorIs(t, got.Err, cataloguesync.ErrCatalogEntryMissing)
	require.Equal(t, target, got.Target)
	require.Len(t, acquisitions.records, 1)
	record := acquisitions.records[0]
	t.Run("records the entry stage and code", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, domain.EPUBAcquisitionFailureEntry, record.Stage)
		require.Equal(t, domain.EPUBAcquisitionCodeCatalogEntryMissing, record.Code)
	})
	t.Run("records the Book and where acquisition looked", func(t *testing.T) {
		t.Parallel()
		require.Equal(t, "owner", record.OwnerID)
		require.Equal(t, "book-1", record.BookID)
		require.Equal(t, "connection-1", record.ConnectionID)
		require.Equal(t, "entry-1", record.EntryIdentifier)
	})
	t.Run("clears nothing", func(t *testing.T) {
		t.Parallel()
		require.Empty(t, acquisitions.cleared)
	})
}

func TestEPUBAcquisitionWithoutAnAttemptRecordsNoFailure(t *testing.T) {
	t.Parallel()
	lookupErr := errors.New("catalog lookup failed")
	tests := []struct {
		name       string
		findTarget func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
		acquire    func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
	}{
		{name: "no acquisition seam"},
		{
			name: "lookup failed",
			findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
				return cataloguesync.AcquisitionTarget{}, lookupErr
			},
		},
		{
			name: "no acquirer",
			findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
				return cataloguesync.AcquisitionTarget{ConnectionID: "connection-1"}, nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			acquisitions := &fakeAcquisitionFailures{}
			preparation := epubPreparation{
				books:        &fakeBooks{results: []bookResult{{book: epubBookWithoutSource()}}},
				analysis:     &fakeAnalysis{},
				findTarget:   tt.findTarget,
				acquire:      tt.acquire,
				acquisitions: acquisitions,
			}

			got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

			require.Equal(t, SubmissionCouldNotStart, got.Kind)
			assert.Empty(t, acquisitions.records, "a missing seam or failed lookup is not acquisition evidence")
			assert.Empty(t, acquisitions.cleared)
		})
	}
}

func TestEPUBAcquisitionSuccessClearsTheRecordedFailure(t *testing.T) {
	t.Parallel()
	acquisitions := &fakeAcquisitionFailures{}
	opdsServer := newFakeOPDS(t, http.StatusOK)
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBook("source-9")}}},
		analysis: &fakeAnalysis{handle: analysis.Handle{DisplayNumber: 12}},
		findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			return opdsServer.target(), nil
		},
		acquire:      opdsServer.service.AcquireForBook,
		acquisitions: acquisitions,
	}

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	require.Equal(t, SubmissionQueued, got.Kind)
	assert.Equal(t, []string{"owner/book-1"}, acquisitions.cleared)
	assert.Empty(t, acquisitions.records)
}

func TestEPUBAcquisitionRecordingFailureKeepsTheOriginalOutcome(t *testing.T) {
	t.Parallel()
	acquisitions := &fakeAcquisitionFailures{recordErr: errors.New("database unavailable")}
	preparation := epubPreparation{
		books:    &fakeBooks{results: []bookResult{{book: epubBookWithoutSource()}}},
		analysis: &fakeAnalysis{},
		findTarget: func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
			return cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Entry: opds.Entry{ID: "entry-1"}}, nil
		},
		acquire: func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
			return epub.ImportResult{}, errors.New("download failed")
		},
		acquisitions: acquisitions,
	}
	logs := captureLog(t)

	got := preparation.Prepare(context.Background(), "owner", epubBookWithoutSource())

	require.Equal(t, SubmissionCouldNotStart, got.Kind)
	require.ErrorContains(t, got.Err, "download failed")
	assert.Contains(t, logs.String(), "record EPUB acquisition failure")
}

// captureLog redirects the standard logger for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &output
}
