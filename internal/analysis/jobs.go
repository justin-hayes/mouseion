// Package analysis implements the durable, owner-scoped analysis job API.
package analysis

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/selection"
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
	// Version 2 identifies the current ordinary analysis contract, including
	// lemma-boundary cleanup. Bump it whenever ordinary normalization or
	// analyzer output semantics change.
	ordinaryAnalysisContractVersion = "2"
)

type Service struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
}

var liveJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateRetryable,
	rivertype.JobStateScheduled,
}

func isLiveJobState(state rivertype.JobState) bool {
	return slices.Contains(liveJobStates, state)
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
	if err = tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE id=$1 FOR UPDATE`, runID).Scan(&state); err != nil {
		return 0, err
	}
	if state != "queued" {
		if state == "running" && failOrphaned {
			_, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='failed',last_error=$2,updated_at=now(),completed_at=now() WHERE id=$1 AND state='running'`, runID, safeAnalysisError(errors.New("analysis worker is no longer active")))
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='failed',error=$2,finalized_at=now() WHERE run_id=$1 AND state IN ('queued','running')`, runID, safeAnalysisError(errors.New("analysis worker is no longer active")))
			}
		}
		return 0, err
	}
	var attempt int
	if err = tx.QueryRow(ctx, `UPDATE analysis_runs SET attempt_count=attempt_count+1,updated_at=now() WHERE id=$1 RETURNING attempt_count`, runID).Scan(&attempt); err != nil {
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

func NewService(pool *pgxpool.Pool, client *river.Client[pgx.Tx]) *Service {
	return &Service{pool: pool, client: client}
}

// SubmitAnalysis atomically records one snapshot-bound analysis run and inserts
// its River job. The worker reloads the immutable extracted snapshot.
func (s *Service) SubmitAnalysis(ctx context.Context, owner, sourceID string) (Handle, error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(sourceID) == "" {
		return Handle{}, fmt.Errorf("analysis owner and source are required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, fmt.Errorf("begin analysis submission: %w", err)
	}
	defer tx.Rollback(ctx)
	var args JobArgs
	var revisionID, snapshotID, mediaType string
	err = tx.QueryRow(ctx, `SELECT s.owner_id,s.id,s.language,s.source_identifier,s.title,
		COALESCE(r.revision_id::text,''),CASE WHEN r.digest_version=1 THEN r.content_digest ELSE s.content_hash END,
		COALESCE(s.current_snapshot_id::text,''),s.media_type
		FROM source_materials s
		LEFT JOIN source_content_revisions r ON r.owner_id=s.owner_id AND r.source_material_id=s.id AND r.revision_id=s.current_content_revision_id
		WHERE s.owner_id=$1 AND s.id=$2`, owner, sourceID).
		Scan(&args.OwnerID, &args.SourceMaterialID, &args.Language, &args.SourceIdentifier, &args.Title, &revisionID, &args.ContentHash, &snapshotID, &mediaType)
	if errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, ErrNotFound
	}
	if err != nil {
		return Handle{}, fmt.Errorf("load EPUB for analysis: %w", err)
	}
	if mediaType != "application/epub+zip" {
		return Handle{}, ErrEPUBRequired
	}
	if revisionID == "" || snapshotID == "" {
		return Handle{}, domain.ErrExtractedUnitsUnavailable
	}
	args.RunID = ""
	args.ContentRevisionID, args.SnapshotID = revisionID, snapshotID
	args.AnalyzerName, args.AnalyzerVersion, args.ConfigIdentity = snapshotAnalyzerName, snapshotAnalyzerVersion, snapshotConfigIdentity
	var readable int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2 AND snapshot_id=$3 AND btrim(text)<>''`, owner, sourceID, snapshotID).Scan(&readable); err != nil {
		return Handle{}, fmt.Errorf("check extracted EPUB units: %w", err)
	}
	if readable == 0 {
		return Handle{}, domain.ErrExtractedUnitsUnavailable
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 191))`, owner); err != nil {
		return Handle{}, fmt.Errorf("lock analysis submission: %w", err)
	}
	var runID, state string
	err = tx.QueryRow(ctx, `SELECT id::text,state FROM analysis_runs WHERE owner_id=$1 AND content_revision_id=$2 AND analyzer_name=$3 AND analyzer_version=$4 AND config_identity=$5 FOR UPDATE`, owner, revisionID, args.AnalyzerName, args.AnalyzerVersion, args.ConfigIdentity).Scan(&runID, &state)
	if err == nil {
		var id, display int64
		if err = tx.QueryRow(ctx, `SELECT river_job_id,display_number FROM analysis_jobs WHERE owner_id=$1 AND analysis_run_id=$2`, owner, runID).Scan(&id, &display); err != nil {
			return Handle{}, fmt.Errorf("load existing analysis handle: %w", err)
		}
		if state == "queued" || state == "running" {
			args.RunID = runID
			if _, err = s.ensureAttemptTx(ctx, tx, args, runID, state == "running"); err != nil {
				return Handle{}, err
			}
		} else if state == "failed" || state == "cancelled" {
			if _, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='queued',last_error='',started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2`, owner, runID); err != nil {
				return Handle{}, err
			}
			args.RunID = runID
			jobID, retryErr := s.ensureAttemptTx(ctx, tx, args, runID, false)
			if retryErr != nil {
				return Handle{}, retryErr
			}
			details, _ := json.Marshal(map[string]any{"run_id": runID, "from": state})
			if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details) VALUES($1,'analysis','queued',$2)`, owner, details); err != nil {
				return Handle{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return Handle{}, err
			}
			return Handle{ID: id, JobID: jobID, DisplayNumber: display, RunID: runID}, nil
		}
		if err = tx.Commit(ctx); err != nil {
			return Handle{}, err
		}
		return Handle{ID: id, JobID: id, DisplayNumber: display, RunID: runID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, fmt.Errorf("check existing analysis: %w", err)
	}
	runID = uuid.NewString()
	if err = tx.QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,attempt_count) VALUES($1,$2,$3,$4,$5,$6,$7,'queued',1) RETURNING id::text`, owner, sourceID, revisionID, snapshotID, args.AnalyzerName, args.AnalyzerVersion, args.ConfigIdentity).Scan(&runID); err != nil {
		return Handle{}, fmt.Errorf("create analysis run: %w", err)
	}
	args.RunID, args.Attempt = runID, 1
	inserted, err := s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: 1})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue analysis: %w", err)
	}
	var display int64
	if err = tx.QueryRow(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,analysis_run_id,display_number) VALUES($1,$2,$3,$4,$5::uuid,(SELECT COALESCE(MAX(display_number),0)+1 FROM analysis_jobs WHERE owner_id=$2)) RETURNING display_number`, inserted.Job.ID, owner, sourceID, args.ContentHash, runID).Scan(&display); err != nil {
		return Handle{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO analysis_run_attempts(run_id,owner_id,source_material_id,attempt_number,river_job_id,state) VALUES($1,$2,$3,1,$4,'queued')`, runID, owner, sourceID, inserted.Job.ID); err != nil {
		return Handle{}, err
	}
	if err = tx.Commit(ctx); err != nil {
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
	if runID != "" {
		var state, lastError string
		if err = s.pool.QueryRow(ctx, `SELECT state,last_error,COALESCE(corpus_id::text,''),attempt_count FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner, runID).
			Scan(&state, &lastError, &status.CorpusID, &status.AttemptCount); err != nil {
			return Status{}, fmt.Errorf("get snapshot analysis run: %w", err)
		}
		status.RunID, status.LogicalState = runID, state
		status.State = logicalStateRiverState(state)
		status.Attempt = status.AttemptCount
		if lastError != "" {
			status.Error = lastError
		}
		var attemptNumber int
		var jobID int64
		if err = s.pool.QueryRow(ctx, `SELECT attempt_number,river_job_id FROM analysis_run_attempts WHERE run_id=$1 ORDER BY attempt_number DESC LIMIT 1`, runID).Scan(&attemptNumber, &jobID); err == nil {
			status.Attempt = attemptNumber
			status.FinalizedAt = nil
			if state == "completed" || state == "failed" || state == "cancelled" {
				_ = s.pool.QueryRow(ctx, `SELECT finalized_at FROM analysis_run_attempts WHERE run_id=$1 AND attempt_number=$2`, runID, attemptNumber).Scan(&status.FinalizedAt)
			}
		}
		return status, nil
	}
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
	if status.RunID != "" {
		tx, txErr := s.pool.Begin(ctx)
		if txErr != nil {
			return Status{}, txErr
		}
		defer tx.Rollback(ctx)
		_, txErr = tx.Exec(ctx, `UPDATE analysis_runs SET state='cancelled',last_error='',updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2 AND state IN ('queued','running')`, owner, status.RunID)
		if txErr != nil {
			return Status{}, txErr
		}
		_, txErr = tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='cancelled',finalized_at=now() WHERE run_id=$1 AND state IN ('queued','running')`, status.RunID)
		if txErr != nil {
			return Status{}, txErr
		}
		if txErr = tx.Commit(ctx); txErr != nil {
			return Status{}, txErr
		}
		if jobID, queryErr := s.liveJobID(ctx, owner, status.RunID); queryErr == nil {
			_, _ = s.client.JobCancel(ctx, jobID)
		}
		return s.Get(ctx, owner, id)
	}
	if _, err := s.client.JobCancel(ctx, id); err != nil {
		return Status{}, fmt.Errorf("cancel analysis job: %w", err)
	}
	return s.Get(ctx, owner, id)
}

