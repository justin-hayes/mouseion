// Package analysis implements the durable, owner-scoped analysis job API.
package analysis

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/sourcecheck"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
	"github.com/riverqueue/river/rivertype"
)

const (
	Queue             = "analysis"
	jobTimeoutEnv     = "MOUSEION_ANALYSIS_JOB_TIMEOUT"
	defaultJobTimeout = 30 * time.Minute
)

var ErrNotFound = errors.New("analysis job: not found")
var ErrEPUBRequired = errors.New("analysis requires an EPUB source")
var ErrCurrentBook = errors.New("current reading cannot be refreshed")
var ErrDependencyParsingUnavailable = errors.New("NLP service is misconfigured: dependency parsing is unavailable")

type JobArgs struct {
	RunID             string `json:"run_id,omitempty" river:"unique"`
	Attempt           int    `json:"attempt,omitempty"`
	OwnerID           string `json:"owner_id" river:"unique"`
	SourceMaterialID  string `json:"source_material_id"`
	ContentHash       string `json:"content_hash" river:"unique"`
	Language          string `json:"language"`
	Text              string `json:"text"`
	SourceIdentifier  string `json:"source_identifier"`
	Title             string `json:"title"`
	AnalysisIdentity  string `json:"analysis_identity,omitempty" river:"unique"`
	ContentRevisionID string `json:"content_revision_id,omitempty"`
	SnapshotID        string `json:"snapshot_id,omitempty"`
	AnalyzerName      string `json:"analyzer_name,omitempty"`
	AnalyzerVersion   string `json:"analyzer_version,omitempty"`
	ConfigIdentity    string `json:"config_identity,omitempty"`
}

func (JobArgs) Kind() string { return "analyze_corpus" }

type Handle struct {
	ID, DisplayNumber int64
	RunID             string
	JobID             int64
}

type Status struct {
	ID               int64
	DisplayNumber    int64
	SourceMaterialID string
	State            rivertype.JobState
	Progress         int
	Error            string
	CorpusID         string
	Attempt          int
	CreatedAt        time.Time
	FinalizedAt      *time.Time
	RunID            string
	LogicalState     string
	AttemptCount     int
}

// CompletedAnalysis is the immutable, book-bound result used by the learner
// result page. It is deliberately resolved by analysis-run identity rather
// than by a mutable book-level latest projection.
type CompletedAnalysis struct {
	RunID            string
	OwnerID          string
	SourceMaterialID string
	SnapshotID       string
	AnalyzerName     string
	AnalyzerVersion  string
	ConfigIdentity   string
	CompletedAt      *time.Time
	JobID            int64
	DisplayNumber    int64
	Source           domain.SourceMaterial
	Corpus           domain.Corpus
	Artifact         domain.NormalizedArtifact
}

const (
	snapshotAnalyzerName = "mouseion-snapshot-analyzer"
	// Version 3 identifies lemma-boundary cleanup so an existing immutable run
	// is not silently reused after normalization.
	snapshotAnalyzerVersion = "3"
	snapshotConfigIdentity  = "selection-default-v1"
	mainTextConfigIdentity  = "selection-maintext-landmarks-v1"
	// distributorConfigIdentity identifies a run whose scope excludes Project
	// Gutenberg Distributor text (ADR 0108).
	distributorConfigIdentity = "selection-distributor-gutenberg-v1"
	// mainTextDistributorConfigIdentity identifies a run where landmark and
	// Distributor rules both exclude something.
	mainTextDistributorConfigIdentity = "selection-maintext-landmarks-distributor-gutenberg-v1"
	// Version 2 identifies the current ordinary analysis contract, including
	// lemma-boundary cleanup. Bump it whenever ordinary normalization or
	// analyzer output semantics change.
	ordinaryAnalysisContractVersion = "2"
)

type Service struct {
	pool         *pgxpool.Pool
	client       riverClient
	findLiveJob  func(context.Context, string, string) (int64, error)
	capabilities analyzer.CapabilityProvider
}

type riverClient interface {
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
	JobGet(context.Context, int64) (*rivertype.JobRow, error)
	JobCancel(context.Context, int64) (*rivertype.JobRow, error)
}

func (s *Service) ensureAttemptTx(ctx context.Context, tx pgx.Tx, args JobArgs, runID string, failOrphaned bool) (int64, error) {
	var riverJobID int64
	err := tx.QueryRow(ctx, `SELECT job.id FROM river_job job JOIN analysis_run_attempts attempt ON attempt.river_job_id=job.id AND attempt.run_id=$2::uuid AND attempt.state IN ('queued','running') WHERE job.kind=$1 AND job.args->>'run_id'=$2::text AND job.state IN ('available','pending','running','retryable','scheduled') ORDER BY job.id DESC LIMIT 1`, (JobArgs{}).Kind(), runID).Scan(&riverJobID)
	if err == nil {
		return riverJobID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&state); err != nil {
		return 0, err
	}
	if state != "queued" && state != "completed" {
		if state == "running" && failOrphaned {
			_, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='failed',last_error=$2,updated_at=now(),completed_at=now() WHERE id=$1 AND state='running'`, runID, safeAnalysisError(errors.New("analysis worker is no longer active")))
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='failed',error=$2,finalized_at=now() WHERE run_id=$1 AND state IN ('queued','running')`, runID, safeAnalysisError(errors.New("analysis worker is no longer active")))
			}
			return 0, err
		}
		return 0, nil
	}
	var attempt int
	if err := tx.QueryRow(ctx, `UPDATE analysis_runs SET attempt_count=attempt_count+1,updated_at=now() WHERE id=$1 RETURNING attempt_count`, runID).Scan(&attempt); err != nil {
		return 0, err
	}
	args.RunID, args.Attempt = runID, attempt
	inserted, err := s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: 1})
	if err != nil {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO analysis_run_attempts(run_id,owner_id,source_material_id,attempt_number,river_job_id,state) VALUES($1,$2,$3,$4,$5,'queued')`, runID, args.OwnerID, args.SourceMaterialID, attempt, inserted.Job.ID); err != nil {
		return 0, err
	}
	return inserted.Job.ID, nil
}

const noNewerAnalysisRun = `NOT EXISTS (
	SELECT 1 FROM analysis_runs newer
	LEFT JOIN analysis_jobs candidate_job ON candidate_job.owner_id=r.owner_id AND candidate_job.analysis_run_id=r.id
	LEFT JOIN analysis_jobs newer_job ON newer_job.owner_id=newer.owner_id AND newer_job.analysis_run_id=newer.id
	WHERE newer.owner_id=r.owner_id AND newer.source_material_id=r.source_material_id
	  AND newer.content_revision_id=r.content_revision_id AND newer.snapshot_id=r.snapshot_id
	  AND ((candidate_job.display_number IS NOT NULL AND newer_job.display_number > candidate_job.display_number)
	    OR (candidate_job.display_number IS NULL AND newer.created_at > r.created_at))
)`

func analysisNeedsPublication(ctx context.Context, tx pgx.Tx, ownerID, runID string) (bool, error) {
	var needsPublication bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM source_materials s
		JOIN analysis_runs r ON r.owner_id=s.owner_id AND r.source_material_id=s.id
		JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id AND c.analysis_run_id=r.id AND c.status='complete'
		JOIN books b ON b.owner_id=s.owner_id AND b.id=s.book_id
		LEFT JOIN current_analysis_identity current ON current.owner_id=b.owner_id AND current.book_id=b.id AND current.analysis_run_id=r.id
		WHERE r.owner_id=$1 AND r.id=$2
		  AND s.current_content_revision_id=r.content_revision_id AND s.current_snapshot_id=r.snapshot_id
		  AND current.analysis_run_id IS NULL
		  AND `+noNewerAnalysisRun+`
	)`, ownerID, runID).Scan(&needsPublication)
	return needsPublication, err
}

func (s *Service) retryCompletedPublicationTx(ctx context.Context, tx pgx.Tx, args JobArgs, ownerID, runID string, id, display int64) (Handle, bool, error) {
	needsPublication, err := analysisNeedsPublication(ctx, tx, ownerID, runID)
	if err != nil {
		return Handle{}, false, fmt.Errorf("check completed analysis publication: %w", err)
	}
	if !needsPublication {
		return Handle{}, false, nil
	}
	args.RunID = runID
	jobID, err := s.ensureAttemptTx(ctx, tx, args, runID, false)
	if err != nil {
		return Handle{}, false, err
	}
	return Handle{ID: id, JobID: jobID, DisplayNumber: display, RunID: runID}, true, nil
}

func completePublicationAttempt(ctx context.Context, tx pgx.Tx, runID string, riverJobID int64) error {
	_, err := tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='completed',error='',finalized_at=now() WHERE run_id=$1 AND river_job_id=$2 AND state IN ('queued','running')`, runID, riverJobID)
	return err
}

func NewService(pool *pgxpool.Pool, client riverClient) *Service {
	service := &Service{pool: pool, client: client}
	service.findLiveJob = service.liveJobID
	return service
}

// SubmitAnalysis atomically records one snapshot-bound analysis run and inserts
// its River job. The worker reloads the immutable extracted snapshot.
func (s *Service) SubmitAnalysis(ctx context.Context, owner, sourceID string) (result Handle, err error) {
	return s.submitAnalysis(ctx, owner, "", sourceID)
}

// SubmitToReadBookAnalysis submits a recovery attempt only while the Book is
// still To Read and is not the current Book. The Book row lock is shared with
// current-reading transitions, so the check and job insertion linearize with
// a concurrent start.
func (s *Service) SubmitToReadBookAnalysis(ctx context.Context, owner, bookID, sourceID string) (result Handle, err error) {
	if strings.TrimSpace(bookID) == "" {
		return Handle{}, errors.New("analysis Book is required")
	}
	return s.submitAnalysis(ctx, owner, bookID, sourceID)
}

func (s *Service) submitAnalysis(ctx context.Context, owner, bookID, sourceID string) (result Handle, err error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(sourceID) == "" {
		return Handle{}, errors.New("analysis owner and source are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin analysis submission: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner); err != nil {
		return Handle{}, fmt.Errorf("lock analysis submission: %w", err)
	}
	if err := admitToReadBookAnalysis(ctx, tx, owner, bookID); err != nil {
		return Handle{}, err
	}
	args, err := loadSubmissionInputs(ctx, tx, owner, bookID, sourceID)
	if err != nil {
		return Handle{}, err
	}
	result, handled, err := s.submitExistingRun(ctx, tx, owner, args, nil)
	if err != nil {
		return Handle{}, err
	}
	if !handled {
		result, err = s.createAnalysisRun(ctx, tx, owner, args)
		if err != nil {
			return Handle{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return result, nil
}

// admitToReadBookAnalysis locks the optional To Read Book and rejects the
// submission when the Book is the current Book or is no longer To Read. The
// Book row lock is shared with current-reading transitions, so the checks and
// job insertion linearize with a concurrent start.
func admitToReadBookAnalysis(ctx context.Context, tx pgx.Tx, owner, bookID string) error {
	if bookID == "" {
		return nil
	}
	var lockedBook string
	err := tx.QueryRow(ctx, `SELECT id::text FROM books WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, bookID).Scan(&lockedBook)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("lock To Read Book for analysis: %w", err)
	}
	var disposition string
	var current bool
	err = tx.QueryRow(ctx, `SELECT d.disposition, EXISTS(SELECT 1 FROM primary_goals g WHERE g.owner_id=b.owner_id AND g.book_id=b.id)
		FROM books b JOIN book_dispositions d ON d.owner_id=b.owner_id AND d.book_id=b.id
		WHERE b.owner_id=$1 AND b.id=$2`, owner, bookID).Scan(&disposition, &current)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check To Read Book for analysis: %w", err)
	}
	if current {
		return ErrCurrentBook
	}
	if disposition != "to_read" {
		return ErrNotFound
	}
	return nil
}

// loadSubmissionInputs loads the immutable EPUB source and its extracted-unit
// snapshot and resolves the main-text decision into the River job arguments.
func loadSubmissionInputs(ctx context.Context, tx pgx.Tx, owner, bookID, sourceID string) (JobArgs, error) {
	var args JobArgs
	var revisionID, snapshotID, mediaType string
	var sourceBookID string
	err := tx.QueryRow(ctx, `SELECT s.owner_id,s.id,s.language,s.source_identifier,s.title,
		COALESCE(r.revision_id::text,''),CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END,
		COALESCE(s.current_snapshot_id::text,''),s.media_type,COALESCE(s.book_id::text,'')
		FROM source_materials s
		LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.source_material_id=s.id AND r.revision_id=s.current_content_revision_id
		WHERE s.owner_id=$1 AND s.id=$2`, owner, sourceID).
		Scan(&args.OwnerID, &args.SourceMaterialID, &args.Language, &args.SourceIdentifier, &args.Title, &revisionID, &args.ContentHash, &snapshotID, &mediaType, &sourceBookID)
	if errors.Is(err, pgx.ErrNoRows) {
		return JobArgs{}, ErrNotFound
	}
	if err != nil {
		return JobArgs{}, fmt.Errorf("load EPUB for analysis: %w", err)
	}
	if bookID != "" && sourceBookID != bookID {
		return JobArgs{}, ErrNotFound
	}
	if mediaType != "application/epub+zip" {
		return JobArgs{}, ErrEPUBRequired
	}
	if revisionID == "" || snapshotID == "" {
		return JobArgs{}, domain.ErrExtractedUnitsUnavailable
	}
	args.ContentRevisionID, args.SnapshotID = revisionID, snapshotID
	allUnits, err := loadSnapshotUnits(ctx, tx, owner, sourceID, revisionID, snapshotID)
	if err != nil {
		return JobArgs{}, fmt.Errorf("load extracted EPUB units: %w", err)
	}
	decision := identifyMainText(allUnits)
	args.AnalyzerName, args.AnalyzerVersion = snapshotAnalyzerName, snapshotAnalyzerVersion
	args.ConfigIdentity = snapshotConfigIdentityFor(decision)
	return args, nil
}

