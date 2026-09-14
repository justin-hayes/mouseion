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
	if w == nil || w.Store == nil || w.Provider == nil || w.Client == nil || strings.TrimSpace(args.OwnerID) == "" || strings.TrimSpace(args.PreparationID) == "" || strings.TrimSpace(args.RunID) == "" || args.Ordinal < 0 || args.Generation < 0 {
		return ErrInvalidInput
	}
	run, err := w.Store.GetPreparedDeckRun(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return err
	}
	if run.ExecutionMode != domain.PreparedDeckExecutionStandard || run.State != domain.PreparedDeckRunTranslating {
		return nil
	}
	token := uuid.NewString()
	claimed, err := w.Store.ClaimPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, w.now().Add(standardTranslationLease))
	if err != nil {
		if errors.Is(err, persistence.ErrPreparedDeckClaimLost) || errors.Is(err, persistence.ErrInvalidTransition) {
			return nil
		}
		return err
	}
	_ = claimed

	snapshot, _, err := w.Store.LoadPreparedDeckManifest(ctx, args.OwnerID, args.PreparationID, args.RunID)
	if err != nil {
		return w.fail(ctx, args, token, "orchestration", "manifest", true, false)
	}
	var item cardexport.ManifestItem
	var found bool
	for _, candidate := range snapshot.Items {
		if candidate.Ordinal == args.Ordinal {
			item, found = candidate, true
			break
		}
	}
	if !found || item.CacheKey == nil {
		return w.fail(ctx, args, token, "identity", "manifest_item", true, false)
	}
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricTranslationUnits, Phase: "translating", State: "in_progress", Provider: "openai", Value: 1})
	key := *item.CacheKey
	if entry, hit, cacheErr := w.Store.Get(ctx, key); cacheErr != nil {
		return w.fail(ctx, args, token, "persistence", "cache_lookup", false, false)
	} else if hit && enrichment.HasRequiredTranslationFields(entry, item.Entry.Sentence) {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricCacheHits, Phase: "cache", State: "completed", Provider: "openai", Value: 1})
		_, _, finishErr := w.Store.FinishPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, persistence.PreparedDeckOutcomeTerminalUpdate{State: domain.PreparedDeckOutcomeCompleted, CacheHit: true, CacheLatency: 0}, w.finalizer)
		return finishErr
	} else {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricCacheMisses, Phase: "cache", State: "pending", Provider: "openai", Value: 1})
		_ = entry
	}

	request := enrichment.TranslationRequest{Language: item.Entry.Language, TargetLanguage: snapshotTarget(item), CanonicalLemma: item.Entry.CanonicalLemma, UPOS: item.Entry.UPOS, TargetWord: item.Entry.TargetWord, ExampleSentence: item.Entry.Sentence, CandidateSenses: item.Entry.CandidateSenses}
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
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageInputTokens, Phase: "provider", State: "completed", Provider: "openai", Value: float64(usage.PromptTokens)})
	observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchUsageOutputTokens, Phase: "provider", State: "completed", Provider: "openai", Value: float64(usage.CompletionTokens)})
	if callErr != nil {
		class, code, retryable := classifyStandardProviderError(callErr, ctx)
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricProviderErrors, Phase: "provider", State: "failed", ErrorClass: standardMetricErrorClass(class, code), Provider: "openai", Value: 1})
		if retryable && claimed.ProviderAttemptCount+1 < claimed.MaxProviderAttempts {
			return w.retry(ctx, args, token, claimed.ProviderAttemptCount+1, class, code)
		}
		return w.failWithLatency(ctx, args, token, class, code, true, providerLatency)
	}
	response, callErr = enrichment.NormalizeTranslationResponse(request, response)
	if callErr != nil {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchValidationFailures, Phase: "provider", State: "failed", ErrorClass: "validation", Provider: "openai", Value: 1})
		return w.failWithLatency(ctx, args, token, "validation", "invalid_response", true, providerLatency)
	}
	for _, warning := range response.Warnings {
		log.Printf("prepared deck translation: %s", warning)
	}
	entry := enrichment.CacheEntry{CacheKey: key, Translation: response.Translation, FallbackGloss: response.FallbackGloss, SenseSelection: append([]int{}, response.SenseOrder...), SentenceTranslation: response.SentenceTranslation, SentenceTranslationTarget: response.SentenceTranslationTarget, CachedAt: w.now()}
	stored, err := w.Store.Put(ctx, entry)
	if err != nil {
		return w.failWithLatency(ctx, args, token, "persistence", "cache_write", false, providerLatency)
	}
	_, _, err = w.Store.FinishPreparedDeckTranslationOutcome(ctx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, args.Generation, token, persistence.PreparedDeckOutcomeTerminalUpdate{State: domain.PreparedDeckOutcomeCompleted, ProviderAttempt: true, ProviderCall: true, ProviderLatency: providerLatency}, w.finalizer)
	_ = stored
	if err == nil {
		observeBatchMetric(w.Metrics, BatchMetric{Mode: "standard", Name: MetricBatchProviderTransitions, Phase: "translating", State: "completed", Provider: "openai", Value: 1})
	}
	return err
}