// Retry starts a new execution attempt for a failed or cancelled snapshot run.
// The logical run identity and status URL remain stable across attempts.
func (s *Service) Retry(ctx context.Context, owner string, id int64) (Handle, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil {
		return Handle{}, err
	}
	if status.RunID == "" {
		return Handle{}, fmt.Errorf("legacy analysis jobs cannot be retried through the snapshot lifecycle")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, status.RunID).Scan(&state); err != nil {
		return Handle{}, err
	}
	if state == "queued" || state == "running" {
		jobID, jobErr := s.liveJobIDTx(ctx, tx, owner, status.RunID)
		if jobErr == nil {
			if err = tx.Commit(ctx); err != nil {
				return Handle{}, err
			}
			return Handle{ID: id, JobID: jobID, DisplayNumber: status.DisplayNumber, RunID: status.RunID}, nil
		}
		if !errors.Is(jobErr, pgx.ErrNoRows) {
			return Handle{}, jobErr
		}
		if state == "running" {
			_, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='failed',last_error=$3,updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2`, owner, status.RunID, safeAnalysisError(errors.New("analysis worker is no longer active")))
			if err != nil {
				return Handle{}, err
			}
			state = "failed"
		} else {
			args, argsErr := snapshotArgsTx(ctx, tx, owner, status.RunID)
			if argsErr != nil {
				return Handle{}, argsErr
			}
			jobID, argsErr = s.ensureAttemptTx(ctx, tx, args, status.RunID, false)
			if argsErr != nil {
				return Handle{}, argsErr
			}
			if err = tx.Commit(ctx); err != nil {
				return Handle{}, err
			}
			return Handle{ID: id, JobID: jobID, DisplayNumber: status.DisplayNumber, RunID: status.RunID}, nil
		}
	}
	if state != "failed" && state != "cancelled" {
		return Handle{}, fmt.Errorf("analysis run is not retryable from %s", state)
	}
	if _, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='queued',last_error='',started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2`, owner, status.RunID); err != nil {
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
	details, _ := json.Marshal(map[string]any{"run_id": status.RunID, "from": state})
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details) VALUES($1,'analysis','queued',$2)`, owner, details); err != nil {
		return Handle{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{ID: id, JobID: jobID, DisplayNumber: status.DisplayNumber, RunID: status.RunID}, nil
}

// Reconcile makes an orphaned queued/running run actionable. A queued run is
// re-enqueued; a running run without viable River work is failed safely.
func (s *Service) Reconcile(ctx context.Context, owner string, id int64) (Status, error) {
	status, err := s.Get(ctx, owner, id)
	if err != nil || status.RunID == "" {
		return status, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Status{}, err
	}
	defer tx.Rollback(ctx)
	var state string
	if err = tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, status.RunID).Scan(&state); err != nil {
		return Status{}, err
	}
	if state == "queued" {
		if _, err = s.liveJobIDTx(ctx, tx, owner, status.RunID); errors.Is(err, pgx.ErrNoRows) {
			args, argsErr := snapshotArgsTx(ctx, tx, owner, status.RunID)
			if argsErr != nil {
				return Status{}, argsErr
			}
			_, err = s.ensureAttemptTx(ctx, tx, args, status.RunID, false)
		} else if err != nil {
			return Status{}, err
		}
	} else if state == "running" {
		if _, err = s.liveJobIDTx(ctx, tx, owner, status.RunID); errors.Is(err, pgx.ErrNoRows) {
			message := safeAnalysisError(errors.New("analysis worker is no longer active"))
			_, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='failed',last_error=$3,updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2`, owner, status.RunID, message)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='failed',error=$2,finalized_at=now() WHERE run_id=$1 AND state IN ('queued','running')`, status.RunID, message)
			}
		} else if err != nil {
			return Status{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Status{}, err
	}
	return s.Get(ctx, owner, id)
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
		s.language,s.source_identifier,s.title,rev.content_digest,rev.revision_id::text,r.snapshot_id::text
		FROM analysis_runs r JOIN source_materials s ON s.owner_id=r.owner_id AND s.id=r.source_material_id
		JOIN source_content_revisions rev ON rev.owner_id=r.owner_id AND rev.source_material_id=r.source_material_id AND rev.revision_id=r.content_revision_id
		WHERE r.owner_id=$1 AND r.id=$2`, owner, runID).Scan(&args.RunID, &args.Attempt, &args.OwnerID, &args.SourceMaterialID, &args.Language, &args.SourceIdentifier, &args.Title, &args.ContentHash, &args.ContentRevisionID, &args.SnapshotID)
	args.AnalyzerName, args.AnalyzerVersion, args.ConfigIdentity = snapshotAnalyzerName, snapshotAnalyzerVersion, snapshotConfigIdentity
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
	SnapshotID, UnitID, Title, SourceHref, ResolvedHref, Text string
	Order                                                     int
	StartOffset, EndOffset                                    uint64
}

