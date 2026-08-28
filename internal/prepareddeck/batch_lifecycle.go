package prepareddeck

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
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

const batchReconciliationLease = 5 * time.Minute

type batchPollProvider interface {
	GetBatch(context.Context, string) (enrichment.Batch, error)
	FileContent(context.Context, string, io.Writer) error
}

type BatchPollWorker struct {
	river.WorkerDefaults[BatchPollJobArgs]
	Store        *persistence.PostgresStore
	Client       riverClient
	Provider     batchPollProvider
	Codec        *enrichment.TranslationCodec
	PollInterval time.Duration
	Now          func() time.Time
	Jitter       func(time.Duration) time.Duration
	Metrics      BatchMetrics
}

func (w *BatchPollWorker) Work(ctx context.Context, job *river.Job[BatchPollJobArgs]) error {
	if job == nil {
		return ErrInvalidInput
	}
	return w.Poll(ctx, job.Args)
}

// Poll owns one short provider observation. Nonterminal states are persisted
// before the same River job snoozes for the configured, bounded-jitter delay.
func (w *BatchPollWorker) Poll(ctx context.Context, args BatchPollJobArgs) error {
	if w == nil || w.Store == nil || w.Client == nil || w.Provider == nil || w.Codec == nil || strings.TrimSpace(args.OwnerID) == "" || strings.TrimSpace(args.PreparationID) == "" || strings.TrimSpace(args.RunID) == "" || strings.TrimSpace(args.ChunkID) == "" || args.Generation < 0 {
		return ErrInvalidInput
	}
	started := w.now()
	var totalStarted time.Time
	defer func() {
		observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchPhaseLatency, Phase: "waiting", Provider: "openai", Value: seconds(w.now().Sub(started))})
		if totalStarted.IsZero() {
			totalStarted = started
		}
		observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchTotalLatency, Phase: "waiting", Provider: "openai", Value: seconds(w.now().Sub(totalStarted))})
	}()
	chunk, err := w.Store.GetPreparedDeckBatchChunk(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID)
	if err != nil {
		return err
	}
	totalStarted = chunk.CreatedAt
	if chunk.ReconciliationGeneration != args.Generation {
		return nil
	}
	switch chunk.State {
	case domain.PreparedDeckBatchCompleted, domain.PreparedDeckBatchFailed, domain.PreparedDeckBatchCancelled, domain.PreparedDeckBatchAmbiguous:
		return nil
	}
	claimToken := uuid.NewString()
	claimed, err := w.Store.ClaimPreparedDeckBatchReconciliation(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, claimToken, w.now().Add(batchReconciliationLease))
	if err != nil {
		if errors.Is(err, persistence.ErrInvalidTransition) || errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return nil
		}
		return err
	}
	claimed.Ordinals = append([]int(nil), chunk.Ordinals...)
	batch, err := w.Provider.GetBatch(ctx, claimed.BatchID)
	if err != nil {
		if temporaryProviderError(err) {
			return w.snoozeAfterPollError(ctx, claimed, args, claimToken, providerErrorCode(err))
		}
		return w.failUntrustworthy(ctx, args, claimToken, string(enrichment.BatchStatusFailed), "reconciliation", providerErrorCode(err))
	}
	if !knownBatchStatus(batch.Status) {
		return w.failUntrustworthy(ctx, args, claimToken, string(enrichment.BatchStatusFailed), "validation", "unsupported_status")
	}
	if err = validatePolledBatch(claimed, batch); err != nil {
		return w.failUntrustworthy(ctx, args, claimToken, string(batch.Status), "validation", "batch_identity")
	}
	if string(batch.Status) != claimed.ProviderStatus {
		observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchProviderTransitions, Phase: "waiting", State: string(batch.Status), Provider: "openai", Value: 1})
	}
	if !terminalBatchStatus(batch.Status) {
		_, err = w.Store.FinishPreparedDeckBatchReconciliation(ctx, args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, claimToken, batchReconciliationUpdate(batch, domain.PreparedDeckBatchPolling, "", "", 0))
		if err != nil {
			if errors.Is(err, persistence.ErrInvalidTransition) || errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
				return nil
			}
			return err
		}
		return river.JobSnooze(w.pollDelay())
	}
	if terminalBatchFailsRun(batch) {
		// A provider-level terminal failure prevents trusted reconciliation. There
		// is no trustworthy basis for publishing a partial artifact, and a
		// cancelled provider Batch must never turn into a local retry.
		return w.failUntrustworthy(ctx, args, claimToken, string(batch.Status), terminalChunkErrorClass(batch.Status), terminalChunkErrorCode(batch.Status))
	}
	return w.reconcileTerminal(ctx, claimed, args, claimToken, batch)
}

