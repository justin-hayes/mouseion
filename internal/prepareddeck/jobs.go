// Package prepareddeck runs durable, owner-scoped deck preparation as River work.
package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

const orphanedPreparationError = "the preparation worker is no longer active; retry the preparation"

var livePreparationJobStates = []rivertype.JobState{
	rivertype.JobStateAvailable,
	rivertype.JobStatePending,
	rivertype.JobStateRunning,
	rivertype.JobStateRetryable,
	rivertype.JobStateScheduled,
}

var ErrInvalidInput = errors.New("prepareddeck: invalid input")

type JobArgs struct {
	// PreparationID is the logical idempotency key. The other fields are
	// immutable worker input or request options and must not create competing
	// jobs for the same preparation.
	PreparationID              string `json:"preparation_id" river:"unique"`
	OwnerID                    string `json:"owner_id"`
	SourceMaterialID           string `json:"source_material_id"`
	ContentHash                string `json:"content_hash"`
	ExternalTranslationConsent bool   `json:"external_translation_consent"`
}

func (JobArgs) Kind() string { return "prepared_deck" }

type Handle struct {
	Preparation domain.DeckPreparation
	JobID       int64
}

type Service struct {
	pool   *pgxpool.Pool
	client riverClient
	store  *persistence.PostgresStore
}

type riverClient interface {
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
	JobCancel(context.Context, int64) (*rivertype.JobRow, error)
}

func NewService(store *persistence.PostgresStore, client *river.Client[pgx.Tx]) *Service {
	return &Service{pool: store.Pool(), client: client, store: store}
}