type queryRower interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func loadSnapshotUnits(ctx context.Context, q queryRower, owner, sourceID, revisionID, snapshotID string) ([]snapshotUnit, error) {
	rows, err := q.Query(ctx, `SELECT u.snapshot_id,u.unit_id,u.unit_order,u.title,u.source_href,u.resolved_href,u.text,u.start_offset,u.end_offset
		FROM source_material_units u
		JOIN source_material_unit_snapshots snap ON snap.owner_id=u.owner_id AND snap.source_material_id=u.source_material_id AND snap.snapshot_id=u.snapshot_id AND snap.content_revision_id=$4
		JOIN source_materials source ON source.owner_id=u.owner_id AND source.id=u.source_material_id AND source.current_content_revision_id=$4 AND source.current_snapshot_id=u.snapshot_id
		WHERE u.owner_id=$1 AND u.source_material_id=$2 AND u.snapshot_id=$3 AND btrim(u.text)<>''
		ORDER BY u.unit_order`, owner, sourceID, snapshotID, revisionID)
	if err != nil {
		return nil, fmt.Errorf("load extracted snapshot: %w", err)
	}
	defer rows.Close()
	var units []snapshotUnit
	lastOrder := -1
	for rows.Next() {
		var unit snapshotUnit
		if err = rows.Scan(&unit.SnapshotID, &unit.UnitID, &unit.Order, &unit.Title, &unit.SourceHref, &unit.ResolvedHref, &unit.Text, &unit.StartOffset, &unit.EndOffset); err != nil {
			return nil, fmt.Errorf("load extracted snapshot unit: %w", err)
		}
		if unit.UnitID == "" || unit.Order <= lastOrder || unit.EndOffset < unit.StartOffset {
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

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) (workErr error) {
	return w.workSnapshot(ctx, job)
}

func (w *Worker) workSnapshot(ctx context.Context, job *river.Job[JobArgs]) (workErr error) {
	a := job.Args
	if _, err := w.Pool.Exec(ctx, `UPDATE analysis_runs SET state='running',started_at=COALESCE(started_at,now()),updated_at=now() WHERE owner_id=$1 AND id=$2 AND state='queued'`, a.OwnerID, a.RunID); err != nil {
		return err
	}
	if _, err := w.Pool.Exec(ctx, `UPDATE analysis_run_attempts SET state='running',started_at=COALESCE(started_at,now()) WHERE run_id=$1 AND river_job_id=$2 AND state='queued'`, a.RunID, job.ID); err != nil {
		return err
	}
	var runState string
	if err := w.Pool.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2`, a.OwnerID, a.RunID).Scan(&runState); err != nil {
		return err
	}
	if runState == "cancelled" || runState == "completed" {
		return nil
	}
	defer func() {
		if workErr == nil {
			return
		}
		failureCtx := context.WithoutCancel(ctx)
		message := safeAnalysisError(workErr)
		_, _ = w.Pool.Exec(failureCtx, `UPDATE analysis_runs SET state=CASE WHEN state='cancelled' THEN state ELSE 'failed' END,last_error=CASE WHEN state='cancelled' THEN last_error ELSE $3 END,updated_at=now(),completed_at=CASE WHEN state='cancelled' THEN completed_at ELSE now() END WHERE owner_id=$1 AND id=$2 AND state IN ('queued','running')`, a.OwnerID, a.RunID, message)
		_, _ = w.Pool.Exec(failureCtx, `UPDATE analysis_run_attempts SET state=CASE WHEN state='cancelled' THEN state ELSE 'failed' END,error=CASE WHEN state='cancelled' THEN error ELSE $3 END,finalized_at=now() WHERE run_id=$1 AND river_job_id=$2 AND state IN ('queued','running')`, a.RunID, job.ID, message)
		_, _ = w.Pool.Exec(failureCtx, `UPDATE analysis_jobs SET error=$3,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$2`, a.OwnerID, a.RunID, message)
		details, _ := json.Marshal(map[string]any{"run_id": a.RunID, "attempt": a.Attempt, "error": message})
		_, _ = w.Pool.Exec(failureCtx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'analysis','failed',$2,now())`, a.OwnerID, details)
	}()

	var valid bool
	if err := w.Pool.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM analysis_runs r JOIN source_materials s ON s.owner_id=r.owner_id AND s.id=r.source_material_id
			JOIN source_content_revisions rev ON rev.owner_id=r.owner_id AND rev.source_material_id=r.source_material_id AND rev.revision_id=r.content_revision_id
			WHERE r.id=$1 AND r.owner_id=$2 AND r.source_material_id=$3 AND r.content_revision_id=$4 AND r.snapshot_id=$5
			  AND s.current_content_revision_id=r.content_revision_id AND s.current_snapshot_id=r.snapshot_id AND rev.content_digest=$6
		)`, a.RunID, a.OwnerID, a.SourceMaterialID, a.ContentRevisionID, a.SnapshotID, a.ContentHash).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("the source content revision is no longer current")
	}
	if err := requireDependencyParsing(ctx, w.Capabilities, a.Language); err != nil {
		return err
	}
	var selectedUnits []snapshotUnit
	var err error
	selectedUnits, err = loadSnapshotUnits(ctx, w.Pool, a.OwnerID, a.SourceMaterialID, a.ContentRevisionID, a.SnapshotID)
	if err != nil {
		return err
	}
	inputs := make([]struct{ id, identifier, title, text string }, 0, len(selectedUnits))
	for _, unit := range selectedUnits {
		identifier := unit.ResolvedHref
		if identifier == "" {
			identifier = unit.SourceHref
		}
		inputs = append(inputs, struct{ id, identifier, title, text string }{unit.UnitID, identifier, unit.Title, unit.Text})
	}
	var merged analyzer.Result
	totalChunks := 0
	for _, input := range inputs {
		totalChunks += len(chunkText(input.text, w.MaxChunkChars))
	}
	completedChunks := 0
	for _, input := range inputs {
		chunks := chunkText(input.text, w.MaxChunkChars)
		var chunkStartOffset uint64
		for i, chunk := range chunks {
			chunkResult, analyzeErr := w.Analyzer.Analyze(ctx, analyzer.AnalyzeRequest{Language: a.Language, Document: analyzer.SourceDocument{ID: input.id, SourceIdentifier: input.identifier, Title: input.title, Text: chunk}})
			if analyzeErr != nil {
				return fmt.Errorf("snapshot analysis failed: %w", analyzeErr)
			}
			if completedChunks == 0 {
				merged = chunkResult
				merged.Sentences = nil
				merged.SourceDocuments = nil
			}
			offsetResultLocations(&chunkResult, chunkStartOffset)
			merged.Sentences = append(merged.Sentences, chunkResult.Sentences...)
			if i == 0 {
				merged.SourceDocuments = append(merged.SourceDocuments, chunkResult.SourceDocuments...)
			}
			chunkStartOffset += uint64(len([]rune(chunk)))
			completedChunks++
			progress := 10 + completedChunks*80/totalChunks
			if _, updateErr := w.Pool.Exec(ctx, `UPDATE analysis_jobs SET progress=$2,updated_at=now() WHERE owner_id=$1 AND analysis_run_id=$3`, a.OwnerID, progress, a.RunID); updateErr != nil {
				return updateErr
			}
		}
	}
	artifactHash := normalizedArtifactHash(a.ContentHash, "", a.AnalysisIdentity, a.AnalyzerName, a.AnalyzerVersion, a.ConfigIdentity, merged)
	lemmas := aggregateLemmas(artifactHash, merged)
	statistics := selection.AnalyzableStatistics(merged, selection.DefaultConfig(""))
	artifact := domain.NormalizedArtifact{ContentHash: artifactHash, Language: merged.Language, SchemaVersion: merged.SchemaVersion, NormalizationProfile: merged.NormalizationProfile.Name, NormalizationVersion: merged.NormalizationProfile.Version, AnalyzerName: merged.Analysis.AnalyzerName, AnalyzerVersion: merged.Analysis.AnalyzerVersion}
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `SELECT state FROM analysis_runs WHERE owner_id=$1 AND id=$2 FOR UPDATE`, a.OwnerID, a.RunID).Scan(&runState); err != nil {
		return err
	}
	if runState == "cancelled" || runState == "completed" {
		return nil
	}
	if _, err = tx.Exec(ctx, `INSERT INTO normalized_corpus_artifacts(content_hash,language,schema_version,normalization_profile,normalization_version,analyzer_name,analyzer_version) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(content_hash) DO NOTHING`, artifact.ContentHash, artifact.Language, artifact.SchemaVersion, artifact.NormalizationProfile, artifact.NormalizationVersion, artifact.AnalyzerName, artifact.AnalyzerVersion); err != nil {
		return err
	}
	for _, lemma := range lemmas {
		if _, err = tx.Exec(ctx, `INSERT INTO shared_lemmas(content_hash,language,canonical_lemma,upos,morphology,frequency) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(content_hash,canonical_lemma,upos,morphology) DO UPDATE SET frequency=excluded.frequency`, lemma.ContentHash, lemma.Language, lemma.CanonicalLemma, lemma.UPOS, lemma.Morphology, lemma.Frequency); err != nil {
			return err
		}
	}
	profile := statistics.TextProfile
	var corpusID string
	err = tx.QueryRow(ctx, `INSERT INTO corpora(owner_id,source_material_id,artifact_hash,analysis_run_id,status,analyzable_token_count,distinct_lemma_count,sentence_count,normalized_token_count,empty_sentence_count,median_sentence_token_count,p90_sentence_token_count,long_sentence_count)
		VALUES($1,$2,$3,$4::uuid,'complete',$5,$6,$7,$8,$9,$10,$11,$12)
		ON CONFLICT(owner_id,analysis_run_id) WHERE analysis_run_id IS NOT NULL DO NOTHING RETURNING id::text`, a.OwnerID, a.SourceMaterialID, artifactHash, a.RunID, statistics.AnalyzableTokenCount, statistics.DistinctLemmaCount, profile.SentenceCount, profile.NormalizedTokenCount, profile.EmptySentenceCount, profile.MedianSentenceTokenCount, profile.P90SentenceTokenCount, profile.LongSentenceCount).Scan(&corpusID)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = tx.QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND analysis_run_id=$2`, a.OwnerID, a.RunID).Scan(&corpusID); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if err = persistNormalizedCorpus(ctx, tx, a.OwnerID, merged.Language, a.RunID, corpusID, merged); err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]any{"run_id": a.RunID, "attempt": a.Attempt})
	if _, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'analysis','complete',$3,now())`, a.OwnerID, corpusID, details); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE analysis_runs SET state='completed',corpus_id=$3::uuid,last_error='',updated_at=now(),completed_at=now() WHERE owner_id=$1 AND id=$2 AND state<>'cancelled'`, a.OwnerID, a.RunID, corpusID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE analysis_run_attempts SET state='completed',error='',finalized_at=now() WHERE run_id=$1 AND river_job_id=$2 AND state<>'cancelled'`, a.RunID, job.ID); err != nil {
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
	if bookID != "" {
		var eligible bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1
			FROM analysis_runs r
			JOIN source_materials s ON s.owner_id=r.owner_id AND s.id=r.source_material_id AND s.book_id=$3
			JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id AND c.source_material_id=r.source_material_id AND c.analysis_run_id=r.id AND c.status='complete'
			WHERE r.owner_id=$1 AND r.id=$2 AND r.state='completed'
			  AND s.current_content_revision_id=r.content_revision_id AND s.current_snapshot_id=r.snapshot_id
		)`, a.OwnerID, a.RunID, bookID).Scan(&eligible); err != nil {
			return err
		}
		if eligible {
			if _, err = tx.Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id,promoted_at)
				VALUES($1,$2,$3,$4,now())
				ON CONFLICT(owner_id,book_id) DO UPDATE SET source_material_id=excluded.source_material_id,analysis_run_id=excluded.analysis_run_id,promoted_at=excluded.promoted_at`, a.OwnerID, bookID, a.SourceMaterialID, a.RunID); err != nil {
				return err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	selectionConfig := selection.DefaultConfig(corpusID)
	if _, err = w.Selection.Select(ctx, a.OwnerID, merged, selectionConfig); err != nil {
		w.recordCandidateGenerationFailure(ctx, a.OwnerID, corpusID, job.ID, "selection", err)
	}
	return nil
}

const (
	normalizedCorpusSentenceInsertBatchSize = 500
	normalizedCorpusTokenInsertBatchSize    = 4_000
)

func persistNormalizedCorpus(ctx context.Context, tx pgx.Tx, ownerID, language, runID, corpusID string, result analyzer.Result) error {
	for start := 0; start < len(result.Sentences); start += normalizedCorpusSentenceInsertBatchSize {
		end := min(start+normalizedCorpusSentenceInsertBatchSize, len(result.Sentences))
		var query strings.Builder
		query.WriteString(`INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES `)
		args := make([]any, 0, (end-start)*8)
		for ordinal := start; ordinal < end; ordinal++ {
			if ordinal > start {
				query.WriteString(",")
			}
			sentence := result.Sentences[ordinal]
			arg := len(args) + 1
			fmt.Fprintf(&query, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", arg, arg+1, arg+2, arg+3, arg+4, arg+5, arg+6, arg+7)
			args = append(args, ownerID, runID, corpusID, sentence.Location.SourceDocumentID, int64(ordinal), sentence.Text, int64(sentence.Location.StartOffset), int64(sentence.Location.EndOffset))
		}
		if _, err := tx.Exec(ctx, query.String(), args...); err != nil {
			return err
		}
	}

	var query strings.Builder
	var args []any
	tokenCount := 0
	flushTokens := func() error {
		if tokenCount == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx, query.String(), args...); err != nil {
			return err
		}
		query.Reset()
		args = args[:0]
		tokenCount = 0
		return nil
	}
	for sentenceOrdinal, sentence := range result.Sentences {
		for tokenOrdinal, token := range sentence.Tokens {
			if tokenCount == normalizedCorpusTokenInsertBatchSize {
				if err := flushTokens(); err != nil {
					return err
				}
			}
			if tokenCount == 0 {
				query.WriteString(`INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,named_entity,start_offset,end_offset) VALUES `)
			} else {
				query.WriteString(",")
			}
			morphology := token.Morphology
			if morphology == nil {
				morphology = map[string]string{}
			}
			morphologyJSON, err := json.Marshal(morphology)
			if err != nil {
				return err
			}
			arg := len(args) + 1
			fmt.Fprintf(&query, "($%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d,$%d)", arg, arg+1, arg+2, arg+3, arg+4, arg+5, arg+6, arg+7, arg+8, arg+9, arg+10, arg+11, arg+12, arg+13)
			var namedEntity any
			if token.NamedEntity != nil {
				namedEntity = *token.NamedEntity
			}
			args = append(args, ownerID, language, runID, corpusID, int64(sentenceOrdinal), int64(tokenOrdinal), token.Surface, token.RawLemma, token.CanonicalLemma, token.UPOS, morphologyJSON, namedEntity, int64(token.Location.StartOffset), int64(token.Location.EndOffset))
			tokenCount++
		}
	}
	if err := flushTokens(); err != nil {
		return err
	}
	return nil
}

func (w *Worker) recordCandidateGenerationFailure(ctx context.Context, owner, corpusID string, jobID int64, stage string, cause error) {
	details, _ := json.Marshal(map[string]any{"river_job_id": jobID, "stage": stage, "error": cause.Error()})
	failureCtx := context.WithoutCancel(ctx)
	_, _ = w.Pool.Exec(failureCtx, `INSERT INTO processing_history(owner_id,corpus_id,operation,status,details,completed_at) VALUES($1,$2,'candidate_generation','failed',$3,now())`, owner, corpusID, details)
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

func aggregateLemmas(hash string, result analyzer.Result) []domain.SharedLemma {
	type entry struct {
		lemma analyzer.Token
		count int64
	}
	values := map[string]entry{}
	for _, sentence := range result.Sentences {
		for _, token := range sentence.Tokens {
			if !lexical.IsLemma(token.CanonicalLemma) {
				continue
			}
			raw, _ := json.Marshal(token.Morphology)
			key := token.CanonicalLemma + "\x00" + token.UPOS + "\x00" + string(raw)
			e := values[key]
			e.lemma = token
			e.count++
			values[key] = e
		}
	}
	out := make([]domain.SharedLemma, 0, len(values))
	for _, e := range values {
		raw, _ := json.Marshal(e.lemma.Morphology)
		out = append(out, domain.SharedLemma{ContentHash: hash, Language: result.Language, CanonicalLemma: e.lemma.CanonicalLemma, UPOS: e.lemma.UPOS, Morphology: raw, Frequency: e.count})
	}
	return out
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
	if standardWorkers < 1 {
		standardWorkers = 1
	}
	return river.NewClient(riverpgxv5.New(pool), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, "catalogue_sync": {MaxWorkers: 1}, "known_vocabulary": {MaxWorkers: 1}, "prepared_decks": {MaxWorkers: 1}, "prepared_deck_translation": {MaxWorkers: standardWorkers}}, Workers: workers, JobTimeout: jobTimeout})
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