// submitExistingRun attaches the submission to the newest run for its snapshot
// identity. The identity includes the exact extracted snapshot, so a source
// structure adoption under the same content revision never reuses the earlier
// snapshot's run. Queued or running runs ensure an attempt; a completed run
// retries publication; a failed or cancelled run is requeued with history. It
// reports handled=false when no run exists, so the caller creates one.
//
// An Analysis update passes its target Analysis pipeline identity: a completed
// run whose artifact records another normalization identity is then not
// reused, so the update creates a new run (ADR 0107). Runs that have not
// completed carry no identity yet and run under the current pipeline, so
// double submit and retry still converge on one run. Ordinary submissions
// pass nil and never start an update.
func (s *Service) submitExistingRun(ctx context.Context, tx pgx.Tx, owner string, args JobArgs, target *domain.AnalysisPipelineIdentity) (Handle, bool, error) {
	var runID, state string
	err := tx.QueryRow(ctx, `SELECT id::text,state FROM analysis_runs WHERE owner_id=$1 AND content_revision_id=$2 AND snapshot_id=$3 AND analyzer_name=$4 AND analyzer_version=$5 AND config_identity=$6 ORDER BY created_at DESC,id DESC LIMIT 1 FOR UPDATE`, owner, args.ContentRevisionID, args.SnapshotID, args.AnalyzerName, args.AnalyzerVersion, args.ConfigIdentity).Scan(&runID, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, false, nil
	}
	if err != nil {
		return Handle{}, false, fmt.Errorf("check existing analysis: %w", err)
	}
	if state == "completed" && target != nil {
		matches, err := completedRunMatches(ctx, tx, owner, runID, *target)
		if err != nil || !matches {
			return Handle{}, false, err
		}
	}
	var id, display int64
	if err = tx.QueryRow(ctx, `SELECT river_job_id,display_number FROM analysis_jobs WHERE owner_id=$1 AND analysis_run_id=$2`, owner, runID).Scan(&id, &display); err != nil {
		return Handle{}, false, fmt.Errorf("load existing analysis handle: %w", err)
	}
	switch state {
	case "queued", "running":
		if err := s.resumeExistingRun(ctx, tx, args, runID, state); err != nil {
			return Handle{}, false, err
		}
	case "completed":
		retry, shouldRetry, retryErr := s.retryCompletedPublicationTx(ctx, tx, args, owner, runID, id, display)
		if retryErr != nil {
			return Handle{}, false, retryErr
		}
		if shouldRetry {
			return retry, true, nil
		}
	case "failed", "cancelled":
		return s.requeueExistingRun(ctx, tx, owner, args, runID, state, id, display)
	}
	return Handle{ID: id, JobID: id, DisplayNumber: display, RunID: runID}, true, nil
}

// resumeExistingRun attaches the submission to a queued or running run. A
// running run fails its orphaned attempt before a fresh one is enqueued, while a
// queued run resumes in place.
func (s *Service) resumeExistingRun(ctx context.Context, tx pgx.Tx, args JobArgs, runID, state string) error {
	args.RunID = runID
	_, err := s.ensureAttemptTx(ctx, tx, args, runID, state == "running")
	return err
}

// requeueExistingRun resets a failed or cancelled run to queued, enqueues a
// fresh attempt, and records the retry in operational history.
func (s *Service) requeueExistingRun(ctx context.Context, tx pgx.Tx, owner string, args JobArgs, runID, state string, id, display int64) (Handle, bool, error) {
	if _, err := tx.Exec(ctx, `UPDATE analysis_runs SET state='queued',last_error='',started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2`, owner, runID); err != nil {
		return Handle{}, false, err
	}
	args.RunID = runID
	jobID, err := s.ensureAttemptTx(ctx, tx, args, runID, false)
	if err != nil {
		return Handle{}, false, err
	}
	details, err := json.Marshal(map[string]any{"run_id": runID, "from": state})
	if err != nil {
		return Handle{}, false, fmt.Errorf("encode analysis retry history: %w", err)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details) VALUES($1,'analysis','queued',$2)`, owner, details); err != nil {
		return Handle{}, false, err
	}
	return Handle{ID: id, JobID: jobID, DisplayNumber: display, RunID: runID}, true, nil
}

// createAnalysisRun creates the immutable run for the submission's snapshot,
// enqueues its River job, and records the display handle and first attempt.
func (s *Service) createAnalysisRun(ctx context.Context, tx pgx.Tx, owner string, args JobArgs) (Handle, error) {
	var runID string
	if err := tx.QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,attempt_count) VALUES($1,$2,$3,$4,$5,$6,$7,'queued',1) RETURNING id::text`, owner, args.SourceMaterialID, args.ContentRevisionID, args.SnapshotID, args.AnalyzerName, args.AnalyzerVersion, args.ConfigIdentity).Scan(&runID); err != nil {
		return Handle{}, fmt.Errorf("create analysis run: %w", err)
	}
	args.RunID, args.Attempt = runID, 1
	inserted, err := s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: 1})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue analysis: %w", err)
	}
	var display int64
	if err := tx.QueryRow(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,analysis_run_id,display_number) VALUES($1,$2,$3,$4,$5::uuid,(SELECT COALESCE(MAX(display_number),0)+1 FROM analysis_jobs WHERE owner_id=$2)) RETURNING display_number`, inserted.Job.ID, owner, args.SourceMaterialID, args.ContentHash, runID).Scan(&display); err != nil {
		return Handle{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO analysis_run_attempts(run_id,owner_id,source_material_id,attempt_number,river_job_id,state) VALUES($1,$2,$3,1,$4,'queued')`, runID, owner, args.SourceMaterialID, inserted.Job.ID); err != nil {
		return Handle{}, err
	}
	return Handle{ID: inserted.Job.ID, JobID: inserted.Job.ID, DisplayNumber: display, RunID: runID}, nil
}

func (s *Service) Get(ctx context.Context, owner string, id int64) (Status, error) {
	var status Status
	var runID string
	err := s.pool.QueryRow(ctx, `SELECT river_job_id,display_number,source_material_id::text,progress,error,COALESCE(corpus_id::text,''),created_at,COALESCE(analysis_run_id::text,'') FROM analysis_jobs WHERE owner_id=$1 AND river_job_id=$2`, owner, id).
		Scan(&status.ID, &status.DisplayNumber, &status.SourceMaterialID, &status.Progress, &status.Error, &status.CorpusID, &status.CreatedAt, &runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, ErrNotFound
	}
	if err != nil {
		return Status{}, fmt.Errorf("get analysis handle: %w", err)
	}
	if runID == "" {
		return s.legacyJobStatus(ctx, status, id)
	}
	return s.snapshotJobStatus(ctx, owner, status, runID)
}

// analysisRunState is the durable state of one analysis run, read by status,
// cancellation and retry through loadRunState. It carries the run's state, last
// error, corpus and attempt count, and the latest run attempt when there is one.
type analysisRunState struct {
	State        string
	LastError    string
	CorpusID     string
	AttemptCount int
	Attempt      *analysisRunAttempt
}

type analysisRunAttempt struct {
	Number int
	JobID  int64
	State  string
	Error  string
}

