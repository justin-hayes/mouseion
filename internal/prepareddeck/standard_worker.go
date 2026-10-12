package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net"
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

const standardTranslationLease = 5 * time.Minute

func (w *StandardTranslationWorker) now() time.Time {
	if w != nil && w.Now != nil {
		return w.Now().UTC()
	}
	return time.Now().UTC()
}

func (w *StandardTranslationWorker) execute(ctx context.Context, args StandardTranslationJobArgs) error {
	if !w.canExecute(args) {
		return ErrInvalidInput
	}
	token, claimed, proceed, err := w.claimTranslation(ctx, args)
	if err != nil || !proceed {
		return err
	}
	work, found, err := w.restoreWork(ctx, args, token)
	if !found {
		return err
	}
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricTranslationUnits, Phase: "translating", State: "in_progress", Provider: "openai", Value: 1})
	if done, err := w.completeFromCache(ctx, args, token, work); done {
		return err
	}
	if err := w.Store.VerifyPreparedDeckTranslationClaim(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token); err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return nil
		}
		return err
	}
	return w.translateClaimed(ctx, args, token, claimed, work)
}

// claimTranslation loads the frozen run and fences one item. It reports
// proceed=false when the run no longer admits standard translation or another
// owner holds the item claim.
func (w *StandardTranslationWorker) claimTranslation(ctx context.Context, args StandardTranslationJobArgs) (string, domain.PreparedDeckTranslationOutcome, bool, error) {
	run, err := w.Store.GetPreparedDeckRun(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return "", domain.PreparedDeckTranslationOutcome{}, false, err
	}
	if run.ExecutionMode != domain.PreparedDeckExecutionStandard || run.State != domain.PreparedDeckRunTranslating {
		return "", domain.PreparedDeckTranslationOutcome{}, false, nil
	}
	token := uuid.NewString()
	claimed, err := w.Store.ClaimPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, w.now().Add(standardTranslationLease))
	if err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
			return "", domain.PreparedDeckTranslationOutcome{}, false, nil
		}
		return "", domain.PreparedDeckTranslationOutcome{}, false, err
	}
	return token, claimed, true, nil
}

// translateClaimed makes the bounded provider call for one claimed item,
// validates the response, and persists the outcome.
func (w *StandardTranslationWorker) translateClaimed(ctx context.Context, args StandardTranslationJobArgs, token string, claimed domain.PreparedDeckTranslationOutcome, work cardexport.WorkItem) error {
	request := work.Request
	response, providerLatency, callErr := w.translate(ctx, request)
	if callErr != nil {
		class, code, retryable := classifyStandardProviderError(callErr, ctx)
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricProviderErrors, Phase: "provider", State: "failed", ErrorClass: standardMetricErrorClass(class, code), Provider: "openai", Value: 1})
		return w.retryOrFail(ctx, args, token, claimed, retryable, class, code, providerLatency)
	}
	response, callErr = enrichment.NormalizeTranslationResponse(request, response)
	if callErr != nil {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchValidationFailures, Phase: "provider", State: "failed", ErrorClass: "validation", Provider: "openai", Value: 1})
		return w.retryOrFail(ctx, args, token, claimed, true, "validation", "invalid_response", providerLatency)
	}
	for _, warning := range response.Warnings {
		log.Printf("prepared deck translation: %s", warning)
	}
	return w.persistTranslation(ctx, args, token, work, response, providerLatency)
}

func (w *StandardTranslationWorker) canExecute(args StandardTranslationJobArgs) bool {
	return w != nil && w.Store != nil && w.Provider != nil && w.Provider.Enabled() && w.Client != nil && strings.TrimSpace(args.OwnerID) != "" && strings.TrimSpace(args.PreparationID) != "" && strings.TrimSpace(args.RunID) != "" && args.Ordinal >= 0 && args.Generation >= 0
}

// restoreWork finds the claimed item in the frozen manifest. When the item
// cannot be restored it fails the item and reports found=false with the
// result of recording that failure.
func (w *StandardTranslationWorker) restoreWork(ctx context.Context, args StandardTranslationJobArgs, token string) (cardexport.WorkItem, bool, error) {
	snapshot, _, err := w.Store.LoadPreparedDeckStorageProjection(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return cardexport.WorkItem{}, false, w.fail(ctx, args, token, "orchestration", "manifest", true)
	}
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	if err != nil {
		return cardexport.WorkItem{}, false, w.fail(ctx, args, token, "identity", "manifest_item", true)
	}
	work, found := deck.WorkByOrdinal(args.Ordinal)
	if !found {
		return cardexport.WorkItem{}, false, w.fail(ctx, args, token, "identity", "manifest_item", true)
	}
	return work, true, nil
}

