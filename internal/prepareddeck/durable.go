package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
)

const durableJobMaxAttempts = 5

type fallbackGlossEvent struct {
	Event        string  `json:"event"`
	Selected     int     `json:"selected"`
	FallbackUsed int     `json:"fallback_used"`
	FallbackRate float64 `json:"fallback_rate"`
}

func logFallbackGlossUsage(completeness cardexport.Completeness) {
	fallbackRate := 0.0
	if completeness.TotalCards > 0 {
		fallbackRate = float64(completeness.CardsWithFallbackGloss) / float64(completeness.TotalCards)
	}
	if math.IsNaN(fallbackRate) || math.IsInf(fallbackRate, 0) {
		return
	}
	payload, err := json.Marshal(fallbackGlossEvent{Event: "fallback_gloss_usage", Selected: completeness.TotalCards, FallbackUsed: completeness.CardsWithFallbackGloss, FallbackRate: fallbackRate})
	if err == nil {
		log.Printf("fallback_gloss_usage %s", payload)
	}
}

var ErrRequiresRepreparation = errors.New("prepared deck requires re-preparation")

// BatchSubmitJobArgs deliberately contains only durable orchestration
// identities; the provider transport worker never receives source content.
type BatchSubmitJobArgs struct {
	OwnerID       string `json:"owner_id"`
	PreparationID string `json:"preparation_id"`
	RunID         string `json:"run_id"`
	ChunkID       string `json:"chunk_id" river:"unique"`
	Generation    int    `json:"generation"`
}

// StandardTranslationJobArgs carries only the frozen run identity. Provider
// execution is intentionally a later issue; this job is the durable dispatch
// boundary and its full argument set is the idempotency key.
type StandardTranslationJobArgs struct {
	OwnerID       string `json:"owner_id" river:"unique"`
	PreparationID string `json:"preparation_id" river:"unique"`
	RunID         string `json:"run_id" river:"unique"`
	Ordinal       int    `json:"ordinal" river:"unique"`
	Generation    int    `json:"generation" river:"unique"`
}

func (StandardTranslationJobArgs) Kind() string { return "prepared_deck_translation" }

func (BatchSubmitJobArgs) Kind() string { return "prepared_deck_batch_submit" }

// BatchPollJobArgs is intentionally short-lived work. It identifies one
// already-created provider Batch and never carries a prompt, response, or
// source-derived field.
type BatchPollJobArgs struct {
	OwnerID       string `json:"owner_id"`
	PreparationID string `json:"preparation_id"`
	RunID         string `json:"run_id"`
	ChunkID       string `json:"chunk_id" river:"unique"`
	Generation    int    `json:"generation"`
}

func (BatchPollJobArgs) Kind() string { return "prepared_deck_batch_poll" }

type FinalizeJobArgs struct {
	OwnerID       string `json:"owner_id"`
	PreparationID string `json:"preparation_id"`
	RunID         string `json:"run_id" river:"unique"`
	Generation    int    `json:"generation"`
}

func (FinalizeJobArgs) Kind() string { return "prepared_deck_finalize" }

type RerenderJobArgs struct {
	OwnerID             string `json:"owner_id"`
	PreparationID       string `json:"preparation_id"`
	RunID               string `json:"run_id" river:"unique"`
	PresentationVersion int    `json:"presentation_version" river:"unique"`
}

func (RerenderJobArgs) Kind() string { return "prepared_deck_rerender" }

// DurableRunPlanner performs every selection/cache-identity read through the
// supplied repeatable-read transaction and returns the complete frozen plan.
// Provider transport is intentionally outside this boundary.
type DurableRunPlanner interface {
	PlanPreparedDeckRun(context.Context, pgx.Tx, domain.DeckPreparation, bool) (persistence.FreezePreparedDeckRunParams, error)
}

type DurableCoordinator struct {
	pool    *pgxpool.Pool
	store   *persistence.PostgresStore
	client  riverClient
	planner DurableRunPlanner
}

func NewDurableCoordinator(store *persistence.PostgresStore, client riverClient, planner DurableRunPlanner) *DurableCoordinator {
	if store == nil {
		return &DurableCoordinator{client: client, planner: planner}
	}
	return &DurableCoordinator{pool: store.Pool(), store: store, client: client, planner: planner}
}

type DurableFreezeRequest struct {
	OwnerID, PreparationID     string
	ExpectedManifestDigest     string
	ExternalTranslationConsent bool
}