// loadRunState reads the durable state of one analysis run through q. The
// unlocked read projects what status reports and loads the latest attempt,
// which status resolves against the live River job. The locked read takes the
// run row lock used by cancellation and retry and keeps their existing
// single-column statement, so Attempt stays nil.
func (s *Service) loadRunState(ctx context.Context, q analysisQueryer, owner, runID string, lock bool) (analysisRunState, error) {
	var run analysisRunState
	if lock {
		if err := q.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, runID).Scan(&run.State); err != nil {
			return analysisRunState{}, err
		}
		return run, nil
	}
	if err := q.QueryRow(ctx, `SELECT state,last_error,COALESCE(corpus_id::text,''),attempt_count FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner, runID).
		Scan(&run.State, &run.LastError, &run.CorpusID, &run.AttemptCount); err != nil {
		return analysisRunState{}, fmt.Errorf("get snapshot analysis run: %w", err)
	}
	attempt := analysisRunAttempt{}
	err := q.QueryRow(ctx, `SELECT attempt_number,river_job_id,state,error FROM analysis_run_attempts WHERE run_id=$1 ORDER BY attempt_number DESC LIMIT 1`, runID).
		Scan(&attempt.Number, &attempt.JobID, &attempt.State, &attempt.Error)
	if err == nil {
		run.Attempt = &attempt
		return run, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return analysisRunState{}, fmt.Errorf("get analysis attempt: %w", err)
	}
	return run, nil
}

// snapshotJobStatus projects the durable run state of a snapshot-bound analysis
// onto a status.
func (s *Service) snapshotJobStatus(ctx context.Context, owner string, status Status, runID string) (Status, error) {
	run, err := s.loadRunState(ctx, s.pool, owner, runID, false)
	if err != nil {
		return Status{}, err
	}
	status.RunID, status.LogicalState = runID, run.State
	status.State = logicalStateRiverState(run.State)
	status.CorpusID = run.CorpusID
	status.AttemptCount = run.AttemptCount
	status.Attempt = run.AttemptCount
	if run.LastError != "" {
		status.Error = run.LastError
	}
	if run.Attempt == nil {
		return status, nil
	}
	attempt := s.resolveFinalizationAttempt(ctx, run.State, *run.Attempt)
	status.Attempt = attempt.Number
	status.FinalizedAt = nil
	if run.State == "completed" {
		applyFinalizationStatus(&status, attempt)
	}
	if run.State == "completed" || run.State == "failed" || run.State == "cancelled" {
		if err := s.pool.QueryRow(ctx, `SELECT finalized_at FROM analysis_run_attempts WHERE run_id=$1 AND attempt_number=$2`, runID, attempt.Number).Scan(&status.FinalizedAt); err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Status{}, fmt.Errorf("get analysis finalization time: %w", err)
		}
	}
	return status, nil
}

// resolveFinalizationAttempt returns the finalization attempt with its
// effective state: a still-running attempt whose River job River has already
// discarded or cancelled becomes failed and carries River's last error, while a
// live or unreadable River job leaves the durable attempt unchanged.
func (s *Service) resolveFinalizationAttempt(ctx context.Context, runState string, attempt analysisRunAttempt) analysisRunAttempt {
	if runState != "completed" || attempt.State != "running" || s.client == nil {
		return attempt
	}
	row, err := s.client.JobGet(ctx, attempt.JobID)
	if err != nil || (row.State != rivertype.JobStateDiscarded && row.State != rivertype.JobStateCancelled) {
		return attempt
	}
	attempt.State = "failed"
	if len(row.Errors) > 0 {
		attempt.Error = row.Errors[len(row.Errors)-1].Error
	}
	return attempt
}

// applyFinalizationStatus maps a completed run's finalization attempt onto the
// status, keeping the pre-finalization logical states.
func applyFinalizationStatus(status *Status, attempt analysisRunAttempt) {
	switch attempt.State {
	case "queued", "running":
		status.LogicalState = "finalizing"
		status.State = rivertype.JobStateAvailable
		if attempt.State == "running" {
			status.State = rivertype.JobStateRunning
		}
	case "failed":
		status.LogicalState = "finalization-failed"
		status.State = rivertype.JobStateDiscarded
		if attempt.Error != "" {
			status.Error = attempt.Error
		}
	case "cancelled":
		status.LogicalState = "finalization-cancelled"
		status.State = rivertype.JobStateCancelled
	}
}

// legacyJobStatus projects an analysis handle without a snapshot run from the
// live River job.
func (s *Service) legacyJobStatus(ctx context.Context, status Status, id int64) (Status, error) {
	row, err := s.client.JobGet(ctx, id)
	if err != nil {
		return Status{}, fmt.Errorf("get River job: %w", err)
	}
	status.State, status.Attempt, status.FinalizedAt = row.State, row.Attempt, row.FinalizedAt
	if status.Error == "" && len(row.Errors) > 0 {
		status.Error = row.Errors[len(row.Errors)-1].Error
	}
	return status, nil
}

// GetCompletedAnalysis returns a completed snapshot analysis only when every
// identity in the source, scope, run, corpus, artifact, and job chain agrees.
// All absent, foreign-owner, mismatched-book, and non-completed cases return
// ErrNotFound so callers do not disclose which part of the chain differed.
func (s *Service) GetCompletedAnalysis(ctx context.Context, owner, sourceID, runID string) (CompletedAnalysis, error) {
	if s == nil || s.pool == nil || strings.TrimSpace(owner) == "" {
		return CompletedAnalysis{}, ErrNotFound
	}
	if _, err := uuid.Parse(strings.TrimSpace(sourceID)); err != nil {
		return CompletedAnalysis{}, ErrNotFound
	}
	if _, err := uuid.Parse(strings.TrimSpace(runID)); err != nil {
		return CompletedAnalysis{}, ErrNotFound
	}

	var result CompletedAnalysis
	var completedAt *time.Time
	var sourceCreatedAt, corpusCreatedAt, artifactCreatedAt time.Time
	var analyzableTokenCount, distinctLemmaCount, sentenceCount, normalizedTokenCount, emptySentenceCount, p90SentenceTokenCount, longSentenceCount *int64
	var medianSentenceTokenCount *float64
	var artifactHash, artifactLanguage, artifactSchemaVersion, artifactProfile, artifactNormalizationVersion, artifactAnalyzerName, artifactAnalyzerVersion string
	var sourceDigest, sourceRevisionID string
	var sourceDigestVersion int
	var snapshotID sql.NullString

	err := s.pool.QueryRow(ctx, `SELECT r.id::text,r.owner_id::text,r.source_material_id::text,r.snapshot_id::text,
		r.analyzer_name,r.analyzer_version,r.config_identity,r.completed_at,
		j.river_job_id,j.display_number,
		s.language,s.source_identifier,s.title,s.media_type,rev.content_digest,rev.revision_id::text,rev.digest_version,s.created_at,
		c.id::text,c.artifact_hash,c.status,c.analyzable_token_count,c.distinct_lemma_count,c.sentence_count,c.normalized_token_count,c.empty_sentence_count,c.median_sentence_token_count,c.p90_sentence_token_count,c.long_sentence_count,c.created_at,
		a.language,a.schema_version,a.normalization_profile,a.normalization_version,a.analyzer_name,a.analyzer_version,a.created_at
		FROM analysis_runs r
		JOIN analysis_jobs j ON j.owner_id=r.owner_id AND j.analysis_run_id=r.id AND j.source_material_id=r.source_material_id
		JOIN source_materials s ON s.owner_id=r.owner_id AND s.id=r.source_material_id
		JOIN source_content_revisions rev ON rev.owner_id=r.owner_id AND rev.source_material_id=r.source_material_id AND rev.revision_id=r.content_revision_id
		JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id AND c.source_material_id=r.source_material_id AND c.analysis_run_id=r.id AND c.status='complete'
		JOIN normalized_corpus_artifacts a ON a.content_hash=c.artifact_hash
		WHERE r.owner_id=$1 AND r.source_material_id=$2::uuid AND r.id=$3::uuid AND r.state='completed'`, owner, sourceID, runID).
		Scan(&result.RunID, &result.OwnerID, &result.SourceMaterialID, &snapshotID,
			&result.AnalyzerName, &result.AnalyzerVersion, &result.ConfigIdentity, &completedAt,
			&result.JobID, &result.DisplayNumber,
			&result.Source.Language, &result.Source.SourceIdentifier, &result.Source.Title, &result.Source.MediaType, &sourceDigest, &sourceRevisionID, &sourceDigestVersion, &sourceCreatedAt,
			&result.Corpus.ID, &artifactHash, &result.Corpus.Status, &analyzableTokenCount, &distinctLemmaCount, &sentenceCount, &normalizedTokenCount, &emptySentenceCount, &medianSentenceTokenCount, &p90SentenceTokenCount, &longSentenceCount, &corpusCreatedAt,
			&artifactLanguage, &artifactSchemaVersion, &artifactProfile, &artifactNormalizationVersion, &artifactAnalyzerName, &artifactAnalyzerVersion, &artifactCreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return CompletedAnalysis{}, ErrNotFound
	}
	if err != nil {
		return CompletedAnalysis{}, fmt.Errorf("get completed analysis: %w", err)
	}
	result.CompletedAt = completedAt
	result.SnapshotID = snapshotID.String
	result.Source.ID = result.SourceMaterialID
	result.Source.OwnerID = result.OwnerID
	result.Source.ContentHash = sourceDigest
	result.Source.ContentDigest = sourceDigest
	result.Source.ContentRevisionID = sourceRevisionID
	result.Source.ContentDigestVersion = sourceDigestVersion
	result.Source.CreatedAt = sourceCreatedAt
	result.Corpus.OwnerID = result.OwnerID
	result.Corpus.SourceMaterialID = result.SourceMaterialID
	result.Corpus.ArtifactHash = artifactHash
	result.Corpus.AnalysisRunID = result.RunID
	result.Corpus.CreatedAt = corpusCreatedAt
	result.Artifact = domain.NormalizedArtifact{ContentHash: artifactHash, Language: artifactLanguage, SchemaVersion: artifactSchemaVersion, NormalizationProfile: artifactProfile, NormalizationVersion: artifactNormalizationVersion, AnalyzerName: artifactAnalyzerName, AnalyzerVersion: artifactAnalyzerVersion, CreatedAt: artifactCreatedAt}
	if analyzableTokenCount != nil && distinctLemmaCount != nil {
		result.Corpus.Statistics = &domain.AnalysisStatistics{AnalyzableTokenCount: *analyzableTokenCount, DistinctLemmaCount: *distinctLemmaCount}
		if sentenceCount != nil && normalizedTokenCount != nil && emptySentenceCount != nil && medianSentenceTokenCount != nil && p90SentenceTokenCount != nil && longSentenceCount != nil {
			result.Corpus.Statistics.TextProfile = &domain.TextProfile{SentenceCount: *sentenceCount, NormalizedTokenCount: *normalizedTokenCount, EmptySentenceCount: *emptySentenceCount, MedianSentenceTokenCount: *medianSentenceTokenCount, P90SentenceTokenCount: *p90SentenceTokenCount, LongSentenceCount: *longSentenceCount}
		}
	}

	return result, nil
}

func logicalStateRiverState(state string) rivertype.JobState {
	switch state {
	case "queued":
		return rivertype.JobStateAvailable
	case "running":
		return rivertype.JobStateRunning
	case "completed":
		return rivertype.JobStateCompleted
	case "cancelled":
		return rivertype.JobStateCancelled
	case "failed":
		return rivertype.JobStateDiscarded
	default:
		return rivertype.JobStateAvailable
	}
}

func (s *Service) Result(ctx context.Context, owner string, id int64) (domain.Corpus, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return domain.Corpus{}, err
	}
	if status.State != rivertype.JobStateCompleted || status.CorpusID == "" {
		return domain.Corpus{}, fmt.Errorf("analysis job %d has no result", id)
	}
	var corpus domain.Corpus
	var analyzableTokenCount, distinctLemmaCount, sentenceCount, normalizedTokenCount, emptySentenceCount, p90SentenceTokenCount, longSentenceCount *int64
	var medianSentenceTokenCount *float64
	err = s.pool.QueryRow(ctx, `SELECT id,owner_id,source_material_id,artifact_hash,COALESCE(analysis_run_id::text,''),status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count,created_at FROM corpora WHERE owner_id=$1 AND id=$2`, owner, status.CorpusID).
		Scan(&corpus.ID, &corpus.OwnerID, &corpus.SourceMaterialID, &corpus.ArtifactHash, &corpus.AnalysisRunID, &corpus.Status, &analyzableTokenCount, &distinctLemmaCount, &sentenceCount, &normalizedTokenCount, &emptySentenceCount, &medianSentenceTokenCount, &p90SentenceTokenCount, &longSentenceCount, &corpus.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Corpus{}, ErrNotFound
	}
	if err == nil && analyzableTokenCount != nil && distinctLemmaCount != nil {
		corpus.Statistics = &domain.AnalysisStatistics{AnalyzableTokenCount: *analyzableTokenCount, DistinctLemmaCount: *distinctLemmaCount}
		if sentenceCount != nil && normalizedTokenCount != nil && emptySentenceCount != nil && medianSentenceTokenCount != nil && p90SentenceTokenCount != nil && longSentenceCount != nil {
			corpus.Statistics.TextProfile = &domain.TextProfile{SentenceCount: *sentenceCount, NormalizedTokenCount: *normalizedTokenCount, EmptySentenceCount: *emptySentenceCount, MedianSentenceTokenCount: *medianSentenceTokenCount, P90SentenceTokenCount: *p90SentenceTokenCount, LongSentenceCount: *longSentenceCount}
		}
	}
	return corpus, err
}

func (s *Service) Cancel(ctx context.Context, owner string, id int64) (Status, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return Status{}, err
	}
	if status.RunID == "" {
		return s.cancelLegacyJob(ctx, owner, id)
	}
	return s.cancelSnapshotRun(ctx, owner, id, status)
}

// cancelLegacyJob cancels a handle without a snapshot run through River.
func (s *Service) cancelLegacyJob(ctx context.Context, owner string, id int64) (Status, error) {
	if _, err := s.client.JobCancel(ctx, id); err != nil {
		return Status{}, fmt.Errorf("cancel analysis job: %w", err)
	}
	return s.Get(ctx, owner, id)
}

// cancelSnapshotRun cancels the snapshot run behind a handle. The advisory lock
// and the run row lock serialize cancellation with retry and publication.
func (s *Service) cancelSnapshotRun(ctx context.Context, owner string, id int64, status Status) (result Status, err error) {
	tx, txErr := s.pool.Begin(ctx)
	if txErr != nil {
		return Status{}, txErr
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, txErr = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner); txErr != nil {
		return Status{}, fmt.Errorf("lock analysis cancellation: %w", txErr)
	}
	run, txErr := s.loadRunState(ctx, tx, owner, status.RunID, true)
	if txErr != nil {
		return Status{}, txErr
	}
	if run.State == "completed" {
		return s.cancelCompletedFinalization(ctx, tx, owner, id, status)
	}
	return s.cancelActiveRun(ctx, tx, owner, id, status)
}

// cancelCompletedFinalization cancels the live finalization attempt of a
// completed run, leaving the immutable corpus in place.
func (s *Service) cancelCompletedFinalization(ctx context.Context, tx pgx.Tx, owner string, id int64, status Status) (Status, error) {
	var riverJobID int64
	txErr := tx.QueryRow(ctx, `SELECT river_job_id FROM analysis_run_attempts WHERE run_id=$1 AND state IN ('queued','running') ORDER BY attempt_number DESC LIMIT 1 FOR UPDATE`, status.RunID).Scan(&riverJobID)
	if errors.Is(txErr, pgx.ErrNoRows) {
		if txErr := tx.Commit(ctx); txErr != nil {
			return Status{}, txErr
		}
		return s.Get(ctx, owner, id)
	}
	if txErr != nil {
		return Status{}, fmt.Errorf("find live finalization attempt: %w", txErr)
	}
	if _, txErr = tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='cancelled',error='',finalized_at=now() WHERE run_id=$1 AND river_job_id=$2 AND state IN ('queued','running')`, status.RunID, riverJobID); txErr != nil {
		return Status{}, txErr
	}
	if txErr := tx.Commit(ctx); txErr != nil {
		return Status{}, txErr
	}
	if s.client != nil {
		if _, cancelErr := s.client.JobCancel(ctx, riverJobID); cancelErr != nil {
			log.Printf("analysis finalization cancellation cleanup failed: owner=%s run_id=%s job_id=%d: %v", owner, status.RunID, riverJobID, cancelErr)
		}
	}
	return s.Get(ctx, owner, id)
}