// completeFromCache finishes the item from an exact, complete cache entry.
// It reports done=false on a cache miss, when the provider must be called.
func (w *StandardTranslationWorker) completeFromCache(ctx context.Context, args StandardTranslationJobArgs, token string, work cardexport.WorkItem) (bool, error) {
	entry, hit, err := w.Store.Get(ctx, work.CacheKey)
	if err != nil {
		return true, w.fail(ctx, args, token, "cache", "cache_lookup", false)
	}
	if !hit || !enrichment.HasRequiredTranslationFields(entry, work.Request.ExampleSentence) || (work.Request.RequireContextualGloss && strings.TrimSpace(entry.FallbackGloss) == "") {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricCacheMisses, Phase: "cache", State: "pending", Provider: "openai", Value: 1})
		return false, nil
	}
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricCacheHits, Phase: "cache", State: "completed", Provider: "openai", Value: 1})
	_, _, err = w.Store.FinishPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, persistence.PreparedDeckOutcomeTerminalUpdate{State: domain.PreparedDeckOutcomeCompleted, CacheHit: true, CacheLatency: 0}, w.finalizer)
	return true, err
}

// translate makes one bounded provider call and records its request metrics.
func (w *StandardTranslationWorker) translate(ctx context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, time.Duration, error) {
	started := w.now()
	timeout := w.AttemptTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	var usage enrichment.TranslationUsage
	var response enrichment.TranslationResponse
	var callErr error
	if usageProvider, ok := w.Provider.(interface {
		TranslateWithUsage(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, enrichment.TranslationUsage, error)
	}); ok {
		response, usage, callErr = usageProvider.TranslateWithUsage(callCtx, request)
	} else {
		response, callErr = w.Provider.Translate(callCtx, request)
	}
	cancel()
	providerLatency := w.now().Sub(started)
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchRequests, Phase: "provider", State: "completed", Provider: "openai", Value: 1})
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricProviderRequestLatency, Phase: "provider", State: "completed", Provider: "openai", Value: seconds(providerLatency)})
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageInputCount, Phase: "provider", State: "completed", Provider: "openai", Value: float64(usage.PromptTokens)})
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageOutputCount, Phase: "provider", State: "completed", Provider: "openai", Value: float64(usage.CompletionTokens)})
	return response, providerLatency, callErr
}

func (w *StandardTranslationWorker) retryOrFail(ctx context.Context, args StandardTranslationJobArgs, token string, claimed domain.PreparedDeckTranslationOutcome, retryable bool, class, code string, providerLatency time.Duration) error {
	if shouldRetryStandardProviderAttempt(retryable, claimed.ProviderAttemptCount, claimed.MaxProviderAttempts) {
		return w.retry(ctx, args, token, claimed.ProviderAttemptCount+1, class, code)
	}
	return w.failWithLatency(ctx, args, token, class, code, true, providerLatency)
}

// persistTranslation completes the item from a validated provider response:
// as an explicit omission when the meaning is unresolved, otherwise by
// storing the claimed cache entry.
func (w *StandardTranslationWorker) persistTranslation(ctx context.Context, args StandardTranslationJobArgs, token string, work cardexport.WorkItem, response enrichment.TranslationResponse, providerLatency time.Duration) error {
	request := work.Request
	if request.RequireContextualGloss && response.Gloss == "" {
		_, _, finishErr := w.Store.FinishPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, persistence.PreparedDeckOutcomeTerminalUpdate{
			State: domain.PreparedDeckOutcomeCompleted, OmissionReason: response.UnresolvedReason,
			ProviderAttempt: true, ProviderCall: true, ProviderLatency: providerLatency,
		}, w.finalizer)
		return finishErr
	}
	selection := evidenceSelectionIndices(request.CandidateSenses, response.EvidenceIDs)
	entry := enrichment.CacheEntry{CacheKey: work.CacheKey, Translation: response.Translation, FallbackGloss: response.Gloss, SenseSelection: selection, SentenceTranslation: response.SentenceTranslation, SentenceTranslationTargets: append([]string(nil), response.SentenceTranslationTargets...), CachedAt: w.now()}
	stored, err := w.Store.PutPreparedDeckTranslationIfClaimed(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, entry)
	if err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return nil
		}
		return w.failWithLatency(ctx, args, token, "cache", "cache_write", false, providerLatency)
	}
	if !enrichment.HasRequiredTranslationFields(stored, request.ExampleSentence) {
		return w.failWithLatency(ctx, args, token, "cache", "incomplete_entry", true, providerLatency)
	}
	_, _, err = w.Store.FinishPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, persistence.PreparedDeckOutcomeTerminalUpdate{State: domain.PreparedDeckOutcomeCompleted, ProviderAttempt: true, ProviderCall: true, ProviderLatency: providerLatency}, w.finalizer)
	if err == nil {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchProviderTransitions, Phase: "translating", State: "completed", Provider: "openai", Value: 1})
	}
	return err
}