func terminalBatchFailsRun(batch enrichment.Batch) bool {
	return batch.Status == enrichment.BatchStatusCancelled || (batch.Status == enrichment.BatchStatusFailed && !retryableBatchFailure(batchFailureClass(batch)))
}

func (w *BatchPollWorker) reconcileTerminal(ctx context.Context, chunk domain.PreparedDeckBatchChunk, args BatchPollJobArgs, token string, batch enrichment.Batch) error {
	if (batch.RequestCounts.Completed > 0 && batch.OutputFileID == "") || (batch.RequestCounts.Failed > 0 && batch.ErrorFileID == "") {
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "missing_result", "missing_provider_file")
	}
	snapshot, _, err := w.Store.LoadPreparedDeckManifest(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "validation", "manifest_load")
	}
	items, byOrdinal, err := batchResultItems(snapshot, chunk.Ordinals)
	if err != nil {
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "validation", "chunk_membership")
	}
	decoded, missingOrdinals, downloadErr := w.decodeProviderFiles(ctx, args.RunID, chunk.Generation, items, batch.OutputFileID, batch.ErrorFileID)
	if downloadErr != nil {
		var resultErr *enrichment.BatchResultError
		if errors.As(downloadErr, &resultErr) {
			class := batchResultFailureClass(resultErr.Kind)
			return w.failUntrustworthy(ctx, args, token, string(batch.Status), class, string(resultErr.Kind))
		}
		if temporaryProviderError(downloadErr) {
			return w.snoozeAfterPollError(ctx, chunk, args, token, providerErrorCode(downloadErr))
		}
		// Terminal files are part of the provider result contract. A missing
		// file or permanent download failure is therefore an explicit
		// reconciliation failure.
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "reconciliation", providerErrorCode(downloadErr))
	}
	providerCompleted, providerFailed := batchProviderResultCounts(decoded)
	successes, failures, expiredFailures := 0, 0, 0
	for _, outcome := range decoded {
		if outcome.Successful() {
			successes++
		} else {
			failures++
			if outcome.ErrorClass == enrichment.ProviderErrorExpired {
				expiredFailures++
			}
		}
	}
	if providerCompleted != batch.RequestCounts.Completed || providerFailed != batch.RequestCounts.Failed {
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "reconciliation", "contradictory_counts")
	}
	missing := len(missingOrdinals)
	if missing != chunk.RequestCount-len(decoded) {
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "reconciliation", "contradictory_counts")
	}
	if missing < 0 || (batch.Status == enrichment.BatchStatusCompleted && missing != 0) {
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "missing_result", "missing_custom_id")
	}
	run, err := w.Store.GetPreparedDeckRun(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return w.failUntrustworthy(ctx, args, token, string(batch.Status), "validation", "run_load")
	}
	updates := make([]persistence.PreparedDeckBatchItemReconciliation, 0, len(items))
	retryItems := make([]enrichment.BatchTranslationItem, 0, missing+failures)
	for _, item := range items {
		providerOutcome, found := decoded[item.Ordinal]
		if found && providerOutcome.Successful() {
			manifestItem := byOrdinal[item.Ordinal]
			response := providerOutcome.Response
			updates = append(updates, persistence.PreparedDeckBatchItemReconciliation{Ordinal: item.Ordinal, State: domain.PreparedDeckOutcomeCompleted, CacheEntry: &enrichment.CacheEntry{CacheKey: *manifestItem.CacheKey, Translation: response.Translation, Gloss: response.Gloss, SentenceTranslation: response.SentenceTranslation, SentenceTranslationTarget: response.SentenceTranslationTarget, CachedAt: w.now()}})
			continue
		}
		class := enrichment.ProviderErrorExpired
		if found {
			class = providerOutcome.ErrorClass
		} else if batch.Status != enrichment.BatchStatusExpired {
			class = batchFailureClass(batch)
		}
		retryable := retryableBatchFailure(class) && chunk.Generation < run.MaxBatchGenerations
		state, errorClass := domain.PreparedDeckOutcomeFailed, outcomeErrorClass(class)
		if retryable {
			state = domain.PreparedDeckOutcomePending
			retryItems = append(retryItems, item)
		} else if retryableBatchFailure(class) {
			errorClass = "retry_exhausted"
		}
		errorCode := string(class)
		if found && providerOutcome.ErrorCode != "" {
			errorCode = providerOutcome.ErrorCode
		} else if batch.Status == enrichment.BatchStatusFailed {
			errorCode = batchFailureCode(batch)
		}
		updates = append(updates, persistence.PreparedDeckBatchItemReconciliation{Ordinal: item.Ordinal, State: state, ErrorClass: errorClass, ErrorCode: boundedProviderCode(errorCode)})
	}
	var retryPlans []persistence.PreparedDeckBatchChunkPlan
	if len(retryItems) > 0 {
		retryPlans, err = PlanBatchChunks(w.Codec, args.RunID, chunk.Generation+1, run.Model, run.Endpoint, retryItems, BatchChunkLimits{MaxRequests: run.BatchMaxRequests, MaxBytes: run.BatchMaxBytes})
		if err != nil {
			return w.failUntrustworthy(ctx, args, token, string(batch.Status), "validation", "retry_plan")
		}
	}
	failedCount, expiredCount := batch.RequestCounts.Failed-expiredFailures, expiredFailures
	if batch.Status == enrichment.BatchStatusExpired {
		expiredCount += missing
	} else if batch.Status == enrichment.BatchStatusFailed || batch.Status == enrichment.BatchStatusCancelled {
		failedCount += missing
	}
	update := batchReconciliationUpdate(batch, domain.PreparedDeckBatchCompleted, terminalChunkErrorClass(batch.Status), terminalBatchErrorCode(batch), expiredCount)
	update.FailedCount = failedCount
	_, err = w.Store.ReconcilePreparedDeckBatch(ctx, persistence.PreparedDeckBatchReconcileParams{
		OwnerID: args.OwnerID, PreparationID: args.PreparationID, RunID: args.RunID, ChunkID: args.ChunkID,
		ReconciliationGeneration: args.Generation, ClaimToken: token, Chunk: update, Items: updates, RetryChunks: retryPlans,
	}, w.insertSubmissionJob, w.insertFinalizerJob)
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	if err == nil {
		// Cleanup is deliberately best effort. The durable recovery projection
		// will enqueue the bounded retry if this immediate attempt cannot run.
		if cleaner, ok := w.Provider.(batchFileDeleter); ok {
			cleanup := &BatchCleanupWorker{Store: w.Store, Provider: cleaner, Now: w.Now, Metrics: w.Metrics}
			_ = cleanup.Cleanup(context.WithoutCancel(ctx), BatchCleanupJobArgs{OwnerID: args.OwnerID, PreparationID: args.PreparationID, RunID: args.RunID, ChunkID: args.ChunkID, Generation: chunk.Generation})
		}
		for _, item := range updates {
			if item.State == domain.PreparedDeckOutcomePending {
				observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchRetries, Phase: "reconciling", State: "pending", ErrorClass: item.ErrorClass, Provider: "openai", Value: 1})
			}
		}
		if batch.Status == enrichment.BatchStatusCompleted {
			observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchRequests, Phase: "reconciling", State: "completed", Provider: "openai", Value: float64(successes)})
		}
		observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchUsageInputTokens, Phase: "reconciling", State: string(batch.Status), Provider: "openai", Value: float64(batch.Usage.InputTokens)})
		observeBatchMetric(w.Metrics, BatchMetric{Name: MetricBatchUsageOutputTokens, Phase: "reconciling", State: string(batch.Status), Provider: "openai", Value: float64(batch.Usage.OutputTokens)})
	}
	return err
}