// cancelActiveRun cancels a queued or running run and its live attempts, then
// clears the River work best-effort after the durable cancellation commits.
func (s *Service) cancelActiveRun(ctx context.Context, tx pgx.Tx, owner string, id int64, status Status) (Status, error) {
	updated, txErr := tx.Exec(ctx, `UPDATE analysis_runs SET state='cancelled',last_error='',updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2 AND state IN ('queued','running')`, owner, status.RunID)
	if txErr != nil {
		return Status{}, txErr
	}
	if updated.RowsAffected() == 0 {
		if txErr := tx.Commit(ctx); txErr != nil {
			return Status{}, txErr
		}
		return s.Get(ctx, owner, id)
	}
	if _, txErr = tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='cancelled',finalized_at=now() WHERE run_id=$1 AND state IN ('queued','running')`, status.RunID); txErr != nil {
		return Status{}, txErr
	}
	if txErr := tx.Commit(ctx); txErr != nil {
		return Status{}, txErr
	}
	// Durable cancellation commits before River cleanup. The cancelled run
	// and attempt fence any worker that races with best-effort cleanup.
	if cleanupErr := cleanupCancelledAnalysis(ctx, owner, status.RunID, s.findLiveJob, s.client.JobCancel); cleanupErr != nil {
		log.Printf("analysis cancellation cleanup failed: owner=%s run_id=%s: %v", owner, status.RunID, cleanupErr)
	}
	status.State = rivertype.JobStateCancelled
	status.LogicalState = "cancelled"
	status.Error = ""
	return status, nil
}

func cleanupCancelledAnalysis(ctx context.Context, owner, runID string, findLiveJob func(context.Context, string, string) (int64, error), cancelJob func(context.Context, int64) (*rivertype.JobRow, error)) error {
	jobID, err := findLiveJob(ctx, owner, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find analysis job to cancel: %w", err)
	}
	if _, err = cancelJob(ctx, jobID); err != nil {
		return fmt.Errorf("cancel analysis job: %w", err)
	}
	return nil
}

// Retry starts a new execution attempt for a failed or cancelled snapshot run.
// The logical run identity and status URL remain stable across attempts.
func (s *Service) Retry(ctx context.Context, owner string, id int64) (result Handle, err error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return Handle{}, err
	}
	if status.RunID == "" {
		return Handle{}, errors.New("legacy analysis jobs cannot be retried through the snapshot lifecycle")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner); err != nil {
		return Handle{}, fmt.Errorf("lock analysis retry: %w", err)
	}
	run, err := s.loadRunState(ctx, tx, owner, status.RunID, true)
	if err != nil {
		return Handle{}, err
	}
	state := run.State
	if state == "queued" || state == "running" {
		handle, orphaned, pendingErr := s.retryPendingRun(ctx, tx, owner, id, status, state)
		if pendingErr != nil {
			return Handle{}, pendingErr
		}
		if !orphaned {
			return handle, nil
		}
		state = "failed"
	}
	if state == "completed" {
		return s.retryCompletedRun(ctx, tx, owner, id, status)
	}
	if state != "failed" && state != "cancelled" {
		return Handle{}, fmt.Errorf("analysis run is not retryable from %s", state)
	}
	return s.requeueRun(ctx, tx, owner, id, status, state)
}

// retryPendingRun resumes a queued run, or a running run that still has live
// River work. It reports orphaned=true when a running run's worker is gone and
// the run was failed, so the caller requeues it from the failed state.
func (s *Service) retryPendingRun(ctx context.Context, tx pgx.Tx, owner string, id int64, status Status, state string) (Handle, bool, error) {
	jobID, jobErr := s.liveJobIDTx(ctx, tx, owner, status.RunID)
	if jobErr == nil {
		if err := tx.Commit(ctx); err != nil {
			return Handle{}, false, err
		}
		return Handle{ID: id, JobID: jobID, DisplayNumber: status.DisplayNumber, RunID: status.RunID}, false, nil
	}
	if !errors.Is(jobErr, pgx.ErrNoRows) {
		return Handle{}, false, jobErr
	}
	if state == "running" {
		_, err := tx.Exec(ctx, `UPDATE analysis_runs SET state='failed',last_error=$3,updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2`, owner, status.RunID, safeAnalysisError(errors.New("analysis worker is no longer active")))
		if err != nil {
			return Handle{}, false, err
		}
		return Handle{}, true, nil
	}
	args, err := snapshotArgsTx(ctx, tx, owner, status.RunID)
	if err != nil {
		return Handle{}, false, err
	}
	jobID, err = s.ensureAttemptTx(ctx, tx, args, status.RunID, false)
	if err != nil {
		return Handle{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, false, err
	}
	return Handle{ID: id, JobID: jobID, DisplayNumber: status.DisplayNumber, RunID: status.RunID}, false, nil
}

// retryCompletedRun replays the publication of a completed run whose corpus is
// still the current evidence, or reports that no retry is needed.
func (s *Service) retryCompletedRun(ctx context.Context, tx pgx.Tx, owner string, id int64, status Status) (Handle, error) {
	args, err := snapshotArgsTx(ctx, tx, owner, status.RunID)
	if err != nil {
		return Handle{}, err
	}
	retry, shouldRetry, err := s.retryCompletedPublicationTx(ctx, tx, args, owner, status.RunID, id, status.DisplayNumber)
	if err != nil {
		return Handle{}, err
	}
	if !shouldRetry {
		return Handle{}, errors.New("analysis publication does not need retry")
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return retry, nil
}

// requeueRun resets a failed or cancelled run to queued, enqueues a fresh
// attempt, and records the retry in operational history.
func (s *Service) requeueRun(ctx context.Context, tx pgx.Tx, owner string, id int64, status Status, state string) (Handle, error) {
	if _, err := tx.Exec(ctx, `UPDATE analysis_runs SET state='queued',last_error='',started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2`, owner, status.RunID); err != nil {
		return Handle{}, err
	}
	args, err := snapshotArgsTx(ctx, tx, owner, status.RunID)
	if err != nil {
		return Handle{}, err
	}
	jobID, err := s.ensureAttemptTx(ctx, tx, args, status.RunID, false)
	if err != nil {
		return Handle{}, err
	}
	details, marshalErr := json.Marshal(map[string]any{"run_id": status.RunID, "from": state})
	if marshalErr != nil {
		return Handle{}, fmt.Errorf("encode analysis retry history: %w", marshalErr)
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details) VALUES($1,'analysis','queued',$2)`, owner, details); err != nil {
		return Handle{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{ID: id, JobID: jobID, DisplayNumber: status.DisplayNumber, RunID: status.RunID}, nil
}

// Reconcile makes an orphaned queued/running run actionable. A queued run is
// re-enqueued; a running run without viable River work is failed safely.
func (s *Service) Reconcile(ctx context.Context, owner string, id int64) (result Status, err error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil || status.RunID == "" {
		return status, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Status{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	run, err := s.loadRunState(ctx, tx, owner, status.RunID, true)
	if err != nil {
		return Status{}, err
	}
	switch run.State {
	case "queued":
		err = s.reconcileQueuedRun(ctx, tx, owner, status.RunID)
	case "running":
		err = s.reconcileRunningRun(ctx, tx, owner, status.RunID)
	}
	if err != nil {
		return Status{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Status{}, err
	}
	return s.Get(ctx, owner, id)
}

// reconcileQueuedRun re-enqueues a queued run only when it has no live River
// work, loading its arguments for the fresh attempt.
func (s *Service) reconcileQueuedRun(ctx context.Context, tx pgx.Tx, owner, runID string) error {
	if _, err := s.liveJobIDTx(ctx, tx, owner, runID); !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	args, err := snapshotArgsTx(ctx, tx, owner, runID)
	if err != nil {
		return err
	}
	_, err = s.ensureAttemptTx(ctx, tx, args, runID, false)
	return err
}

// reconcileRunningRun fails a running run whose River work is gone. It reports
// the attempt-update failure directly, while a run-update failure is left for
// the caller's commit to report, matching the inline reconcile flow.
func (s *Service) reconcileRunningRun(ctx context.Context, tx pgx.Tx, owner, runID string) error {
	if _, err := s.liveJobIDTx(ctx, tx, owner, runID); !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := failOrphanedRunningRun(ctx, tx, owner, runID); err != nil && !errors.Is(err, errOrphanedRunUpdateDeferred) {
		return err
	}
	return nil
}

// errOrphanedRunUpdateDeferred marks a failed running-run update whose aborted
// transaction the caller's commit reports. It is returned instead of the run
// update error because the inline reconcile flow discarded that error and let
// the commit surface the failure.
var errOrphanedRunUpdateDeferred = errors.New("running run update deferred to commit")

// failOrphanedRunningRun marks a running run without viable River work as
// failed and fails its open attempts.
func failOrphanedRunningRun(ctx context.Context, tx pgx.Tx, owner, runID string) error {
	message := safeAnalysisError(errors.New("analysis worker is no longer active"))
	if _, err := tx.Exec(ctx, `UPDATE analysis_runs SET state='failed',last_error=$3,updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2`, owner, runID, message); err != nil {
		return errOrphanedRunUpdateDeferred
	}
	_, err := tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='failed',error=$2,finalized_at=now() WHERE run_id=$1 AND state IN ('queued','running')`, runID, message)
	return err
}

func (s *Service) liveJobID(ctx context.Context, owner, runID string) (int64, error) {
	return s.liveJobIDTx(ctx, s.pool, owner, runID)
}

type analysisQueryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) liveJobIDTx(ctx context.Context, q analysisQueryer, owner, runID string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'run_id'=$3 AND state IN ('available','pending','running','retryable','scheduled') ORDER BY id DESC LIMIT 1`, (JobArgs{}).Kind(), owner, runID).Scan(&id)
	return id, err
}

func snapshotArgsTx(ctx context.Context, q analysisQueryer, owner, runID string) (JobArgs, error) {
	var args JobArgs
	err := q.QueryRow(ctx, `SELECT r.id::text,r.attempt_count,r.owner_id::text,r.source_material_id::text,
		s.language,s.source_identifier,s.title,rev.content_digest,rev.revision_id::text,r.snapshot_id::text,r.config_identity
		FROM analysis_runs r JOIN source_materials s ON s.owner_id=r.owner_id AND s.id=r.source_material_id
		JOIN source_content_revisions rev ON rev.owner_id=r.owner_id AND rev.source_material_id=r.source_material_id AND rev.revision_id=r.content_revision_id
		WHERE r.owner_id=$1 AND r.id=$2`, owner, runID).Scan(&args.RunID, &args.Attempt, &args.OwnerID, &args.SourceMaterialID, &args.Language, &args.SourceIdentifier, &args.Title, &args.ContentHash, &args.ContentRevisionID, &args.SnapshotID, &args.ConfigIdentity)
	args.AnalyzerName, args.AnalyzerVersion = snapshotAnalyzerName, snapshotAnalyzerVersion
	if args.ConfigIdentity == "" {
		args.ConfigIdentity = snapshotConfigIdentity
	}
	return args, err
}

func (s *Service) Wait(ctx context.Context, owner string, id int64) (Status, error) {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		status, err := s.Get(ctx, owner, id)
		if err != nil {
			return Status{}, err
		}
		switch status.State {
		case rivertype.JobStateCompleted, rivertype.JobStateCancelled, rivertype.JobStateDiscarded:
			return status, nil
		default:
			// River's JobState is an open upstream enumeration; unknown states remain pending until context cancellation.
		}
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

type Worker struct {
	river.WorkerDefaults[JobArgs]
	Pool          *pgxpool.Pool
	Analyzer      analyzer.Analyzer
	Capabilities  analyzer.CapabilityProvider
	Selection     *selection.Service
	MaxChunkChars int
}

type snapshotUnit struct {
	SnapshotID, UnitID, ManifestID, Title, SourceHref, ResolvedHref, Text string
	Order                                                                 int
	StartOffset, EndOffset                                                uint64
	LandmarkTypes                                                         []string
}

type queryRower interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

type snapshotUnitQueryer interface {
	queryRower
	QueryRow(context.Context, string, ...any) pgx.Row
}

func loadSnapshotUnits(ctx context.Context, q queryRower, owner, sourceID, revisionID, snapshotID string) ([]snapshotUnit, error) {
	return loadSnapshotUnitsWithText(ctx, q, owner, sourceID, revisionID, snapshotID, true)
}

// loadSnapshotUnitsForSelection loads the units the selection rules need. The
// text scan is skipped for snapshots without a pg-header unit, because only a
// Project Gutenberg snapshot can recognise Distributor text (ADR 0108).
func loadSnapshotUnitsForSelection(ctx context.Context, q snapshotUnitQueryer, owner, sourceID, revisionID, snapshotID string) ([]snapshotUnit, error) {
	hasHeader, err := snapshotHasGutenbergHeader(ctx, q, owner, sourceID, revisionID, snapshotID)
	if err != nil {
		return nil, err
	}
	return loadSnapshotUnitsWithText(ctx, q, owner, sourceID, revisionID, snapshotID, hasHeader)
}

const snapshotUnitsFrom = `FROM source_material_units u
	JOIN source_material_unit_snapshots snap ON snap.owner_id=u.owner_id AND snap.source_material_id=u.source_material_id AND snap.snapshot_id=u.snapshot_id AND snap.content_revision_id=$4
	WHERE u.owner_id=$1 AND u.source_material_id=$2 AND u.snapshot_id=$3 AND btrim(u.text)<>''`

func snapshotHasGutenbergHeader(ctx context.Context, q snapshotUnitQueryer, owner, sourceID, revisionID, snapshotID string) (bool, error) {
	var has bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 `+snapshotUnitsFrom+` AND u.manifest_id='pg-header')`, owner, sourceID, snapshotID, revisionID).Scan(&has); err != nil {
		return false, fmt.Errorf("find Gutenberg header unit: %w", err)
	}
	return has, nil
}

func loadSnapshotUnitsWithText(ctx context.Context, q queryRower, owner, sourceID, revisionID, snapshotID string, includeText bool) ([]snapshotUnit, error) {
	text := "''"
	if includeText {
		text = "u.text"
	}
	rows, err := q.Query(ctx, `SELECT u.snapshot_id,u.unit_id,u.unit_order,u.manifest_id,u.title,u.source_href,u.resolved_href,`+text+`,u.start_offset,u.end_offset,u.landmark_types `+snapshotUnitsFrom+`
		ORDER BY u.unit_order`, owner, sourceID, snapshotID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("load extracted snapshot: %w", err)
	}
	defer rows.Close()
	var units []snapshotUnit
	lastOrder := -1
	for rows.Next() {
		var unit snapshotUnit
		var landmarkTypes []byte
		if err = rows.Scan(&unit.SnapshotID, &unit.UnitID, &unit.Order, &unit.ManifestID, &unit.Title, &unit.SourceHref, &unit.ResolvedHref, &unit.Text, &unit.StartOffset, &unit.EndOffset, &landmarkTypes); err != nil {
			return nil, fmt.Errorf("load extracted snapshot unit: %w", err)
		}
		if err = json.Unmarshal(landmarkTypes, &unit.LandmarkTypes); err != nil {
			return nil, fmt.Errorf("load extracted snapshot landmark provenance: %w", err)
		}
		if unit.UnitID == "" || unit.Order < 0 || unit.Order <= lastOrder || unit.EndOffset < unit.StartOffset {
			return nil, errors.New("extracted snapshot is invalid")
		}
		lastOrder = unit.Order
		units = append(units, unit)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("load extracted snapshot units: %w", err)
	}
	if len(units) == 0 {
		return nil, domain.ErrExtractedUnitsUnavailable
	}
	return units, nil
}

func identifyMainText(units []snapshotUnit) epub.MainTextSelection {
	provenance := make([]epub.ExtractedUnit, len(units))
	for i, unit := range units {
		order, err := checked.Uint64FromInt(unit.Order)
		if err != nil {
			return epub.MainTextSelection{}
		}
		provenance[i] = epub.ExtractedUnit{ID: unit.UnitID, Order: order, ManifestID: unit.ManifestID, Text: unit.Text, LandmarkTypes: unit.LandmarkTypes}
	}
	return epub.IdentifyMainText(provenance)
}

func snapshotConfigIdentityFor(decision epub.MainTextSelection) string {
	switch {
	case decision.Distributor.Excludes() && decision.LandmarkApplies:
		return mainTextDistributorConfigIdentity
	case decision.Distributor.Excludes():
		return distributorConfigIdentity
	case decision.LandmarkApplies:
		return mainTextConfigIdentity
	default:
		return snapshotConfigIdentity
	}
}

// scopeSnapshotUnits applies the selection a run's configuration identity
// names. The identity is stored with the run, so an older run keeps the scope it
// was submitted under even if the current selection would differ.
func scopeSnapshotUnits(units []snapshotUnit, decision epub.MainTextSelection, configIdentity string) ([]snapshotUnit, []epub.UnitTextRange) {
	switch configIdentity {
	case mainTextConfigIdentity:
		if !decision.Identified {
			return units, nil
		}
		return append([]snapshotUnit(nil), units[decision.BodyMatterStart:decision.BackMatterStart]...), nil
	case distributorConfigIdentity:
		return selectedSnapshotUnits(units, unitsOutsideDistributor(units, decision)), decision.Distributor.TrimmedRanges
	case mainTextDistributorConfigIdentity:
		return selectedSnapshotUnits(units, decision.SelectedUnitIDs), decision.Distributor.TrimmedRanges
	default:
		return units, nil
	}
}

// unitsOutsideDistributor returns every unit id except the whole-unit
// Distributor exclusions, so a distributor-only run is independent of landmark
// selection.
func unitsOutsideDistributor(units []snapshotUnit, decision epub.MainTextSelection) []string {
	excluded := make(map[string]struct{}, len(decision.Distributor.ExcludedUnitIDs))
	for _, id := range decision.Distributor.ExcludedUnitIDs {
		excluded[id] = struct{}{}
	}
	ids := make([]string, 0, len(units))
	for _, unit := range units {
		if _, ok := excluded[unit.UnitID]; !ok {
			ids = append(ids, unit.UnitID)
		}
	}
	return ids
}

func selectedSnapshotUnits(units []snapshotUnit, selectedIDs []string) []snapshotUnit {
	selected := make(map[string]struct{}, len(selectedIDs))
	for _, id := range selectedIDs {
		selected[id] = struct{}{}
	}
	out := make([]snapshotUnit, 0, len(selectedIDs))
	for _, unit := range units {
		if _, ok := selected[unit.UnitID]; ok {
			out = append(out, unit)
		}
	}
	return out
}

func analysisHistoryDetails(args JobArgs, decision epub.MainTextSelection, total, selected int) ([]byte, error) {
	details, err := json.Marshal(map[string]any{
		"run_id":               args.RunID,
		"attempt":              args.Attempt,
		"selection_algorithm":  args.ConfigIdentity,
		"selection_identified": decision.Identified,
		"selection_applied":    decision.Applies,
		"body_matter_start":    decision.BodyMatterStart,
		"back_matter_start":    decision.BackMatterStart,
		"excluded_unit_ids":    decision.ExcludedUnitIDs,
		"selected_unit_count":  selected,
		"total_unit_count":     total,

		"distributor_recognised":        decision.Distributor.Recognised,
		"distributor_convention":        decision.Distributor.Convention,
		"distributor_trimmed_ranges":    decision.Distributor.TrimmedRanges,
		"distributor_excluded_unit_ids": decision.Distributor.ExcludedUnitIDs,
	})
	return details, err
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) (workErr error) {
	return w.workSnapshot(ctx, job)
}

// snapshotDocumentInput is one selected snapshot unit as submitted to the
// analyzer, carrying the provenance the analyzer echoes back.
type snapshotDocumentInput struct {
	id, identifier, title, text string
}

// snapshotPlan is the fetched and selected snapshot work the analyzer runs on.
type snapshotPlan struct {
	decision      epub.MainTextSelection
	trimmedText   []epub.UnitTextRange
	structure     domain.SnapshotStructure
	totalUnits    int
	selectedUnits int
	inputs        []snapshotDocumentInput
}

// snapshotAnalysis is the analyzed result plus the derived persistence inputs.
type snapshotAnalysis struct {
	result     analyzer.Result
	lemmas     []domain.SharedLemma
	statistics domain.AnalysisStatistics
	artifact   domain.NormalizedArtifact
}

// beginSnapshotRun marks the run and its attempt running and reports the run
// state so the caller can skip a run cancelled before it started.
func (w *Worker) beginSnapshotRun(ctx context.Context, a JobArgs, riverJobID int64) (string, error) {
	if _, err := w.Pool.Exec(ctx, `UPDATE analysis_runs SET state='running',started_at=COALESCE(started_at,now()),updated_at=now() WHERE owner_id=$1 AND id=$2 AND state='queued'`, a.OwnerID, a.RunID); err != nil {
		return "", err
	}
	if _, err := w.Pool.Exec(ctx, `UPDATE analysis_run_attempts SET state='running',started_at=COALESCE(started_at,now()) WHERE run_id=$1 AND river_job_id=$2 AND state='queued'`, a.RunID, riverJobID); err != nil {
		return "", err
	}
	var runState string
	if err := w.Pool.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, a.OwnerID, a.RunID).Scan(&runState); err != nil {
		return "", err
	}
	return runState, nil
}

// recordSnapshotFailure records one failed snapshot run across the run, its
// attempt, the analysis job, and processing history. Any recording failure is
// joined onto the original work error.
func (w *Worker) recordSnapshotFailure(ctx context.Context, workErr error, a JobArgs, riverJobID int64) error {
	failureCtx := context.WithoutCancel(ctx)
	message := safeAnalysisError(workErr)
	recordFailure := func(operation string, recordErr error) {
		if recordErr != nil {
			workErr = errors.Join(workErr, fmt.Errorf("record analysis failure in %s: %w", operation, recordErr))
		}
	}
	_, recordErr := w.Pool.Exec(failureCtx, `UPDATE analysis_runs SET state=CASE WHEN state='cancelled' THEN state ELSE 'failed' END,last_error=CASE WHEN state='cancelled' THEN last_error ELSE $3 END,updated_at=now(),completed_at=CASE WHEN state='cancelled' THEN completed_at ELSE now() END WHERE owner_id=$1 AND id=$2 AND state IN ('queued','running')`, a.OwnerID, a.RunID, message)
	recordFailure("analysis run", recordErr)
	_, recordErr = w.Pool.Exec(failureCtx, `UPDATE analysis_run_attempts SET state=CASE WHEN state='cancelled' THEN state ELSE 'failed' END,error=CASE WHEN state='cancelled' THEN error ELSE $3 END,finalized_at=now() WHERE run_id=$1 AND river_job_id=$2 AND state IN ('queued','running','completed')`, a.RunID, riverJobID, message)
	recordFailure("analysis attempt", recordErr)
	_, recordErr = w.Pool.Exec(failureCtx, `UPDATE analysis_jobs SET error=$3,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$2`, a.OwnerID, a.RunID, message)
	recordFailure("analysis job", recordErr)
	details, marshalErr := json.Marshal(map[string]any{"run_id": a.RunID, "attempt": a.Attempt, "error": message})
	if marshalErr != nil {
		recordFailure("analysis failure history encoding", marshalErr)
		return workErr
	}
	_, recordErr = w.Pool.Exec(failureCtx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'analysis','failed',$2,now())`, a.OwnerID, details)
	recordFailure("analysis failure history", recordErr)
	return workErr
}

// fetchSnapshotWork validates that the run still points at the current source
// revision, then loads and selects the units the analyzer runs on.
func (w *Worker) fetchSnapshotWork(ctx context.Context, a JobArgs) (snapshotPlan, error) {
	var valid bool
	if err := w.Pool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM analysis_runs r JOIN source_materials s ON s.owner_id=r.owner_id AND s.id=r.source_material_id
			JOIN source_content_revisions rev ON rev.owner_id=r.owner_id AND rev.source_material_id=r.source_material_id AND rev.revision_id=r.content_revision_id
			WHERE r.id=$1 AND r.owner_id=$2 AND r.source_material_id=$3 AND r.content_revision_id=$4 AND r.snapshot_id=$5
			  AND s.current_content_revision_id=r.content_revision_id AND s.current_snapshot_id=r.snapshot_id AND rev.content_digest=$6
		)`, a.RunID, a.OwnerID, a.SourceMaterialID, a.ContentRevisionID, a.SnapshotID, a.ContentHash).Scan(&valid); err != nil {
		return snapshotPlan{}, err
	}
	if !valid {
		return snapshotPlan{}, errors.New("the source content revision is no longer current")
	}
	if err := requireDependencyParsing(ctx, w.Capabilities, a.Language); err != nil {
		return snapshotPlan{}, err
	}
	allUnits, err := loadSnapshotUnits(ctx, w.Pool, a.OwnerID, a.SourceMaterialID, a.ContentRevisionID, a.SnapshotID)
	if err != nil {
		return snapshotPlan{}, err
	}
	structure, err := persistence.LoadSnapshotStructure(ctx, w.Pool, a.OwnerID, a.SourceMaterialID, a.SnapshotID)
	if err != nil {
		return snapshotPlan{}, err
	}
	decision := identifyMainText(allUnits)
	selectedUnits, trimmedText := scopeSnapshotUnits(allUnits, decision, a.ConfigIdentity)
	inputs := make([]snapshotDocumentInput, 0, len(selectedUnits))
	for _, unit := range selectedUnits {
		identifier := unit.ResolvedHref
		if identifier == "" {
			identifier = unit.SourceHref
		}
		inputs = append(inputs, snapshotDocumentInput{id: unit.UnitID, identifier: identifier, title: unit.Title, text: unit.Text})
	}
	return snapshotPlan{decision: decision, trimmedText: trimmedText, structure: structure, totalUnits: len(allUnits), selectedUnits: len(selectedUnits), inputs: inputs}, nil
}

// snapshotChunkMerge folds analyzed chunks into the snapshot's merged result
// and tracks batch progress.
type snapshotChunkMerge struct {
	result    analyzer.Result
	total     int
	completed int
}

// merge folds one analyzed chunk into the merged result. chunkIndex is the
// chunk's index within its document, so only the first chunk contributes source
// documents.
func (m *snapshotChunkMerge) merge(chunkIndex int, chunk analyzer.Result) {
	if m.completed == 0 {
		m.result = chunk
		m.result.Sentences = nil
		m.result.SourceDocuments = nil
	}
	m.result.Sentences = append(m.result.Sentences, chunk.Sentences...)
	if chunkIndex == 0 {
		m.result.SourceDocuments = append(m.result.SourceDocuments, chunk.SourceDocuments...)
	}
	m.completed++
}

// progress reports the analysis progress once the latest chunk completed.
func (m *snapshotChunkMerge) progress() int {
	return 10 + m.completed*80/m.total
}

// analysis derives the artifact identity, shared lemmas, and statistics from
// the merged result.
func (m *snapshotChunkMerge) analysis(a JobArgs, structure domain.SnapshotStructure) (snapshotAnalysis, error) {
	artifactHash := normalizedArtifactHash(a.ContentHash, "", a.AnalysisIdentity, a.AnalyzerName, a.AnalyzerVersion, a.ConfigIdentity, m.result)
	lemmas, err := aggregateLemmas(artifactHash, m.result)
	if err != nil {
		return snapshotAnalysis{}, fmt.Errorf("aggregate analyzed lemmas: %w", err)
	}
	cfg := selection.DefaultConfig("")
	cfg.Structure = structure
	return snapshotAnalysis{
		result:     m.result,
		lemmas:     lemmas,
		statistics: selection.AnalyzableStatistics(m.result, cfg),
		artifact:   domain.NormalizedArtifact{ContentHash: artifactHash, Language: m.result.Language, SchemaVersion: m.result.SchemaVersion, NormalizationProfile: m.result.NormalizationProfile.Name, NormalizationVersion: m.result.NormalizationProfile.Version, AnalyzerName: m.result.Analysis.AnalyzerName, AnalyzerVersion: m.result.Analysis.AnalyzerVersion},
	}, nil
}

// analyzeSnapshotWork runs the analyzer over every chunk and derives the
// artifact identity, shared lemmas, and statistics from the merged result.
func (w *Worker) analyzeSnapshotWork(ctx context.Context, a JobArgs, plan snapshotPlan) (snapshotAnalysis, error) {
	var merge snapshotChunkMerge
	for _, input := range plan.inputs {
		merge.total += len(chunkText(input.text, w.MaxChunkChars))
	}
	for _, input := range plan.inputs {
		if err := w.analyzeDocumentChunks(ctx, a, input, &merge); err != nil {
			return snapshotAnalysis{}, err
		}
	}
	dropDistributorSentences(&merge.result, plan.trimmedText)
	return merge.analysis(a, plan.structure)
}

// dropDistributorSentences removes sentences overlapping recognised Distributor
// text before tokens, lemmas, statistics, and the artifact are derived. Unit
// text and snapshot offsets are untouched, so retained source locations are
// identical to a whole-snapshot analysis (ADR 0108).
func dropDistributorSentences(result *analyzer.Result, trimmedText []epub.UnitTextRange) {
	if len(trimmedText) == 0 {
		return
	}
	byUnit := make(map[string][]epub.UnitTextRange, len(trimmedText))
	for _, trimmed := range trimmedText {
		byUnit[trimmed.UnitID] = append(byUnit[trimmed.UnitID], trimmed)
	}
	kept := result.Sentences[:0]
	for _, sentence := range result.Sentences {
		if overlapsDistributorText(sentence.Location, byUnit[sentence.Location.SourceDocumentID]) {
			continue
		}
		kept = append(kept, sentence)
	}
	result.Sentences = kept
}

func overlapsDistributorText(location analyzer.SourceLocation, trimmed []epub.UnitTextRange) bool {
	for _, span := range trimmed {
		if location.StartOffset < span.EndOffset && location.EndOffset > span.StartOffset {
			return true
		}
	}
	return false
}

// analyzeDocumentChunks analyzes and folds every chunk of one snapshot document,
// recording progress after each chunk.
func (w *Worker) analyzeDocumentChunks(ctx context.Context, a JobArgs, input snapshotDocumentInput, merge *snapshotChunkMerge) error {
	chunks := chunkText(input.text, w.MaxChunkChars)
	var chunkStartOffset uint64
	for i, chunk := range chunks {
		chunkResult, err := w.Analyzer.Analyze(ctx, analyzer.AnalyzeRequest{Language: a.Language, Document: analyzer.SourceDocument{ID: input.id, SourceIdentifier: input.identifier, Title: input.title, Text: chunk}})
		if err != nil {
			return fmt.Errorf("snapshot analysis failed: %w", err)
		}
		offsetResultLocations(&chunkResult, chunkStartOffset)
		merge.merge(i, chunkResult)
		chunkStartOffset += uint64(len([]rune(chunk)))
		if _, err := w.Pool.Exec(ctx, `UPDATE analysis_jobs SET progress=$2,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$3`, a.OwnerID, merge.progress(), a.RunID); err != nil {
			return err
		}
	}
	return nil
}

// persistSnapshotWork writes the analyzed corpus in one transaction. proceed is
// false when the run was cancelled or completed concurrently, so the caller
// must not finalize it.
func (w *Worker) persistSnapshotWork(ctx context.Context, a JobArgs, plan snapshotPlan, analyzed snapshotAnalysis) (corpusID string, proceed bool, err error) {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return "", false, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	var runState string
	if err := tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2 FOR UPDATE`, a.OwnerID, a.RunID).Scan(&runState); err != nil {
		return "", false, err
	}
	if runState == "cancelled" || runState == "completed" {
		return "", false, nil
	}
	if err := insertCorpusArtifact(ctx, tx, analyzed.artifact); err != nil {
		return "", false, err
	}
	if err := insertSharedLemmas(ctx, tx, analyzed.lemmas); err != nil {
		return "", false, err
	}
	corpusID, err = insertCompletedCorpus(ctx, tx, a, analyzed)
	if err != nil {
		return "", false, err
	}
	if err := persistNormalizedCorpus(ctx, tx, a.OwnerID, analyzed.result.Language, a.RunID, corpusID, analyzed.result); err != nil {
		return "", false, err
	}
	if err := completeSnapshotRun(ctx, tx, a, plan, corpusID); err != nil {
		return "", false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", false, err
	}
	return corpusID, true, nil
}