// Freeze commits the run, immutable manifest, outcomes, Batch placeholders,
// current-run pointer, and initial River jobs atomically. A concurrent retry
// observes the committed run without invoking the planner again.
func (c *DurableCoordinator) Freeze(ctx context.Context, request DurableFreezeRequest) (persistence.FreezePreparedDeckRunResult, error) {
	if c == nil || c.pool == nil || c.store == nil || c.client == nil || c.planner == nil || strings.TrimSpace(request.OwnerID) == "" || strings.TrimSpace(request.PreparationID) == "" {
		return persistence.FreezePreparedDeckRunResult{}, ErrInvalidInput
	}
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	defer tx.Rollback(ctx)
	preparation, err := scanPreparation(tx.QueryRow(ctx, `SELECT `+preparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, request.OwnerID, request.PreparationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.FreezePreparedDeckRunResult{}, persistence.ErrNotFound
	}
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	if preparation.CurrentRunID != "" {
		if err = tx.Commit(ctx); err != nil {
			return persistence.FreezePreparedDeckRunResult{}, err
		}
		run, getErr := c.store.GetCurrentPreparedDeckRun(ctx, request.OwnerID, request.PreparationID)
		if getErr != nil {
			return persistence.FreezePreparedDeckRunResult{}, getErr
		}
		_, digest, getErr := c.store.LoadPreparedDeckStorageProjection(ctx, request.OwnerID, request.PreparationID, run.ID)
		if getErr != nil {
			return persistence.FreezePreparedDeckRunResult{}, getErr
		}
		if request.ExpectedManifestDigest != "" && request.ExpectedManifestDigest != digest {
			return persistence.FreezePreparedDeckRunResult{}, persistence.ErrImmutable
		}
		chunks, getErr := c.store.ListPreparedDeckBatchChunks(ctx, request.OwnerID, request.PreparationID, run.ID)
		return persistence.FreezePreparedDeckRunResult{Run: run, ManifestDigest: digest, Chunks: chunks, Existing: true, NeedsFinalizer: run.State == domain.PreparedDeckRunFinalizing}, getErr
	}
	plan, err := c.planner.PlanPreparedDeckRun(ctx, tx, preparation, request.ExternalTranslationConsent)
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, fmt.Errorf("plan durable prepared deck: %w", err)
	}
	plan.OwnerID, plan.PreparationID, plan.ExpectedManifestDigest = request.OwnerID, request.PreparationID, request.ExpectedManifestDigest
	result, err := c.store.FreezePreparedDeckRunTx(ctx, tx, plan)
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	for i := range result.Chunks {
		chunk := &result.Chunks[i]
		inserted, insertErr := c.client.InsertTx(ctx, tx, BatchSubmitJobArgs{OwnerID: request.OwnerID, PreparationID: request.PreparationID, RunID: result.Run.ID, ChunkID: chunk.ID, Generation: chunk.Generation}, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
		if insertErr != nil {
			return persistence.FreezePreparedDeckRunResult{}, fmt.Errorf("enqueue Batch chunk %d: %w", chunk.ChunkIndex, insertErr)
		}
		if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
			return persistence.FreezePreparedDeckRunResult{}, errors.New("River did not return a live Batch submission job")
		}
		chunk.SubmissionJobID = inserted.Job.ID
		chunk.SubmissionGeneration = chunk.Generation
		if err = c.store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, request.OwnerID, request.PreparationID, result.Run.ID, chunk.ID, chunk.Generation, inserted.Job.ID); err != nil {
			return persistence.FreezePreparedDeckRunResult{}, err
		}
	}
	if result.Run.ExecutionMode == domain.PreparedDeckExecutionStandard {
		for _, ordinal := range result.PendingOrdinals {
			args := StandardTranslationJobArgs{OwnerID: request.OwnerID, PreparationID: request.PreparationID, RunID: result.Run.ID, Ordinal: ordinal, Generation: 0}
			inserted, insertErr := c.client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: TranslationQueue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
			if insertErr != nil {
				return persistence.FreezePreparedDeckRunResult{}, fmt.Errorf("enqueue standard translation ordinal %d: %w", ordinal, insertErr)
			}
			if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
				return persistence.FreezePreparedDeckRunResult{}, errors.New("River did not return a live standard translation job")
			}
			if err = c.store.SetPreparedDeckTranslationJobTx(ctx, tx, request.OwnerID, request.PreparationID, result.Run.ID, ordinal, 0, inserted.Job.ID); err != nil {
				return persistence.FreezePreparedDeckRunResult{}, err
			}
		}
	}
	if result.NeedsFinalizer {
		inserted, insertErr := c.client.InsertTx(ctx, tx, FinalizeJobArgs{OwnerID: request.OwnerID, PreparationID: request.PreparationID, RunID: result.Run.ID, Generation: result.Run.FinalizationDispatchGeneration}, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
		if insertErr != nil {
			return persistence.FreezePreparedDeckRunResult{}, fmt.Errorf("enqueue prepared-deck finalizer: %w", insertErr)
		}
		if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
			return persistence.FreezePreparedDeckRunResult{}, errors.New("River did not return a live finalizer job")
		}
		if err = c.store.SetPreparedDeckFinalizationJobTx(ctx, tx, request.OwnerID, request.PreparationID, result.Run.ID, result.Run.FinalizationDispatchGeneration, inserted.Job.ID); err != nil {
			return persistence.FreezePreparedDeckRunResult{}, err
		}
		result.Run.FinalizationJobID = inserted.Job.ID
	}
	if err = tx.Commit(ctx); err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	return result, nil
}

