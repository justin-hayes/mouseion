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
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
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

type fakePreparation struct {
	handle analysis.ClassicalPreparationHandle
	err    error
	calls  int
}

func (f *fakePreparation) SubmitClassicalPreparation(context.Context, string, string) (analysis.ClassicalPreparationHandle, error) {
	f.calls++
	return f.handle, f.err
}

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

type fakeAcquisition struct {
	target     cataloguesync.AcquisitionTarget
	findErr    error
	acquireErr error
	findCalls  int
	bookCalls  int
}

func (f *fakeAcquisition) FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error) {
	f.findCalls++
	if f.findErr != nil {
		return cataloguesync.AcquisitionTarget{}, f.findErr
	}
	return f.target, nil
}

func (f *fakeAcquisition) AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
	f.bookCalls++
	return epub.ImportResult{}, f.acquireErr
}

// harness wires a module over in-memory fakes.
type harness struct {
	books        *fakeBooks
	current      *fakeCurrentReadings
	dispositions *fakeDispositions
	preparation  *fakePreparation
	analysis     *fakeAnalysis
	acquisition  CatalogAcquisition
	module       *Module
}

func newHarness(t *testing.T, book domain.MyBook, acquisition CatalogAcquisition) *harness {
	t.Helper()
	h := &harness{
		books:        &fakeBooks{results: []bookResult{{book: book}}},
		current:      &fakeCurrentReadings{},
		dispositions: &fakeDispositions{applied: true},
		preparation:  &fakePreparation{},
		analysis:     &fakeAnalysis{},
		acquisition:  acquisition,
	}
	module, err := New(Config{
		Books: h.books, CurrentReadings: h.current, Dispositions: h.dispositions,
		Preparation: h.preparation, Analysis: h.analysis, Acquisition: h.acquisition,
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
		Dispositions: &fakeDispositions{},
		Preparation:  &fakePreparation{}, Analysis: &fakeAnalysis{},
	}
	tests := []struct {
		name   string
		change func(*Config)
	}{
		{name: "books", change: func(c *Config) { c.Books = nil }},
		{name: "current readings", change: func(c *Config) { c.CurrentReadings = nil }},
		{name: "dispositions", change: func(c *Config) { c.Dispositions = nil }},
		{name: "classical preparation", change: func(c *Config) { c.Preparation = nil }},
		{name: "analysis", change: func(c *Config) { c.Analysis = nil }},
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
	require.NoError(t, err, "catalog acquisition is optional")
}

func TestMoveNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t, domain.MyBook{}, nil)
	h.books.results = []bookResult{{err: domain.ErrNotFound}}

	outcome := move(t, h)

	assert.Equal(t, OutcomeNotFound, outcome.Kind)
	assert.Zero(t, h.dispositions.calls, "a missing Book is never transitioned")
}

func TestMoveStaleRevision(t *testing.T) {
	t.Parallel()
	h := newHarness(t, classicalBook(), nil)
	h.dispositions.err = domain.ErrStaleBookDisposition

	outcome := move(t, h)

	assert.Equal(t, OutcomeStaleRevision, outcome.Kind)
	assert.Equal(t, "book-1", outcome.Book.Book.ID)
	assert.Zero(t, h.preparation.calls)
	assert.Zero(t, h.analysis.calls)
}

func TestMoveUnchangedDoesNotSubmit(t *testing.T) {
	t.Parallel()
	h := newHarness(t, classicalBook(), nil)
	h.dispositions.applied = false

	outcome := move(t, h)

	assert.Equal(t, OutcomeUnchanged, outcome.Kind)
	assert.Equal(t, "book-1", h.dispositions.bookID)
	assert.EqualValues(t, 3, h.dispositions.revision)
	assert.Equal(t, domain.BookDispositionToRead, h.dispositions.disposition)
	assert.Zero(t, h.preparation.calls, "an unchanged disposition starts no preparation")
	assert.Zero(t, h.analysis.calls, "an unchanged disposition starts no analysis")
}

func TestMoveClassicalOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		handle analysis.ClassicalPreparationHandle
		err    error
		want   OutcomeKind
		job    int64
	}{
		{name: "already current", handle: analysis.ClassicalPreparationHandle{Complete: true}, want: OutcomeAlreadyCurrent},
		{name: "queued", handle: analysis.ClassicalPreparationHandle{JobID: 7, Queued: true}, want: OutcomeJobQueued, job: 7},
		{name: "legacy rows", err: classical.ErrLegacyRows, want: OutcomeLegacyRows},
		{name: "withdrawn definition", err: analysis.ErrWithdrawnClassicalDefinition, want: OutcomeWithdrawnDefinition},
		{name: "source refused", handle: analysis.ClassicalPreparationHandle{Terminal: true}, want: OutcomeSourceRefused},
		{name: "could not start", err: errors.New("preparation failed"), want: OutcomeCouldNotStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, classicalBook(), nil)
			h.preparation.handle, h.preparation.err = tt.handle, tt.err

			outcome := move(t, h)

			assert.Equal(t, tt.want, outcome.Kind)
			assert.Equal(t, RouteClassical, outcome.Route)
			assert.Equal(t, tt.job, outcome.JobNumber)
			assert.Equal(t, 1, h.preparation.calls)
			assert.Zero(t, h.analysis.calls, "a classical Book never uses EPUB analysis")
			assert.Equal(t, "book-1", outcome.Book.Book.ID)
		})
	}
}

func TestMoveClassicalPreparationUnavailable(t *testing.T) {
	h := newHarness(t, classicalBook(), nil)
	h.preparation.err = ErrPreparationUnavailable
	logs := captureLog(t)

	outcome := move(t, h)

	assert.Equal(t, OutcomePreparationUnavailable, outcome.Kind)
	assert.Equal(t, RouteClassical, outcome.Route)
	assert.Empty(t, logs.String(), "an absent preparation seam is not a failed submission")
}

func TestMoveClassicalWithAnalysisInEffectSubmitsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t, analyzedBook(), nil)

	outcome := move(t, h)

	assert.Equal(t, OutcomeMoved, outcome.Kind)
	assert.Equal(t, RouteClassical, outcome.Route)
	assert.Zero(t, h.preparation.calls, "settled evidence is not prepared again")
	assert.Zero(t, h.analysis.calls)
}

func TestMoveEPUBAcquiresThenQueuesAnalysis(t *testing.T) {
	t.Parallel()
	target := cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Language: "de", Entry: opds.Entry{Title: "Der lange Weg"}}
	acquisition := &fakeAcquisition{target: target}
	h := newHarness(t, epubBookWithoutSource(), acquisition)
	h.books.results = []bookResult{{book: epubBookWithoutSource()}, {book: epubBook("source-9")}}
	h.analysis.handle = analysis.Handle{ID: 4, DisplayNumber: 12, RunID: "run-1"}

	outcome := move(t, h)

	assert.Equal(t, OutcomeJobQueued, outcome.Kind)
	assert.Equal(t, RouteEPUB, outcome.Route)
	assert.EqualValues(t, 12, outcome.JobNumber)
	assert.Equal(t, "source-9", h.analysis.sourceID, "analysis uses the source acquired and reloaded")
	assert.Equal(t, target, outcome.Target)
	assert.Equal(t, 2, h.books.calls, "the Book is reloaded after acquisition")
	assert.Equal(t, 1, acquisition.bookCalls)
	assert.Zero(t, h.preparation.calls)
}

func TestSourceAcquisitionWithoutTargetsOrAcquirer(t *testing.T) {
	t.Parallel()
	var acquisition SourceAcquisition
	_, err := acquisition.FindAcquisitionTarget(context.Background(), "owner", "book-1")
	require.ErrorIs(t, err, ErrAcquisitionUnavailable)

	acquisition.Targets = &fakeAcquisition{}
	_, err = acquisition.AcquireForBook(context.Background(), "owner", "connection-1", "de", "book-1", opds.Entry{})
	require.ErrorIs(t, err, ErrAcquisitionUnavailable)
}

func TestMoveEPUBAcquisitionReloadFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("detail read failed")
	h := newHarness(t, epubBookWithoutSource(), &fakeAcquisition{})
	h.books.results = []bookResult{{book: epubBookWithoutSource()}, {err: boom}}

	outcome := move(t, h)

	assert.Equal(t, OutcomeAcquisitionFailed, outcome.Kind)
	require.ErrorIs(t, outcome.Err, boom)
	assert.Empty(t, outcome.Title, "a failed reload has no Book title to name")
	assert.True(t, outcome.AcquisitionFailed)
	assert.Zero(t, h.analysis.calls)
}

func TestMoveEPUBAcquisitionFailures(t *testing.T) {
	t.Parallel()
	target := cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Entry: opds.Entry{Title: "Der lange Weg"}}
	acquireErr := errors.New("catalog refused the download")
	tests := []struct {
		name        string
		acquisition CatalogAcquisition
		target      cataloguesync.AcquisitionTarget
		wantErr     error
	}{
		{name: "no acquisition seam", acquisition: nil, wantErr: ErrAcquisitionUnavailable},
		{name: "lookup failed", acquisition: &fakeAcquisition{target: target, findErr: acquireErr}, wantErr: acquireErr},
		{name: "download failed", acquisition: &fakeAcquisition{target: target, acquireErr: acquireErr}, target: target, wantErr: acquireErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, epubBookWithoutSource(), tt.acquisition)

			outcome := move(t, h)

			assert.Equal(t, OutcomeAcquisitionFailed, outcome.Kind)
			assert.Equal(t, RouteEPUB, outcome.Route)
			assert.True(t, outcome.AcquisitionFailed, "the failing step is acquisition")
			assert.Equal(t, tt.target, outcome.Target)
			assert.Equal(t, "Der lange Weg", outcome.Title)
			require.ErrorIs(t, outcome.Err, tt.wantErr)
			assert.Zero(t, h.analysis.calls, "a failed acquisition submits nothing")
		})
	}
}

func TestMoveEPUBAcquisitionWithoutReloadedSourceFails(t *testing.T) {
	t.Parallel()
	h := newHarness(t, epubBookWithoutSource(), &fakeAcquisition{})
	h.books.results = []bookResult{{book: epubBookWithoutSource()}, {book: epubBookWithoutSource()}}

	outcome := move(t, h)

	assert.Equal(t, OutcomeAcquisitionFailed, outcome.Kind)
	assert.Contains(t, outcome.Err.Error(), "could not be loaded after acquisition")
	assert.True(t, outcome.AcquisitionFailed)
	assert.Zero(t, h.analysis.calls)
}

func TestMoveEPUBAnalysisOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want OutcomeKind
	}{
		{name: "no usable units", err: domain.ErrExtractedUnitsUnavailable, want: OutcomeNoUsableEPUBUnits},
		{name: "EPUB required", err: analysis.ErrEPUBRequired, want: OutcomeNoUsableEPUBUnits},
		{name: "could not start", err: errors.New("analysis failed"), want: OutcomeCouldNotStart},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, epubBook("source-1"), nil)
			h.analysis.err = tt.err

			outcome := move(t, h)

			assert.Equal(t, tt.want, outcome.Kind)
			assert.Equal(t, RouteEPUB, outcome.Route)
			assert.False(t, outcome.AcquisitionFailed)
			assert.Equal(t, 1, h.analysis.calls)
		})
	}
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
			h := newHarness(t, classicalBook(), nil)
			tt.setup(h)
			_, err := h.module.Move(context.Background(), "owner", "book-1", 3)
			require.ErrorIs(t, err, boom)
		})
	}
}

func TestRetryNotFound(t *testing.T) {
	t.Parallel()
	h := newHarness(t, domain.MyBook{}, nil)
	h.books.results = []bookResult{{err: domain.ErrNotFound}}

	outcome := retry(t, h)

	assert.Equal(t, OutcomeNotFound, outcome.Kind)
	assert.Zero(t, h.current.calls, "a missing Book is never checked against the current reading")
	assert.Zero(t, h.dispositions.calls)
	assert.Zero(t, h.preparation.calls)
	assert.Zero(t, h.analysis.calls)
}

