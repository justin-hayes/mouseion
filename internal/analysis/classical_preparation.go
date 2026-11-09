package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const (
	// ClassicalPreparationQueue is the dedicated serial queue classical Books
	// prepare on. It is never the shared analysis queue, so a whole-work move
	// never delays EPUB analysis.
	ClassicalPreparationQueue = "classical_preparation"

	// maxClassicalSourceBytes is the same 512 MiB hard ceiling the generator
	// applies to a pinned work file.
	maxClassicalSourceBytes int64 = 512 << 20

	// classicalPreparationTimeout bounds one download and preparation.
	classicalPreparationTimeout = 10 * time.Minute
)

// liveJobStates are the River states in which an equivalent job still counts as
// pending work. A completed job never blocks the next submission or import.
var liveJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateRetryable,
	rivertype.JobStateScheduled,
}

// ErrWithdrawnClassicalDefinition refuses preparation of a Reading unit the
// running release no longer carries.
var ErrWithdrawnClassicalDefinition = errors.New("analysis: the Reading unit is no longer in the Classical catalog")

// ClassicalPreparationArgs names one Book and the definition revision to
// prepare. River uniqueness is scoped to the live states, so a completed run is
// a no-op and a failed or interrupted run can be submitted again.
type ClassicalPreparationArgs struct {
	OwnerID            string `json:"owner_id" river:"unique"`
	BookID             string `json:"book_id" river:"unique"`
	ReadingUnitKey     string `json:"reading_unit_key" river:"unique"`
	DefinitionRevision int    `json:"definition_revision" river:"unique"`
}

// Kind implements river.JobArgs.
func (ClassicalPreparationArgs) Kind() string { return ClassicalPreparationQueue }

// ClassicalPreparationHandle reports the outcome of one submission. Complete
// means the published analysis already matches the current definition.
type ClassicalPreparationHandle struct {
	JobID    int64
	Queued   bool
	Complete bool
	// Terminal reports a current source refusal that must not be retried until
	// a newer definition revision arrives.
	Terminal bool
}

// ClassicalPreparationStore is the persistence the preparation service and
// worker need. *persistence.PostgresStore implements it.
type ClassicalPreparationStore interface {
	LoadClassicalPreparationTarget(context.Context, string, string) (domain.ClassicalPreparationTarget, error)
	ClassicalPreparationComplete(context.Context, domain.ClassicalPreparationTarget) (bool, error)
	GetClassicalPreparationFailure(context.Context, string, string) (domain.ClassicalPreparationFailureRecord, bool, error)
	PutClassicalPreparationFailure(context.Context, domain.ClassicalPreparationFailureRecord) error
	ListLegacyLanguageRows(context.Context, string) ([]domain.LegacyLanguageRows, error)
	GetClassicalSource(context.Context, string) (domain.ClassicalSource, bool, error)
	PutClassicalSource(context.Context, domain.ClassicalSource) error
	PutClassicalPreparedDefinition(context.Context, domain.ClassicalPreparedDefinition) error
	HasReferenceTranslations(context.Context, string) (bool, error)
}