// batchProviderResultCounts mirrors OpenAI's request_counts semantics: a
// request with an HTTP 2xx response is completed even when its response body
// fails Mouseion's translation validation. Those application-level failures
// are persisted as item outcomes below, rather than treated as contradictory
// provider counts.
func batchProviderResultCounts(outcomes map[int]enrichment.BatchTranslationOutcome) (completed, failed int) {
	for _, outcome := range outcomes {
		if outcome.StatusCode >= 200 && outcome.StatusCode < 300 {
			completed++
		} else {
			failed++
		}
	}
	return completed, failed
}

func (w *BatchPollWorker) insertSubmissionJob(ctx context.Context, tx pgx.Tx, chunk domain.PreparedDeckBatchChunk) (int64, error) {
	inserted, err := w.Client.InsertTx(ctx, tx, BatchSubmitJobArgs{OwnerID: chunk.OwnerID, PreparationID: chunk.PreparationID, RunID: chunk.RunID, ChunkID: chunk.ID, Generation: chunk.Generation}, durableInsertOpts())
	if err != nil {
		return 0, err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return 0, errors.New("River did not return a live Batch submission job")
	}
	return inserted.Job.ID, nil
}

func (w *BatchPollWorker) insertFinalizerJob(ctx context.Context, tx pgx.Tx, run domain.PreparedDeckRun) error {
	inserted, err := w.Client.InsertTx(ctx, tx, FinalizeJobArgs{OwnerID: run.OwnerID, PreparationID: run.PreparationID, RunID: run.ID, Generation: run.FinalizationDispatchGeneration}, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("River did not return a live finalizer job")
	}
	return w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, run.OwnerID, run.PreparationID, run.ID, run.FinalizationDispatchGeneration, inserted.Job.ID)
}

