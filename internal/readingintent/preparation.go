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

// Preparation prepares an owner's Book for reading and returns a typed
// submission. Move and Retry depend on this one seam; the two implementations
// below choose between Classical preparation and EPUB acquisition and
// analysis.
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

// AcquisitionFailures records the latest EPUB acquisition failure for a Book
// and clears it after a successful acquisition.
type AcquisitionFailures interface {
	RecordEPUBAcquisitionFailure(context.Context, domain.EPUBAcquisitionFailureRecord) error
	ClearEPUBAcquisitionFailure(context.Context, string, string) error
}

// PreparationConfig supplies the collaborators the two preparation
// implementations need. Books, Classical and Analysis are required.
// FindTarget, Acquire and Acquisitions are optional; a missing acquisition
// step refuses a Book that needs a source as acquisition-unavailable, and a
// missing failure recorder leaves the durable reason unrecorded.
type PreparationConfig struct {
	Books     BookReader
	Classical ClassicalPreparation
	Analysis  Analysis
	// FindTarget resolves the catalogue entry a Book's EPUB can be acquired
	// from.
	FindTarget func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
	// Acquire downloads and imports the catalogue entry.
	Acquire func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
	// Acquisitions records and clears the EPUB acquisition failure evidence.
	Acquisitions AcquisitionFailures
}