// classicalJobInserter is the River enqueue seam the preparation worker uses to
// start the separate translation import. It is set after the River client is
// created, mirroring the catalogue-sync worker.
type classicalJobInserter interface {
	Insert(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// ClassicalPreparationConfig supplies the release's reviewed layer pins and the
// acquisition bounds.
type ClassicalPreparationConfig struct {
	// Layers pins every upstream layer. Main passes the embedded release's
	// layers; tests supply a local layer server.
	Layers classical.Layers
	// HTTPClient fetches a pinned source. Nil uses a bounded default client.
	HTTPClient *http.Client
	// TempDir is where a source download is staged. Empty uses the system
	// temporary directory.
	TempDir string
}

// WithClassicalPreparationStore supplies the store the To Read path uses to
// read a Book's definition and enqueue its preparation.
func (s *Service) WithClassicalPreparationStore(store ClassicalPreparationStore) *Service {
	if s != nil {
		s.classicalStore = store
	}
	return s
}

// SubmitClassicalPreparation enqueues one ensure-once preparation job for a
// classical Book. A published analysis for the same definition revision,
// source pin and interpretation version is a no-op. A live duplicate reuses its
// job; a failed or interrupted run is submitted again and resumes.
func (s *Service) SubmitClassicalPreparation(ctx context.Context, owner, bookID string) (result ClassicalPreparationHandle, err error) {
	if s == nil || s.pool == nil || s.client == nil || s.classicalStore == nil {
		return result, errors.New("analysis: classical preparation service is not configured")
	}
	if strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return result, errors.New("analysis: classical preparation owner and Book are required")
	}
	target, err := s.classicalStore.LoadClassicalPreparationTarget(ctx, owner, bookID)
	if err != nil {
		return result, err
	}
	if target.Withdrawn {
		s.recordRefusedPreparation(ctx, target)
		return result, ErrWithdrawnClassicalDefinition
	}
	submission, err := s.classicalPreparationSubmissionState(ctx, owner, bookID, target)
	if err != nil {
		return result, err
	}
	if handle, done := submissionHandle(submission); done {
		return handle, nil
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return result, fmt.Errorf("begin classical preparation submission: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 471))`, owner+":"+bookID); err != nil {
		return result, fmt.Errorf("lock classical preparation submission: %w", err)
	}
	// Re-check under the lock so two To Read requests cannot both enqueue.
	submission, err = s.classicalPreparationSubmissionState(ctx, owner, bookID, target)
	if err != nil {
		return result, err
	}
	if handle, done := submissionHandle(submission); done {
		return handle, nil
	}
	inserted, err := s.client.InsertTx(ctx, tx, ClassicalPreparationArgs{
		OwnerID:            owner,
		BookID:             bookID,
		ReadingUnitKey:     target.ReadingUnitKey,
		DefinitionRevision: target.DefinitionRevision,
	}, &river.InsertOpts{
		Queue:       ClassicalPreparationQueue,
		MaxAttempts: 1,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
			// A completed job must not block the next submission: the job checks
			// the prepared-definition record and no-ops. Failed and interrupted
			// runs are submitted again.
			ByState: liveJobStates,
		},
	})
	if err != nil {
		return result, fmt.Errorf("enqueue classical preparation: %w", err)
	}
	if commitErr := tx.Commit(ctx); commitErr != nil {
		return result, commitErr
	}
	// A unique duplicate returns the existing live job so the learner sees one
	// preparation, not two.
	return ClassicalPreparationHandle{JobID: inserted.Job.ID, Queued: true}, nil
}

// classicalPreparationSubmission is the outcome of the ensure-once and terminal
// checks that a submission performs before inserting a job.
type classicalPreparationSubmission struct {
	Complete bool
	Terminal bool
}

// recordRefusedPreparation keeps the withdrawn state visible on the jobs
// surface even though no River job exists for a refused submission. A failure
// to record it is not fatal: the current Reading unit already carries the
// withdrawn tombstone the learner surfaces read.
func (s *Service) recordRefusedPreparation(ctx context.Context, target domain.ClassicalPreparationTarget) {
	err := s.classicalStore.PutClassicalPreparationFailure(ctx, domain.ClassicalPreparationFailureRecord{
		OwnerID:            target.OwnerID,
		BookID:             target.BookID,
		ReadingUnitKey:     target.ReadingUnitKey,
		DefinitionRevision: target.DefinitionRevision,
		Stage:              domain.ClassicalPreparationFailureWithdrawn,
		Code:               classical.WithdrawnCode,
	})
	if err != nil {
		// The Book's current Reading unit already carries the withdrawn
		// tombstone every learner surface reads, so this is best effort.
		log.Printf("mouseion: record refused classical preparation owner=%s book=%s code=%s",
			target.OwnerID, target.BookID, classical.WithdrawnCode)
	}
}

// submissionHandle reports the settled outcome of a submission that must not
// insert a job.
func submissionHandle(submission classicalPreparationSubmission) (ClassicalPreparationHandle, bool) {
	switch {
	case submission.Complete:
		return ClassicalPreparationHandle{Complete: true}, true
	case submission.Terminal:
		return ClassicalPreparationHandle{Terminal: true}, true
	}
	return ClassicalPreparationHandle{}, false
}

// classicalPreparationSubmissionState applies the catalog precondition, the
// ensure-once check and the terminal source-failure check. Callers run it before
// and again under the submission lock.
func (s *Service) classicalPreparationSubmissionState(ctx context.Context, owner, bookID string, target domain.ClassicalPreparationTarget) (classicalPreparationSubmission, error) {
	if err := refuseClassicalPreparationOnLegacyRows(ctx, s.classicalStore); err != nil {
		return classicalPreparationSubmission{}, err
	}
	complete, err := s.classicalStore.ClassicalPreparationComplete(ctx, target)
	if err != nil {
		return classicalPreparationSubmission{}, err
	}
	if complete {
		return classicalPreparationSubmission{Complete: true}, nil
	}
	terminal, err := classicalPreparationTerminal(ctx, s.classicalStore, owner, bookID, target)
	if err != nil {
		return classicalPreparationSubmission{}, err
	}
	return classicalPreparationSubmission{Terminal: terminal}, nil
}

// refuseClassicalPreparationOnLegacyRows applies the installation-wide
// fail-closed catalog precondition to preparation as well as Add. Existing
// Books stay readable; no new preparation starts while legacy rows exist.
func refuseClassicalPreparationOnLegacyRows(ctx context.Context, store ClassicalPreparationStore) error {
	legacy, err := store.ListLegacyLanguageRows(ctx, classical.Language)
	if err != nil {
		return fmt.Errorf("inspect classical catalog legacy rows: %w", err)
	}
	if len(legacy) > 0 {
		return classical.ErrLegacyRows
	}
	return nil
}

// classicalPreparationTerminal reports a settled source refusal for exactly the
// current definition revision. Such a failure clears when a newer revision
// arrives, so it must not be retried against the same reviewed definition.
func classicalPreparationTerminal(ctx context.Context, store ClassicalPreparationStore, owner, bookID string, target domain.ClassicalPreparationTarget) (bool, error) {
	failure, found, err := store.GetClassicalPreparationFailure(ctx, owner, bookID)
	if err != nil || !found {
		return false, err
	}
	if failure.DefinitionRevision < target.DefinitionRevision {
		return false, nil
	}
	return failure.Stage == domain.ClassicalPreparationFailureSource ||
		failure.Stage == domain.ClassicalPreparationFailureWithdrawn, nil
}

// ClassicalPreparationFailure returns the stable stage and code to record for a
// failed preparation. It never returns error text, source text or Greek.
func ClassicalPreparationFailure(err error) (domain.ClassicalPreparationFailure, string) {
	switch {
	case err == nil:
		return domain.ClassicalPreparationFailureNone, ""
	case errors.Is(err, ErrWithdrawnClassicalDefinition):
		return domain.ClassicalPreparationFailureWithdrawn, classical.WithdrawnCode
	case errors.Is(err, classical.ErrLegacyRows):
		return domain.ClassicalPreparationFailureCatalog, classical.LegacyRowsCode
	}
	if importErr, ok := errors.AsType[*ancientgreek.ImportError](err); ok {
		switch importErr.Code {
		case ancientgreek.CodeSourceDownloadFailed:
			return domain.ClassicalPreparationFailureFetch, importErr.Code
		case ancientgreek.CodePinMismatch:
			return domain.ClassicalPreparationFailureSource, importErr.Code
		case ancientgreek.CodeOversize:
			return domain.ClassicalPreparationFailureSource, importErr.Code
		case ancientgreek.CodeSourceLayerUnknown:
			return domain.ClassicalPreparationFailureSource, importErr.Code
		case ancientgreek.CodeUnknownSourceFormat,
			ancientgreek.CodeMalformed, ancientgreek.CodeUnknownPOS,
			ancientgreek.CodeUnknownInterpret, ancientgreek.CodeSentenceUnassigned,
			ancientgreek.CodeSentenceAmbiguous, ancientgreek.CodeEmptyCitationRange:
			return domain.ClassicalPreparationFailureSource, ancientgreek.CodeAdapterRefused
		default:
			return domain.ClassicalPreparationFailureImport, classical.PreparationFailedCode
		}
	}
	return domain.ClassicalPreparationFailureImport, classical.PreparationFailedCode
}

// ClassicalPreparationWorker prepares one classical Book: it makes the pinned
// work file present, parses it, cuts the Book's Reading unit, stores only the
// unit text and publishes an ordinary analysis. It never calls the NLP service
// and never enqueues analyze_corpus.
type ClassicalPreparationWorker struct {
	river.WorkerDefaults[ClassicalPreparationArgs]

	Store   ClassicalPreparationStore
	Writer  ancientgreek.ImportWriter
	Client  classicalJobInserter
	Config  ClassicalPreparationConfig
	mu      sync.Mutex
	cached  *ancientgreek.ParsedWork
	cacheID string
}

// Timeout bounds one preparation job.
func (w *ClassicalPreparationWorker) Timeout(*river.Job[ClassicalPreparationArgs]) time.Duration {
	return classicalPreparationTimeout
}

// Work implements river.Worker.
func (w *ClassicalPreparationWorker) Work(ctx context.Context, job *river.Job[ClassicalPreparationArgs]) (err error) {
	if w == nil || w.Store == nil {
		return errors.New("analysis: classical preparation store is required")
	}
	if w.Writer == nil {
		return errors.New("analysis: classical preparation writer is required")
	}
	target, err := w.Store.LoadClassicalPreparationTarget(ctx, job.Args.OwnerID, job.Args.BookID)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			w.recordFailure(ctx, target, err)
		}
	}()
	if target.Withdrawn {
		return ErrWithdrawnClassicalDefinition
	}
	if err := refuseClassicalPreparationOnLegacyRows(ctx, w.Store); err != nil {
		return err
	}
	complete, err := w.Store.ClassicalPreparationComplete(ctx, target)
	if err != nil {
		return err
	}
	if complete {
		logClassicalPreparation("complete", target, 0, 0)
		w.ensureTranslationImport(ctx, target)
		return nil
	}
	data, err := w.sourceBytes(ctx, target)
	if err != nil {
		return err
	}
	parsed, err := w.parse(target, data)
	if err != nil {
		return err
	}
	definition := targetDefinition(target)
	unit, err := parsed.CutUnit(definition)
	if err != nil {
		return err
	}
	written, err := w.Writer.WriteImportedAnalysis(ctx, ancientgreek.ImportWriteRequest{
		OwnerID:         target.OwnerID,
		BookID:          target.BookID,
		Definition:      definition,
		SourceBytes:     unit.SourceBytes,
		FullText:        unit.FullText,
		UnitID:          unit.UnitID,
		UnitTitle:       target.WorkTitle,
		Result:          unit.Result,
		SyntaxState:     unit.SyntaxState,
		AnalyzerName:    unit.AnalyzerName,
		AnalyzerVersion: unit.AnalyzerVersion,
		ConfigIdentity:  unit.ConfigIdentity,
	})
	if err != nil {
		return err
	}
	if err := w.Store.PutClassicalPreparedDefinition(ctx, domain.ClassicalPreparedDefinition{
		OwnerID:               target.OwnerID,
		BookID:                target.BookID,
		ReadingUnitKey:        target.ReadingUnitKey,
		DefinitionRevision:    target.DefinitionRevision,
		SourceFileSHA256:      target.SourceFileSHA256,
		InterpretationVersion: target.InterpretationVersion,
		AnalysisRunID:         written.RunID,
	}); err != nil {
		return err
	}
	logClassicalPreparation("published", target, unit.Report.Sentences, unit.Report.Tokens)
	// Translation import is deliberately after the published analysis and is
	// best effort from here: a missing translation never un-publishes a Book,
	// and the learner sees no separate translation state.
	w.ensureTranslationImport(ctx, target)
	return nil
}