// Submit creates the preparation and its River job in one transaction. The
// content hash in the job freezes the source identity used by all retries.
func (s *Service) Submit(ctx context.Context, owner, sourceID string, consent bool) (Handle, error) {
	if s == nil || s.pool == nil || s.client == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(sourceID) == "" {
		return Handle{}, ErrInvalidInput
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback(ctx)
	var source domain.SourceMaterial
	err = tx.QueryRow(ctx, `SELECT id::text,owner_id::text,language,source_identifier,title,media_type,content_hash,content,full_text,created_at FROM source_materials WHERE owner_id=$1 AND id=$2`, owner, sourceID).
		Scan(&source.ID, &source.OwnerID, &source.Language, &source.SourceIdentifier, &source.Title, &source.MediaType, &source.ContentHash, &source.Content, &source.FullText, &source.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Handle{}, persistence.ErrNotFound
	}
	if err != nil {
		return Handle{}, err
	}
	deckName := cardexport.DeckName(source.Language, source.Title)
	filename := cardexport.DownloadFilename(source.Title)
	row := tx.QueryRow(ctx, `INSERT INTO deck_preparations(owner_id,source_material_id,filename,deck_name,content_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,source_material_id,content_hash) DO NOTHING RETURNING `+preparationColumns, owner, sourceID, filename, deckName, source.ContentHash)
	p, err := scanPreparation(row)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		p, err = scanPreparation(tx.QueryRow(ctx, `SELECT `+preparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND source_material_id=$2 AND content_hash=$3 FOR UPDATE`, owner, sourceID, source.ContentHash))
	}
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

func (s *Service) Get(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.Reconcile(ctx, owner, id)
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
	return p, nil
}

// Download returns the persisted artifact only after preparation is ready.
// It deliberately delegates to the read-only persistence operation so HTTP
// downloads can never trigger providers, rendering, or study-state changes.
func (s *Service) Download(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.store.DownloadDeckPreparation(ctx, owner, id)
}

func (s *Service) Cancel(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	p, err := s.store.CancelDeckPreparation(ctx, owner, id)
	if err != nil {
		return p, err
	}
	var jobID int64
	if err = s.pool.QueryRow(ctx, `SELECT id FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND args->>'preparation_id'=$3 AND state IN ('available','pending','running','retryable','scheduled') ORDER BY id DESC LIMIT 1`, (JobArgs{}).Kind(), owner, id).Scan(&jobID); err == nil {
		_, _ = s.client.JobCancel(ctx, jobID)
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return p, err
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
		p, err = scanPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET state='queued',error='',started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2 RETURNING `+preparationColumns, owner, id))
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
		result, insertErr := s.client.InsertTx(ctx, tx, JobArgs{PreparationID: p.ID, OwnerID: p.OwnerID, SourceMaterialID: p.SourceMaterialID, ContentHash: p.ContentHash, ExternalTranslationConsent: consent}, &river.InsertOpts{Queue: Queue, MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
		if insertErr != nil {
			return p, 0, insertErr
		}
		if result == nil || result.Job == nil || !isLivePreparationJobState(result.Job.State) {
			return p, 0, errors.New("River did not return a live preparation job")
		}
		return p, result.Job.ID, nil
	}
	if p.State == domain.DeckPreparationPreparing {
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
	for _, live := range livePreparationJobStates {
		if state == live {
			return true
		}
	}
	return false
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

type preparationStore interface {
	ClaimDeckPreparation(context.Context, string, string) (domain.DeckPreparation, error)
	GetSourceMaterial(context.Context, string, string) (domain.SourceMaterial, error)
	FailDeckPreparation(context.Context, string, string, string) (domain.DeckPreparation, error)
	CompletePreparedDeck(context.Context, string, string, cardexport.Artifact) (domain.DeckPreparation, error)
}

type builder interface {
	BuildCoverage(context.Context, string, string) (cardexport.Artifact, error)
}

type externalEnricher interface {
	ExternalConfigured() bool
	EnrichExternal(context.Context, enrichment.Candidate) (enrichment.Result, error)
}

type Worker struct {
	river.WorkerDefaults[JobArgs]
	Store      preparationStore
	Builder    builder
	Enrichment externalEnricher
}

func (w *Worker) Work(ctx context.Context, job *river.Job[JobArgs]) error {
	a := job.Args
	p, err := w.Store.ClaimDeckPreparation(ctx, a.OwnerID, a.PreparationID)
	if errors.Is(err, persistence.ErrInvalidTransition) {
		return nil // already completed, cancelled, failed, or claimed by another attempt
	}
	if err != nil {
		return err
	}
	fail := func(cause error) error {
		_, transitionErr := w.Store.FailDeckPreparation(context.WithoutCancel(ctx), a.OwnerID, a.PreparationID, cause.Error())
		if transitionErr != nil && !errors.Is(transitionErr, persistence.ErrInvalidTransition) {
			return fmt.Errorf("%v; mark preparation failed: %w", cause, transitionErr)
		}
		return nil
	}
	source, err := w.Store.GetSourceMaterial(ctx, a.OwnerID, a.SourceMaterialID)
	if err != nil {
		return fail(fmt.Errorf("load source material: %w", err))
	}
	if p.SourceMaterialID != a.SourceMaterialID || p.ContentHash != a.ContentHash || source.ContentHash != a.ContentHash {
		return fail(errors.New("source material identity changed"))
	}
	artifact, err := w.Builder.BuildCoverage(ctx, a.OwnerID, a.SourceMaterialID)
	if err != nil {
		return fail(fmt.Errorf("build prepared deck: %w", err))
	}
	if a.ExternalTranslationConsent && w.Enrichment != nil && w.Enrichment.ExternalConfigured() {
		for _, candidate := range artifact.EnrichmentCandidates {
			if _, err = w.Enrichment.EnrichExternal(ctx, candidate); err != nil {
				return fail(fmt.Errorf("contextual translation %s/%s: %w", candidate.CanonicalLemma, candidate.UPOS, err))
			}
		}
		artifact, err = w.Builder.BuildCoverage(ctx, a.OwnerID, a.SourceMaterialID)
		if err != nil {
			return fail(fmt.Errorf("render enriched prepared deck: %w", err))
		}
	}
	if _, err = w.Store.CompletePreparedDeck(ctx, a.OwnerID, a.PreparationID, artifact); err != nil {
		if errors.Is(err, persistence.ErrInvalidTransition) { // cancellation won the race
			return nil
		}
		return fail(fmt.Errorf("complete prepared deck: %w", err))
	}
	return nil
}

func AddWorker(workers *river.Workers, store *persistence.PostgresStore, export *cardexport.Service, enrich *enrichment.Service) {
	river.AddWorker(workers, &Worker{Store: store, Builder: export, Enrichment: enrich})
}

const preparationColumns = `id::text,owner_id::text,source_material_id::text,state,artifact,filename,deck_name,content_hash,total_cards,cards_with_english,cards_with_contextual_sentence_translations,quality_omissions,error,created_at,updated_at,started_at,completed_at`

type rowScanner interface{ Scan(...any) error }

func scanPreparation(row rowScanner) (domain.DeckPreparation, error) {
	var p domain.DeckPreparation
	err := row.Scan(&p.ID, &p.OwnerID, &p.SourceMaterialID, &p.State, &p.Artifact, &p.Filename, &p.DeckName, &p.ContentHash, &p.TotalCards, &p.CardsWithEnglish, &p.CardsWithContextualSentenceTranslations, &p.QualityOmissions, &p.Error, &p.CreatedAt, &p.UpdatedAt, &p.StartedAt, &p.CompletedAt)
	return p, err
}
