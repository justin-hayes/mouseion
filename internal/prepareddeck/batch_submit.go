package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
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
}

func (w *BatchSubmitWorker) Work(ctx context.Context, job *river.Job[BatchSubmitJobArgs]) error {
	if job == nil {
		return ErrInvalidInput
	}
	return w.Submit(ctx, job.Args)
}

func (w *BatchSubmitWorker) Submit(ctx context.Context, args BatchSubmitJobArgs) error {
	if w == nil || w.Store == nil || w.Provider == nil || w.Codec == nil || strings.TrimSpace(args.OwnerID) == "" || strings.TrimSpace(args.PreparationID) == "" || strings.TrimSpace(args.RunID) == "" || strings.TrimSpace(args.ChunkID) == "" || args.Generation < 1 {
		return ErrInvalidInput
	}
	chunk, err := w.Store.GetPreparedDeckBatchChunk(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID)
	if err != nil {
		return err
	}
	if chunk.Generation != args.Generation {
		return nil
	}
	switch chunk.State {
	case domain.PreparedDeckBatchSubmitted, domain.PreparedDeckBatchPolling, domain.PreparedDeckBatchReconciling, domain.PreparedDeckBatchCompleted:
		return nil
	case domain.PreparedDeckBatchFailed, domain.PreparedDeckBatchCancelled, domain.PreparedDeckBatchAmbiguous:
		return nil
	}

	wasSubmitting := chunk.State == domain.PreparedDeckBatchSubmitting
	claimToken := uuid.NewString()
	claimed, err := w.Store.ClaimPreparedDeckBatchSubmission(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, claimToken, w.now().UTC().Add(batchSubmissionLease))
	if err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
			return nil
		}
		return err
	}
	// Claim transitions return the scalar chunk row; retain the immutable
	// ordered membership loaded immediately before the claim.
	claimed.Ordinals = append([]int(nil), chunk.Ordinals...)

	if _, err = w.Store.CompletePreparedDeckBatchCacheHits(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, claimToken); err != nil {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "validation", "cache_lookup")
	}
	run, err := w.Store.GetPreparedDeckRun(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "validation", "run_load")
	}
	if run.State != domain.PreparedDeckRunTranslating || !run.ExternalTranslationConsent || !run.ExternalTranslationConfigured || run.Model != claimed.Model || run.Endpoint != claimed.Endpoint || run.BatchMaxRequests < claimed.RequestCount || run.BatchMaxBytes < claimed.InputBytes {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "configuration", "frozen_contract")
	}
	snapshot, _, err := w.Store.LoadPreparedDeckManifest(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "validation", "manifest_load")
	}
	outcomes, err := w.Store.ListPreparedDeckTranslationOutcomes(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "validation", "outcome_load")
	}
	byOrdinal := make(map[int]domain.PreparedDeckTranslationOutcome, len(outcomes))
	for _, outcome := range outcomes {
		byOrdinal[outcome.Ordinal] = outcome
	}
	items, err := activeBatchItems(snapshot, claimed.Ordinals, byOrdinal)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "validation", "chunk_membership")
	}
	if len(items) == 0 {
		_, finishErr := w.Store.FinishPreparedDeckBatchSubmission(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, claimToken, domain.PreparedDeckBatchCompleted, "", "")
		return finishErr
	}
	if len(items) != claimed.RequestCount || claimed.InputBytes <= 0 || claimed.InputDigest == "" {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "validation", "chunk_changed")
	}
	if claimed.Model != w.Codec.Model() || claimed.Endpoint != enrichment.OpenAIChatCompletionsEndpoint {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "configuration", "frozen_contract")
	}
	if err = w.Store.VerifyPreparedDeckBatchSubmissionClaim(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, claimToken); err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return nil
		}
		return err
	}

	metadata := batchMetadata(args.RunID, args.ChunkID, args.Generation)
	if wasSubmitting {
		match, recoverErr := w.findExistingBatch(ctx, metadata, claimed.InputFileID)
		if recoverErr != nil {
			return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", recoverErr.Error())
		}
		if match == nil {
			return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", "creation_unconfirmed")
		}
		return w.recordSubmitted(ctx, args, claimToken, inputFileIDOr(claimed, match.InputFileID), match.ID)
	}

	content, err := writeChunkJSONL(w.Codec, args.RunID, args.Generation, items, claimed)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "validation", "serialized_chunk")
	}
	filename := fmt.Sprintf("batch_%s.jsonl", claimed.ID)
	inputFile, err := w.Provider.UploadFile(ctx, filename, content)
	if err != nil {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "upload", providerErrorCode(err))
	}
	if inputFile.ID == "" || inputFile.Bytes != claimed.InputBytes {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchFailed, "upload", "file_contract")
	}
	created, err := w.Provider.CreateBatch(ctx, enrichment.CreateBatchRequest{InputFileID: inputFile.ID, Metadata: metadata})
	if err != nil {
		// The provider may have accepted creation before the response was lost.
		// Never retry this boundary automatically.
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", "create_response_lost")
	}
	if created.InputFileID != inputFile.ID || created.Endpoint != enrichment.OpenAIChatCompletionsEndpoint || created.CompletionWindow != "24h" || created.RequestCounts.Total != claimed.RequestCount || !metadataMatches(created.Metadata, metadata) {
		return w.finishSubmissionFailure(ctx, args, claimToken, domain.PreparedDeckBatchAmbiguous, "ambiguous_submission", "create_contract")
	}
	return w.recordSubmitted(ctx, args, claimToken, inputFile.ID, created.ID)
}

