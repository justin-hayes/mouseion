// Package prepareddeck runs durable, owner-scoped deck preparation as River work.
package prepareddeck

import (
	"context"
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
)

const Queue = "prepared_decks"

var ErrInvalidInput = errors.New("prepareddeck: invalid input")

type JobArgs struct {
	PreparationID              string `json:"preparation_id" river:"unique"`
	OwnerID                    string `json:"owner_id" river:"unique"`
	SourceMaterialID           string `json:"source_material_id" river:"unique"`
	ContentHash                string `json:"content_hash" river:"unique"`
	ExternalTranslationConsent bool   `json:"external_translation_consent" river:"unique"`
}

func (JobArgs) Kind() string { return "prepared_deck" }

type Handle struct {
	Preparation domain.DeckPreparation
	JobID       int64
}

type Service struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
	store  *persistence.PostgresStore
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
	source, err := s.store.GetSourceMaterial(ctx, owner, sourceID)
	if err != nil {
		return Handle{}, err
	}
	deckName := cardexport.DeckName(source.Language, source.Title)
	filename := cardexport.DownloadFilename(source.Title)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanPreparation(tx.QueryRow(ctx, `INSERT INTO deck_preparations(owner_id,source_material_id,filename,deck_name,content_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,source_material_id,content_hash) DO UPDATE SET owner_id=excluded.owner_id RETURNING `+preparationColumns, owner, sourceID, filename, deckName, source.ContentHash))
	if err != nil {
		return Handle{}, err
	}
	inserted, err := s.client.InsertTx(ctx, tx, JobArgs{PreparationID: p.ID, OwnerID: owner, SourceMaterialID: sourceID, ContentHash: source.ContentHash, ExternalTranslationConsent: consent}, &river.InsertOpts{Queue: Queue, MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	if err != nil {
		return Handle{}, fmt.Errorf("enqueue prepared deck: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{Preparation: p, JobID: inserted.Job.ID}, nil
}

func (s *Service) Get(ctx context.Context, owner, id string) (domain.DeckPreparation, error) {
	return s.store.GetDeckPreparation(ctx, owner, id)
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
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Handle{}, err
	}
	defer tx.Rollback(ctx)
	p, err := scanPreparation(tx.QueryRow(ctx, `UPDATE deck_preparations SET state='queued',error='',started_at=NULL,completed_at=NULL,updated_at=now() WHERE owner_id=$1 AND id=$2 AND state IN ('failed','cancelled') RETURNING `+preparationColumns, owner, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			if _, getErr := s.store.GetDeckPreparation(ctx, owner, id); getErr != nil {
				return Handle{}, getErr
			}
			return Handle{}, persistence.ErrInvalidTransition
		}
		return Handle{}, err
	}
	inserted, err := s.client.InsertTx(ctx, tx, JobArgs{PreparationID: p.ID, OwnerID: owner, SourceMaterialID: p.SourceMaterialID, ContentHash: p.ContentHash, ExternalTranslationConsent: consent}, &river.InsertOpts{Queue: Queue, MaxAttempts: 1, UniqueOpts: river.UniqueOpts{ByArgs: true}})
	if err != nil {
		return Handle{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Handle{}, err
	}
	return Handle{Preparation: p, JobID: inserted.Job.ID}, nil
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