type durableFinalizerStore interface {
	ClaimPreparedDeckFinalization(context.Context, string, string, string, int, string, time.Time) (domain.PreparedDeckRun, error)
	LoadPreparedDeckFinalization(context.Context, string, string, string) (cardexport.StorageProjection, []cardexport.StoredResult, error)
	CompletePreparedDeckRun(context.Context, string, string, string, string, cardexport.Artifact) (domain.DeckPreparation, error)
	GetDeckPreparation(context.Context, string, string) (domain.DeckPreparation, error)
	FailPreparedDeckFinalization(context.Context, string, string, string, string, string, string) error
}

type durablePresentation interface {
	Restore(cardexport.StorageProjection) (cardexport.FrozenDeck, error)
	Finalize(context.Context, cardexport.FrozenDeck, []cardexport.StoredResult, cardexport.RunFacts) (cardexport.FinalArtifact, cardexport.FinalizeDiagnostics, error)
}

type DurableFinalizer struct {
	Store         durableFinalizerStore
	Renderer      durablePresentation
	Now           func() time.Time
	LeaseDuration time.Duration
	Metrics       BatchMetrics
}

func preparedDeckRunFacts(run domain.PreparedDeckRun) cardexport.RunFacts {
	return cardexport.RunFacts{
		Consent: run.ExternalTranslationConsent, Configured: run.ExternalTranslationConfigured,
		ExecutionMode: string(run.ExecutionMode), TargetLanguage: run.TargetLanguage,
		Provider: run.Provider, ProviderVersion: run.ProviderVersion,
	}
}

type durableRerenderStore interface {
	GetPreparedDeckRun(context.Context, string, string, string) (domain.PreparedDeckRun, error)
	LoadPreparedDeckFinalization(context.Context, string, string, string) (cardexport.StorageProjection, []cardexport.StoredResult, error)
	SupersedePreparedDeckArtifact(context.Context, string, string, string, int, cardexport.Artifact) (domain.DeckPreparation, error)
	GetDeckPreparation(context.Context, string, string) (domain.DeckPreparation, error)
	MarkPreparedDeckRequiresRepreparation(context.Context, string, string, string) error
}

type durableRerenderCorpusStore interface {
	ListCorpusSentences(context.Context, string, string, []int64) (map[int64]analyzer.Sentence, error)
}

type DurableRerenderer struct {
	Store    durableRerenderStore
	Renderer durablePresentation
}

// Rerender replays a completed run's frozen specification and exact overlay.
// It never enters the preparation or translation state machines.
func (r *DurableRerenderer) Rerender(ctx context.Context, owner, preparationID, runID string, presentationVersion int) (domain.DeckPreparation, error) {
	if r == nil || r.Store == nil || r.Renderer == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(preparationID) == "" || strings.TrimSpace(runID) == "" {
		return domain.DeckPreparation{}, ErrInvalidInput
	}
	if presentationVersion <= 0 {
		presentationVersion = cardexport.PresentationVersion
	}
	run, err := r.Store.GetPreparedDeckRun(ctx, owner, preparationID, runID)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if run.State != domain.PreparedDeckRunCompleted {
		return domain.DeckPreparation{}, persistence.ErrInvalidTransition
	}
	preparation, err := r.Store.GetDeckPreparation(ctx, owner, preparationID)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	if preparation.State != domain.DeckPreparationReady || preparation.CurrentRunID != runID || preparation.RetiredAt != nil {
		return domain.DeckPreparation{}, persistence.ErrInvalidTransition
	}
	if run.PresentationVersion > presentationVersion {
		presentationVersion = run.PresentationVersion
	}
	if run.PresentationVersion >= presentationVersion && run.RenderInputVersion >= cardexport.RenderInputVersion {
		return preparation, nil
	}
	projection, stored, err := r.Store.LoadPreparedDeckFinalization(ctx, owner, preparationID, runID)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	projection, err = recoverLegacyRenderInputs(ctx, r.Store, owner, run.RenderInputVersion, projection)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	deck, err := r.Renderer.Restore(projection)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	artifact, _, err := r.Renderer.Finalize(ctx, deck, stored, preparedDeckRunFacts(run))
	if err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("render prepared deck revision: %w", err)
	}
	return r.Store.SupersedePreparedDeckArtifact(ctx, owner, preparationID, runID, presentationVersion, artifact)
}

