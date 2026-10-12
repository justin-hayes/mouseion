package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
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
	PlanPreparedDeckRun(context.Context, pgx.Tx, domain.DeckPreparation) (persistence.FreezePreparedDeckRunParams, error)
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
	OwnerID, PreparationID string
	ExpectedManifestDigest string
}

// Freeze commits the run, immutable manifest, outcomes, Batch placeholders,
// current-run pointer, and initial River jobs atomically. A concurrent retry
// observes the committed run without invoking the planner again.
func (c *DurableCoordinator) Freeze(ctx context.Context, request DurableFreezeRequest) (result persistence.FreezePreparedDeckRunResult, err error) {
	if !c.validFreezeRequest(request) {
		return persistence.FreezePreparedDeckRunResult{}, ErrInvalidInput
	}
	tx, err := c.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	preparation, err := scanPreparation(tx.QueryRow(ctx, `SELECT `+preparationColumns+` FROM deck_preparations WHERE owner_id=$1 AND id=$2 FOR UPDATE`, request.OwnerID, request.PreparationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return persistence.FreezePreparedDeckRunResult{}, persistence.ErrNotFound
	}
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	if preparation.CurrentRunID != "" {
		return c.freezeExistingRun(ctx, tx, request)
	}
	return c.freezeNewRun(ctx, tx, request, preparation)
}

// validFreezeRequest reports whether the coordinator and request carry every
// identity Freeze needs before it opens a transaction.
func (c *DurableCoordinator) validFreezeRequest(request DurableFreezeRequest) bool {
	return c != nil && c.pool != nil && c.store != nil && c.client != nil && c.planner != nil && strings.TrimSpace(request.OwnerID) != "" && strings.TrimSpace(request.PreparationID) != ""
}

// freezeExistingRun commits the lock and returns the already-frozen run without
// invoking the planner again, verifying the requested manifest digest when one
// was supplied.
func (c *DurableCoordinator) freezeExistingRun(ctx context.Context, tx pgx.Tx, request DurableFreezeRequest) (persistence.FreezePreparedDeckRunResult, error) {
	if err := tx.Commit(ctx); err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	run, err := c.store.GetCurrentPreparedDeckRun(ctx, request.OwnerID, request.PreparationID)
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	_, digest, err := c.store.LoadPreparedDeckStorageProjection(ctx, request.OwnerID, request.PreparationID, run.ID)
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	if request.ExpectedManifestDigest != "" && request.ExpectedManifestDigest != digest {
		return persistence.FreezePreparedDeckRunResult{}, persistence.ErrImmutable
	}
	chunks, err := c.store.ListPreparedDeckBatchChunks(ctx, request.OwnerID, request.PreparationID, run.ID)
	return persistence.FreezePreparedDeckRunResult{Run: run, ManifestDigest: digest, Chunks: chunks, Existing: true, NeedsFinalizer: run.State == domain.PreparedDeckRunFinalizing}, err
}

// freezeNewRun plans and commits the run, immutable manifest, outcomes, chunk
// placeholders, current-run pointer and initial River jobs atomically.
func (c *DurableCoordinator) freezeNewRun(ctx context.Context, tx pgx.Tx, request DurableFreezeRequest, preparation domain.DeckPreparation) (persistence.FreezePreparedDeckRunResult, error) {
	plan, err := c.planner.PlanPreparedDeckRun(ctx, tx, preparation)
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, fmt.Errorf("plan durable prepared deck: %w", err)
	}
	plan.OwnerID, plan.PreparationID, plan.ExpectedManifestDigest = request.OwnerID, request.PreparationID, request.ExpectedManifestDigest
	result, err := c.store.FreezePreparedDeckRunTx(ctx, tx, plan)
	if err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	if err := c.insertBatchSubmissionJobsTx(ctx, tx, request, &result); err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	if err := c.insertStandardTranslationJobsTx(ctx, tx, request, &result); err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	if err := c.insertFinalizerJobTx(ctx, tx, request, &result); err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return persistence.FreezePreparedDeckRunResult{}, err
	}
	return result, nil
}

