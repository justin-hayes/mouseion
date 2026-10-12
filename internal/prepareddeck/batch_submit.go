package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
)

const batchSubmissionLease = 5 * time.Minute

type batchProvider interface {
	UploadFile(context.Context, string, io.Reader) (enrichment.OpenAIFile, error)
	CreateBatch(context.Context, enrichment.CreateBatchRequest) (enrichment.Batch, error)
	ListBatches(context.Context, enrichment.ListBatchesRequest) (enrichment.BatchList, error)
}

// BatchSubmitWorker owns one short submission attempt. It never waits for
// provider completion; the returned provider Batch ID is handed to a polling
// River job in the same PostgreSQL transaction as the local state transition.
type BatchSubmitWorker struct {
	river.WorkerDefaults[BatchSubmitJobArgs]
	Store    *persistence.PostgresStore
	Client   riverClient
	Provider batchProvider
	Codec    *enrichment.TranslationCodec
	Now      func() time.Time
	Metrics  BatchMetrics
}

func (w *BatchSubmitWorker) Work(ctx context.Context, job *river.Job[BatchSubmitJobArgs]) error {
	if job == nil {
		return ErrInvalidInput
	}
	return w.Submit(ctx, job.Args)
}

func (w *BatchSubmitWorker) Submit(ctx context.Context, args BatchSubmitJobArgs) error {
	if !w.validSubmissionInput(args) {
		return ErrInvalidInput
	}
	started := w.now()
	var totalStarted time.Time
	defer func() {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchPhaseLatency, Phase: "submitting", Provider: "openai", Value: seconds(w.now().Sub(started))})
		if totalStarted.IsZero() {
			totalStarted = started
		}
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchTotalLatency, Phase: "submitting", Provider: "openai", Value: seconds(w.now().Sub(totalStarted))})
	}()
	chunk, proceed, err := w.loadSubmissionChunk(ctx, args)
	if err != nil {
		return err
	}
	totalStarted = chunk.CreatedAt
	if !proceed {
		return nil
	}
	claimed, token, proceed, err := w.claimSubmissionWork(ctx, args, chunk)
	if err != nil || !proceed {
		return err
	}
	items, proceed, err := w.buildProviderBatch(ctx, args, claimed, token)
	if err != nil || !proceed {
		return err
	}
	return w.submitProviderBatch(ctx, args, claimed, token, chunk.State == domain.PreparedDeckBatchSubmitting, items)
}

func (w *BatchSubmitWorker) validSubmissionInput(args BatchSubmitJobArgs) bool {
	return w != nil && w.Store != nil && w.Provider != nil && w.Codec != nil && strings.TrimSpace(args.OwnerID) != "" && strings.TrimSpace(args.PreparationID) != "" && strings.TrimSpace(args.RunID) != "" && strings.TrimSpace(args.ChunkID) != "" && args.Generation >= 1
}

// loadSubmissionChunk loads the immutable chunk row for this attempt and
// reports whether the attempt still owns submittable work.
func (w *BatchSubmitWorker) loadSubmissionChunk(ctx context.Context, args BatchSubmitJobArgs) (domain.PreparedDeckBatchChunk, bool, error) {
	chunk, err := w.Store.GetPreparedDeckBatchChunk(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID)
	if err != nil {
		return domain.PreparedDeckBatchChunk{}, false, err
	}
	if chunk.Generation != args.Generation {
		return chunk, false, nil
	}
	switch chunk.State {
	case domain.PreparedDeckBatchSubmitted, domain.PreparedDeckBatchPolling, domain.PreparedDeckBatchReconciling, domain.PreparedDeckBatchCompleted,
		domain.PreparedDeckBatchFailed, domain.PreparedDeckBatchCancelled, domain.PreparedDeckBatchAmbiguous:
		return chunk, false, nil
	default:
		return chunk, true, nil
	}
}