func (w *BatchPollWorker) decodeProviderFiles(ctx context.Context, runID string, generation int, items []enrichment.BatchTranslationItem, outputFileID, errorFileID string) (map[int]enrichment.BatchTranslationOutcome, []int, error) {
	output := w.streamFile(ctx, outputFileID)
	errorOutput := w.streamFile(ctx, errorFileID)
	var outputReader, errorReader io.Reader
	if output != nil {
		outputReader = output.reader
		defer output.reader.Close()
	}
	if errorOutput != nil {
		errorReader = errorOutput.reader
		defer errorOutput.reader.Close()
	}
	decoded, missing, err := w.Codec.DecodeBatchResultsPartial(runID, generation, items, outputReader, errorReader)
	var downloadErr error
	if output != nil {
		_ = output.reader.Close()
		if fileErr := <-output.done; downloadErr == nil {
			downloadErr = fileErr
		}
	}
	if errorOutput != nil {
		_ = errorOutput.reader.Close()
		if fileErr := <-errorOutput.done; downloadErr == nil {
			downloadErr = fileErr
		}
	}
	if downloadErr != nil {
		return nil, nil, downloadErr
	}
	return decoded, missing, err
}

type streamedBatchFile struct {
	reader *io.PipeReader
	done   <-chan error
}