// NewPreparation builds the preparation seam. It chooses Classical
// preparation for a Book with a Reading unit and EPUB acquisition and analysis
// otherwise.
func NewPreparation(cfg PreparationConfig) (Preparation, error) {
	switch {
	case cfg.Books == nil:
		return nil, errors.New("reading intent: a Book reader is required for preparation")
	case cfg.Classical == nil:
		return nil, errors.New("reading intent: Classical preparation is required")
	case cfg.Analysis == nil:
		return nil, errors.New("reading intent: To Read analysis is required")
	}
	return preparations{
		classical: classicalPreparation{submission: cfg.Classical},
		epub: epubPreparation{
			books:        cfg.Books,
			analysis:     cfg.Analysis,
			findTarget:   cfg.FindTarget,
			acquire:      cfg.Acquire,
			acquisitions: cfg.Acquisitions,
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

// epubPreparation acquires a source when the Book needs one, then submits the
// analysis job.
type epubPreparation struct {
	books        BookReader
	analysis     Analysis
	findTarget   func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)
	acquire      func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)
	acquisitions AcquisitionFailures
}

// Prepare acquires a missing source, then submits analysis. A failed
// acquisition keeps the Book To Read, records the stable reason and reports
// where acquisition looked.
func (p epubPreparation) Prepare(ctx context.Context, owner string, book domain.MyBook) Submission {
	target := cataloguesync.AcquisitionTarget{}
	title := book.Book.Title
	if needsAcquisition(book) {
		acquired, durableFailure, err := p.acquireForBook(ctx, owner, book.Book.ID)
		target = acquired
		if err != nil {
			if durableFailure {
				p.recordFailure(ctx, owner, book.Book.ID, target, err)
			}
			return failedAcquisition(owner, book, target, title, err)
		}
		p.clearFailure(ctx, owner, book.Book.ID)
		reloaded, err := p.books.GetBookDetail(ctx, owner, book.Book.ID)
		if err != nil {
			return failedAcquisition(owner, book, target, "", err)
		}
		book = reloaded
		title = book.Book.Title
		if book.Acquired == nil {
			err = errors.New("the current EPUB could not be loaded after acquisition")
			return failedAcquisition(owner, book, target, title, err)
		}
	}

	handle, err := p.analysis.SubmitToReadBookAnalysis(ctx, owner, book.Book.ID, book.Acquired.Source.ID)
	if err != nil {
		logAnalysisFailure(owner, book.Book.ID, "analysis submission", err)
		if errors.Is(err, domain.ErrExtractedUnitsUnavailable) || errors.Is(err, analysis.ErrEPUBRequired) {
			return Submission{Kind: SubmissionRefused, Refusal: RefusalNoUsableEPUBUnits, Route: RouteEPUB, Book: book}
		}
		return Submission{Kind: SubmissionCouldNotStart, Route: RouteEPUB, Book: book, Err: err}
	}
	return Submission{Kind: SubmissionQueued, Route: RouteEPUB, JobNumber: handle.DisplayNumber, Book: book, Target: target}
}

// needsAcquisition reports a Book with no current EPUB source.
func needsAcquisition(book domain.MyBook) bool {
	return book.Acquired == nil || book.Acquired.EvidenceState() == domain.BookUnavailable
}

// acquireForBook resolves the Book's indexed catalogue entry and downloads it.
// durableFailure reports that a durable acquisition failure really happened; a
// missing seam or an unrelated lookup failure never becomes durable acquisition
// evidence. A catalog entry that has left the index is the exception: that is
// the `entry` stage failure the learner can retry after a sync.
func (p epubPreparation) acquireForBook(ctx context.Context, owner, bookID string) (target cataloguesync.AcquisitionTarget, durableFailure bool, err error) {
	if p.findTarget == nil {
		return cataloguesync.AcquisitionTarget{}, false, ErrAcquisitionUnavailable
	}
	target, err = p.findTarget(ctx, owner, bookID)
	if err != nil {
		return target, errors.Is(err, cataloguesync.ErrCatalogEntryMissing), err
	}
	if p.acquire == nil {
		return target, false, ErrAcquisitionUnavailable
	}
	_, err = p.acquire(ctx, owner, target.ConnectionID, target.Language, bookID, target.Entry)
	return target, true, err
}

// recordFailure writes the latest failed acquisition as Book evidence. The To
// Read transition already committed, so a recording failure is joined to the
// operator log rather than allowed to roll the choice back.
func (p epubPreparation) recordFailure(ctx context.Context, owner, bookID string, target cataloguesync.AcquisitionTarget, err error) {
	if p.acquisitions == nil {
		return
	}
	stage, code := classifyAcquisitionFailure(err)
	record := domain.EPUBAcquisitionFailureRecord{
		OwnerID:         owner,
		BookID:          bookID,
		ConnectionID:    target.ConnectionID,
		EntryIdentifier: target.Entry.ID,
		Stage:           stage,
		Code:            code,
	}
	if recordErr := p.acquisitions.RecordEPUBAcquisitionFailure(ctx, record); recordErr != nil {
		log.Printf("mouseion: record EPUB acquisition failure owner=%s book=%s stage=%s code=%s: %v",
			strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(string(stage)), strconv.Quote(code), recordErr)
	}
}

// clearFailure deletes a previous failure after a successful acquisition. It
// never changes the learner-visible outcome of the success.
func (p epubPreparation) clearFailure(ctx context.Context, owner, bookID string) {
	if p.acquisitions == nil {
		return
	}
	if err := p.acquisitions.ClearEPUBAcquisitionFailure(ctx, owner, bookID); err != nil {
		log.Printf("mouseion: clear EPUB acquisition failure owner=%s book=%s: %v",
			strconv.Quote(owner), strconv.Quote(bookID), err)
	}
}

// classifyAcquisitionFailure maps one failed download or import attempt to the
// stable stage and code the domain vocabulary defines. It never classifies a
// missing seam or a failed lookup: those have no catalog attempt to record.
func classifyAcquisitionFailure(err error) (domain.EPUBAcquisitionFailureStage, string) {
	switch {
	case errors.Is(err, cataloguesync.ErrCatalogEntryMissing):
		return domain.EPUBAcquisitionFailureEntry, domain.EPUBAcquisitionCodeCatalogEntryMissing
	case errors.Is(err, opds.ErrDownloadTooLarge), errors.Is(err, opds.ErrRetainedEPUBTooLarge):
		return domain.EPUBAcquisitionFailureSource, domain.EPUBAcquisitionCodeSourceOversize
	case errors.Is(err, opds.ErrNoEPUB),
		errors.Is(err, epub.ErrInvalidEPUB),
		errors.Is(err, opds.ErrIncompatibleMediaType):
		return domain.EPUBAcquisitionFailureSource, domain.EPUBAcquisitionCodeSourceFormatUnknown
	case errors.Is(err, opds.ErrImportFailed):
		return domain.EPUBAcquisitionFailureImport, domain.EPUBAcquisitionCodeEPUBImportFailed
	default:
		// A refused or failed download is retryable, including a catalog that
		// could not be reached.
		return domain.EPUBAcquisitionFailureFetch, domain.EPUBAcquisitionCodeSourceDownloadFailed
	}
}

// failedAcquisition logs the failing step and carries the facts the web app
// needs to name the acquisition connection and entry in the learner-facing
// sentence.
func failedAcquisition(owner string, book domain.MyBook, target cataloguesync.AcquisitionTarget, title string, err error) Submission {
	logAnalysisFailure(owner, book.Book.ID, "acquisition", err)
	return Submission{
		Kind: SubmissionCouldNotStart, Route: RouteEPUB, Book: book,
		Target: target, Title: title, Err: err, AcquisitionFailed: true,
	}
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
	// quoting every identifier keeps a crafted one from forging log lines and
	// matches the classical failure line.
	log.Printf("mouseion: To Read %s failed for owner %s book %s: %s", step, strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(err.Error()))
}