func recoverLegacyRenderInputs(ctx context.Context, store durableRerenderStore, owner string, renderInputVersion int, projection cardexport.StorageProjection) (cardexport.StorageProjection, error) {
	if renderInputVersion >= cardexport.RenderInputVersion {
		return projection, nil
	}

	snapshot := projection
	ordinalsByCorpus := make(map[string][]int64)
	itemsByCorpus := make(map[string][]int)
	for index, item := range snapshot.Items {
		if item.Disposition != cardexport.ManifestAccepted || len(item.Entry.SentenceTokens) > 0 {
			continue
		}
		if strings.TrimSpace(item.CorpusID) == "" || item.SentenceOrdinal < 0 {
			return cardexport.StorageProjection{}, fmt.Errorf("%w: manifest item %d has no corpus coordinate", ErrRequiresRepreparation, item.Ordinal)
		}
		if _, ok := itemsByCorpus[item.CorpusID]; !ok {
			ordinalsByCorpus[item.CorpusID] = nil
		}
		ordinalsByCorpus[item.CorpusID] = append(ordinalsByCorpus[item.CorpusID], item.SentenceOrdinal)
		itemsByCorpus[item.CorpusID] = append(itemsByCorpus[item.CorpusID], index)
	}
	if len(itemsByCorpus) == 0 {
		return projection, nil
	}

	corpusStore, ok := store.(durableRerenderCorpusStore)
	if !ok {
		return cardexport.StorageProjection{}, fmt.Errorf("%w: corpus reader is unavailable", ErrRequiresRepreparation)
	}
	for corpusID, indexes := range itemsByCorpus {
		sentences, err := corpusStore.ListCorpusSentences(ctx, owner, corpusID, ordinalsByCorpus[corpusID])
		if err != nil {
			return cardexport.StorageProjection{}, fmt.Errorf("recover corpus %s: %w", corpusID, err)
		}
		for _, index := range indexes {
			item := snapshot.Items[index]
			sentence, found := sentences[item.SentenceOrdinal]
			if !found || strings.TrimSpace(sentence.Text) == "" || sentence.Text != item.Entry.Sentence || !hasDependencyParse(sentence) {
				return cardexport.StorageProjection{}, fmt.Errorf("%w: corpus %s sentence %d is unavailable", ErrRequiresRepreparation, corpusID, item.SentenceOrdinal)
			}
			snapshot.Items[index].Entry.SentenceTokens = sentence.Tokens
		}
	}
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	if err != nil {
		return cardexport.StorageProjection{}, fmt.Errorf("rebuild recovered projection: %w", err)
	}
	return deck.StorageProjection(), nil
}

func hasDependencyParse(sentence analyzer.Sentence) bool {
	if len(sentence.Tokens) == 0 {
		return false
	}
	for index, token := range sentence.Tokens {
		if strings.TrimSpace(token.Dependency) == "" || int(token.Head) >= len(sentence.Tokens) {
			return false
		}
		if token.Dependency == "root" && int(token.Head) != index {
			return false
		}
	}
	return true
}

type RerenderWorker struct {
	river.WorkerDefaults[RerenderJobArgs]
	Rerenderer *DurableRerenderer
}

func (w *RerenderWorker) Work(ctx context.Context, job *river.Job[RerenderJobArgs]) error {
	if w == nil || w.Rerenderer == nil || job == nil {
		return ErrInvalidInput
	}
	_, err := w.Rerenderer.Rerender(ctx, job.Args.OwnerID, job.Args.PreparationID, job.Args.RunID, job.Args.PresentationVersion)
	if errors.Is(err, ErrRequiresRepreparation) || errors.Is(err, persistence.ErrPreparedDeckIdentity) {
		return w.Rerenderer.Store.MarkPreparedDeckRequiresRepreparation(ctx, job.Args.OwnerID, job.Args.PreparationID, job.Args.RunID)
	}
	if errors.Is(err, persistence.ErrInvalidTransition) || errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
		return nil
	}
	return err
}