func TestRetryReturnsLoadError(t *testing.T) {
	t.Parallel()
	boom := errors.New("database unavailable")
	h := newHarness(t, domain.MyBook{}, nil)
	h.books.results = []bookResult{{err: boom}}

	_, err := h.module.Retry(context.Background(), "owner", "book-1")

	require.ErrorIs(t, err, boom)
	assert.Zero(t, h.current.calls)
	assert.Zero(t, h.preparation.calls)
	assert.Zero(t, h.analysis.calls)
}

func TestRetryReturnsCurrentReadingError(t *testing.T) {
	t.Parallel()
	boom := errors.New("current reading read failed")
	h := newHarness(t, toReadClassicalBook(), nil)
	h.current.err = boom

	_, err := h.module.Retry(context.Background(), "owner", "book-1")

	require.ErrorIs(t, err, boom)
	assert.Zero(t, h.dispositions.calls)
	assert.Zero(t, h.preparation.calls)
	assert.Zero(t, h.analysis.calls)
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
			h := newHarness(t, tt.book, nil)
			h.current.current = domain.CurrentReading{BookID: "book-1"}

			outcome := retry(t, h)

			assert.Equal(t, OutcomeCurrentBookRefused, outcome.Kind, "the current Book guard runs before the To Read guard")
			assert.Equal(t, "book-1", outcome.Book.Book.ID)
			assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
			assert.Zero(t, h.preparation.calls, "the current Book is not prepared again")
			assert.Zero(t, h.analysis.calls, "the current Book is not analyzed again")
		})
	}
}

func TestRetryRefusesBookNotToRead(t *testing.T) {
	t.Parallel()
	book := classicalBook()
	book.Book.LanguageTag = "grc"
	h := newHarness(t, book, nil)

	outcome := retry(t, h)

	assert.Equal(t, OutcomeNotToRead, outcome.Kind)
	assert.Equal(t, "grc", h.current.language, "the current reading is read for the Book's language")
	assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
	assert.Zero(t, h.preparation.calls)
	assert.Zero(t, h.analysis.calls)
}

func TestRetryClassicalOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		book      domain.MyBook
		handle    analysis.ClassicalPreparationHandle
		err       error
		want      OutcomeKind
		job       int64
		prepCalls int
	}{
		{name: "already current", book: toReadAnalyzedBook(), handle: analysis.ClassicalPreparationHandle{Complete: true}, want: OutcomeAlreadyCurrent, prepCalls: 1},
		{name: "settled evidence with a withdrawn definition", book: toReadAnalyzedBook(), err: analysis.ErrWithdrawnClassicalDefinition, want: OutcomeWithdrawnDefinition, prepCalls: 1},
		{name: "queued", book: toReadClassicalBook(), handle: analysis.ClassicalPreparationHandle{JobID: 7, Queued: true}, want: OutcomeJobQueued, job: 7, prepCalls: 1},
		{name: "legacy rows", book: toReadClassicalBook(), err: classical.ErrLegacyRows, want: OutcomeLegacyRows, prepCalls: 1},
		{name: "withdrawn definition", book: toReadClassicalBook(), err: analysis.ErrWithdrawnClassicalDefinition, want: OutcomeWithdrawnDefinition, prepCalls: 1},
		{name: "source refused", book: toReadClassicalBook(), handle: analysis.ClassicalPreparationHandle{Terminal: true}, want: OutcomeSourceRefused, prepCalls: 1},
		{name: "could not start", book: toReadClassicalBook(), err: errors.New("preparation failed"), want: OutcomeCouldNotStart, prepCalls: 1},
		{name: "preparation unavailable", book: toReadClassicalBook(), err: ErrPreparationUnavailable, want: OutcomePreparationUnavailable, prepCalls: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, tt.book, nil)
			h.preparation.handle, h.preparation.err = tt.handle, tt.err

			outcome := retry(t, h)

			assert.Equal(t, tt.want, outcome.Kind)
			assert.Equal(t, RouteClassical, outcome.Route)
			assert.Equal(t, tt.job, outcome.JobNumber)
			assert.Equal(t, tt.prepCalls, h.preparation.calls)
			assert.Zero(t, h.analysis.calls, "a classical Book never uses EPUB analysis")
			assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
			assert.Equal(t, "book-1", outcome.Book.Book.ID)
		})
	}
}