// claimSubmissionWork fences the chunk for this attempt and closes exact cache
// hits. An all-hit chunk completes without returning submittable work.
func (w *BatchSubmitWorker) claimSubmissionWork(ctx context.Context, args BatchSubmitJobArgs, chunk domain.PreparedDeckBatchChunk) (domain.PreparedDeckBatchChunk, string, bool, error) {
	claimToken := uuid.NewString()
	claimed, err := w.Store.ClaimPreparedDeckBatchSubmission(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, claimToken, w.now().UTC().Add(batchSubmissionLease))
	if err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
			return domain.PreparedDeckBatchChunk{}, "", false, nil
		}
		return domain.PreparedDeckBatchChunk{}, "", false, err
	}
	// Claim transitions return the scalar chunk row; retain the immutable
	// ordered membership loaded immediately before the claim.
	claimed.Ordinals = append([]int(nil), chunk.Ordinals...)
	done, err := w.completeSubmissionCacheHits(ctx, args, claimed, claimToken)
	if err != nil || done {
		return claimed, claimToken, false, err
	}
	return claimed, claimToken, true, nil
}

// completeSubmissionCacheHits closes members whose exact cache row already
// exists. A fully cached chunk dispatches finalization instead of a provider
// submission.
func (w *BatchSubmitWorker) completeSubmissionCacheHits(ctx context.Context, args BatchSubmitJobArgs, claimed domain.PreparedDeckBatchChunk, token string) (bool, error) {
	cacheHits, err := w.Store.CompletePreparedDeckBatchCacheHits(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token)
	if err != nil {
		return false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "cache_lookup")
	}
	if cacheHits != claimed.RequestCount {
		return false, nil
	}
	if _, err := w.Store.FinishPreparedDeckBatchSubmission(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, domain.PreparedDeckBatchCompleted, "", ""); err != nil {
		return false, err
	}
	return true, w.advanceIfTerminal(ctx, args)
}

// buildProviderBatch restores the run contract and the chunk's exact item set,
// then re-verifies the claim immediately before the external boundary.
func (w *BatchSubmitWorker) buildProviderBatch(ctx context.Context, args BatchSubmitJobArgs, claimed domain.PreparedDeckBatchChunk, token string) ([]enrichment.BatchTranslationItem, bool, error) {
	proceed, err := w.checkSubmissionRunContract(ctx, args, claimed, token)
	if err != nil || !proceed {
		return nil, false, err
	}
	items, proceed, err := w.loadSubmissionItems(ctx, args, claimed, token)
	if err != nil || !proceed {
		return nil, false, err
	}
	if err := w.Store.VerifyPreparedDeckBatchSubmissionClaim(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token); err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return items, true, nil
}

// checkSubmissionRunContract loads the frozen run and confirms it still admits
// the chunk's frozen contract.
func (w *BatchSubmitWorker) checkSubmissionRunContract(ctx context.Context, args BatchSubmitJobArgs, claimed domain.PreparedDeckBatchChunk, token string) (bool, error) {
	run, err := w.Store.GetPreparedDeckRun(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "run_load")
	}
	if run.State != domain.PreparedDeckRunTranslating || !run.ExternalTranslationConsent || !run.ExternalTranslationConfigured || run.Model != claimed.Model || run.Endpoint != claimed.Endpoint || run.BatchMaxRequests < claimed.RequestCount || run.BatchMaxBytes < claimed.InputBytes {
		return false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "configuration", "frozen_contract")
	}
	return true, nil
}

// loadSubmissionItems restores the frozen deck and outcomes and rebuilds the
// exact item set the chunk was planned from.
func (w *BatchSubmitWorker) loadSubmissionItems(ctx context.Context, args BatchSubmitJobArgs, claimed domain.PreparedDeckBatchChunk, token string) ([]enrichment.BatchTranslationItem, bool, error) {
	snapshot, _, err := w.Store.LoadPreparedDeckStorageProjection(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return nil, false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "manifest_load")
	}
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	if err != nil {
		return nil, false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "manifest_load")
	}
	outcomes, err := w.Store.ListPreparedDeckTranslationOutcomes(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return nil, false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "outcome_load")
	}
	byOrdinal := make(map[int]domain.PreparedDeckTranslationOutcome, len(outcomes))
	for _, outcome := range outcomes {
		byOrdinal[outcome.Ordinal] = outcome
	}
	// Keep every immutable member in the uploaded JSONL when only part of the
	// chunk became a late cache hit. Completed hit outcomes are already fenced
	// from provider results, while changing the member set here would contradict
	// the persisted byte count and digest. An all-hit chunk returns above without
	// crossing the provider boundary.
	items, err := submissionBatchItems(deck, claimed.Ordinals, byOrdinal)
	if err != nil {
		return nil, false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "chunk_membership")
	}
	if len(items) != claimed.RequestCount || claimed.InputBytes <= 0 || claimed.InputDigest == "" {
		return nil, false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "chunk_changed")
	}
	if claimed.Model != w.Codec.Model() || claimed.Endpoint != enrichment.OpenAIChatCompletionsEndpoint {
		return nil, false, w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "configuration", "frozen_contract")
	}
	return items, true, nil
}