// ensureTranslationImport starts the separate, non-blocking translation import
// when the definition pins a translation and the installation has no rows for
// that pin yet. A failure to check or enqueue is logged and ignored, because the
// Book is already analyzed and translation failure must not affect it.
func (w *ClassicalPreparationWorker) ensureTranslationImport(ctx context.Context, target domain.ClassicalPreparationTarget) {
	if w.Client == nil || !target.HasTranslation() {
		return
	}
	exists, err := w.Store.HasReferenceTranslations(ctx, target.TranslationFileSHA256)
	if err != nil {
		log.Printf("mouseion: check reference translation pin=%s: %v", target.TranslationFileSHA256, err)
		return
	}
	if exists {
		return
	}
	if _, err := w.Client.Insert(ctx, ClassicalTranslationArgs{
		Layer:         target.TranslationLayer,
		FileName:      target.TranslationFileName,
		Size:          target.TranslationFileSize,
		SHA256:        target.TranslationFileSHA256,
		CitationStart: target.TranslationCitationStart,
		CitationEnd:   target.TranslationCitationEnd,
		Language:      classical.Language,
	}, &river.InsertOpts{
		Queue:       ClassicalTranslationQueue,
		MaxAttempts: classicalTranslationMaxAttempts,
		UniqueOpts: river.UniqueOpts{
			ByArgs:  true,
			ByQueue: true,
			ByState: liveJobStates,
		},
	}); err != nil {
		log.Printf("mouseion: enqueue reference translation pin=%s: %v", target.TranslationFileSHA256, err)
	}
}