func insertLivePreparationJobTx(ctx context.Context, tx pgx.Tx, client riverClient, args river.JobArgs, queue, kind string) (int64, error) {
	inserted, err := client.InsertTx(ctx, tx, args, &river.InsertOpts{Queue: queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
	if err != nil {
		return 0, err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return 0, fmt.Errorf("prepareddeck: River did not return a live %s job", kind)
	}
	return inserted.Job.ID, nil
}

func (c *DurableCoordinator) insertBatchSubmissionJobsTx(ctx context.Context, tx pgx.Tx, request DurableFreezeRequest, result *persistence.FreezePreparedDeckRunResult) error {
	for index := range result.Chunks {
		chunk := &result.Chunks[index]
		jobID, err := insertLivePreparationJobTx(ctx, tx, c.client, BatchSubmitJobArgs{OwnerID: request.OwnerID, PreparationID: request.PreparationID, RunID: result.Run.ID, ChunkID: chunk.ID, Generation: chunk.Generation}, Queue, "Batch submission")
		if err != nil {
			return fmt.Errorf("enqueue Batch chunk %d: %w", chunk.ChunkIndex, err)
		}
		chunk.SubmissionJobID = jobID
		chunk.SubmissionGeneration = chunk.Generation
		if err := c.store.SetPreparedDeckBatchSubmissionJobTx(ctx, tx, request.OwnerID, request.PreparationID, result.Run.ID, chunk.ID, chunk.Generation, jobID); err != nil {
			return err
		}
	}
	return nil
}

func (c *DurableCoordinator) insertStandardTranslationJobsTx(ctx context.Context, tx pgx.Tx, request DurableFreezeRequest, result *persistence.FreezePreparedDeckRunResult) error {
	if result.Run.ExecutionMode != domain.PreparedDeckExecutionStandard {
		return nil
	}
	for _, ordinal := range result.PendingOrdinals {
		args := StandardTranslationJobArgs{OwnerID: request.OwnerID, PreparationID: request.PreparationID, RunID: result.Run.ID, Ordinal: ordinal, Generation: 0}
		jobID, err := insertLivePreparationJobTx(ctx, tx, c.client, args, TranslationQueue, "standard translation")
		if err != nil {
			return fmt.Errorf("enqueue standard translation ordinal %d: %w", ordinal, err)
		}
		if err := c.store.SetPreparedDeckTranslationJobTx(ctx, tx, request.OwnerID, request.PreparationID, result.Run.ID, ordinal, 0, jobID); err != nil {
			return err
		}
	}
	return nil
}

func (c *DurableCoordinator) insertFinalizerJobTx(ctx context.Context, tx pgx.Tx, request DurableFreezeRequest, result *persistence.FreezePreparedDeckRunResult) error {
	if !result.NeedsFinalizer {
		return nil
	}
	args := FinalizeJobArgs{OwnerID: request.OwnerID, PreparationID: request.PreparationID, RunID: result.Run.ID, Generation: result.Run.FinalizationDispatchGeneration}
	jobID, err := insertLivePreparationJobTx(ctx, tx, c.client, args, Queue, "finalizer")
	if err != nil {
		return fmt.Errorf("enqueue prepared-deck finalizer: %w", err)
	}
	if err := c.store.SetPreparedDeckFinalizationJobTx(ctx, tx, request.OwnerID, request.PreparationID, result.Run.ID, result.Run.FinalizationDispatchGeneration, jobID); err != nil {
		return err
	}
	result.Run.FinalizationJobID = jobID
	return nil
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
	RestoreWithRecoveredRenderInputs(cardexport.StorageProjection, []cardexport.RecoveredRenderInput) (cardexport.FrozenDeck, error)
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
	projection, recovered, err := recoverLegacyRenderInputs(ctx, r.Store, owner, run.RenderInputVersion, projection)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	deck, err := r.Renderer.RestoreWithRecoveredRenderInputs(projection, recovered)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	artifact, diagnostics, err := r.Renderer.Finalize(ctx, deck, stored, preparedDeckRunFacts(run))
	if err != nil {
		return domain.DeckPreparation{}, fmt.Errorf("render prepared deck revision: %w", err)
	}
	logFinalizeDiagnostics(diagnostics)
	return r.Store.SupersedePreparedDeckArtifact(ctx, owner, preparationID, runID, presentationVersion, artifact)
}

func recoverLegacyRenderInputs(ctx context.Context, store durableRerenderStore, owner string, renderInputVersion int, projection cardexport.StorageProjection) (cardexport.StorageProjection, []cardexport.RecoveredRenderInput, error) {
	if renderInputVersion >= cardexport.RenderInputVersion {
		return projection, nil, nil
	}
	ordinalsByCorpus, itemsByCorpus, err := legacyRecoveryCoordinates(projection)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	if len(itemsByCorpus) == 0 {
		return projection, nil, nil
	}
	corpusStore, ok := store.(durableRerenderCorpusStore)
	if !ok {
		return cardexport.StorageProjection{}, nil, fmt.Errorf("%w: corpus reader is unavailable", ErrRequiresRepreparation)
	}
	recovered, err := recoverCorpusRenderInputs(ctx, corpusStore, owner, ordinalsByCorpus, itemsByCorpus)
	if err != nil {
		return cardexport.StorageProjection{}, nil, err
	}
	return projection, recovered, nil
}

// legacyRecoveryCoordinates groups the accepted manifest items that predate
// stored sentence tokens by the corpus and ordinals they need recovered. An
// item without a corpus coordinate cannot be recovered.
func legacyRecoveryCoordinates(projection cardexport.StorageProjection) (map[string][]int64, map[string][]cardexport.ManifestItem, error) {
	ordinalsByCorpus := make(map[string][]int64)
	itemsByCorpus := make(map[string][]cardexport.ManifestItem)
	for _, item := range projection.Items {
		if item.Disposition != cardexport.ManifestAccepted || len(item.Entry.SentenceTokens) > 0 {
			continue
		}
		if strings.TrimSpace(item.CorpusID) == "" || item.SentenceOrdinal < 0 {
			return nil, nil, fmt.Errorf("%w: manifest item %d has no corpus coordinate", ErrRequiresRepreparation, item.Ordinal)
		}
		if _, ok := itemsByCorpus[item.CorpusID]; !ok {
			ordinalsByCorpus[item.CorpusID] = nil
		}
		ordinalsByCorpus[item.CorpusID] = append(ordinalsByCorpus[item.CorpusID], item.SentenceOrdinal)
		itemsByCorpus[item.CorpusID] = append(itemsByCorpus[item.CorpusID], item)
	}
	return ordinalsByCorpus, itemsByCorpus, nil
}

// recoverCorpusRenderInputs reads each affected corpus once and rebuilds the
// exact render inputs, refusing any sentence whose stored parse is unavailable.
func recoverCorpusRenderInputs(ctx context.Context, corpusStore durableRerenderCorpusStore, owner string, ordinalsByCorpus map[string][]int64, itemsByCorpus map[string][]cardexport.ManifestItem) ([]cardexport.RecoveredRenderInput, error) {
	recovered := make([]cardexport.RecoveredRenderInput, 0)
	for corpusID, items := range itemsByCorpus {
		sentences, err := corpusStore.ListCorpusSentences(ctx, owner, corpusID, ordinalsByCorpus[corpusID])
		if err != nil {
			return nil, fmt.Errorf("recover corpus %s: %w", corpusID, err)
		}
		for _, item := range items {
			sentence, found := sentences[item.SentenceOrdinal]
			if !found || strings.TrimSpace(sentence.Text) == "" || sentence.Text != item.Entry.Sentence || !hasDependencyParse(sentence) {
				return nil, fmt.Errorf("%w: corpus %s sentence %d is unavailable", ErrRequiresRepreparation, corpusID, item.SentenceOrdinal)
			}
			recovered = append(recovered, cardexport.RecoveredRenderInput{ManifestOrdinal: item.Ordinal, Sentence: sentence.Text, SentenceTokens: sentence.Tokens})
		}
	}
	return recovered, nil
}

func hasDependencyParse(sentence analyzer.Sentence) bool {
	if len(sentence.Tokens) == 0 {
		return false
	}
	for index, token := range sentence.Tokens {
		if strings.TrimSpace(token.Dependency) == "" || uint64(token.Head) >= uint64(len(sentence.Tokens)) {
			return false
		}
		if token.Dependency == "root" && uint64(token.Head) != uint64(index) {
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
	now := f.clock()
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
	run, err := f.Store.ClaimPreparedDeckFinalization(ctx, owner, preparationID, runID, generation, token, claimedAt.Add(f.lease()))
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	mode = finalizeMetricMode(run.ExecutionMode)
	totalStarted = run.CreatedAt
	if run.State == domain.PreparedDeckRunCompleted {
		return f.Store.GetDeckPreparation(ctx, owner, preparationID)
	}
	claim := finalizationClaim{owner: owner, preparationID: preparationID, runID: runID, token: token}
	deck, stored, err := f.restoreFrozenDeck(ctx, claim)
	if err != nil {
		return domain.DeckPreparation{}, err
	}
	artifact, diagnostics, err := f.Renderer.Finalize(ctx, deck, stored, preparedDeckRunFacts(run))
	if err != nil {
		failureClass, failureCode := renderFailure(err)
		err = f.recordFailure(ctx, claim, failureClass, failureCode, err)
		observeBatchMetric(f.Metrics, BatchMetric{Mode: mode, Name: MetricAPKGOutcome, Phase: "finalizing", State: "failed", ErrorClass: "terminal", Provider: "openai", Value: 1})
		return domain.DeckPreparation{}, fmt.Errorf("render durable prepared deck: %w", err)
	}
	return f.publish(ctx, claim, run, mode, artifact, diagnostics)
}

// finalizationClaim identifies the run a finalizer holds and the token that
// fences its writes.
type finalizationClaim struct {
	owner, preparationID, runID, token string
}

func (f *DurableFinalizer) clock() func() time.Time {
	if f.Now != nil {
		return f.Now
	}
	return time.Now
}

func (f *DurableFinalizer) lease() time.Duration {
	if f.LeaseDuration <= 0 {
		return 5 * time.Minute
	}
	return f.LeaseDuration
}

func finalizeMetricMode(executionMode domain.PreparedDeckExecutionMode) string {
	mode := string(executionMode)
	if mode != "standard" && mode != "batch" {
		return "unknown"
	}
	return mode
}

// restoreFrozenDeck loads the frozen manifest and its stored results. A
// manifest that fails its identity or presentation check fails the run; any
// other load error leaves the run for a later attempt.
func (f *DurableFinalizer) restoreFrozenDeck(ctx context.Context, claim finalizationClaim) (cardexport.FrozenDeck, []cardexport.StoredResult, error) {
	projection, stored, err := f.Store.LoadPreparedDeckFinalization(ctx, claim.owner, claim.preparationID, claim.runID)
	if err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckIdentity) || errors.Is(err, cardexport.ErrInvalidInput) {
			err = f.recordFailure(ctx, claim, "presentation", "invalid_manifest", err)
		}
		return cardexport.FrozenDeck{}, nil, err
	}
	deck, err := f.Renderer.Restore(projection)
	if err != nil {
		return cardexport.FrozenDeck{}, nil, f.recordFailure(ctx, claim, "presentation", "invalid_manifest", err)
	}
	return deck, stored, nil
}

func renderFailure(err error) (failureClass, failureCode string) {
	if errors.Is(err, cardexport.ErrAllMeaningsUnresolved) {
		return "validation", "all_meanings_unresolved"
	}
	return "presentation", "render_failed"
}

// recordFailure fails the claimed run unless the finalizer itself was
// cancelled, and joins any error from recording the failure to err.
func (f *DurableFinalizer) recordFailure(ctx context.Context, claim finalizationClaim, failureClass, failureCode string, err error) error {
	if ctx.Err() != nil {
		return err
	}
	if failErr := f.Store.FailPreparedDeckFinalization(ctx, claim.owner, claim.preparationID, claim.runID, claim.token, failureClass, failureCode); failErr != nil {
		return errors.Join(err, fmt.Errorf("record finalization failure: %w", failErr))
	}
	return err
}

func (f *DurableFinalizer) publish(ctx context.Context, claim finalizationClaim, run domain.PreparedDeckRun, mode string, artifact cardexport.FinalArtifact, diagnostics cardexport.FinalizeDiagnostics) (domain.DeckPreparation, error) {
	logFinalizeDiagnostics(diagnostics)
	result, err := f.Store.CompletePreparedDeckRun(ctx, claim.owner, claim.preparationID, claim.runID, claim.token, artifact)
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

func logFinalizeDiagnostics(diagnostics cardexport.FinalizeDiagnostics) {
	if slices.Contains(diagnostics.DegradationCodes, cardexport.DegradationInvalidSenseSelection) {
		log.Printf("prepared deck translation: invalid sense selection; using deterministic order")
		return
	}
}