// submitProviderBatch either recovers an in-flight ambiguous creation or uploads
// the JSONL and creates the provider Batch.
func (w *BatchSubmitWorker) submitProviderBatch(ctx context.Context, args BatchSubmitJobArgs, claimed domain.PreparedDeckBatchChunk, token string, wasSubmitting bool, items []enrichment.BatchTranslationItem) error {
	metadata := batchMetadata(args.RunID, args.ChunkID, args.Generation)
	if wasSubmitting {
		return w.recoverSubmittedBatch(ctx, args, claimed, token, metadata)
	}
	return w.createProviderBatch(ctx, args, claimed, token, metadata, items)
}

// recoverSubmittedBatch resolves a claim that was already submitting before a
// restart. Creation is never retried: an exact match is recorded, and a missing
// or duplicated match is ambiguous.
func (w *BatchSubmitWorker) recoverSubmittedBatch(ctx context.Context, args BatchSubmitJobArgs, claimed domain.PreparedDeckBatchChunk, token string, metadata map[string]string) error {
	match, err := w.findExistingBatch(ctx, metadata, claimed.InputFileID)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", err.Error())
	}
	if match == nil {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", "creation_unconfirmed")
	}
	return w.recordSubmitted(ctx, args, token, inputFileIDOr(claimed, match.InputFileID), match.ID)
}

// createProviderBatch uploads the JSONL and creates the provider Batch, then
// records the outcome. A failed CreateBatch response is always ambiguous because
// the provider may have accepted creation before the response was lost.
func (w *BatchSubmitWorker) createProviderBatch(ctx context.Context, args BatchSubmitJobArgs, claimed domain.PreparedDeckBatchChunk, token string, metadata map[string]string, items []enrichment.BatchTranslationItem) error {
	content, err := writeChunkJSONL(w.Codec, args.RunID, args.Generation, items, claimed)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "validation", "serialized_chunk")
	}
	filename := fmt.Sprintf("batch_%s.jsonl", claimed.ID)
	inputFile, err := w.Provider.UploadFile(ctx, filename, content)
	if err != nil {
		if temporaryProviderError(err) {
			return w.snoozeAfterSubmissionError(ctx, args, token, providerErrorCode(err))
		}
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "upload", providerErrorCode(err))
	}
	if inputFile.ID == "" || inputFile.Bytes != claimed.InputBytes {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "upload", "file_contract")
	}
	created, err := w.Provider.CreateBatch(ctx, enrichment.CreateBatchRequest{InputFileID: inputFile.ID, Metadata: metadata})
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", "create_response_lost")
	}
	if !validCreatedBatch(created, inputFile.ID, metadata) {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", "create_contract")
	}
	if err := w.recordSubmitted(ctx, args, token, inputFile.ID, created.ID); err != nil {
		return err
	}
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchSubmissions, Phase: "submitting", State: "submitted", Provider: "openai", Value: 1})
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchRequests, Phase: "submitting", State: "submitted", Provider: "openai", Value: float64(claimed.RequestCount)})
	if !claimed.CreatedAt.IsZero() {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "batch", Name: MetricBatchQueueAge, Phase: "submitting", State: "submitted", Provider: "openai", Value: seconds(w.now().Sub(claimed.CreatedAt))})
	}
	return nil
}