// recordFailure writes one stable failure outcome for the Book and logs only
// identifiers, counts and codes. It never logs the error text, which could carry
// source detail, and it never logs Greek.
func (w *ClassicalPreparationWorker) recordFailure(ctx context.Context, target domain.ClassicalPreparationTarget, err error) {
	stage, code := ClassicalPreparationFailure(err)
	if stage == domain.ClassicalPreparationFailureNone {
		return
	}
	record := domain.ClassicalPreparationFailureRecord{
		OwnerID:            target.OwnerID,
		BookID:             target.BookID,
		ReadingUnitKey:     target.ReadingUnitKey,
		DefinitionRevision: target.DefinitionRevision,
		Stage:              stage,
		Code:               code,
	}
	if putErr := w.Store.PutClassicalPreparationFailure(ctx, record); putErr != nil {
		log.Printf("mouseion: record classical preparation failure owner=%s book=%s unit=%s revision=%d stage=%s code=%s",
			target.OwnerID, target.BookID, target.ReadingUnitKey, target.DefinitionRevision, stage, code)
		return
	}
	log.Printf("mouseion: classical preparation failed owner=%s book=%s unit=%s revision=%d stage=%s code=%s",
		target.OwnerID, target.BookID, target.ReadingUnitKey, target.DefinitionRevision, stage, code)
}