func AddRerenderWorker(workers *river.Workers, rerenderer *DurableRerenderer) {
	river.AddWorker(workers, &RerenderWorker{Rerenderer: rerenderer})
}

// Finalize renders only the frozen manifest and publishes through the existing
// atomic artifact/provenance boundary. The claim token fences cancellation and
// a newer finalizer generation.
func (f *DurableFinalizer) Finalize(ctx context.Context, owner, preparationID, runID string, generation int) (domain.DeckPreparation, error) {
	if f == nil || f.Store == nil || f.Renderer == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(preparationID) == "" || strings.TrimSpace(runID) == "" {
		return domain.DeckPreparation{}, ErrInvalidInput
	}
	now := time.Now
	if f.Now != nil {
		now = f.Now
	}
	lease := f.LeaseDuration
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	started := now().UTC()
	var totalStarted time.Time
	mode := ""
	defer func() {
		observeBatchMetric(f.Metrics, BatchMetric{Mode: mode, Name: MetricBatchPhaseLatency, Phase: "finalizing", Provider: "openai", Value: seconds(now().Sub(started))})
		if totalStarted.IsZero() {
			totalStarted = started
		}
		observeBatchMetric(f.Metrics, BatchMetric{Mode: mode, Name: MetricBatchTotalLatency, Phase: "finalizing", Provider: "openai", Value: seconds(now().Sub(totalStarted))})
	}()
	claimedAt := now().UTC()
	token := uuid.NewString()
	run, err := f.Store.ClaimPreparedDeckFinalization(ctx, owner, preparationID, runID, generation, token, claimedAt.Add(lease))
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	mode = string(run.ExecutionMode)
	if mode != "standard" && mode != "batch" {
		mode = "unknown"
	}
	totalStarted = run.CreatedAt
	if run.State == domain.PreparedDeckRunCompleted {
		return f.Store.GetDeckPreparation(ctx, owner, preparationID)
	}
	projection, stored, err := f.Store.LoadPreparedDeckFinalization(ctx, owner, preparationID, runID)
	if err != nil {
		if ctx.Err() == nil && (errors.Is(err, persistence.ErrPreparedDeckIdentity) || errors.Is(err, cardexport.ErrInvalidInput)) {
			_ = f.Store.FailPreparedDeckFinalization(ctx, owner, preparationID, runID, token, "presentation", "invalid_manifest")
		}
		return domain.DeckPreparation{}, err
	}
	deck, err := f.Renderer.Restore(projection)
	if err != nil {
		if ctx.Err() == nil {
			_ = f.Store.FailPreparedDeckFinalization(ctx, owner, preparationID, runID, token, "presentation", "invalid_manifest")
		}
		return domain.DeckPreparation{}, err
	}
	artifact, _, err := f.Renderer.Finalize(ctx, deck, stored, preparedDeckRunFacts(run))
	if err != nil {
		if ctx.Err() == nil {
			_ = f.Store.FailPreparedDeckFinalization(ctx, owner, preparationID, runID, token, "presentation", "render_failed")
		}
		observeBatchMetric(f.Metrics, BatchMetric{Mode: mode, Name: MetricAPKGOutcome, Phase: "finalizing", State: "failed", ErrorClass: "terminal", Provider: "openai", Value: 1})
		return domain.DeckPreparation{}, fmt.Errorf("render durable prepared deck: %w", err)
	}
	result, err := f.Store.CompletePreparedDeckRun(ctx, owner, preparationID, runID, token, artifact)
	if err != nil {
		observeBatchMetric(f.Metrics, BatchMetric{Mode: mode, Name: MetricAPKGOutcome, Phase: "finalizing", State: "failed", ErrorClass: "terminal", Provider: "openai", Value: 1})
		return domain.DeckPreparation{}, err
	}
	if run.ExternalTranslationConsent && run.ExternalTranslationConfigured {
		logFallbackGlossUsage(artifact.Completeness)
	}
	observeBatchMetric(f.Metrics, BatchMetric{Mode: mode, Name: MetricAPKGOutcome, Phase: "finalizing", State: "completed", Provider: "openai", Value: 1})
	return result, nil
}
