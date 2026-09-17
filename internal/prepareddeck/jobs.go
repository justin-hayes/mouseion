// Package prepareddeck runs durable, owner-scoped deck preparation as River work.
package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

const Queue = "prepared_decks"
const TranslationQueue = "prepared_deck_translation"

const orphanedPreparationError = "the preparation worker is no longer active; retry the preparation"

var livePreparationJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateRetryable,
	rivertype.JobStateScheduled,
}

var ErrInvalidInput = errors.New("prepareddeck: invalid input")
var ErrAnalysisUnavailable = errors.New("prepareddeck: completed scoped analysis required")

type JobArgs struct {
	// PreparationID is the logical idempotency key. The other fields are
	// immutable worker input or request options and must not create competing
	// jobs for the same preparation.
	PreparationID              string `json:"preparation_id" river:"unique"`
	OwnerID                    string `json:"owner_id"`
	SourceMaterialID           string `json:"source_material_id"`
	ContentHash                string `json:"content_hash"`
	AnalysisRunID              string `json:"analysis_run_id,omitempty"`
	ExternalTranslationConsent bool   `json:"external_translation_consent"`
}

// StandardTranslationWorker executes one durable manifest item per River
// invocation. Its dependencies are optional so worker registration tests can
// still validate the job type without a database or provider.
type StandardTranslationWorker struct {
	river.WorkerDefaults[StandardTranslationJobArgs]
	Store          *persistence.PostgresStore
	Client         riverClient
	Provider       enrichment.TranslationProvider
	Metrics        BatchMetrics
	Config         PreparedDeckConfig
	AttemptTimeout time.Duration
	Now            func() time.Time
	Jitter         func(time.Duration) time.Duration
}

func (w *StandardTranslationWorker) Work(ctx context.Context, job *river.Job[StandardTranslationJobArgs]) error {
	if job == nil {
		return ErrInvalidInput
	}
	return w.execute(ctx, job.Args)
}

func AddStandardTranslationWorker(workers *river.Workers) {
	river.AddWorker(workers, &StandardTranslationWorker{})
}

func AddStandardTranslationWorkerWithDependencies(workers *river.Workers, store *persistence.PostgresStore, client riverClient, provider enrichment.TranslationProvider, config PreparedDeckConfig, timeout time.Duration, metrics ...BatchMetrics) {
	var collector BatchMetrics
	if len(metrics) > 0 {
		collector = metrics[0]
	}
	river.AddWorker(workers, &StandardTranslationWorker{Store: store, Client: client, Provider: provider, Config: config, AttemptTimeout: timeout, Metrics: collector})
}

func (JobArgs) Kind() string { return "prepared_deck" }

type Handle struct {
	Preparation domain.DeckPreparation
	JobID       int64
}

type Service struct {
	pool           *pgxpool.Pool
	client         riverClient
	store          *persistence.PostgresStore
	batchCanceller batchCanceller
}

type batchCanceller interface {
	CancelBatch(context.Context, string) (enrichment.Batch, error)
}

type riverClient interface {
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
	JobCancel(context.Context, int64) (*rivertype.JobRow, error)
}

func NewService(store *persistence.PostgresStore, client *river.Client[pgx.Tx]) *Service {
	return &Service{pool: store.Pool(), client: client, store: store}
}

func NewServiceWithBatchCanceller(store *persistence.PostgresStore, client *river.Client[pgx.Tx], canceller batchCanceller) *Service {
	service := NewService(store, client)
	service.batchCanceller = canceller
	return service
}

