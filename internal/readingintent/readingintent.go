// Package readingintent owns a learner's reading intent for one Book: the To
// Read disposition transition, the choice between Classical preparation and
// EPUB acquisition and analysis, and the classification of the result.
// Handlers translate its typed outcome into a sentence; it returns no
// learner-facing text and knows nothing about HTTP.
package readingintent

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
)

// ErrAcquisitionUnavailable reports a deployment with no catalogue connection
// that could acquire a Book's source. It keeps the error the web app returned
// before this module owned the rule.
var ErrAcquisitionUnavailable = errors.New("catalogue acquisition cannot promote this book")

// ErrPreparationUnavailable reports a deployment with no Classical preparation
// seam. It is not a submission failure: the move succeeds and starts no job.
var ErrPreparationUnavailable = errors.New("reading intent: classical preparation is not configured")

// BookReader loads the owner-scoped Book detail the decision is made from.
type BookReader interface {
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
}

// CurrentReadingReader loads the owner's current reading for a language, so a
// retry can refuse the Book whose evidence is frozen.
type CurrentReadingReader interface {
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
}

// DispositionWriter applies the To Read transition against an expected revision.
type DispositionWriter interface {
	TransitionBookDisposition(context.Context, string, string, int64, domain.BookDisposition) (bool, error)
}

// ClassicalPreparation submits the ensure-once preparation job a classical
// Book needs before its analysis can publish.
type ClassicalPreparation interface {
	SubmitClassicalPreparation(context.Context, string, string) (analysis.ClassicalPreparationHandle, error)
}

// Analysis submits the analysis job an EPUB Book needs.
type Analysis interface {
	SubmitToReadBookAnalysis(context.Context, string, string, string) (analysis.Handle, error)
}

// CatalogAcquisition finds a Book's catalogue entry and acquires its source. It
// is optional: without it, a Book that needs a source cannot be promoted.
type CatalogAcquisition interface {
	FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
	AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
}