func (w *BatchPollWorker) streamFile(ctx context.Context, fileID string) *streamedBatchFile {
	if fileID == "" {
		return nil
	}
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := w.Provider.FileContent(ctx, fileID, writer)
		_ = writer.CloseWithError(err)
		done <- err
	}()
	return &streamedBatchFile{reader: reader, done: done}
}

func (w *BatchPollWorker) snoozeAfterPollError(ctx context.Context, chunk domain.PreparedDeckBatchChunk, args BatchPollJobArgs, token, code string) error {
	_, err := w.Store.FinishPreparedDeckBatchReconciliation(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, persistence.PreparedDeckBatchReconciliationUpdate{State: domain.PreparedDeckBatchPolling, ProviderStatus: chunk.ProviderStatus, OutputFileID: chunk.OutputFileID, ErrorFileID: chunk.ErrorFileID, CompletedCount: chunk.CompletedCount, FailedCount: chunk.FailedCount, ExpiredCount: chunk.ExpiredCount, InputTokens: chunk.InputTokens, OutputTokens: chunk.OutputTokens, ErrorClass: "poll", ErrorCode: boundedProviderCode(code), ProviderCompletedAt: chunk.ProviderCompletedAt})
	if err != nil && !errors.Is(err, persistence.ErrInvalidTransition) && !errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
		return err
	}
	return river.JobSnooze(w.pollDelay())
}

func (w *BatchPollWorker) failUntrustworthy(ctx context.Context, args BatchPollJobArgs, token, providerStatus, class, code string) error {
	metricName := MetricBatchReconciliationErrors
	if class == "validation" || strings.HasSuffix(class, "result") {
		metricName = MetricBatchValidationFailures
	}
	observeBatchMetric(w.Metrics, BatchMetric{Name: metricName, Phase: "reconciling", State: providerStatus, ErrorClass: class, Provider: "openai", Value: 1})
	if providerStatus == "" || !terminalBatchStatus(enrichment.BatchStatus(providerStatus)) {
		providerStatus = string(enrichment.BatchStatusFailed)
	}
	err := w.Store.FailPreparedDeckBatchReconciliation(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, args.RunID, args.ChunkID, args.Generation, token, providerStatus, class, boundedProviderCode(code))
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
		return nil
	}
	return err
}

func validatePolledBatch(chunk domain.PreparedDeckBatchChunk, batch enrichment.Batch) error {
	if batch.ID != chunk.BatchID || batch.InputFileID != chunk.InputFileID || batch.Endpoint != chunk.Endpoint || !metadataMatches(batch.Metadata, batchMetadata(chunk.RunID, chunk.ID, chunk.Generation)) {
		return errors.New("provider Batch contradicts durable chunk")
	}
	// OpenAI documents request_counts as optional and may return it absent or
	// zero while a Batch is still validating. The immutable input file and
	// opaque metadata establish identity at that point; the exact request count
	// becomes mandatory for completed and expired Batches because those states
	// are reconciled against provider files. Validation failure and cancellation
	// can become terminal before OpenAI has populated the counts, and neither
	// state publishes item results.
	if batch.RequestCounts.Total == chunk.RequestCount {
		return nil
	}
	if batch.RequestCounts.Total == 0 && batch.RequestCounts.Completed == 0 && batch.RequestCounts.Failed == 0 && batchCanOmitRequestCounts(batch.Status) {
		return nil
	}
	return errors.New("provider Batch contradicts durable chunk")
}

func batchCanOmitRequestCounts(status enrichment.BatchStatus) bool {
	switch status {
	case enrichment.BatchStatusValidating, enrichment.BatchStatusInProgress, enrichment.BatchStatusFinalizing,
		enrichment.BatchStatusFailed, enrichment.BatchStatusCancelling, enrichment.BatchStatusCancelled:
		return true
	default:
		return false
	}
}