// insertCorpusArtifact records the normalized artifact identity, ignoring an
// artifact that already has the same content hash.
func insertCorpusArtifact(ctx context.Context, tx pgx.Tx, artifact domain.NormalizedArtifact) error {
	_, err := tx.Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(content_hash) DO NOTHING`, artifact.ContentHash, artifact.Language, artifact.SchemaVersion, artifact.NormalizationProfile, artifact.NormalizationVersion, artifact.AnalyzerName, artifact.AnalyzerVersion)
	return err
}

// insertSharedLemmas upserts the analyzed shared lemmas.
func insertSharedLemmas(ctx context.Context, tx pgx.Tx, lemmas []domain.SharedLemma) error {
	for _, lemma := range lemmas {
		if _, err := tx.Exec(ctx, `INSERT INTO shared_lemmas(content_hash,language,canonical_lemma,upos,morphology,frequency) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(content_hash,canonical_lemma,upos,morphology) DO UPDATE SET frequency=excluded.frequency`, lemma.ContentHash, lemma.Language, lemma.CanonicalLemma, lemma.UPOS, lemma.Morphology, lemma.Frequency); err != nil {
			return err
		}
	}
	return nil
}

// insertCompletedCorpus inserts the completed corpus row, or loads the existing
// one when this run already persisted it.
func insertCompletedCorpus(ctx context.Context, tx pgx.Tx, a JobArgs, analyzed snapshotAnalysis) (string, error) {
	profile := analyzed.statistics.TextProfile
	var corpusID string
	err := tx.QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count)
		VALUES($1,$2,$3,$4::uuid,'complete',$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT(owner_id,analysis_run_id) WHERE analysis_run_id IS NOT NULL DO NOTHING RETURNING id::text`, a.OwnerID, a.SourceMaterialID, analyzed.artifact.ContentHash, a.RunID, analyzed.statistics.AnalyzableTokenCount, analyzed.statistics.DistinctLemmaCount, profile.SentenceCount, profile.NormalizedTokenCount, profile.EmptySentenceCount, profile.MedianSentenceTokenCount, profile.P90SentenceTokenCount, profile.LongSentenceCount).Scan(&corpusID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND analysis_run_id=$2`, a.OwnerID, a.RunID).Scan(&corpusID); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	return corpusID, nil
}

// completeSnapshotRun records the completion history, completes the run and job,
// and takes the source Book lock before the caller commits.
func completeSnapshotRun(ctx context.Context, tx pgx.Tx, a JobArgs, plan snapshotPlan, corpusID string) error {
	details, err := analysisHistoryDetails(a, plan.decision, plan.totalUnits, plan.selectedUnits)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'analysis','complete',$3,now())`, a.OwnerID, corpusID, details); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='completed',corpus_id=$3::uuid,last_error='',updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2 AND state<>'cancelled'`, a.OwnerID, a.RunID, corpusID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE analysis_jobs SET corpus_id=$3::uuid,progress=100,error='',updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$2`, a.OwnerID, a.RunID, corpusID); err != nil {
		return err
	}
	// The run lock is acquired before the owner/book lock for every snapshot
	// completion. The book lock serializes replacement of one book's pointer.
	var bookID string
	if err = tx.QueryRow(ctx, `SELECT b.id::text FROM books b JOIN source_materials s ON s.owner_id=b.owner_id AND s.book_id=b.id AND s.owner_id=$1 AND s.id=$2 WHERE b.owner_id=$1 FOR UPDATE OF b`, a.OwnerID, a.SourceMaterialID).Scan(&bookID); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}