func (w *BatchSubmitWorker) recordSubmitted(ctx context.Context, args BatchSubmitJobArgs, token, inputFileID, batchID string) (err error) {
	if w.Client == nil {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "submission", "poll_dispatch_unavailable")
	}
	tx, err := w.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	_, err = w.Store.RecordPreparedDeckBatchSubmittedTx(ctx, tx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, inputFileID, batchID, w.now().UTC(), func(ctx context.Context, tx pgx.Tx, chunk domain.PreparedDeckBatchChunk) (int64, error) {
		inserted, insertErr := w.Client.InsertTx(ctx, tx, BatchPollJobArgs{OwnerID: args.OwnerID, PreparationID: args.PreparationID, RunID: args.RunID, ChunkID: args.ChunkID, Generation: chunk.ReconciliationGeneration}, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
		if insertErr != nil {
			return 0, insertErr
		}
		if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
			return 0, errors.New("prepareddeck: River did not return a live Batch polling job")
		}
		return inserted.Job.ID, nil
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *BatchSubmitWorker) finishSubmissionFailure(ctx context.Context, args BatchSubmitJobArgs, token string, state domain.PreparedDeckBatchChunkState, class, code string) error {
	err := w.Store.FailPreparedDeckBatchSubmission(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, state, class, boundedProviderCode(code))
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return err
}

func (w *BatchSubmitWorker) snoozeAfterSubmissionError(ctx context.Context, args BatchSubmitJobArgs, token, code string) error {
	_, err := w.Store.RetryPreparedDeckBatchSubmission(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, "upload", boundedProviderCode(code))
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	if err != nil {
		return err
	}
	return river.JobSnooze(DefaultBatchPollInterval)
}

func (w *BatchSubmitWorker) advanceIfTerminal(ctx context.Context, args BatchSubmitJobArgs) error {
	if w.Client == nil {
		return errors.New("prepareddeck: finalizer dispatch unavailable")
	}
	_, err := w.Store.AdvancePreparedDeckRunIfTerminal(ctx, args.OwnerID, args.PreparationID, args.RunID, func(ctx context.Context, tx pgx.Tx, run domain.PreparedDeckRun) error {
		inserted, insertErr := w.Client.InsertTx(ctx, tx, FinalizeJobArgs{OwnerID: args.OwnerID, PreparationID: args.PreparationID, RunID: args.RunID, Generation: run.FinalizationDispatchGeneration}, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
		if insertErr != nil {
			return insertErr
		}
		if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
			return errors.New("prepareddeck: River did not return a live finalizer job")
		}
		return w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, args.OwnerID, args.PreparationID, args.RunID, run.FinalizationDispatchGeneration, inserted.Job.ID)
	})
	if errors.Is(err, persistence.ErrInvalidTransition) || errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
		return nil
	}
	return err
}

// findExistingBatch lists the provider's recent Batches and matches them
// against the local chunk identity, requiring exactly one match.
func (w *BatchSubmitWorker) findExistingBatch(ctx context.Context, metadata map[string]string, inputFileID string) (*enrichment.Batch, error) {
	recentAfter := w.now().Add(-7 * 24 * time.Hour).Unix()
	var match *enrichment.Batch
	err := w.forEachProviderBatch(ctx, func(batch enrichment.Batch) error {
		if batch.CreatedAt > 0 && batch.CreatedAt < recentAfter {
			return nil
		}
		if !batchMatchesChunk(batch, metadata, inputFileID) {
			return nil
		}
		if match != nil {
			return errors.New("multiple provider Batches match local chunk")
		}
		candidate := batch
		match = &candidate
		return nil
	})
	if err != nil {
		return nil, err
	}
	return match, nil
}

// forEachProviderBatch pages through the provider listing once, bounded to 1000
// pages so a misbehaving cursor cannot loop forever, and stops as soon as visit
// returns an error.
func (w *BatchSubmitWorker) forEachProviderBatch(ctx context.Context, visit func(enrichment.Batch) error) error {
	var after string
	for range 1000 {
		list, err := w.Provider.ListBatches(ctx, enrichment.ListBatchesRequest{After: after, Limit: 100})
		if err != nil {
			return errors.New("provider Batch listing failed")
		}
		for i := range list.Data {
			if err := visit(list.Data[i]); err != nil {
				return err
			}
		}
		if !list.HasMore || list.LastID == "" || list.LastID == after {
			return nil
		}
		after = list.LastID
	}
	return nil
}