// Submit creates a preparation for one completed scoped analysis and its
// River job in one transaction. The analysis run and content hash in the job
// freeze the immutable input used by all retries.
func (s *Service) Submit(ctx context.Context, owner, analysisID string, consent bool) (Handle, error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(analysisID) == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback(ctx)
	analysis, err := loadCompletedAnalysis(ctx, tx, owner, analysisID)
	if err != nil {
		return Handle{}, err
	}
	source := analysis.Source
	deckName := cardexport.DeckName(source.Language, source.Title)
	filename := cardexport.DownloadFilename(source.Title)
	p, created, err := persistence.CreateDeckPreparationTx(ctx, tx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, AnalysisRunID: analysis.RunID, Filename: filename, DeckName: deckName, ContentHash: source.ContentHash})
	if err != nil {
		return Handle{}, err
	}
	p, jobID, err := s.ensurePreparationJob(ctx, tx, p, consent)
	if err != nil {
		if !created {
			p, err = markPreparationFailedTx(ctx, tx, owner, p.ID, fmt.Sprintf("could not enqueue preparation: %v", err))
			if err != nil {
				return Handle{}, err
			}
			if err = tx.Commit(ctx); err != nil {
				return Handle{}, err
			}
			return Handle{Preparation: p}, nil
		}
		return Handle{}, fmt.Errorf("enqueue prepared deck: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{Preparation: p, JobID: jobID}, nil
}

type completedAnalysis struct {
	RunID  string
	Source domain.SourceMaterial
}

func loadCompletedAnalysis(ctx context.Context, q queryRower, owner, analysisID string) (completedAnalysis, error) {
	var result completedAnalysis
	var runState, snapshotID, corpusID string
	var runRevisionID, jobContentHash, jobCorpusID string
	var corpusOwner, corpusSource, corpusRun, corpusStatus string
	var contentDigest string
	var runID string
	condition := "j.river_job_id=$2"
	args := []any{owner}
	if parsed, err := strconv.ParseInt(analysisID, 10, 64); err == nil && parsed > 0 {
		args = append(args, parsed)
	} else {
		if _, err := uuid.Parse(strings.TrimSpace(analysisID)); err != nil {
			return completedAnalysis{}, fmt.Errorf("%w: analysis result ID must be a result ID", ErrAnalysisUnavailable)
		}
		condition = "r.id=$2::uuid"
		args = append(args, strings.TrimSpace(analysisID))
	}
	err := q.QueryRow(ctx, `SELECT COALESCE(j.analysis_run_id::text,''),s.id::text,s.owner_id::text,s.language,s.source_identifier,s.title,s.media_type,j.content_hash,COALESCE(j.corpus_id::text,''),
		COALESCE(rev.content_digest,''),
		COALESCE(r.state,''),COALESCE(r.snapshot_id::text,''),COALESCE(r.content_revision_id::text,''),COALESCE(r.corpus_id::text,''),
		COALESCE(c.owner_id::text,''),COALESCE(c.source_material_id::text,''),COALESCE(c.analysis_run_id::text,''),COALESCE(c.status,'')
		FROM analysis_jobs j
		JOIN source_materials s ON s.owner_id=j.owner_id AND s.id=j.source_material_id
		LEFT JOIN analysis_runs r ON r.owner_id=j.owner_id AND r.id=j.analysis_run_id AND r.source_material_id=j.source_material_id
		LEFT JOIN source_content_revisions rev ON rev.owner_id=r.owner_id AND rev.source_material_id=r.source_material_id AND rev.revision_id=r.content_revision_id
		LEFT JOIN corpora c ON c.owner_id=r.owner_id AND c.id=r.corpus_id AND c.source_material_id=r.source_material_id
		WHERE j.owner_id=$1 AND `+condition, args...).Scan(&runID, &result.Source.ID, &result.Source.OwnerID, &result.Source.Language, &result.Source.SourceIdentifier, &result.Source.Title, &result.Source.MediaType, &jobContentHash, &jobCorpusID, &contentDigest, &runState, &snapshotID, &runRevisionID, &corpusID, &corpusOwner, &corpusSource, &corpusRun, &corpusStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return completedAnalysis{}, fmt.Errorf("%w: analysis result is missing or belongs to another owner", ErrAnalysisUnavailable)
	}
	if err != nil {
		return completedAnalysis{}, fmt.Errorf("load analysis result: %w", err)
	}
	if runID == "" {
		return completedAnalysis{}, fmt.Errorf("%w: legacy analysis results cannot prepare a deck", ErrAnalysisUnavailable)
	}
	checks := []struct {
		valid   bool
		message string
	}{
		{runState == "completed", "analysis is not completed (current state: " + runState + ")"},
		{runRevisionID != "" && contentDigest != "" && jobContentHash == contentDigest, "analysis content revision is missing or contradictory"},
		{corpusID != "" && jobCorpusID == corpusID && corpusOwner == result.Source.OwnerID && corpusSource == result.Source.ID && corpusRun == runID && corpusStatus == "complete", "analysis has no completed matching corpus"},
	}
	for _, check := range checks {
		if !check.valid {
			return completedAnalysis{}, fmt.Errorf("%w: %s", ErrAnalysisUnavailable, check.message)
		}
	}
	result.Source.ContentHash = jobContentHash
	result.RunID = runID
	return result, nil
}

func (s *Service) Get(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.Reconcile(ctx, owner, id)
}

// GetForAnalysis returns the preparation for one exact owner-scoped analysis.
// Reconciliation is still performed through the preparation identity so a
// status read can repair a missing queued job without weakening the result
// binding.
func (s *Service) GetForAnalysis(ctx context.Context, owner, sourceMaterialID, analysisRunID string) (domain.DeckPreparation, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(sourceMaterialID) == "" || strings.TrimSpace(analysisRunID) == "" {
		return domain.DeckPreparation{}, ErrInvalidInput
	}
	p, err := s.store.GetDeckPreparationForAnalysis(ctx, owner, sourceMaterialID, analysisRunID)
	if err != nil {
		return p, err
	}
	return s.Get(ctx, owner, p.ID)
}

// Reconcile makes a waiting preparation actionable. A queued preparation with
// no live River job is safely re-enqueued. A preparing preparation without a
// live job is failed because it may have been claimed by a worker that died;
// rerunning it would not be safe without a durable attempt lease.
func (s *Service) Reconcile(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(id) == "" {
		return domain.DeckPreparation{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanPreparation(tx.QueryRow(ctx, `SELECT `+preparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeckPreparation{}, persistence.ErrNotFound
	}
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	p, _, err = s.ensurePreparationJob(ctx, tx, p, false)
	if err != nil {
		p, err = markPreparationFailedTx(ctx, tx, owner, id, fmt.Sprintf("could not enqueue preparation: %v", err))
		if err != nil {
			return domain.DeckPreparation{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeckPreparation{}, err
	}
	if p.CurrentRunID != "" {
		return s.store.GetDeckPreparationStatus(ctx, owner, id)
	}
	return p, nil
}

// Download returns the persisted artifact only after preparation is ready.
// It deliberately delegates to the read-only persistence operation so HTTP
// downloads can never trigger providers, rendering, or study-state changes.
func (s *Service) Download(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.store.DownloadDeckPreparation(ctx, owner, id)
}

// Rerender queues a presentation-only rebuild for a completed preparation. The
// run identity is captured before enqueueing so a newer preparation cannot be
// rendered into this artifact.
func (s *Service) Rerender(ctx context.Context, owner, id string) (Handle, error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(id) == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanPreparation(tx.QueryRow(ctx, `SELECT `+preparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, persistence.ErrNotFound
	}
	if err != nil {
		return Handle{}, err
	}
	if p.State != domain.DeckPreparationReady || p.RetiredAt != nil || p.CurrentRunID == "" {
		return Handle{}, persistence.ErrInvalidTransition
	}
	if p.PresentationVersion >= cardexport.PresentationVersion {
		if err = tx.Commit(ctx); err != nil {
			return Handle{}, err
		}
		return Handle{Preparation: p}, nil
	}
	args := RerenderJobArgs{OwnerID: owner, PreparationID: id, RunID: p.CurrentRunID, PresentationVersion: cardexport.PresentationVersion}
	result, err := s.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
	if err != nil {
		return Handle{}, err
	}
	var jobID int64
	if result != nil && result.Job != nil && isLivePreparationJobState(result.Job.State) {
		jobID = result.Job.ID
	} else if confirmed, confirmErr := liveRerenderJobID(ctx, tx, owner, id, p.CurrentRunID, cardexport.PresentationVersion); confirmErr == nil {
		jobID = confirmed
	} else if !errors.Is(confirmErr, pgx.ErrNoRows) {
		return Handle{}, confirmErr
	} else {
		return Handle{}, errors.New("River did not return a live rerender job")
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{Preparation: p, JobID: jobID}, nil
}

func liveRerenderJobID(ctx context.Context, q queryRower, owner, preparationID, runID string, presentationVersion int) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'preparation_id'=$3 AND args->>'run_id'=$4 AND (args->>'presentation_version')::integer=$5 AND state IN ('available','pending','running','retryable','scheduled') ORDER BY id DESC LIMIT 1`, (RerenderJobArgs{}).Kind(), owner, preparationID, runID, presentationVersion).Scan(&id)
	return id, err
}

func (s *Service) Cancel(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	p, err := s.store.CancelDeckPreparation(ctx, owner, id)
	if err != nil {
		return p, err
	}
	// Local state commits first. Provider and River cancellation are explicitly
	// best effort because their work is fenced by the cancelled run.
	rows, queryErr := s.pool.Query(ctx, `SELECT id FROM river_job WHERE args->>'owner_id'=$1 AND args->>'preparation_id'=$2 AND state IN ('available','pending','running','retryable','scheduled') ORDER BY id`, owner, id)
	if queryErr == nil {
		var jobIDs []int64
		for rows.Next() {
			var jobID int64
			if scanErr := rows.Scan(&jobID); scanErr != nil {
				break
			}
			jobIDs = append(jobIDs, jobID)
		}
		rows.Close()
		if s.client != nil {
			for _, jobID := range jobIDs {
				_, _ = s.client.JobCancel(ctx, jobID)
			}
		}
	}
	if s.batchCanceller != nil && p.CurrentRunID != "" {
		if batchIDs, listErr := s.store.ListPreparedDeckLiveBatchIDs(ctx, owner, id, p.CurrentRunID); listErr == nil {
			for _, batchID := range batchIDs {
				_, _ = s.batchCanceller.CancelBatch(ctx, batchID)
			}
		}
		if cleaner, ok := s.batchCanceller.(batchFileDeleter); ok {
			if chunks, listErr := s.store.ListPreparedDeckBatchChunks(ctx, owner, id, p.CurrentRunID); listErr == nil {
				cleanup := &BatchCleanupWorker{Store: s.store, Provider: cleaner}
				for _, chunk := range chunks {
					_ = cleanup.Cleanup(context.WithoutCancel(ctx), BatchCleanupJobArgs{OwnerID: owner, PreparationID: id, RunID: p.CurrentRunID, ChunkID: chunk.ID, Generation: chunk.Generation})
				}
			}
		}
	}
	return p, nil
}

func (s *Service) Retry(ctx context.Context, owner, id string, consent bool) (Handle, error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(id) == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanPreparation(tx.QueryRow(ctx, `SELECT `+preparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, persistence.ErrNotFound
	}
	if err != nil {
		return Handle{}, err
	}
	if p.State == domain.DeckPreparationFailed || p.State == domain.DeckPreparationCancelled {
		previousState := p.State
		p, err = scanPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET state='queued',error='',current_run_id=NULL,started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+preparationColumns, owner, id))
		if err != nil {
			return Handle{}, err
		}
		if err = recordPreparationHistoryTx(ctx, tx, owner, id, string(domain.DeckPreparationQueued), map[string]any{"from": previousState}); err != nil {
			return Handle{}, err
		}
	} else if p.State != domain.DeckPreparationQueued && p.State != domain.DeckPreparationPreparing {
		return Handle{}, persistence.ErrInvalidTransition
	}
	p, jobID, err := s.ensurePreparationJob(ctx, tx, p, consent)
	if err != nil {
		p, err = markPreparationFailedTx(ctx, tx, owner, id, fmt.Sprintf("could not enqueue preparation: %v", err))
		if err != nil {
			return Handle{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{Preparation: p, JobID: jobID}, nil
}

type queryRower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func (s *Service) ensurePreparationJob(ctx context.Context, tx pgx.Tx, p domain.DeckPreparation, consent bool) (domain.DeckPreparation, int64, error) {
	jobID, err := livePreparationJobID(ctx, tx, p.OwnerID, p.ID)
	if err == nil {
		return p, jobID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return p, 0, err
	}
	if p.State == domain.DeckPreparationQueued {
		result, insertErr := s.client.InsertTx(ctx, tx, JobArgs{PreparationID: p.ID, OwnerID: p.OwnerID, SourceMaterialID: p.SourceMaterialID, ContentHash: p.ContentHash, AnalysisRunID: p.AnalysisRunID, ExternalTranslationConsent: consent}, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
		if insertErr != nil {
			return p, 0, insertErr
		}
		if result == nil || result.Job == nil || !isLivePreparationJobState(result.Job.State) {
			// River may have skipped a duplicate insert and not return the
			// existing row. Confirm the job from the same transaction before
			// treating the submission as unsafe; otherwise a concurrent submit
			// can incorrectly fail an otherwise live preparation.
			if confirmedJobID, confirmErr := livePreparationJobID(ctx, tx, p.OwnerID, p.ID); confirmErr == nil {
				return p, confirmedJobID, nil
			} else if !errors.Is(confirmErr, pgx.ErrNoRows) {
				return p, 0, confirmErr
			}
			return p, 0, errors.New("River did not return a live preparation job")
		}
		return p, result.Job.ID, nil
	}
	if p.State == domain.DeckPreparationPreparing {
		// A committed durable run owns its recovery. Status polling must not
		// turn a long-lived Batch into the legacy orphan failure path.
		if p.CurrentRunID != "" {
			return p, 0, nil
		}
		failed, failErr := markPreparationFailedTx(ctx, tx, p.OwnerID, p.ID, orphanedPreparationError)
		return failed, 0, failErr
	}
	return p, 0, nil
}

func livePreparationJobID(ctx context.Context, q queryRower, owner, preparationID string) (int64, error) {
	var id int64
	err := q.QueryRow(ctx, `SELECT id FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'preparation_id'=$3 AND state IN ('available','pending','running','retryable','scheduled') ORDER BY id DESC LIMIT 1`, (JobArgs{}).Kind(), owner, preparationID).Scan(&id)
	return id, err
}

func isLivePreparationJobState(state rivertype.JobState) bool {
	return slices.Contains(livePreparationJobStates, state)
}

func markPreparationFailedTx(ctx context.Context, tx pgx.Tx, owner, id, message string) (domain.DeckPreparation, error) {
	p, err := scanPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET state='failed',error=$3,completed_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND state IN ('queued','preparing') RETURNING `+preparationColumns, owner, id, message))
	if err != nil {
		return p, err
	}
	if err = recordPreparationHistoryTx(ctx, tx, owner, id, string(domain.DeckPreparationFailed), map[string]any{"error": message}); err != nil {
		return domain.DeckPreparation{}, err
	}
	return p, nil
}

func recordPreparationHistoryTx(ctx context.Context, tx pgx.Tx, owner, id, status string, details map[string]any) error {
	details["preparation_id"] = id
	encoded, err := json.Marshal(details)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO processing_history(owner_id,operation,status,details,completed_at) VALUES($1,'prepared_deck',$2,$3,now())`, owner, status, encoded)
	return err
}

const preparationColumns = `id::text,owner_id::text,source_material_id::text,COALESCE(analysis_run_id::text,''),COALESCE(current_run_id::text,''),state,artifact,filename,deck_name,content_hash,total_cards,cards_with_english,cards_with_contextual_sentence_translations,quality_omissions,error,created_at,updated_at,started_at,completed_at,studying_at,reviewed_at,graduated_at,released_at,retired_at,cards_with_fallback_gloss,render_input_version,presentation_version,deck_revision`

type rowScanner interface{ Scan(...any) error }

func scanPreparation(row rowScanner) (domain.DeckPreparation, error) {
	var p domain.DeckPreparation
	err := row.Scan(&p.ID, &p.OwnerID, &p.SourceMaterialID, &p.AnalysisRunID, &p.CurrentRunID, &p.State, &p.Artifact, &p.Filename, &p.DeckName, &p.ContentHash, &p.TotalCards, &p.CardsWithEnglish, &p.CardsWithContextualSentenceTranslations, &p.QualityOmissions, &p.Error, &p.CreatedAt, &p.UpdatedAt, &p.StartedAt, &p.CompletedAt, &p.StudyingAt, &p.ReviewedAt, &p.GraduatedAt, &p.ReleasedAt, &p.RetiredAt, &p.CardsWithFallbackGloss, &p.RenderInputVersion, &p.PresentationVersion, &p.DeckRevision)
	return p, err
}
