package readingintent

import (
	"context"
	"errors"
	"log"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// Preparation prepares an owner's Book for reading and returns a typed
// submission. Move and Retry depend on this one seam; the two implementations
// below choose between Classical preparation and EPUB preparation.
type Preparation interface {
	Prepare(context.Context, string, domain.MyBook) Submission
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

// EPUBAcquisition enqueues the durable acquisition job a Book without a current
// EPUB needs.
type EPUBAcquisition interface {
	SubmitEPUBAcquisition(context.Context, string, string) (analysis.EPUBAcquisitionHandle, error)
}

// PreparationConfig supplies the collaborators the two preparation
// implementations need. Classical and Analysis are required. EPUBAcquisition is
// optional; a missing acquisition seam refuses a Book that needs a source.
type PreparationConfig struct {
	Classical       ClassicalPreparation
	Analysis        Analysis
	EPUBAcquisition EPUBAcquisition
}

// NewPreparation builds the preparation seam. It chooses Classical
// preparation for a Book with a Reading unit and EPUB preparation otherwise.
func NewPreparation(cfg PreparationConfig) (Preparation, error) {
	switch {
	case cfg.Classical == nil:
		return nil, errors.New("reading intent: Classical preparation is required")
	case cfg.Analysis == nil:
		return nil, errors.New("reading intent: To Read analysis is required")
	}
	return preparations{
		classical: classicalPreparation{submission: cfg.Classical},
		epub: epubPreparation{
			analysis:    cfg.Analysis,
			acquisition: cfg.EPUBAcquisition,
		},
	}, nil
}

// preparations chooses the implementation for one Book. Reading intent owns
// the choice; each implementation owns its submission protocol.
type preparations struct {
	classical Preparation
	epub      Preparation
}

// Prepare applies the implementation the Book's evidence calls for.
func (p preparations) Prepare(ctx context.Context, owner string, book domain.MyBook) Submission {
	if book.ReadingUnit == nil {
		return p.epub.Prepare(ctx, owner, book)
	}
	return p.classical.Prepare(ctx, owner, book)
}

// classicalPreparation wraps the existing ensure-once Classical preparation
// submission.
type classicalPreparation struct {
	submission ClassicalPreparation
}

// Prepare submits the Classical preparation job and classifies its result.
func (p classicalPreparation) Prepare(ctx context.Context, owner string, book domain.MyBook) Submission {
	handle, err := p.submission.SubmitClassicalPreparation(ctx, owner, book.Book.ID)
	if err != nil {
		if errors.Is(err, ErrPreparationUnavailable) {
			return Submission{Kind: SubmissionRefused, Refusal: RefusalPreparationUnavailable, Route: RouteClassical, Book: book}
		}
		logClassicalFailure(owner, book.Book.ID, err)
		switch {
		case errors.Is(err, classical.ErrLegacyRows):
			return Submission{Kind: SubmissionRefused, Refusal: RefusalLegacyRows, Route: RouteClassical, Book: book}
		case errors.Is(err, analysis.ErrWithdrawnClassicalDefinition):
			return Submission{Kind: SubmissionRefused, Refusal: RefusalWithdrawnDefinition, Route: RouteClassical, Book: book}
		default:
			return Submission{Kind: SubmissionCouldNotStart, Route: RouteClassical, Book: book}
		}
	}
	switch {
	case handle.Complete:
		return Submission{Kind: SubmissionAlreadyCurrent, Route: RouteClassical, Book: book}
	case handle.Terminal:
		return Submission{Kind: SubmissionRefused, Refusal: RefusalSourceRefused, Route: RouteClassical, Book: book}
	default:
		return Submission{Kind: SubmissionQueued, Route: RouteClassical, JobNumber: handle.JobID, Book: book}
	}
}

// epubPreparation enqueues a missing source's acquisition, then submits the
// analysis job directly for a Book that already has its EPUB.
type epubPreparation struct {
	analysis    Analysis
	acquisition EPUBAcquisition
}

// Prepare enqueues acquisition for a Book that needs one. A Book that already
// has its EPUB submits analysis directly.
func (p epubPreparation) Prepare(ctx context.Context, owner string, book domain.MyBook) Submission {
	if needsAcquisition(book) {
		if p.acquisition == nil {
			logAnalysisFailure(owner, book.Book.ID, "acquisition submission", ErrAcquisitionUnavailable)
			return Submission{Kind: SubmissionCouldNotStart, Route: RouteEPUB, Book: book, Err: ErrAcquisitionUnavailable}
		}
		handle, err := p.acquisition.SubmitEPUBAcquisition(ctx, owner, book.Book.ID)
		if err != nil {
			logAnalysisFailure(owner, book.Book.ID, "acquisition submission", err)
			return Submission{Kind: SubmissionCouldNotStart, Route: RouteEPUB, Book: book, Err: err}
		}
		return Submission{Kind: SubmissionQueued, Route: RouteEPUB, JobNumber: handle.JobID, Acquisition: true, Book: book}
	}

	handle, err := p.analysis.SubmitToReadBookAnalysis(ctx, owner, book.Book.ID, book.Acquired.Source.ID)
	if err != nil {
		logAnalysisFailure(owner, book.Book.ID, "analysis submission", err)
		if errors.Is(err, domain.ErrExtractedUnitsUnavailable) || errors.Is(err, analysis.ErrEPUBRequired) {
			return Submission{Kind: SubmissionRefused, Refusal: RefusalNoUsableEPUBUnits, Route: RouteEPUB, Book: book}
		}
		return Submission{Kind: SubmissionCouldNotStart, Route: RouteEPUB, Book: book, Err: err}
	}
	return Submission{Kind: SubmissionQueued, Route: RouteEPUB, JobNumber: handle.DisplayNumber, Book: book}
}

// needsAcquisition reports a Book with no current EPUB source.
func needsAcquisition(book domain.MyBook) bool {
	return book.Acquired == nil || book.Acquired.EvidenceState() == domain.BookUnavailable
}

// logClassicalFailure writes the stable stage and code only: raw error text can
// carry source detail, and Greek never belongs in operator logs.
func logClassicalFailure(owner, bookID string, err error) {
	stage, code := analysis.ClassicalPreparationFailure(err)
	log.Printf("mouseion: classical preparation submission failed for owner %s book %s stage=%s code=%s",
		strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(string(stage)), strconv.Quote(string(code)))
}

// logAnalysisFailure writes the failing step and the quoted error. A refused
// refresh of the current Book is an expected outcome, not a failure.
func logAnalysisFailure(owner, bookID, step string, err error) {
	if errors.Is(err, analysis.ErrCurrentBook) {
		return
	}
	// The Book id comes from the request path, and the error may echo it;
	// quoting every identifier keeps a crafted one from forging log lines and
	// matches the classical failure line.
	log.Printf("mouseion: To Read %s failed for owner %s book %s: %s", step, strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(err.Error()))
}