func batchResultItems(snapshot cardexport.ManifestSnapshot, ordinals []int) ([]enrichment.BatchTranslationItem, map[int]cardexport.ManifestItem, error) {
	manifestItems := make(map[int]cardexport.ManifestItem, len(snapshot.Items))
	for _, item := range snapshot.Items {
		manifestItems[item.Ordinal] = item
	}
	items := make([]enrichment.BatchTranslationItem, 0, len(ordinals))
	selected := make(map[int]cardexport.ManifestItem, len(ordinals))
	for _, ordinal := range ordinals {
		item, ok := manifestItems[ordinal]
		if !ok || item.Disposition != cardexport.ManifestAccepted || item.CacheKey == nil {
			return nil, nil, errors.New("invalid Batch manifest item")
		}
		selected[ordinal] = item
		items = append(items, enrichment.BatchTranslationItem{Ordinal: ordinal, Request: enrichment.TranslationRequest{Language: item.Entry.Language, CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence}})
	}
	return items, selected, nil
}

func batchReconciliationUpdate(batch enrichment.Batch, state domain.PreparedDeckBatchChunkState, errorClass, errorCode string, expired int) persistence.PreparedDeckBatchReconciliationUpdate {
	completedAt := providerTerminalTime(batch)
	return persistence.PreparedDeckBatchReconciliationUpdate{State: state, ProviderStatus: string(batch.Status), OutputFileID: batch.OutputFileID, ErrorFileID: batch.ErrorFileID, CompletedCount: batch.RequestCounts.Completed, FailedCount: batch.RequestCounts.Failed, ExpiredCount: expired, InputTokens: batch.Usage.InputTokens, OutputTokens: batch.Usage.OutputTokens, ErrorClass: errorClass, ErrorCode: errorCode, ProviderCompletedAt: completedAt}
}

func providerTerminalTime(batch enrichment.Batch) *time.Time {
	var unix int64
	switch batch.Status {
	case enrichment.BatchStatusCompleted:
		unix = batch.CompletedAt
	case enrichment.BatchStatusFailed:
		unix = batch.FailedAt
	case enrichment.BatchStatusExpired:
		unix = batch.ExpiredAt
	case enrichment.BatchStatusCancelled:
		unix = batch.CancelledAt
	}
	if unix <= 0 {
		return nil
	}
	result := time.Unix(unix, 0).UTC()
	return &result
}

func terminalBatchStatus(status enrichment.BatchStatus) bool {
	switch status {
	case enrichment.BatchStatusCompleted, enrichment.BatchStatusFailed, enrichment.BatchStatusExpired, enrichment.BatchStatusCancelled:
		return true
	default:
		return false
	}
}

func knownBatchStatus(status enrichment.BatchStatus) bool {
	switch status {
	case enrichment.BatchStatusValidating, enrichment.BatchStatusInProgress, enrichment.BatchStatusFinalizing,
		enrichment.BatchStatusCompleted, enrichment.BatchStatusFailed, enrichment.BatchStatusExpired,
		enrichment.BatchStatusCancelling, enrichment.BatchStatusCancelled:
		return true
	default:
		return false
	}
}

func retryableBatchFailure(class enrichment.ProviderErrorClass) bool {
	switch class {
	case enrichment.ProviderErrorRateLimit, enrichment.ProviderErrorTimeout, enrichment.ProviderErrorUnavailable, enrichment.ProviderErrorTransport, enrichment.ProviderErrorExpired, enrichment.ProviderErrorCancelled:
		return true
	default:
		return false
	}
}

func batchFailureClass(batch enrichment.Batch) enrichment.ProviderErrorClass {
	if len(batch.Errors) == 0 {
		if batch.Status == enrichment.BatchStatusCancelled {
			return enrichment.ProviderErrorCancelled
		}
		return enrichment.ProviderErrorRequestFailed
	}
	class := batch.Errors[0].Class
	for _, issue := range batch.Errors[1:] {
		if issue.Class != class {
			return enrichment.ProviderErrorRequestFailed
		}
	}
	return class
}