// selectSnapshotRun generates the candidate selection for a persisted corpus
// and publishes the completed run. A completed run reloads its corpus from
// persisted evidence instead of the in-memory result.
func (w *Worker) selectSnapshotRun(ctx context.Context, a JobArgs, riverJobID int64, corpusID string, result *analyzer.Result) error {
	if err := w.generateSelection(ctx, a, riverJobID, corpusID, result); err != nil {
		return err
	}
	return w.publishCompletedRun(ctx, a, riverJobID)
}

func (w *Worker) workSnapshot(ctx context.Context, job *river.Job[JobArgs]) (workErr error) {
	a := job.Args
	runState, err := w.beginSnapshotRun(ctx, a, job.ID)
	if err != nil {
		return err
	}
	if runState == "cancelled" {
		return nil
	}
	defer func() {
		if workErr != nil {
			workErr = w.recordSnapshotFailure(ctx, workErr, a, job.ID)
		}
	}()
	if runState == "completed" {
		return w.selectSnapshotRun(ctx, a, job.ID, "", nil)
	}
	plan, err := w.fetchSnapshotWork(ctx, a)
	if err != nil {
		return err
	}
	analyzed, err := w.analyzeSnapshotWork(ctx, a, plan)
	if err != nil {
		return err
	}
	corpusID, proceed, err := w.persistSnapshotWork(ctx, a, plan, analyzed)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	return w.selectSnapshotRun(ctx, a, job.ID, corpusID, &analyzed.result)
}

func (w *Worker) generateSelection(ctx context.Context, args JobArgs, riverJobID int64, corpusID string, result *analyzer.Result) error {
	if corpusID == "" {
		if err := w.Pool.QueryRow(ctx, `SELECT c.id::text FROM analysis_runs r JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id AND c.analysis_run_id=r.id AND c.status='complete' WHERE r.owner_id=$1 AND r.id=$2 AND r.state='completed'`, args.OwnerID, args.RunID).Scan(&corpusID); err != nil {
			return fmt.Errorf("load completed corpus for selection retry: %w", err)
		}
	}
	var generated bool
	if err := w.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM processing_history WHERE owner_id=$1 AND corpus_id=$2 AND operation='candidate_generation' AND status='complete' AND details->>'stage'='selection')`, args.OwnerID, corpusID).Scan(&generated); err != nil {
		return fmt.Errorf("check completed candidate generation: %w", err)
	}
	if generated {
		return nil
	}
	var persisted analyzer.Result
	if result == nil {
		var err error
		persisted, err = loadNormalizedCorpusForSelection(ctx, w.Pool, args.OwnerID, corpusID, args.Language)
		if err != nil {
			return err
		}
		result = &persisted
	}
	config, err := w.selectionConfig(ctx, args, corpusID)
	if err != nil {
		return w.recordSelectionFailure(ctx, args.OwnerID, corpusID, riverJobID, err)
	}
	if _, err := w.Selection.Select(ctx, args.OwnerID, *result, config); err != nil {
		return w.recordSelectionFailure(ctx, args.OwnerID, corpusID, riverJobID, err)
	}
	details, err := json.Marshal(map[string]any{"river_job_id": riverJobID, "stage": "selection"})
	if err != nil {
		return fmt.Errorf("encode candidate-generation completion: %w", err)
	}
	if _, err = w.Pool.Exec(ctx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'candidate_generation','complete',$3,now())`, args.OwnerID, corpusID, details); err != nil {
		return fmt.Errorf("record candidate-generation completion: %w", err)
	}
	return nil
}

// recordSelectionFailure records a retryable candidate-generation failure and
// returns it together with any recording failure.
func (w *Worker) recordSelectionFailure(ctx context.Context, owner, corpusID string, riverJobID int64, failure error) error {
	if recordErr := w.recordCandidateGenerationFailure(ctx, owner, corpusID, riverJobID, "selection", failure); recordErr != nil {
		return errors.Join(failure, recordErr)
	}
	return failure
}

// selectionConfig passes the exact snapshot's recorded structure to the
// selection rule. A run without a snapshot id has no structure to supply.
func (w *Worker) selectionConfig(ctx context.Context, args JobArgs, corpusID string) (selection.SelectionConfig, error) {
	config := selection.DefaultConfig(corpusID)
	if args.SnapshotID == "" {
		return config, nil
	}
	structure, err := persistence.LoadSnapshotStructure(ctx, w.Pool, args.OwnerID, args.SourceMaterialID, args.SnapshotID)
	if err != nil {
		return config, err
	}
	config.Structure = structure
	return config, nil
}