func (w *BatchSubmitWorker) recordSubmitted(ctx context.Context, args BatchSubmitJobArgs, token, inputFileID, batchID string) error {
	if w.Client == nil {
		return w.finishSubmissionFailure(ctx, args, token, domain.PreparedDeckBatchFailed, "submission", "poll_dispatch_unavailable")
	}
	tx, err := w.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = w.Store.RecordPreparedDeckBatchSubmittedTx(ctx, tx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, inputFileID, batchID, w.now().UTC(), func(ctx context.Context, tx pgx.Tx, chunk domain.PreparedDeckBatchChunk) (int64, error) {
		inserted, insertErr := w.Client.InsertTx(ctx, tx, BatchPollJobArgs{OwnerID: args.OwnerID, PreparationID: args.PreparationID, RunID: args.RunID, ChunkID: args.ChunkID, Generation: chunk.ReconciliationGeneration}, &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
		if insertErr != nil {
			return 0, insertErr
		}
		if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
			return 0, errors.New("River did not return a live Batch polling job")
		}
		return inserted.Job.ID, nil
	})
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (w *BatchSubmitWorker) finishSubmissionFailure(ctx context.Context, args BatchSubmitJobArgs, token string, state domain.PreparedDeckBatchChunkState, class, code string) error {
	_, err := w.Store.FinishPreparedDeckBatchSubmission(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, state, class, boundedProviderCode(code))
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return err
}

func (w *BatchSubmitWorker) findExistingBatch(ctx context.Context, metadata map[string]string, inputFileID string) (*enrichment.Batch, error) {
	var match *enrichment.Batch
	var after string
	recentAfter := w.now().Add(-7 * 24 * time.Hour).Unix()
	for page := 0; page < 1000; page++ {
		list, err := w.Provider.ListBatches(ctx, enrichment.ListBatchesRequest{After: after, Limit: 100})
		if err != nil {
			return nil, errors.New("provider Batch listing failed")
		}
		for i := range list.Data {
			batch := list.Data[i]
			if batch.CreatedAt > 0 && batch.CreatedAt < recentAfter {
				continue
			}
			if metadataMatches(batch.Metadata, metadata) && batch.Endpoint == enrichment.OpenAIChatCompletionsEndpoint && (inputFileID == "" || batch.InputFileID == inputFileID) {
				if match != nil {
					return nil, errors.New("multiple provider Batches match local chunk")
				}
				candidate := batch
				match = &candidate
			}
		}
		if !list.HasMore || list.LastID == "" || list.LastID == after {
			break
		}
		after = list.LastID
	}
	return match, nil
}

func activeBatchItems(snapshot cardexport.ManifestSnapshot, ordinals []int, outcomes map[int]domain.PreparedDeckTranslationOutcome) ([]enrichment.BatchTranslationItem, error) {
	byOrdinal := make(map[int]cardexport.ManifestItem, len(snapshot.Items))
	for _, item := range snapshot.Items {
		byOrdinal[item.Ordinal] = item
	}
	items := make([]enrichment.BatchTranslationItem, 0, len(ordinals))
	for _, ordinal := range ordinals {
		item, ok := byOrdinal[ordinal]
		outcome, outcomeOK := outcomes[ordinal]
		if !ok || !outcomeOK || item.Disposition != cardexport.ManifestAccepted || item.CacheKey == nil {
			return nil, errors.New("invalid durable Batch item")
		}
		if outcome.State != domain.PreparedDeckOutcomePending {
			continue
		}
		items = append(items, enrichment.BatchTranslationItem{Ordinal: ordinal, Request: enrichment.TranslationRequest{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence}})
	}
	return items, nil
}

func batchMetadata(runID, chunkID string, generation int) map[string]string {
	return map[string]string{"mouseion_run": runID, "mouseion_chunk": chunkID, "mouseion_generation": fmt.Sprintf("%d", generation)}
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

func providerErrorCode(err error) string {
	var providerErr *enrichment.ProviderError
	if errors.As(err, &providerErr) {
		return string(providerErr.Class)
	}
	return "provider_error"
}

func boundedProviderCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '.' || r == ':' || r == '-' {
			b.WriteRune(r)
		}
		if b.Len() == 80 {
			break
		}
	}
	if b.Len() == 0 {
		return "provider_error"
	}
	return b.String()
}

func (w *BatchSubmitWorker) now() time.Time {
	if w != nil && w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func AddBatchSubmitWorker(workers *river.Workers, store *persistence.PostgresStore, client riverClient, provider batchProvider, codec *enrichment.TranslationCodec) {
	river.AddWorker(workers, &BatchSubmitWorker{Store: store, Client: client, Provider: provider, Codec: codec})
}

// inputFileIDOr preserves the recovered provider file ID while allowing a
// provider response that omitted it only when the durable ID is already known.
func inputFileIDOr(c domain.PreparedDeckBatchChunk, recovered string) string {
	if c.InputFileID != "" {
		return c.InputFileID
	}
	return recovered
}