func batchMatchesChunk(batch enrichment.Batch, metadata map[string]string, inputFileID string) bool {
	return metadataMatches(batch.Metadata, metadata) && batch.Endpoint == enrichment.OpenAIChatCompletionsEndpoint && (inputFileID == "" || batch.InputFileID == inputFileID)
}

func submissionBatchItems(deck cardexport.FrozenDeck, ordinals []int, outcomes map[int]domain.PreparedDeckTranslationOutcome) ([]enrichment.BatchTranslationItem, error) {
	work := make([]cardexport.WorkItem, 0, len(ordinals))
	for _, ordinal := range ordinals {
		item, ok := deck.WorkByOrdinal(ordinal)
		outcome, outcomeOK := outcomes[ordinal]
		if !ok || !outcomeOK {
			return nil, errors.New("invalid durable Batch item")
		}
		if outcome.State != domain.PreparedDeckOutcomePending && outcome.State != domain.PreparedDeckOutcomeCompleted {
			return nil, errors.New("invalid durable Batch outcome state")
		}
		work = append(work, item)
	}
	return batchTranslationItems(work), nil
}

func batchMetadata(runID, chunkID string, generation int) map[string]string {
	return map[string]string{"mouseion_run": runID, "mouseion_chunk": chunkID, "mouseion_generation": strconv.Itoa(generation)}
}

func metadataMatches(got, want map[string]string) bool {
	if len(got) != len(want) {
		return false
	}
	for key, value := range want {
		if got[key] != value {
			return false
		}
	}
	return true
}

func validCreatedBatch(created enrichment.Batch, inputFileID string, metadata map[string]string) bool {
	return created.ID != "" && created.InputFileID == inputFileID && created.Endpoint == enrichment.OpenAIChatCompletionsEndpoint && created.CompletionWindow == "24h" && metadataMatches(created.Metadata, metadata)
}

func providerErrorCode(err error) string {
	if providerErr, ok := errors.AsType[*enrichment.ProviderError](err); ok {
		return string(providerErr.Class)
	}
	return "provider_error"
}

func temporaryProviderError(err error) bool {
	var providerErr *enrichment.ProviderError
	return errors.As(err, &providerErr) && providerErr.Temporary()
}

func boundedProviderCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	// Provider error codes are untrusted input. Keep only the small vocabulary
	// used by our bounded classifiers; sanitized arbitrary provider strings can
	// still contain request IDs or other credential/source-derived data.
	switch value {
	case "invalid_request", "ineligible_endpoint", "authentication", "permission", "rate_limit", "token_limit_exceeded", "provider_5xx", "timeout", "provider_unavailable", "transport", "malformed_response", "response_too_large", "invalid_translation_response", "expired", "cancelled", "request_failed", "file_processing_failed", "validating", "in_progress", "finalizing", "cancelling", "missing_result", "duplicate_result", "unknown_result", "malformed_result", "contradictory_result", "delete_file",
		"cache_lookup", "run_load", "frozen_contract", "manifest_load", "outcome_load", "chunk_membership", "chunk_changed", "serialized_chunk", "creation_unconfirmed", "file_contract", "create_response_lost", "create_contract", "poll_dispatch_unavailable",
		"unsupported_status", "batch_identity", "missing_provider_file", "contradictory_counts", "missing_custom_id", "retry_plan":
		return value
	}
	return "provider_error"
}

func (w *BatchSubmitWorker) now() time.Time {
	if w != nil && w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func AddBatchSubmitWorker(workers *river.Workers, store *persistence.PostgresStore, client riverClient, provider batchProvider, codec *enrichment.TranslationCodec) {
	AddBatchSubmitWorkerWithMetrics(workers, store, client, provider, codec, nil)
}

func AddBatchSubmitWorkerWithMetrics(workers *river.Workers, store *persistence.PostgresStore, client riverClient, provider batchProvider, codec *enrichment.TranslationCodec, metrics BatchMetrics) {
	river.AddWorker(workers, &BatchSubmitWorker{Store: store, Client: client, Provider: provider, Codec: codec, Metrics: metrics})
}

// inputFileIDOr preserves the recovered provider file ID while allowing a
// provider response that omitted it only when the durable ID is already known.
func inputFileIDOr(c domain.PreparedDeckBatchChunk, recovered string) string {
	if c.InputFileID != "" {
		return c.InputFileID
	}
	return recovered
}