func TestRetryEPUBAcquiresThenQueuesAnalysis(t *testing.T) {
	t.Parallel()
	target := cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Language: "de", Entry: opds.Entry{Title: "Der lange Weg"}}
	acquisition := &fakeAcquisition{target: target}
	h := newHarness(t, toReadEPUBBookWithoutSource(), acquisition)
	h.books.results = []bookResult{{book: toReadEPUBBookWithoutSource()}, {book: toReadEPUBBook("source-9")}}
	h.analysis.handle = analysis.Handle{ID: 4, DisplayNumber: 12, RunID: "run-1"}

	outcome := retry(t, h)

	assert.Equal(t, OutcomeJobQueued, outcome.Kind)
	assert.Equal(t, RouteEPUB, outcome.Route)
	assert.EqualValues(t, 12, outcome.JobNumber)
	assert.Equal(t, "source-9", h.analysis.sourceID, "analysis uses the source acquired and reloaded")
	assert.Equal(t, target, outcome.Target)
	assert.Equal(t, 2, h.books.calls, "the Book is reloaded after acquisition")
	assert.Equal(t, 1, acquisition.bookCalls)
	assert.Zero(t, h.preparation.calls)
	assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
}

func TestRetryEPUBAcquisitionFailures(t *testing.T) {
	t.Parallel()
	target := cataloguesync.AcquisitionTarget{ConnectionID: "connection-1", Entry: opds.Entry{Title: "Der lange Weg"}}
	acquireErr := errors.New("catalog refused the download")
	tests := []struct {
		name        string
		acquisition CatalogAcquisition
		target      cataloguesync.AcquisitionTarget
		wantErr     error
	}{
		{name: "no acquisition seam", acquisition: nil, wantErr: ErrAcquisitionUnavailable},
		{name: "lookup failed", acquisition: &fakeAcquisition{target: target, findErr: acquireErr}, wantErr: acquireErr},
		{name: "download failed", acquisition: &fakeAcquisition{target: target, acquireErr: acquireErr}, target: target, wantErr: acquireErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, toReadEPUBBookWithoutSource(), tt.acquisition)

			outcome := retry(t, h)

			assert.Equal(t, OutcomeAcquisitionFailed, outcome.Kind)
			assert.Equal(t, RouteEPUB, outcome.Route)
			assert.True(t, outcome.AcquisitionFailed, "the failing step is acquisition")
			assert.Equal(t, tt.target, outcome.Target)
			assert.Equal(t, "Der lange Weg", outcome.Title)
			require.ErrorIs(t, outcome.Err, tt.wantErr)
			assert.Zero(t, h.analysis.calls, "a failed acquisition submits nothing")
			assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
		})
	}
}

func TestRetryEPUBAcquisitionReloadFailure(t *testing.T) {
	t.Parallel()
	boom := errors.New("detail read failed")
	h := newHarness(t, toReadEPUBBookWithoutSource(), &fakeAcquisition{})
	h.books.results = []bookResult{{book: toReadEPUBBookWithoutSource()}, {err: boom}}

	outcome := retry(t, h)

	assert.Equal(t, OutcomeAcquisitionFailed, outcome.Kind)
	require.ErrorIs(t, outcome.Err, boom)
	assert.Empty(t, outcome.Title, "a failed reload has no Book title to name")
	assert.True(t, outcome.AcquisitionFailed)
	assert.Zero(t, h.analysis.calls)
}

func TestRetryEPUBAnalysisOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want OutcomeKind
	}{
		{name: "no usable units", err: domain.ErrExtractedUnitsUnavailable, want: OutcomeNoUsableEPUBUnits},
		{name: "EPUB required", err: analysis.ErrEPUBRequired, want: OutcomeNoUsableEPUBUnits},
		{name: "could not start", err: errors.New("analysis failed"), want: OutcomeCouldNotStart},
		{name: "current book raced", err: analysis.ErrCurrentBook, want: OutcomeCurrentBookRefused},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t, toReadEPUBBook("source-1"), nil)
			h.analysis.err = tt.err

			outcome := retry(t, h)

			assert.Equal(t, tt.want, outcome.Kind)
			assert.Equal(t, RouteEPUB, outcome.Route)
			assert.False(t, outcome.AcquisitionFailed)
			assert.Equal(t, 1, h.analysis.calls)
			assert.Zero(t, h.preparation.calls)
			assert.Zero(t, h.dispositions.calls, "Retry never transitions the disposition")
		})
	}
}