// AcquisitionTargetFinder is the catalogue-sync half of a CatalogAcquisition.
type AcquisitionTargetFinder interface {
	FindAcquisitionTarget(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
}

// BookAcquirer is the OPDS half of a CatalogAcquisition.
type BookAcquirer interface {
	AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
}

// SourceAcquisition composes the catalogue lookup with the OPDS acquirer. A
// nil Acquirer keeps ErrAcquisitionUnavailable for a deployment without OPDS.
type SourceAcquisition struct {
	Targets  AcquisitionTargetFinder
	Acquirer BookAcquirer
}

// FindAcquisitionTarget names the connection and entry a Book can be acquired from.
func (s SourceAcquisition) FindAcquisitionTarget(ctx context.Context, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	if s.Targets == nil {
		return cataloguesync.AcquisitionTarget{}, ErrAcquisitionUnavailable
	}
	return s.Targets.FindAcquisitionTarget(ctx, owner, bookID)
}

// AcquireForBook downloads and imports the target entry.
func (s SourceAcquisition) AcquireForBook(ctx context.Context, owner, connectionID, language, bookID string, entry opds.Entry) (epub.ImportResult, error) {
	if s.Acquirer == nil {
		return epub.ImportResult{}, ErrAcquisitionUnavailable
	}
	return s.Acquirer.AcquireForBook(ctx, owner, connectionID, language, bookID, entry)
}

// Config supplies Move's collaborators. Books, CurrentReadings, Dispositions,
// Preparation and Analysis are required; a missing one is a wiring mistake.
// Acquisition is optional, because a deployment may have no catalog connection.
type Config struct {
	Books           BookReader
	CurrentReadings CurrentReadingReader
	Dispositions    DispositionWriter
	Preparation     ClassicalPreparation
	Analysis        Analysis
	Acquisition     CatalogAcquisition
}

// Module carries out reading intent for one Book.
type Module struct {
	books           BookReader
	currentReadings CurrentReadingReader
	dispositions    DispositionWriter
	preparation     ClassicalPreparation
	analysis        Analysis
	acquisition     CatalogAcquisition
}

// New validates the required collaborators and returns the module.
func New(cfg Config) (*Module, error) {
	switch {
	case cfg.Books == nil:
		return nil, errors.New("reading intent: a Book reader is required")
	case cfg.CurrentReadings == nil:
		return nil, errors.New("reading intent: a current reading reader is required")
	case cfg.Dispositions == nil:
		return nil, errors.New("reading intent: a disposition writer is required")
	case cfg.Preparation == nil:
		return nil, errors.New("reading intent: Classical preparation is required")
	case cfg.Analysis == nil:
		return nil, errors.New("reading intent: To Read analysis is required")
	}
	return &Module{
		books:           cfg.Books,
		currentReadings: cfg.CurrentReadings,
		dispositions:    cfg.Dispositions,
		preparation:     cfg.Preparation,
		analysis:        cfg.Analysis,
		acquisition:     cfg.Acquisition,
	}, nil
}

// Move applies the learner's To Read intent for one Book. It loads the Book,
// transitions the disposition against the expected revision, and routes only
// when the disposition really changed. An unexpected infrastructure failure is
// returned as an error; every classified result is an Outcome.
func (m *Module) Move(ctx context.Context, owner, bookID string, expectedRevision int64) (Outcome, error) {
	book, err := m.books.GetBookDetail(ctx, owner, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		return Outcome{Kind: OutcomeNotFound}, nil
	}
	if err != nil {
		return Outcome{}, err
	}

	applied, err := m.dispositions.TransitionBookDisposition(ctx, owner, bookID, expectedRevision, domain.BookDispositionToRead)
	switch {
	case errors.Is(err, domain.ErrStaleBookDisposition):
		return Outcome{Kind: OutcomeStaleRevision, Book: book}, nil
	case err != nil:
		return Outcome{Book: book}, err
	}
	if !applied || book.Disposition == domain.BookDispositionToRead {
		return Outcome{Kind: OutcomeUnchanged, Book: book}, nil
	}
	if book.ReadingUnit != nil && book.Classification().AnalysisInEffect() {
		// Settled evidence is not prepared again merely because the learner
		// moved the Book.
		return Outcome{Kind: OutcomeMoved, Route: RouteClassical, Book: book}, nil
	}
	return m.route(ctx, owner, book)
}

// Retry re-runs the submission route for a Book that is already To Read. It
// refuses the current Book, whose evidence is frozen, and any Book that is not
// To Read, because analysis only follows reading intent. It never transitions
// the disposition.
func (m *Module) Retry(ctx context.Context, owner, bookID string) (Outcome, error) {
	book, err := m.books.GetBookDetail(ctx, owner, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		return Outcome{Kind: OutcomeNotFound}, nil
	}
	if err != nil {
		return Outcome{}, err
	}
	current, err := m.currentReadings.GetCurrentReading(ctx, owner, book.Book.LanguageTag)
	if err != nil {
		return Outcome{Book: book}, err
	}
	if current.IsActive() && current.BookID == book.Book.ID {
		return Outcome{Kind: OutcomeCurrentBookRefused, Book: book}, nil
	}
	if book.Disposition != domain.BookDispositionToRead {
		return Outcome{Kind: OutcomeNotToRead, Book: book}, nil
	}

	outcome, err := m.route(ctx, owner, book)
	if err != nil {
		return outcome, err
	}
	if outcome.Kind == OutcomeCouldNotStart && errors.Is(outcome.Err, analysis.ErrCurrentBook) {
		// The submission lost a race with Start. The current Book's evidence
		// is frozen, exactly as the guard above reports.
		outcome.Kind = OutcomeCurrentBookRefused
	}
	return outcome, nil
}

// route chooses the submission a To Read Book needs. The disposition change is
// the learner's intent; the route only decides how its analysis reaches the
// Book.
func (m *Module) route(ctx context.Context, owner string, book domain.MyBook) (Outcome, error) {
	if book.ReadingUnit == nil {
		return m.submitAnalysis(ctx, owner, book)
	}
	return m.submitClassical(ctx, owner, book)
}

func (m *Module) submitClassical(ctx context.Context, owner string, book domain.MyBook) (Outcome, error) {
	handle, err := m.preparation.SubmitClassicalPreparation(ctx, owner, book.Book.ID)
	if err != nil {
		if errors.Is(err, ErrPreparationUnavailable) {
			return Outcome{Kind: OutcomePreparationUnavailable, Route: RouteClassical, Book: book}, nil
		}
		logClassicalFailure(owner, book.Book.ID, err)
		switch {
		case errors.Is(err, classical.ErrLegacyRows):
			return Outcome{Kind: OutcomeLegacyRows, Route: RouteClassical, Book: book}, nil
		case errors.Is(err, analysis.ErrWithdrawnClassicalDefinition):
			return Outcome{Kind: OutcomeWithdrawnDefinition, Route: RouteClassical, Book: book}, nil
		default:
			return Outcome{Kind: OutcomeCouldNotStart, Route: RouteClassical, Book: book}, nil
		}
	}
	switch {
	case handle.Complete:
		return Outcome{Kind: OutcomeAlreadyCurrent, Route: RouteClassical, Book: book}, nil
	case handle.Terminal:
		return Outcome{Kind: OutcomeSourceRefused, Route: RouteClassical, Book: book}, nil
	default:
		return Outcome{Kind: OutcomeJobQueued, Route: RouteClassical, JobNumber: handle.JobID, Book: book}, nil
	}
}

func (m *Module) submitAnalysis(ctx context.Context, owner string, book domain.MyBook) (Outcome, error) {
	target := cataloguesync.AcquisitionTarget{}
	title := book.Book.Title
	needsAcquisition := book.Acquired == nil || book.Acquired.EvidenceState() == domain.BookUnavailable
	if needsAcquisition {
		acquired, err := m.acquire(ctx, owner, book.Book.ID)
		target = acquired
		if err != nil {
			logAnalysisFailure(owner, book.Book.ID, "acquisition", err)
			return analysisFailure(book, target, title, err), nil
		}
		reloaded, err := m.books.GetBookDetail(ctx, owner, book.Book.ID)
		if err != nil {
			logAnalysisFailure(owner, book.Book.ID, "acquisition", err)
			return analysisFailure(book, target, "", err), nil
		}
		book = reloaded
		title = book.Book.Title
		if book.Acquired == nil {
			err = errors.New("the current EPUB could not be loaded after acquisition")
			logAnalysisFailure(owner, book.Book.ID, "acquisition", err)
			return analysisFailure(book, target, title, err), nil
		}
	}

	handle, err := m.analysis.SubmitToReadBookAnalysis(ctx, owner, book.Book.ID, book.Acquired.Source.ID)
	if err != nil {
		logAnalysisFailure(owner, book.Book.ID, "analysis submission", err)
		if errors.Is(err, domain.ErrExtractedUnitsUnavailable) || errors.Is(err, analysis.ErrEPUBRequired) {
			return Outcome{Kind: OutcomeNoUsableEPUBUnits, Route: RouteEPUB, Book: book}, nil
		}
		return Outcome{Kind: OutcomeCouldNotStart, Route: RouteEPUB, Book: book, Err: err}, nil
	}
	return Outcome{Kind: OutcomeJobQueued, Route: RouteEPUB, JobNumber: handle.DisplayNumber, Book: book, Target: target}, nil
}

// analysisFailure carries the facts the web app needs to name the acquisition
// connection and entry in the learner-facing sentence.
func analysisFailure(book domain.MyBook, target cataloguesync.AcquisitionTarget, title string, err error) Outcome {
	return Outcome{
		Kind: OutcomeAcquisitionFailed, Route: RouteEPUB, Book: book,
		Target: target, Title: title, Err: err, AcquisitionFailed: true,
	}
}

func (m *Module) acquire(ctx context.Context, owner, bookID string) (cataloguesync.AcquisitionTarget, error) {
	if m.acquisition == nil {
		return cataloguesync.AcquisitionTarget{}, ErrAcquisitionUnavailable
	}
	target, err := m.acquisition.FindAcquisitionTarget(ctx, owner, bookID)
	if err != nil {
		return target, err
	}
	_, err = m.acquisition.AcquireForBook(ctx, owner, target.ConnectionID, target.Language, bookID, target.Entry)
	return target, err
}

// logClassicalFailure writes the stable stage and code only: raw error text can
// carry source detail, and Greek never belongs in operator logs.
func logClassicalFailure(owner, bookID string, err error) {
	stage, code := analysis.ClassicalPreparationFailure(err)
	log.Printf("mouseion: classical preparation submission failed for owner %s book %s stage=%s code=%s",
		strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(string(stage)), strconv.Quote(code))
}

// logAnalysisFailure writes the failing step and the quoted error. A refused
// refresh of the current Book is an expected outcome, not a failure.
func logAnalysisFailure(owner, bookID, step string, err error) {
	if errors.Is(err, analysis.ErrCurrentBook) {
		return
	}
	// The Book id comes from the request path, and the error may echo it;
	// quoting keeps a crafted id from forging log lines.
	log.Printf("mouseion: To Read %s failed for owner %s book %s: %s", step, owner, strconv.Quote(bookID), strconv.Quote(err.Error()))
}