func loadNormalizedCorpusForSelection(ctx context.Context, pool *pgxpool.Pool, owner, corpusID, language string) (analyzer.Result, error) {
	rows, err := pool.Query(ctx, `SELECT s.sentence_ordinal,s.sentence_text,s.unit_id,s.start_offset,s.end_offset,
		COALESCE(t.token_ordinal,-1),COALESCE(t.surface,''),COALESCE(t.raw_lemma,''),COALESCE(t.canonical_lemma,''),COALESCE(t.upos,''),COALESCE(t.dependency,''),COALESCE(t.head,0),COALESCE(t.morphology,'{}'::jsonb),COALESCE(t.start_offset,0),COALESCE(t.end_offset,0),COALESCE(t.lexical_discriminator,''),COALESCE(t.headword,'')
		FROM corpus_sentences s LEFT JOIN corpus_tokens t
		 ON t.owner_id=s.owner_id AND t.analysis_run_id=s.analysis_run_id AND t.corpus_id=s.corpus_id AND t.sentence_ordinal=s.sentence_ordinal
		WHERE s.owner_id=$1 AND s.corpus_id=$2 ORDER BY s.sentence_ordinal,t.token_ordinal`, owner, corpusID)
	if err != nil {
		return analyzer.Result{}, fmt.Errorf("load persisted corpus for selection: %w", err)
	}
	defer rows.Close()
	result := analyzer.Result{Language: language, Sentences: make([]analyzer.Sentence, 0)}
	var currentOrdinal int64 = -1
	for rows.Next() {
		row, err := scanPersistedCorpusRow(rows)
		if err != nil {
			return analyzer.Result{}, err
		}
		if err := appendPersistedSentence(&result, &currentOrdinal, row); err != nil {
			return analyzer.Result{}, err
		}
		if row.tokenOrdinal < 0 {
			continue
		}
		if err := appendPersistedToken(&result, row); err != nil {
			return analyzer.Result{}, err
		}
	}
	if err = rows.Err(); err != nil {
		return analyzer.Result{}, fmt.Errorf("iterate persisted corpus for selection: %w", err)
	}
	return result, nil
}

// persistedCorpusRow is one scanned sentence/token row of a persisted corpus.
type persistedCorpusRow struct {
	sentenceOrdinal, sentenceStart, sentenceEnd         int64
	sentenceText, unitID                                string
	tokenOrdinal, head, tokenStart, tokenEnd            int64
	surface, rawLemma, canonicalLemma, upos, dependency string
	lexicalDiscriminator, headword                      string
	morphologyJSON                                      []byte
}

// scanPersistedCorpusRow reads one row of the persisted-corpus join, keeping a
// sentence with no token rows so its token ordinal stays -1.
func scanPersistedCorpusRow(rows pgx.Rows) (persistedCorpusRow, error) {
	var row persistedCorpusRow
	if err := rows.Scan(&row.sentenceOrdinal, &row.sentenceText, &row.unitID, &row.sentenceStart, &row.sentenceEnd, &row.tokenOrdinal, &row.surface, &row.rawLemma, &row.canonicalLemma, &row.upos, &row.dependency, &row.head, &row.morphologyJSON, &row.tokenStart, &row.tokenEnd, &row.lexicalDiscriminator, &row.headword); err != nil {
		return persistedCorpusRow{}, fmt.Errorf("read persisted corpus for selection: %w", err)
	}
	return row, nil
}

// appendPersistedSentence appends the row's sentence when it is the next
// sentence ordinal, so a sentence without tokens still contributes one sentence.
func appendPersistedSentence(result *analyzer.Result, currentOrdinal *int64, row persistedCorpusRow) error {
	if row.sentenceOrdinal == *currentOrdinal {
		return nil
	}
	startOffset, err := checked.Uint64FromInt64(row.sentenceStart)
	if err != nil {
		return fmt.Errorf("decode persisted sentence start offset %d: %w", row.sentenceOrdinal, err)
	}
	endOffset, err := checked.Uint64FromInt64(row.sentenceEnd)
	if err != nil {
		return fmt.Errorf("decode persisted sentence end offset %d: %w", row.sentenceOrdinal, err)
	}
	result.Sentences = append(result.Sentences, analyzer.Sentence{
		Text: row.sentenceText, Tokens: make([]analyzer.Token, 0),
		Location: analyzer.SourceLocation{SourceDocumentID: row.unitID, StartOffset: startOffset, EndOffset: endOffset},
	})
	*currentOrdinal = row.sentenceOrdinal
	return nil
}

// appendPersistedToken decodes and appends the row's token to the last sentence.
func appendPersistedToken(result *analyzer.Result, row persistedCorpusRow) error {
	checkedHead, err := checked.Uint32FromInt64(row.head)
	if err != nil {
		return fmt.Errorf("decode persisted token head for sentence %d: %w", row.sentenceOrdinal, err)
	}
	var morphology map[string]string
	if err = json.Unmarshal(row.morphologyJSON, &morphology); err != nil {
		return fmt.Errorf("decode persisted token morphology for sentence %d: %w", row.sentenceOrdinal, err)
	}
	tokenStartOffset, err := checked.Uint64FromInt64(row.tokenStart)
	if err != nil {
		return fmt.Errorf("decode persisted token start offset for sentence %d: %w", row.sentenceOrdinal, err)
	}
	tokenEndOffset, err := checked.Uint64FromInt64(row.tokenEnd)
	if err != nil {
		return fmt.Errorf("decode persisted token end offset for sentence %d: %w", row.sentenceOrdinal, err)
	}
	result.Sentences[len(result.Sentences)-1].Tokens = append(result.Sentences[len(result.Sentences)-1].Tokens, analyzer.Token{
		Surface: row.surface, RawLemma: row.rawLemma, CanonicalLemma: row.canonicalLemma, UPOS: row.upos,
		Dependency: row.dependency, Head: checkedHead, Morphology: morphology,
		Location:             analyzer.SourceLocation{SourceDocumentID: row.unitID, StartOffset: tokenStartOffset, EndOffset: tokenEndOffset},
		LexicalDiscriminator: row.lexicalDiscriminator, Headword: row.headword,
	})
	return nil
}

// publishCompletedRun promotes only the exact completed corpus whose source
// revision and snapshot are still current. Keeping this transaction separate
// from corpus persistence means publication can fail and replay without
// running NLP again, while an older current analysis remains untouched.
func (w *Worker) publishCompletedRun(ctx context.Context, a JobArgs, riverJobID int64) (err error) {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin analysis publication: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, a.OwnerID); err != nil {
		return fmt.Errorf("lock analysis publication: %w", err)
	}
	runState, attemptState, err := lockPublicationRun(ctx, tx, a, riverJobID)
	if err != nil {
		return err
	}
	if attemptState == "cancelled" {
		return tx.Commit(ctx)
	}
	if runState != "completed" {
		return nil
	}
	bookID, found, err := lockPublicationBook(ctx, tx, a)
	if err != nil {
		return err
	}
	if !found {
		if err := completePublicationAttempt(ctx, tx, a.RunID, riverJobID); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	if err := publishEligibleAnalysis(ctx, tx, a, bookID); err != nil {
		return err
	}
	if err = completePublicationAttempt(ctx, tx, a.RunID, riverJobID); err != nil {
		return fmt.Errorf("complete analysis publication attempt: %w", err)
	}
	if _, err = tx.Exec(ctx, `UPDATE analysis_jobs SET error='',progress=100,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$2`, a.OwnerID, a.RunID); err != nil {
		return fmt.Errorf("complete analysis publication status: %w", err)
	}
	return tx.Commit(ctx)
}

// lockPublicationRun takes the run and attempt row locks publication needs and
// reports their states. A missing attempt leaves its state empty.
func lockPublicationRun(ctx context.Context, tx pgx.Tx, a JobArgs, riverJobID int64) (runState, attemptState string, err error) {
	if err = tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2 FOR UPDATE`, a.OwnerID, a.RunID).Scan(&runState); err != nil {
		return "", "", fmt.Errorf("lock completed analysis for publication: %w", err)
	}
	if err = tx.QueryRow(ctx, `SELECT state FROM analysis_run_attempts WHERE run_id=$1 AND river_job_id=$2 FOR UPDATE`, a.RunID, riverJobID).Scan(&attemptState); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("lock analysis publication attempt: %w", err)
	}
	return runState, attemptState, nil
}

// lockPublicationBook locks the Book that owns the publication's source and
// reports found=false when the source is not linked to a Book.
func lockPublicationBook(ctx context.Context, tx pgx.Tx, a JobArgs) (string, bool, error) {
	var bookID string
	err := tx.QueryRow(ctx, `SELECT b.id::text FROM books b JOIN source_materials s ON s.owner_id=b.owner_id AND s.book_id=b.id AND s.owner_id=$1 AND s.id=$2 WHERE b.owner_id=$1 FOR UPDATE OF b`, a.OwnerID, a.SourceMaterialID).Scan(&bookID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("lock Book for analysis publication: %w", err)
	}
	return bookID, true, nil
}

// publishEligibleAnalysis promotes the completed run to the Book's current
// analysis, with its Browse projection, when no newer run superseded it.
func publishEligibleAnalysis(ctx context.Context, tx pgx.Tx, a JobArgs, bookID string) error {
	var eligible bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM analysis_runs r
		JOIN source_materials s ON s.owner_id=r.owner_id AND s.id=r.source_material_id AND s.book_id=$3
		JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id AND c.source_material_id=r.source_material_id AND c.analysis_run_id=r.id AND c.status='complete'
		WHERE r.owner_id=$1 AND r.id=$2 AND r.state='completed'
		  AND s.current_content_revision_id=r.content_revision_id AND s.current_snapshot_id=r.snapshot_id
		  AND `+noNewerAnalysisRun+`
	)`, a.OwnerID, a.RunID, bookID).Scan(&eligible); err != nil {
		return fmt.Errorf("check completed analysis eligibility: %w", err)
	}
	if !eligible {
		return nil
	}
	var corpusID string
	if err := tx.QueryRow(ctx, `SELECT corpus_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, a.OwnerID, a.RunID).Scan(&corpusID); err != nil {
		return fmt.Errorf("load completed corpus for Browse projection: %w", err)
	}
	if err := persistence.BuildVocabularyBrowseCountsTx(ctx, tx, a.OwnerID, bookID, a.SourceMaterialID, a.RunID, corpusID, a.Language); err != nil {
		return fmt.Errorf("build Browse projection for analysis publication: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id,promoted_at)
		VALUES($1,$2,$3,$4,now())
		ON CONFLICT(owner_id,book_id) DO UPDATE SET source_material_id=excluded.source_material_id,analysis_run_id=excluded.analysis_run_id,promoted_at=excluded.promoted_at
		WHERE book_current_analyses.analysis_run_id IS DISTINCT FROM excluded.analysis_run_id`, a.OwnerID, bookID, a.SourceMaterialID, a.RunID); err != nil {
		return fmt.Errorf("publish completed analysis: %w", err)
	}
	return nil
}

const (
	// corpusTokenParameterCount is the number of bind parameters in one
	// corpus-token INSERT row. The token batch below derives from it so the
	// batch stays under PostgreSQL's 65,535-parameter protocol limit.
	corpusTokenParameterCount               = 18
	normalizedCorpusSentenceInsertBatchSize = 500
	normalizedCorpusTokenInsertBatchSize    = 65535 / corpusTokenParameterCount
)

// persistNormalizedCorpus writes the normalized sentences and then the tokens
// they contain, in the caller's transaction, and refreshes their planner
// statistics.
func persistNormalizedCorpus(ctx context.Context, tx pgx.Tx, ownerID, language, runID, corpusID string, result analyzer.Result) error {
	if err := insertCorpusSentences(ctx, tx, ownerID, runID, corpusID, result.Sentences); err != nil {
		return err
	}
	if err := insertCorpusTokens(ctx, tx, ownerID, language, runID, corpusID, result.Sentences); err != nil {
		return err
	}
	return refreshCorpusStatistics(ctx, tx)
}

// refreshCorpusStatistics analyzes the corpus tables so queries over a corpus
// that was just written see its rows. Until autoanalyze catches up, the planner
// estimates a new run (or a first-seen language) at about one row, and the
// Browse projection read during publication can then pick a plan that runs for
// tens of minutes instead of well under a second. ANALYZE samples the caller's
// uncommitted rows, so running it inside the writing transaction is enough.
func refreshCorpusStatistics(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `ANALYZE corpus_sentences, corpus_tokens`); err != nil {
		return fmt.Errorf("refresh corpus planner statistics: %w", err)
	}
	return nil
}

func insertCorpusSentences(ctx context.Context, tx pgx.Tx, ownerID, runID, corpusID string, sentences []analyzer.Sentence) error {
	for start := 0; start < len(sentences); start += normalizedCorpusSentenceInsertBatchSize {
		end := min(start+normalizedCorpusSentenceInsertBatchSize, len(sentences))
		query, args, err := corpusSentenceInsert(sentences, start, end, ownerID, runID, corpusID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, query, args...); err != nil {
			return err
		}
	}
	return nil
}

