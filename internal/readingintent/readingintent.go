// Package readingintent owns a learner's reading intent for one Book: the To
// Read disposition transition, the choice between Classical preparation and
// EPUB acquisition and analysis, and the classification of the result.
// Handlers translate its typed outcome into a sentence; it returns no
// learner-facing text and knows nothing about HTTP.
package readingintent

import (
	"context"
	"errors"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
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

// Config supplies Move's collaborators. Books, CurrentReadings, Dispositions
// and Preparation are required; a missing one is a wiring mistake.
type Config struct {
	Books           BookReader
	CurrentReadings CurrentReadingReader
	Dispositions    DispositionWriter
	Preparation     Preparation
}

// Module carries out reading intent for one Book.
type Module struct {
	books           BookReader
	currentReadings CurrentReadingReader
	dispositions    DispositionWriter
	preparation     Preparation
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
		return nil, errors.New("reading intent: a preparation seam is required")
	}
	return &Module{
		books:           cfg.Books,
		currentReadings: cfg.CurrentReadings,
		dispositions:    cfg.Dispositions,
		preparation:     cfg.Preparation,
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
	return m.route(ctx, owner, book), nil
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

	outcome := m.route(ctx, owner, book)
	if outcome.Kind == OutcomeCouldNotStart && errors.Is(outcome.Err, analysis.ErrCurrentBook) {
		// The submission lost a race with Start. The current Book's evidence
		// is frozen, exactly as the guard above reports.
		outcome.Kind = OutcomeCurrentBookRefused
	}
	return outcome, nil
}

// route prepares the Book through the one seam and classifies its submission.
// The disposition change is the learner's intent; preparation only decides how
// its evidence reaches the Book.
func (m *Module) route(ctx context.Context, owner string, book domain.MyBook) Outcome {
	return classifySubmission(m.preparation.Prepare(ctx, owner, book))
}

// classifySubmission maps the seam's typed submission to the outcome the web
// app renders. Reading intent owns this classification so both implementations
// stay free of learner-facing outcomes.
func classifySubmission(submission Submission) Outcome {
	outcome := Outcome{
		Kind:        OutcomeCouldNotStart,
		Route:       submission.Route,
		JobNumber:   submission.JobNumber,
		Acquisition: submission.Acquisition,
		Book:        submission.Book,
		Err:         submission.Err,
	}
	switch submission.Kind {
	case SubmissionQueued:
		outcome.Kind = OutcomeJobQueued
	case SubmissionAlreadyCurrent:
		outcome.Kind = OutcomeAlreadyCurrent
	case SubmissionCouldNotStart:
	case SubmissionRefused:
		switch submission.Refusal {
		case RefusalPreparationUnavailable:
			outcome.Kind = OutcomePreparationUnavailable
		case RefusalLegacyRows:
			outcome.Kind = OutcomeLegacyRows
		case RefusalWithdrawnDefinition:
			outcome.Kind = OutcomeWithdrawnDefinition
		case RefusalSourceRefused:
			outcome.Kind = OutcomeSourceRefused
		case RefusalNoUsableEPUBUnits:
			outcome.Kind = OutcomeNoUsableEPUBUnits
		}
	default:
		// The zero and unknown kinds are wiring mistakes: report them as a
		// submission that could not start.
	}
	return outcome
}
