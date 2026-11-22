package analysis

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	// EPUBAcquisitionQueue is the durable queue one Catalog Book's EPUB
	// acquisition runs on. It is separate from analysis so a download never
	// waits behind an analysis and a retried analysis never downloads again.
	EPUBAcquisitionQueue = "epub_acquisition"

	// epubAcquisitionTimeout bounds one download and import.
	epubAcquisitionTimeout = 10 * time.Minute
)

// EPUBAcquisitionArgs names one Book to acquire. River uniqueness is scoped to
// the live states, so a second Add or Retry while acquisition is queued or
// running inserts nothing, while a settled failure can be retried as a new job.
type EPUBAcquisitionArgs struct {
	OwnerID string `json:"owner_id" river:"unique"`
	BookID  string `json:"book_id" river:"unique"`
}

// Kind implements river.JobArgs.
func (EPUBAcquisitionArgs) Kind() string { return EPUBAcquisitionQueue }

// EPUBAcquisitionHandle reports the one live job a submission found or inserted.
type EPUBAcquisitionHandle struct {
	JobID int64
}

// jobInserter is the River enqueue seam the acquisition submission uses.
type jobInserter interface {
	Insert(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// SubmitEPUBAcquisition enqueues one acquisition job for a Book. A live job for
// the same owner and Book is returned as the submission's handle rather than
// duplicated.
func (s *Service) SubmitEPUBAcquisition(ctx context.Context, owner, bookID string) (result EPUBAcquisitionHandle, err error) {
	if s == nil || s.acquisitionInserter == nil {
		return result, errors.New("analysis: EPUB acquisition service is not configured")
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return result, errors.New("analysis: EPUB acquisition owner and Book are required")
	}
	inserted, err := s.acquisitionInserter.Insert(ctx, EPUBAcquisitionArgs{OwnerID: owner, BookID: bookID}, &river.InsertOpts{
		Queue:       EPUBAcquisitionQueue,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
			ByState: liveJobStates,
		},
	})
	if err != nil {
		return result, fmt.Errorf("enqueue EPUB acquisition: %w", err)
	}
	if inserted == nil || inserted.Job == nil {
		return result, errors.New("enqueue EPUB acquisition: River returned no job")
	}
	return EPUBAcquisitionHandle{JobID: inserted.Job.ID}, nil
}

// EPUBAcquisitionStore is the persistence the acquisition worker needs.
// *persistence.PostgresStore implements it.
type EPUBAcquisitionStore interface {
	GetBookDetail(context.Context, string, string) (domain.MyBook, error)
	GetCurrentReading(context.Context, string, string) (domain.CurrentReading, error)
	RecordEPUBAcquisitionFailure(context.Context, domain.EPUBAcquisitionFailureRecord) error
	ClearEPUBAcquisitionFailure(context.Context, string, string) error
}

// EPUBAcquisitionTargetFinder resolves the Catalog index entry a Book's EPUB is
// acquired from.
type EPUBAcquisitionTargetFinder func(context.Context, string, string) (cataloguesync.AcquisitionTarget, error)

// EPUBAcquisitionAcquirer downloads and imports one Catalog entry.
type EPUBAcquisitionAcquirer func(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error)

// EPUBAcquisitionSubmission submits the existing analysis job after an EPUB is
// acquired.
type EPUBAcquisitionSubmission interface {
	SubmitToReadBookAnalysis(context.Context, string, string, string) (Handle, error)
}

// EPUBAcquisitionWorker acquires one Book's Catalog file and then submits the
// existing analysis job unchanged. A failed acquisition records the Book
// preparation failure and leaves the Book To Read; the learner retries from
// My Books.
type EPUBAcquisitionWorker struct {
	river.WorkerDefaults[EPUBAcquisitionArgs]

	Store      EPUBAcquisitionStore
	FindTarget EPUBAcquisitionTargetFinder
	Acquire    EPUBAcquisitionAcquirer
	Analysis   EPUBAcquisitionSubmission
}

// Timeout bounds one acquisition job.
func (w *EPUBAcquisitionWorker) Timeout(*river.Job[EPUBAcquisitionArgs]) time.Duration {
	return epubAcquisitionTimeout
}

// Work implements river.Worker.
func (w *EPUBAcquisitionWorker) Work(ctx context.Context, job *river.Job[EPUBAcquisitionArgs]) error {
	if err := w.validate(); err != nil {
		return err
	}
	owner, bookID := job.Args.OwnerID, job.Args.BookID
	skip, err := w.skip(ctx, owner, bookID)
	if err != nil || skip {
		return err
	}
	return w.acquireAndSubmit(ctx, owner, bookID)
}

// validate rejects a worker whose collaborators were not attached. A missing
// seam is a wiring mistake, not a Book failure.
func (w *EPUBAcquisitionWorker) validate() error {
	switch {
	case w == nil || w.Store == nil:
		return errors.New("analysis: EPUB acquisition store is required")
	case w.FindTarget == nil || w.Acquire == nil:
		return errors.New("analysis: EPUB acquisition catalogue seam is required")
	case w.Analysis == nil:
		return errors.New("analysis: EPUB acquisition analysis submission is required")
	}
	return nil
}

// skip re-checks the learner's intent at work time. A Book removed from My
// Books, no longer To Read, or already current is not downloaded and records no
// failure.
func (w *EPUBAcquisitionWorker) skip(ctx context.Context, owner, bookID string) (bool, error) {
	book, err := w.Store.GetBookDetail(ctx, owner, bookID)
	if errors.Is(err, domain.ErrNotFound) {
		logEPUBAcquisitionSkip(owner, bookID, "missing")
		return true, nil
	}
	if err != nil {
		return false, err
	}
	if book.Disposition != domain.BookDispositionToRead {
		logEPUBAcquisitionSkip(owner, bookID, "not-to-read")
		return true, nil
	}
	if book.Book.LanguageTag == "" {
		return false, nil
	}
	current, err := w.Store.GetCurrentReading(ctx, owner, book.Book.LanguageTag)
	if err != nil {
		return false, err
	}
	if current.IsActive() && current.BookID == bookID {
		logEPUBAcquisitionSkip(owner, bookID, "current")
		return true, nil
	}
	return false, nil
}

// acquireAndSubmit resolves the indexed target, acquires it, clears the old
// failure, and submits analysis against the reloaded current source.
func (w *EPUBAcquisitionWorker) acquireAndSubmit(ctx context.Context, owner, bookID string) error {
	target, err := w.FindTarget(ctx, owner, bookID)
	if err != nil {
		if unresolvedCatalogEntry(err) {
			w.recordFailure(ctx, owner, bookID, target, err)
			return err
		}
		logAcquisitionStepFailure(owner, bookID, "target lookup", err)
		return err
	}
	if _, err := w.Acquire(ctx, owner, target.ConnectionID, target.Language, bookID, target.Entry); err != nil {
		w.recordFailure(ctx, owner, bookID, target, err)
		return err
	}
	w.clearFailure(ctx, owner, bookID)
	return w.submitAnalysis(ctx, owner, bookID)
}

// submitAnalysis submits the existing analysis job against the source the
// import just made current.
func (w *EPUBAcquisitionWorker) submitAnalysis(ctx context.Context, owner, bookID string) error {
	reloaded, err := w.Store.GetBookDetail(ctx, owner, bookID)
	if err != nil {
		logAcquisitionStepFailure(owner, bookID, "reload", err)
		return err
	}
	if reloaded.Acquired == nil {
		err = errors.New("analysis: EPUB acquisition produced no current source")
		logAcquisitionStepFailure(owner, bookID, "reload", err)
		return err
	}
	if _, err := w.Analysis.SubmitToReadBookAnalysis(ctx, owner, bookID, reloaded.Acquired.Source.ID); err != nil {
		logAcquisitionStepFailure(owner, bookID, "analysis submission", err)
		return err
	}
	logEPUBAcquisitionComplete(owner, bookID)
	return nil
}

// recordFailure stores the stable stage and code only. It never writes source
// text, an error message, or a Book title.
func (w *EPUBAcquisitionWorker) recordFailure(ctx context.Context, owner, bookID string, target cataloguesync.AcquisitionTarget, err error) {
	stage, code := classifyAcquisitionFailure(err)
	record := domain.EPUBAcquisitionFailureRecord{
		OwnerID:         owner,
		BookID:          bookID,
		ConnectionID:    target.ConnectionID,
		EntryIdentifier: target.Entry.ID,
		Stage:           stage,
		Code:            code,
	}
	if putErr := w.Store.RecordEPUBAcquisitionFailure(ctx, record); putErr != nil {
		log.Printf("mouseion: record EPUB acquisition failure owner=%s book=%s stage=%s code=%s",
			strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(string(stage)), strconv.Quote(string(code)))
		return
	}
	log.Printf("mouseion: EPUB acquisition failed owner=%s book=%s stage=%s code=%s",
		strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(string(stage)), strconv.Quote(string(code)))
}

// clearFailure deletes a previous failure after a successful acquisition. A
// clearing failure never changes the acquisition outcome.
func (w *EPUBAcquisitionWorker) clearFailure(ctx context.Context, owner, bookID string) {
	if err := w.Store.ClearEPUBAcquisitionFailure(ctx, owner, bookID); err != nil {
		log.Printf("mouseion: clear EPUB acquisition failure owner=%s book=%s: %s",
			strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(err.Error()))
	}
}

// unresolvedCatalogEntry reports whether acquisition could not resolve a
// Catalog entry at all: the Book has no usable catalog-entry alias, the alias's
// connection is gone, or the entry is no longer indexed. They share the entry
// stage and code; only a missing index entry still names the connection and
// entry identifier.
func unresolvedCatalogEntry(err error) bool {
	return errors.Is(err, cataloguesync.ErrCatalogEntryAliasMissing) ||
		errors.Is(err, cataloguesync.ErrConnectionNotFound) ||
		errors.Is(err, cataloguesync.ErrCatalogEntryMissing)
}

// classifyAcquisitionFailure maps one failed download or import attempt to the
// stable stage and code the domain vocabulary defines.
func classifyAcquisitionFailure(err error) (domain.BookPreparationStage, domain.BookPreparationCode) {
	switch {
	case unresolvedCatalogEntry(err):
		return domain.BookPreparationStageEntry, domain.BookPreparationCodeCatalogEntryMissing
	case errors.Is(err, opds.ErrDownloadTooLarge), errors.Is(err, opds.ErrRetainedEPUBTooLarge):
		return domain.BookPreparationStageSource, domain.BookPreparationCodeSourceOversize
	case errors.Is(err, opds.ErrNoEPUB),
		errors.Is(err, epub.ErrInvalidEPUB),
		errors.Is(err, opds.ErrIncompatibleMediaType):
		return domain.BookPreparationStageSource, domain.BookPreparationCodeSourceFormatUnknown
	case errors.Is(err, opds.ErrImportFailed):
		return domain.BookPreparationStageImport, domain.BookPreparationCodeEPUBImportFailed
	default:
		// A refused or failed download is retryable, including a catalog that
		// could not be reached.
		return domain.BookPreparationStageFetch, domain.BookPreparationCodeSourceDownloadFailed
	}
}

func logEPUBAcquisitionSkip(owner, bookID, reason string) {
	log.Printf("mouseion: EPUB acquisition skipped owner=%s book=%s reason=%s",
		strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(reason))
}

func logEPUBAcquisitionComplete(owner, bookID string) {
	log.Printf("mouseion: EPUB acquisition complete owner=%s book=%s",
		strconv.Quote(owner), strconv.Quote(bookID))
}

// logAcquisitionStepFailure logs a non-durable operational failure. Identifiers
// and the quoted error are enough to diagnose it without source text.
func logAcquisitionStepFailure(owner, bookID, step string, err error) {
	log.Printf("mouseion: EPUB acquisition %s failed owner=%s book=%s: %s",
		step, strconv.Quote(owner), strconv.Quote(bookID), strconv.Quote(err.Error()))
}

// AddEPUBAcquisitionWorker registers the acquisition worker on a River worker
// set and returns it so main can attach the analysis seam once the River client
// exists.
func AddEPUBAcquisitionWorker(workers *river.Workers, store EPUBAcquisitionStore, findTarget EPUBAcquisitionTargetFinder, acquire EPUBAcquisitionAcquirer) *EPUBAcquisitionWorker {
	worker := &EPUBAcquisitionWorker{Store: store, FindTarget: findTarget, Acquire: acquire}
	if err := river.AddWorkerSafely(workers, worker); err != nil {
		log.Printf("mouseion: register EPUB acquisition worker: %v", err)
	}
	return worker
}