// sourceBytes returns the verified pinned work file, from the installation
// store when present or from a fresh download otherwise. Every use verifies the
// bytes again, so a corrupted row is never parsed.
func (w *ClassicalPreparationWorker) sourceBytes(ctx context.Context, target domain.ClassicalPreparationTarget) ([]byte, error) {
	stored, found, err := w.Store.GetClassicalSource(ctx, target.SourceFileSHA256)
	if err != nil {
		return nil, err
	}
	if found {
		if err := verifyClassicalSource(stored.Content, target.SourceFileSize, target.SourceFileSHA256); err != nil {
			return nil, err
		}
		return stored.Content, nil
	}
	data, err := w.download(ctx, target)
	if err != nil {
		return nil, err
	}
	if err := verifyClassicalSource(data, target.SourceFileSize, target.SourceFileSHA256); err != nil {
		return nil, err
	}
	if err := w.Store.PutClassicalSource(ctx, domain.ClassicalSource{
		SHA256:   target.SourceFileSHA256,
		Layer:    target.SourceLayer,
		FileName: target.SourceFileName,
		Size:     int64(len(data)),
		Content:  data,
	}); err != nil {
		return nil, err
	}
	return data, nil
}

// download fetches the pinned layer URL into the acquisition temp directory and
// returns the staged bytes. The read is bounded by the reviewed size plus one,
// so a hostile response cannot exhaust memory.
func (w *ClassicalPreparationWorker) download(ctx context.Context, target domain.ClassicalPreparationTarget) (data []byte, err error) {
	layer, ok := w.Config.Layers.Layer(target.SourceLayer)
	if !ok {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceLayerUnknown}
	}
	if target.SourceFileSize > maxClassicalSourceBytes {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeOversize}
	}
	sourceURL, err := classicalSourceURL(layer.BaseURL, target.SourceFileName)
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceLayerUnknown}
	}
	client := w.Config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: classicalPreparationTimeout}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	return stagePinnedSource(response.Body, target.SourceFileSize, target.SourceFileSHA256, w.Config.TempDir)
}