func (w *StandardTranslationWorker) retry(ctx context.Context, args StandardTranslationJobArgs, token string, attempt int, class, code string) (err error) {
	base, maxDelay := w.Config.StandardRetryBaseDelay, w.Config.StandardRetryMaxDelay
	if base <= 0 {
		base = DefaultStandardRetryBaseDelay
	}
	if maxDelay <= 0 {
		maxDelay = DefaultStandardRetryMaxDelay
	}
	delay := standardRetryDelay(PreparedDeckConfig{StandardRetryBaseDelay: base, StandardRetryMaxDelay: maxDelay}, attempt)
	jitter := w.Jitter
	if jitter == nil {
		jitter = func(d time.Duration) time.Duration { return time.Duration(float64(d) * (0.8 + rand.Float64()*0.4)) } //nolint:gosec // retry jitter is not security-sensitive randomness.
	}
	next := w.now().Add(jitter(delay))
	updated, err := w.Store.RetryPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, next, class, code)
	if err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) {
			return nil
		}
		return err
	}
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchRetries, Phase: "translating", State: "retrying", ErrorClass: standardMetricErrorClass(class, code), Provider: "openai", Value: 1})
	newArgs := args
	newArgs.Generation = updated.DispatchGeneration
	tx, err := w.Store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	inserted, err := w.Client.InsertTx(ctx, tx, newArgs, &river.InsertOpts{Queue: TranslationQueue, MaxAttempts: durableJobMaxAttempts, ScheduledAt: next, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("prepareddeck: River did not return a retry job")
	}
	if err := w.Store.SetPreparedDeckTranslationJobTx(ctx, tx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, updated.DispatchGeneration, inserted.Job.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func standardRetryDelay(config PreparedDeckConfig, attempt int) time.Duration {
	delay, maxDelay := config.StandardRetryBaseDelay, config.StandardRetryMaxDelay
	if delay <= 0 {
		delay = DefaultStandardRetryBaseDelay
	}
	if maxDelay <= 0 {
		maxDelay = DefaultStandardRetryMaxDelay
	}
	for i := 1; i < attempt; i++ {
		if delay >= maxDelay/2 {
			return maxDelay
		}
		delay *= 2
	}
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

func shouldRetryStandardProviderAttempt(retryable bool, providerAttemptCount, maxProviderAttempts int) bool {
	return retryable && providerAttemptCount+1 < maxProviderAttempts
}

func (w *StandardTranslationWorker) fail(ctx context.Context, args StandardTranslationJobArgs, token, class, code string, providerAttempt bool) error {
	return w.failWithLatency(ctx, args, token, class, code, providerAttempt, 0)
}

func (w *StandardTranslationWorker) failWithLatency(ctx context.Context, args StandardTranslationJobArgs, token, class, code string, providerAttempt bool, providerLatency time.Duration) error {
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchProviderTransitions, Phase: "translating", State: "failed", ErrorClass: standardMetricErrorClass(class, code), Provider: "openai", Value: 1})
	_, _, err := w.Store.FinishPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, persistence.PreparedDeckOutcomeTerminalUpdate{State: domain.PreparedDeckOutcomeFailed, ErrorClass: class, ErrorCode: code, ProviderAttempt: providerAttempt, ProviderCall: providerAttempt, ProviderLatency: providerLatency}, w.finalizer)
	return err
}

func standardMetricErrorClass(class, code string) string {
	switch {
	case code == "http_429", code == "rate_limit":
		return "rate_limit"
	case code == "transport_timeout" || code == "http_408":
		return "timeout"
	case strings.HasPrefix(code, "http_5"):
		return "provider_5xx"
	case class == "validation":
		return "validation"
	case class == "configuration":
		return "configuration"
	case class == "cancellation":
		return "cancelled"
	default:
		return "terminal"
	}
}

func (w *StandardTranslationWorker) finalizer(ctx context.Context, tx pgx.Tx, run domain.PreparedDeckRun) error {
	inserted, err := w.Client.InsertTx(ctx, tx, FinalizeJobArgs{OwnerID: run.OwnerID, PreparationID: run.PreparationID, RunID: run.ID, Generation: run.FinalizationDispatchGeneration}, durableInsertOpts())
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil || !isLivePreparationJobState(inserted.Job.State) {
		return errors.New("prepareddeck: River did not return a finalizer job")
	}
	return w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, run.OwnerID, run.PreparationID, run.ID, run.FinalizationDispatchGeneration, inserted.Job.ID)
}

func classifyStandardProviderError(err error, parent context.Context) (string, string, bool) {
	if errors.Is(parent.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "cancellation", "cancelled", false
	}
	if httpErr, ok := errors.AsType[*enrichment.LLMHTTPError](err); ok {
		if httpErr.Temporary() {
			return "provider", fmt.Sprintf("http_%d", httpErr.StatusCode), true
		}
		return "provider", fmt.Sprintf("http_%d", httpErr.StatusCode), false
	}
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
		return "provider", "transport_timeout", true
	}
	// A malformed or identity/language-mismatched provider response is
	// transient: it may be retried up to the caller's bounded attempt limit. It
	// must never be persisted as success.
	if errors.Is(err, enrichment.ErrInvalidTranslationResponse) {
		return "validation", "invalid_response", true
	}
	return "validation", "invalid_response", false
}