func batchFailureCode(batch enrichment.Batch) string {
	if len(batch.Errors) == 0 {
		return string(enrichment.ProviderErrorRequestFailed)
	}
	code := batch.Errors[0].Code
	if code == "" {
		code = string(batch.Errors[0].Class)
	}
	for _, issue := range batch.Errors[1:] {
		issueCode := issue.Code
		if issueCode == "" {
			issueCode = string(issue.Class)
		}
		if issueCode != code {
			return string(enrichment.ProviderErrorRequestFailed)
		}
	}
	return boundedProviderCode(code)
}

func outcomeErrorClass(class enrichment.ProviderErrorClass) string {
	switch class {
	case enrichment.ProviderErrorRateLimit:
		return "rate_limit"
	case enrichment.ProviderErrorTimeout:
		return "timeout"
	case enrichment.ProviderErrorExpired:
		return "expired"
	case enrichment.ProviderErrorCancelled:
		return "cancellation"
	case enrichment.ProviderErrorInvalidResponse, enrichment.ProviderErrorMalformedResponse, enrichment.ProviderErrorResponseTooLarge:
		return "validation"
	default:
		return "provider"
	}
}

func terminalChunkErrorClass(status enrichment.BatchStatus) string {
	switch status {
	case enrichment.BatchStatusExpired:
		return "expired"
	case enrichment.BatchStatusCancelled:
		return "cancelled"
	case enrichment.BatchStatusFailed:
		return "provider"
	default:
		return ""
	}
}

func terminalChunkErrorCode(status enrichment.BatchStatus) string {
	if status == enrichment.BatchStatusCompleted {
		return ""
	}
	return boundedProviderCode(string(status))
}

func terminalBatchErrorCode(batch enrichment.Batch) string {
	if batch.Status == enrichment.BatchStatusFailed {
		return batchFailureCode(batch)
	}
	return terminalChunkErrorCode(batch.Status)
}

func batchResultFailureClass(kind enrichment.BatchResultErrorKind) string {
	switch kind {
	case enrichment.BatchResultUnknown:
		return "unknown_result"
	case enrichment.BatchResultDuplicate:
		return "duplicate_result"
	case enrichment.BatchResultMissing:
		return "missing_result"
	default:
		return "malformed_result"
	}
}

func durableInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{Queue: Queue, MaxAttempts: durableJobMaxAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}}
}

func (w *BatchPollWorker) now() time.Time {
	if w != nil && w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *BatchPollWorker) pollDelay() time.Duration {
	interval := w.PollInterval
	if interval <= 0 {
		interval = DefaultBatchPollInterval
	}
	if w.Jitter != nil {
		delay := w.Jitter(interval)
		if delay >= interval-interval/10 && delay <= interval+interval/10 {
			return delay
		}
		return interval
	}
	spread := interval / 10
	if spread <= 0 {
		return interval
	}
	return interval - spread + time.Duration(rand.Int64N(int64(2*spread)+1))
}

type FinalizeWorker struct {
	river.WorkerDefaults[FinalizeJobArgs]
	Finalizer *DurableFinalizer
}

func (w *FinalizeWorker) Work(ctx context.Context, job *river.Job[FinalizeJobArgs]) error {
	if w == nil || w.Finalizer == nil || job == nil {
		return ErrInvalidInput
	}
	_, err := w.Finalizer.Finalize(ctx, job.Args.OwnerID, job.Args.PreparationID, job.Args.RunID, job.Args.Generation)
	if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) || errors.Is(err, persistence.ErrFenced) {
		return nil
	}
	return err
}

func AddBatchPollWorker(workers *river.Workers, worker *BatchPollWorker) {
	river.AddWorker(workers, worker)
}

func AddFinalizeWorker(workers *river.Workers, finalizer *DurableFinalizer) {
	river.AddWorker(workers, &FinalizeWorker{Finalizer: finalizer})
}