// stagePinnedSource bounds, hashes and verifies one response body into a
// temporary file, then returns its bytes. The temporary file is removed on
// every outcome.
func stagePinnedSource(body io.Reader, size int64, digest, dir string) (data []byte, err error) {
	file, err := os.CreateTemp(dir, "mouseion-classical-")
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	path := file.Name()
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, fmt.Errorf("remove classical source temp file: %w", removeErr))
		}
	}()
	hasher := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hasher), io.LimitReader(body, size+1))
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	if written > size {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeOversize}
	}
	if written != size || hex.EncodeToString(hasher.Sum(nil)) != digest {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodePinMismatch}
	}
	//nolint:gosec // path is a file this function created inside the configured temp directory.
	data, err = os.ReadFile(path)
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	return data, nil
}

// parse reuses the last parsed work file when the pin and interpretation
// version match. Parse results are not stored across restarts.
func (w *ClassicalPreparationWorker) parse(target domain.ClassicalPreparationTarget, data []byte) (*ancientgreek.ParsedWork, error) {
	cacheID := target.SourceFileSHA256 + "\x00" + target.InterpretationVersion
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.cached != nil && w.cacheID == cacheID {
		return w.cached, nil
	}
	parsed, err := parseClassicalSource(target.SourceFormat, data)
	if err != nil {
		return nil, err
	}
	w.cached, w.cacheID = parsed, cacheID
	return parsed, nil
}

// parseClassicalSource chooses the import adapter by the definition's source
// format, so a later reader adapter adds an adapter rather than reshaping the
// job.
func parseClassicalSource(format string, data []byte) (*ancientgreek.ParsedWork, error) {
	switch format {
	case classical.SourceFormatOGACoNLLU:
		return ancientgreek.ParseOgaSource(data)
	default:
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeUnknownSourceFormat}
	}
}

// targetDefinition converts a reconciled target into the domain definition the
// adapter and writer consume.
func targetDefinition(target domain.ClassicalPreparationTarget) domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key:                   target.ReadingUnitKey,
		Language:              classical.Language,
		WorkTitle:             target.WorkTitle,
		WorkAuthor:            target.WorkAuthor,
		EditionLabel:          target.EditionLabel,
		CitationStart:         target.CitationStart,
		CitationEnd:           target.CitationEnd,
		SourceFileName:        target.SourceFileName,
		SourceFileSize:        target.SourceFileSize,
		SourceFileSHA256:      target.SourceFileSHA256,
		InterpretationVersion: target.InterpretationVersion,
	}
}

// verifyClassicalSource rechecks a stored or downloaded file against its
// reviewed pin.
func verifyClassicalSource(data []byte, size int64, digest string) error {
	if int64(len(data)) > size {
		return &ancientgreek.ImportError{Code: ancientgreek.CodeOversize}
	}
	if int64(len(data)) != size {
		return &ancientgreek.ImportError{Code: ancientgreek.CodePinMismatch}
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return &ancientgreek.ImportError{Code: ancientgreek.CodePinMismatch}
	}
	return nil
}

// classicalSourceURL joins a reviewed layer base URL and one pinned file name.
func classicalSourceURL(baseURL, fileName string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", errors.New("analysis: layer base URL is invalid")
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	reference, err := url.Parse(fileName)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(reference).String(), nil
}

// AddClassicalPreparationWorker registers the serial worker on a River worker
// set and returns it, so main can attach the River client used to enqueue the
// separate translation import.
func AddClassicalPreparationWorker(workers *river.Workers, store *persistence.PostgresStore, config ClassicalPreparationConfig) *ClassicalPreparationWorker {
	worker := &ClassicalPreparationWorker{Store: store, Writer: NewGreekImportWriter(store), Config: config}
	if err := river.AddWorkerSafely(workers, worker); err != nil {
		log.Printf("mouseion: register classical preparation worker: %v", err)
	}
	return worker
}

// logClassicalPreparation records identifiers, counts and a state word only.
// It never logs a Book title, source text or Greek.
func logClassicalPreparation(state string, target domain.ClassicalPreparationTarget, sentences, tokens int) {
	log.Printf("mouseion: classical preparation %s owner=%s book=%s unit=%s revision=%d sentences=%d tokens=%d",
		state, target.OwnerID, target.BookID, target.ReadingUnitKey, target.DefinitionRevision, sentences, tokens)
}