func TestRetryClassicalFailureLogMatchesMove(t *testing.T) {
	h := newHarness(t, toReadClassicalBook(), nil)
	h.preparation.err = errors.New("cannot import Ἀνάβασις from source")
	logs := captureLog(t)

	outcome := retry(t, h)

	require.Equal(t, OutcomeCouldNotStart, outcome.Kind)
	line := logs.String()
	assert.Contains(t, line, "mouseion: classical preparation submission failed")
	assert.Contains(t, line, `stage="import"`)
	assert.Contains(t, line, `code="classical_preparation_failed"`)
	assert.Contains(t, line, `owner "owner"`, "identifiers are quoted")
	assert.Contains(t, line, `book "book-1"`)
	assert.NotContains(t, line, "Ἀνάβασις", "raw error text must not reach operator logs")
	assert.NotContains(t, line, "cannot import")
}

func TestRetryCurrentBookRaceIsNotLoggedAsFailure(t *testing.T) {
	h := newHarness(t, toReadEPUBBook("source-1"), nil)
	h.analysis.err = analysis.ErrCurrentBook
	logs := captureLog(t)

	outcome := retry(t, h)

	require.Equal(t, OutcomeCurrentBookRefused, outcome.Kind)
	assert.Empty(t, logs.String(), "a refused current Book is an expected outcome")
}

// captureLog redirects the standard logger for one test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &output
}

func TestClassicalFailureLogCarriesStageAndCodeOnly(t *testing.T) {
	h := newHarness(t, classicalBook(), nil)
	h.preparation.err = errors.New("cannot import Ἀνάβασις from source")
	logs := captureLog(t)

	outcome := move(t, h)

	require.Equal(t, OutcomeCouldNotStart, outcome.Kind)
	line := logs.String()
	assert.Contains(t, line, "mouseion: classical preparation submission failed")
	assert.Contains(t, line, `stage="import"`)
	assert.Contains(t, line, `code="classical_preparation_failed"`)
	assert.Contains(t, line, `owner "owner"`, "identifiers are quoted")
	assert.Contains(t, line, `book "book-1"`)
	assert.NotContains(t, line, "Ἀνάβασις", "raw error text must not reach operator logs")
	assert.NotContains(t, line, "cannot import")
}

func TestEPUBAcquisitionFailureLogQuotesTheErrorAndStep(t *testing.T) {
	h := newHarness(t, epubBookWithoutSource(), &fakeAcquisition{acquireErr: errors.New("download failed\nmouseion: injected")})
	logs := captureLog(t)

	outcome := move(t, h)

	require.Equal(t, OutcomeAcquisitionFailed, outcome.Kind)
	line := logs.String()
	assert.Equal(t, 1, strings.Count(line, "\n"), "a newline in the error must not start a new log line")
	assert.Contains(t, line, "mouseion: To Read acquisition failed")
	assert.Contains(t, line, `book "book-1": "download failed\nmouseion: injected"`)
}

func TestEPUBAnalysisFailureLogNamesTheStep(t *testing.T) {
	h := newHarness(t, epubBook("source-1"), nil)
	h.analysis.err = errors.New("submission failed")
	logs := captureLog(t)

	outcome := move(t, h)

	require.Equal(t, OutcomeCouldNotStart, outcome.Kind)
	assert.Contains(t, logs.String(), "mouseion: To Read analysis submission failed for owner owner book ")
}

func TestCurrentBookRefusalIsNotLoggedAsFailure(t *testing.T) {
	h := newHarness(t, epubBook("source-1"), nil)
	h.analysis.err = analysis.ErrCurrentBook
	logs := captureLog(t)

	outcome := move(t, h)

	require.Equal(t, OutcomeCouldNotStart, outcome.Kind)
	assert.Empty(t, logs.String())
}