// corpusSentenceInsert builds one batched corpus-sentence INSERT for the
// sentences in [start, end), including each sentence's cited sections.
func corpusSentenceInsert(sentences []analyzer.Sentence, start, end int, ownerID, runID, corpusID string) (string, []any, error) {
	var query strings.Builder
	query.WriteString(`INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset,citations) VALUES `)
	args := make([]any, 0, (end-start)*9)
	for ordinal := start; ordinal < end; ordinal++ {
		if ordinal > start {
			query.WriteString(",")
		}
		sentence := sentences[ordinal]
		startOffset, offsetErr := checked.Int64FromUint64(sentence.Location.StartOffset)
		if offsetErr != nil {
			return "", nil, fmt.Errorf("persist normalized corpus sentence %d: start offset overflows bigint: %w", ordinal, offsetErr)
		}
		endOffset, offsetErr := checked.Int64FromUint64(sentence.Location.EndOffset)
		if offsetErr != nil {
			return "", nil, fmt.Errorf("persist normalized corpus sentence %d: end offset overflows bigint: %w", ordinal, offsetErr)
		}
		citations := sentence.Citations
		if citations == nil {
			citations = []string{}
		}
		arg := len(args) + 1
		fmt.Fprintf(&query, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", arg, arg+1, arg+2, arg+3, arg+4, arg+5, arg+6, arg+7, arg+8)
		args = append(args, ownerID, runID, corpusID, sentence.Location.SourceDocumentID, int64(ordinal), sentence.Text, startOffset, endOffset, citations)
	}
	return query.String(), args, nil
}

// corpusTokenInserter accumulates one batched corpus-token INSERT, flushing it
// when it reaches the batch size and at the end.
type corpusTokenInserter struct {
	query strings.Builder
	args  []any
	count int
}

// add validates one analyzed token and appends its row, flushing a full batch
// first.
func (b *corpusTokenInserter) add(ctx context.Context, tx pgx.Tx, ownerID, language, runID, corpusID string, sentenceOrdinal, tokenOrdinal, tokenTotal int, token analyzer.Token) error {
	if err := validateCorpusToken(sentenceOrdinal, tokenOrdinal, tokenTotal, token); err != nil {
		return err
	}
	if b.count == normalizedCorpusTokenInsertBatchSize {
		if err := b.flush(ctx, tx); err != nil {
			return err
		}
	}
	if b.count == 0 {
		b.query.WriteString(`INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset,lexical_discriminator,headword,search_key) VALUES `)
	} else {
		b.query.WriteString(",")
	}
	tuple, rowArgs, err := corpusTokenRow(ownerID, language, runID, corpusID, sentenceOrdinal, tokenOrdinal, len(b.args)+1, token)
	if err != nil {
		return err
	}
	b.query.WriteString(tuple)
	b.args = append(b.args, rowArgs...)
	b.count++
	return nil
}

// flush writes the accumulated tokens, if any.
func (b *corpusTokenInserter) flush(ctx context.Context, tx pgx.Tx) error {
	if b.count == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx, b.query.String(), b.args...); err != nil {
		return err
	}
	b.query.Reset()
	b.args = b.args[:0]
	b.count = 0
	return nil
}

func insertCorpusTokens(ctx context.Context, tx pgx.Tx, ownerID, language, runID, corpusID string, sentences []analyzer.Sentence) error {
	var batch corpusTokenInserter
	for sentenceOrdinal, sentence := range sentences {
		for tokenOrdinal, token := range sentence.Tokens {
			if err := batch.add(ctx, tx, ownerID, language, runID, corpusID, sentenceOrdinal, tokenOrdinal, len(sentence.Tokens), token); err != nil {
				return err
			}
		}
	}
	return batch.flush(ctx, tx)
}

// validateCorpusToken rejects an analyzed token whose dependency or head cannot
// describe the sentence it belongs to.
func validateCorpusToken(sentenceOrdinal, tokenOrdinal, tokenTotal int, token analyzer.Token) error {
	if token.Dependency == "" {
		return fmt.Errorf("persist normalized corpus token %d in sentence %d: dependency is required", tokenOrdinal, sentenceOrdinal)
	}
	if int64(token.Head) >= int64(tokenTotal) {
		return fmt.Errorf("persist normalized corpus token %d in sentence %d: head ordinal %d is out of range", tokenOrdinal, sentenceOrdinal, token.Head)
	}
	if token.Dependency == "root" && int64(token.Head) != int64(tokenOrdinal) {
		return fmt.Errorf("persist normalized corpus token %d in sentence %d: root must be self-headed", tokenOrdinal, sentenceOrdinal)
	}
	return nil
}

// corpusTokenRow builds the INSERT value tuple and arguments for one analyzed
// token's row: its identity and morphology, and its checked offsets. firstArg is
// the one-based parameter index the tuple starts at.
func corpusTokenRow(ownerID, language, runID, corpusID string, sentenceOrdinal, tokenOrdinal, firstArg int, token analyzer.Token) (string, []any, error) {
	morphology := token.Morphology
	if morphology == nil {
		morphology = map[string]string{}
	}
	morphologyJSON, err := json.Marshal(morphology)
	if err != nil {
		return "", nil, err
	}
	startOffset, offsetErr := checked.Int64FromUint64(token.Location.StartOffset)
	if offsetErr != nil {
		return "", nil, fmt.Errorf("persist normalized corpus token %d in sentence %d: start offset overflows bigint: %w", tokenOrdinal, sentenceOrdinal, offsetErr)
	}
	endOffset, offsetErr := checked.Int64FromUint64(token.Location.EndOffset)
	if offsetErr != nil {
		return "", nil, fmt.Errorf("persist normalized corpus token %d in sentence %d: end offset overflows bigint: %w", tokenOrdinal, sentenceOrdinal, offsetErr)
	}
	var tuple strings.Builder
	fmt.Fprintf(&tuple, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", firstArg, firstArg+1, firstArg+2, firstArg+3, firstArg+4, firstArg+5, firstArg+6, firstArg+7, firstArg+8, firstArg+9, firstArg+10, firstArg+11, firstArg+12, firstArg+13, firstArg+14, firstArg+15, firstArg+16, firstArg+17)
	args := []any{ownerID, language, runID, corpusID, int64(sentenceOrdinal), int64(tokenOrdinal), token.Surface, token.RawLemma, token.CanonicalLemma, token.UPOS, token.Dependency, int64(token.Head), morphologyJSON, startOffset, endOffset, token.LexicalDiscriminator, token.Headword, token.SearchKey}
	return tuple.String(), args, nil
}

func (w *Worker) recordCandidateGenerationFailure(ctx context.Context, owner, corpusID string, jobID int64, stage string, cause error) error {
	details, err := json.Marshal(map[string]any{"river_job_id": jobID, "stage": stage, "error": cause.Error()})
	if err != nil {
		return fmt.Errorf("encode candidate-generation failure: %w", err)
	}
	failureCtx := context.WithoutCancel(ctx)
	if _, err = w.Pool.Exec(failureCtx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'candidate_generation','failed',$3,now())`, owner, corpusID, details); err != nil {
		return fmt.Errorf("record candidate-generation failure: %w", err)
	}
	return nil
}

func safeAnalysisError(err error) string {
	if errors.Is(err, ErrDependencyParsingUnavailable) {
		return err.Error()
	}
	return "Analysis could not be completed. Retry the analysis or review the current source."
}

func requireDependencyParsing(ctx context.Context, provider analyzer.CapabilityProvider, language string) error {
	if provider == nil {
		return fmt.Errorf("%w: capability provider is not configured", ErrDependencyParsingUnavailable)
	}
	capabilities, err := provider.GetCapabilities(ctx)
	if err != nil {
		return fmt.Errorf("load NLP capabilities: %w", err)
	}
	if capabilities.SupportsLanguage(language, analyzer.FeatureDepparse) {
		return nil
	}
	return fmt.Errorf("%w for language %q", ErrDependencyParsingUnavailable, language)
}

func ordinaryAnalysisIdentity(contentHash string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(contentHash+"\x00"+ordinaryAnalysisContractVersion)))
}

func normalizedArtifactHash(contentHash, scopeID, analysisIdentity, analyzerName, analyzerVersion, configIdentity string, result analyzer.Result) string {
	identity := strings.Join([]string{
		contentHash,
		scopeID,
		analysisIdentity,
		analyzerName,
		analyzerVersion,
		configIdentity,
		result.Language,
		result.SchemaVersion,
		result.NormalizationProfile.Name,
		result.NormalizationProfile.Version,
		result.Analysis.AnalyzerName,
		result.Analysis.AnalyzerVersion,
	}, "\x00")
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(identity)))
}

func offsetResultLocations(result *analyzer.Result, offset uint64) {
	if offset == 0 {
		return
	}
	for i := range result.Sentences {
		result.Sentences[i].Location.StartOffset += offset
		result.Sentences[i].Location.EndOffset += offset
		for j := range result.Sentences[i].Tokens {
			result.Sentences[i].Tokens[j].Location.StartOffset += offset
			result.Sentences[i].Tokens[j].Location.EndOffset += offset
		}
	}
}

func aggregateLemmas(hash string, result analyzer.Result) ([]domain.SharedLemma, error) {
	type entry struct {
		lemma analyzer.Token
		count int64
	}
	values := map[string]entry{}
	for _, sentence := range result.Sentences {
		for _, token := range sentence.Tokens {
			if token.Dependency == "compound:prt" || !lexical.IsLemma(token.CanonicalLemma) {
				continue
			}
			raw, err := json.Marshal(token.Morphology)
			if err != nil {
				return nil, fmt.Errorf("marshal morphology for %s/%s: %w", token.CanonicalLemma, token.UPOS, err)
			}
			key := token.CanonicalLemma + "\x00" + token.UPOS + "\x00" + string(raw)
			e := values[key]
			e.lemma = token
			e.count++
			values[key] = e
		}
	}
	out := make([]domain.SharedLemma, 0, len(values))
	for _, e := range values {
		raw, err := json.Marshal(e.lemma.Morphology)
		if err != nil {
			return nil, fmt.Errorf("marshal morphology for %s/%s: %w", e.lemma.CanonicalLemma, e.lemma.UPOS, err)
		}
		out = append(out, domain.SharedLemma{ContentHash: hash, Language: result.Language, CanonicalLemma: e.lemma.CanonicalLemma, UPOS: e.lemma.UPOS, Morphology: raw, Frequency: e.count})
	}
	return out, nil
}

func NewClient(pool *pgxpool.Pool, a analyzer.Analyzer, capabilities analyzer.CapabilityProvider, selectionService *selection.Service, workerSets ...*river.Workers) (*river.Client[pgx.Tx], error) {
	return newClient(pool, a, capabilities, selectionService, 1, workerSets...)
}

func NewClientWithPreparedDeckConcurrency(pool *pgxpool.Pool, a analyzer.Analyzer, capabilities analyzer.CapabilityProvider, selectionService *selection.Service, standardWorkers int, workerSets ...*river.Workers) (*river.Client[pgx.Tx], error) {
	return newClient(pool, a, capabilities, selectionService, standardWorkers, workerSets...)
}

func newClient(pool *pgxpool.Pool, a analyzer.Analyzer, capabilities analyzer.CapabilityProvider, selectionService *selection.Service, standardWorkers int, workerSets ...*river.Workers) (*river.Client[pgx.Tx], error) {
	if pool == nil || a == nil || capabilities == nil || selectionService == nil {
		return nil, errors.New("analysis client requires pool, analyzer, capabilities, and selection service")
	}
	jobTimeout, err := configuredJobTimeout()
	if err != nil {
		return nil, err
	}
	workers := river.NewWorkers()
	if len(workerSets) > 0 && workerSets[0] != nil {
		workers = workerSets[0]
	}
	river.AddWorker(workers, &Worker{Pool: pool, Analyzer: a, Capabilities: capabilities, Selection: selectionService})
	river.AddWorker(workers, &BrowseCountsRebuildWorker{Pool: pool})
	if standardWorkers < 1 {
		standardWorkers = 1
	}
	return river.NewClient(riverpgxv5.New(pool), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, "catalogue_sync": {MaxWorkers: 1}, "prepared_decks": {MaxWorkers: 1}, "prepared_deck_translation": {MaxWorkers: standardWorkers}, sourcecheck.Queue: {MaxWorkers: 1}}, Workers: workers, JobTimeout: jobTimeout})
}

func configuredJobTimeout() (time.Duration, error) {
	value, ok := os.LookupEnv(jobTimeoutEnv)
	if !ok {
		return defaultJobTimeout, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil || timeout <= 0 {
		return 0, fmt.Errorf("%s must be a positive Go duration: %q", jobTimeoutEnv, value)
	}
	return timeout, nil
}

func MigrateRiver(ctx context.Context, pool *pgxpool.Pool) error {
	m, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return fmt.Errorf("initialize River migrations: %w", err)
	}
	_, err = m.Migrate(ctx, rivermigrate.DirectionUp, nil)
	if err != nil {
		return fmt.Errorf("apply River migrations: %w", err)
	}
	return nil
}