func snapshotTarget(item cardexport.ManifestItem) string {
	if item.CacheKey != nil && item.CacheKey.TargetLanguage != "" {
		return item.CacheKey.TargetLanguage
	}
	return "en"
}

func (w *StandardTranslationWorker) retry(ctx context.Context, args StandardTranslationJobArgs, token string, attempt int, class, code string) error {
	base, max := w.Config.StandardRetryBaseDelay, w.Config.StandardRetryMaxDelay
	if base <= 0 {
		base = DefaultStandardRetryBaseDelay
	}
	if max <= 0 {
		max = DefaultStandardRetryMaxDelay
	}
	delay := standardRetryDelay(PreparedDeckConfig{StandardRetryBaseDelay: base, StandardRetryMaxDelay: max}, attempt)
	jitter := w.Jitter
	if jitter == nil {
		jitter = func(d time.Duration) time.Duration { return time.Duration(float64(d) * (0.8 + rand.Float64()*0.4)) }
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
	defer tx.Rollback(ctx)
	inserted, err := w.Client.InsertTx(ctx, tx, newArgs, &river.InsertOpts{Queue: TranslationQueue, MaxAttempts: durableJobMaxAttempts, ScheduledAt: next, UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates}})
	if err != nil {
		return err
	}
	if inserted == nil || inserted.Job == nil {
		return errors.New("River did not return a retry job")
	}
	if err = w.Store.SetPreparedDeckTranslationJobTx(ctx, tx, args.OwnerID, args.PreparationID, args.RunID, args.Ordinal, updated.DispatchGeneration, inserted.Job.ID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func standardRetryDelay(config PreparedDeckConfig, attempt int) time.Duration {
	delay, max := config.StandardRetryBaseDelay, config.StandardRetryMaxDelay
	if delay <= 0 {
		delay = DefaultStandardRetryBaseDelay
	}
	if max <= 0 {
		max = DefaultStandardRetryMaxDelay
	}
	for i := 1; i < attempt; i++ {
		if delay >= max/2 {
			return max
		}
		delay *= 2
	}
	if delay > max {
		return max
	}
	return delay
}

func (w *StandardTranslationWorker) fail(ctx context.Context, args StandardTranslationJobArgs, token, class, code string, providerAttempt, providerCall bool) error {
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
	if inserted == nil || inserted.Job == nil {
		return fmt.Errorf("River did not return a finalizer job")
	}
	return w.Store.SetPreparedDeckFinalizationJobTx(ctx, tx, run.OwnerID, run.PreparationID, run.ID, run.FinalizationDispatchGeneration, inserted.Job.ID)
}

func classifyStandardProviderError(err error, parent context.Context) (string, string, bool) {
	if errors.Is(parent.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return "cancellation", "cancelled", false
	}
	var httpErr *enrichment.LLMHTTPError
	if errors.As(err, &httpErr) {
		if httpErr.Temporary() {
			return "provider", fmt.Sprintf("http_%d", httpErr.StatusCode), true
		}
		return "provider", fmt.Sprintf("http_%d", httpErr.StatusCode), false
	}
	var netErr net.Error
	if errors.As(err, &netErr) || errors.Is(err, context.DeadlineExceeded) {
		return "provider", "transport_timeout", true
	}
	return "validation", "invalid_response", false
}
