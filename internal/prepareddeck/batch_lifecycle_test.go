package prepareddeck

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchProviderStatusesMapToPollingOrTerminalReconciliation(t *testing.T) {
	for _, test := range []struct {
		status   enrichment.BatchStatus
		terminal bool
	}{
		{enrichment.BatchStatusValidating, false},
		{enrichment.BatchStatusInProgress, false},
		{enrichment.BatchStatusFinalizing, false},
		{enrichment.BatchStatusCompleted, true},
		{enrichment.BatchStatusFailed, true},
		{enrichment.BatchStatusExpired, true},
		{enrichment.BatchStatusCancelling, false},
		{enrichment.BatchStatusCancelled, true},
	} {
		t.Run(string(test.status), func(t *testing.T) {
			terminal := terminalBatchStatus(test.status)
			assert.Equal(t, test.terminal, terminal, "terminalBatchStatus(%q)", test.status)
			state := domain.PreparedDeckBatchPolling
			if terminal {
				state = domain.PreparedDeckBatchCompleted
			}
			update := batchReconciliationUpdate(enrichment.Batch{Status: test.status}, state, terminalChunkErrorClass(test.status), terminalChunkErrorCode(test.status), 0)
			assert.Equal(t, string(test.status), update.ProviderStatus)
			assert.Equal(t, state, update.State)
		})
	}
}

func TestUnknownBatchStatusIsNotMappedToPolling(t *testing.T) {
	assert.False(t, knownBatchStatus(enrichment.BatchStatus("future_status")), "unknown provider status accepted")
}

func TestPolledBatchAcceptsUnavailableCountsUntilResultReconciliation(t *testing.T) {
	runID := "018f64b6-5f2f-7e12-a7a7-832a50f68b7c"
	chunkID := "118f64b6-5f2f-7e12-a7a7-832a50f68b7c"
	chunk := domain.PreparedDeckBatchChunk{
		ID: chunkID, RunID: runID, Generation: 1, BatchID: "batch-validating", InputFileID: "file-input",
		Endpoint: enrichment.OpenAIChatCompletionsEndpoint, RequestCount: 37,
	}
	base := enrichment.Batch{
		ID: chunk.BatchID, InputFileID: chunk.InputFileID, Endpoint: chunk.Endpoint,
		Metadata: batchMetadata(runID, chunkID, chunk.Generation),
	}
	for _, status := range []enrichment.BatchStatus{
		enrichment.BatchStatusValidating,
		enrichment.BatchStatusInProgress,
		enrichment.BatchStatusFinalizing,
		enrichment.BatchStatusFailed,
		enrichment.BatchStatusCancelling,
		enrichment.BatchStatusCancelled,
	} {
		t.Run(string(status), func(t *testing.T) {
			batch := base
			batch.Status = status
			assert.NoError(t, validatePolledBatch(chunk, batch), "normal %s Batch with unavailable counts was rejected", status)
		})
	}
	for _, status := range []enrichment.BatchStatus{enrichment.BatchStatusCompleted, enrichment.BatchStatusExpired} {
		t.Run(string(status), func(t *testing.T) {
			batch := base
			batch.Status = status
			assert.Error(t, validatePolledBatch(chunk, batch), "terminal result-bearing %s Batch with missing counts was accepted", status)
		})
	}
	completed := base
	completed.Status = enrichment.BatchStatusCompleted
	completed.RequestCounts = enrichment.BatchRequestCounts{Total: chunk.RequestCount, Completed: chunk.RequestCount}
	assert.NoError(t, validatePolledBatch(chunk, completed), "completed Batch with exact counts was rejected")
	completed.RequestCounts = enrichment.BatchRequestCounts{Total: chunk.RequestCount + 1, Completed: chunk.RequestCount + 1}
	assert.Error(t, validatePolledBatch(chunk, completed), "contradictory completed Batch count was accepted")
}

func TestBatchFailureClassesChooseBoundedRetryAndTerminalOutcomes(t *testing.T) {
	for _, test := range []struct {
		class     enrichment.ProviderErrorClass
		retryable bool
		itemClass string
	}{
		{enrichment.ProviderErrorRateLimit, true, "rate_limit"},
		{enrichment.ProviderErrorTimeout, true, "timeout"},
		{enrichment.ProviderErrorUnavailable, true, "provider"},
		{enrichment.ProviderErrorTransport, true, "provider"},
		{enrichment.ProviderErrorExpired, true, "expired"},
		{enrichment.ProviderErrorCancelled, true, "cancellation"},
		{enrichment.ProviderErrorInvalidRequest, false, "provider"},
		{enrichment.ProviderErrorInvalidResponse, false, "validation"},
		{enrichment.ProviderErrorRequestFailed, false, "provider"},
	} {
		t.Run(string(test.class), func(t *testing.T) {
			assert.Equal(t, test.retryable, retryableBatchFailure(test.class), "retryable=%t want %t", retryableBatchFailure(test.class), test.retryable)
			assert.Equal(t, test.itemClass, outcomeErrorClass(test.class), "outcome class=%q want %q", outcomeErrorClass(test.class), test.itemClass)
		})
	}
}

func TestRetryableFailedBatchUsesBoundedItemReconciliation(t *testing.T) {
	batch := enrichment.Batch{
		Status: enrichment.BatchStatusFailed,
		Errors: []enrichment.BatchIssue{{Class: enrichment.ProviderErrorRateLimit, Code: "token_limit_exceeded"}},
	}
	assert.False(t, terminalBatchFailsRun(batch), "retryable provider Batch validation failure would fail the whole run")
	assert.Equal(t, "token_limit_exceeded", terminalBatchErrorCode(batch), "durable diagnostic")
	batch.Errors[0] = enrichment.BatchIssue{Class: enrichment.ProviderErrorInvalidRequest, Code: "invalid_request"}
	assert.True(t, terminalBatchFailsRun(batch), "permanent Batch validation failure was made retryable")
}

func TestBatchProviderCountsTreatHTTP200InvalidTranslationAsCompleted(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "model"})
	require.NoError(t, err)
	runID := "123e4567-e89b-12d3-a456-426614174000"
	item := enrichment.BatchTranslationItem{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist groß."}}
	customID, err := enrichment.BatchCustomID(runID, item.Ordinal, 1)
	require.NoError(t, err)
	output := fmt.Sprintf(`{"id":"batch_req_1","custom_id":%q,"response":{"status_code":200,"request_id":"req_1","body":{"choices":[{"index":0,"message":{"role":"assistant","content":%q}}]}},"error":null}`+"\n", customID, fmt.Sprintf(`{"item_id":%q,"source_language":"de","target_language":"en","translation":"house","gloss":"building","sentence_translation":"","sentence_translation_target":""}`, customID))
	outcomes, missing, err := codec.DecodeBatchResultsPartial(runID, 1, []enrichment.BatchTranslationItem{item}, strings.NewReader(output), nil)
	require.NoError(t, err)
	assert.Len(t, missing, 0)
	assert.Equal(t, 200, outcomes[item.Ordinal].StatusCode)
	assert.Equal(t, enrichment.ProviderErrorInvalidResponse, outcomes[item.Ordinal].ErrorClass)
	completed, failed := batchProviderResultCounts(outcomes)
	assert.Equal(t, 1, completed)
	assert.Zero(t, failed)
}

func TestBatchPollingUsesConfiguredBoundedJitter(t *testing.T) {
	worker := &BatchPollWorker{PollInterval: 40 * time.Second, Jitter: func(interval time.Duration) time.Duration { return interval + interval/10 }}
	assert.Equal(t, 44*time.Second, worker.pollDelay(), "poll delay")
	worker.Jitter = func(time.Duration) time.Duration { return time.Hour }
	assert.Equal(t, 40*time.Second, worker.pollDelay(), "out-of-bounds jitter delay")
	err := river.JobSnooze(worker.pollDelay())
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, err, &snooze)
	assert.Equal(t, 40*time.Second, snooze.Duration)
}

func TestBatchResultFailureClassIsPrivacySafeAndConstrained(t *testing.T) {
	for kind, want := range map[enrichment.BatchResultErrorKind]string{
		enrichment.BatchResultMalformed:     "malformed_result",
		enrichment.BatchResultUnknown:       "unknown_result",
		enrichment.BatchResultDuplicate:     "duplicate_result",
		enrichment.BatchResultContradictory: "malformed_result",
		enrichment.BatchResultMissing:       "missing_result",
	} {
		assert.Equal(t, want, batchResultFailureClass(kind), "kind=%q got=%q want=%q", kind, batchResultFailureClass(kind), want)
	}
}

func TestFrozenSerialAndUnorderedBatchResultsRenderIdenticalArtifacts(t *testing.T) {
	runID := "123e4567-e89b-12d3-a456-426614174000"
	manifest := cardexport.NewManifest("owner", "Frozen Book", []cardexport.Entry{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: "Frozen Book", FirstEncounter: 1},
		{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", SourceDocument: "Frozen Book", FirstEncounter: 2},
	})
	candidates := manifest.EnrichmentCandidates()
	keys := make([]enrichment.CacheKey, len(candidates))
	responses := []enrichment.TranslationResponse{
		{Translation: "house", FallbackGloss: "building", SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house"},
		{Translation: "tree", FallbackGloss: "woody plant", SentenceTranslation: "The old tree has many green leaves today.", SentenceTranslationTarget: "tree"},
	}
	serial := make([]cardexport.ExactEnrichment, len(candidates))
	for i, candidate := range candidates {
		keys[i] = enrichment.CacheKey{Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
		provenance := enrichment.Provenance{Provider: "openai", ProviderVersion: "v1", External: true}
		serial[i] = cardexport.ExactEnrichment{CacheKey: keys[i], Result: enrichment.Result{Candidate: candidate,
			Translation:               enrichment.Field[string]{Value: responses[i].Translation, Available: true, Provenance: provenance},
			SentenceTranslation:       enrichment.Field[string]{Value: responses[i].SentenceTranslation, Available: true, Provenance: provenance},
			SentenceTranslationTarget: enrichment.Field[string]{Value: responses[i].SentenceTranslationTarget, Available: true, Provenance: provenance},
		}}
	}
	bound, err := manifest.BindCacheKeys(keys)
	require.NoError(t, err)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	items := []enrichment.BatchTranslationItem{{Ordinal: 0, Request: enrichment.TranslationRequest{Language: candidates[0].Language, CanonicalLemma: candidates[0].CanonicalLemma, UPOS: candidates[0].UPOS, TargetWord: candidates[0].TargetWord, ExampleSentence: candidates[0].ExampleSentence}}, {Ordinal: 1, Request: enrichment.TranslationRequest{Language: candidates[1].Language, CanonicalLemma: candidates[1].CanonicalLemma, UPOS: candidates[1].UPOS, TargetWord: candidates[1].TargetWord, ExampleSentence: candidates[1].ExampleSentence}}}
	var output strings.Builder
	for _, ordinal := range []int{1, 0} {
		customID, _ := enrichment.BatchCustomID(runID, ordinal, 1)
		fmt.Fprintf(&output, `{"custom_id":%q,"response":{"status_code":200,"body":{"choices":[{"message":{"content":%q}}]}}}`+"\n", customID, fmt.Sprintf(`{"item_id":%q,"source_language":"de","target_language":"en","translation":%q,"fallback_gloss":%q,"sentence_translation":%q,"sentence_translation_target":%q}`, customID, responses[ordinal].Translation, responses[ordinal].FallbackGloss, responses[ordinal].SentenceTranslation, responses[ordinal].SentenceTranslationTarget))
	}
	decoded, err := codec.DecodeBatchResults(runID, 1, items, strings.NewReader(output.String()), nil)
	require.NoError(t, err)
	batchExact := make([]cardexport.ExactEnrichment, len(items))
	for i, item := range items {
		response := decoded[item.Ordinal].Response
		provenance := enrichment.Provenance{Provider: "openai", ProviderVersion: "v1", External: true}
		batchExact[i] = cardexport.ExactEnrichment{CacheKey: keys[i], Result: enrichment.Result{Candidate: candidates[i],
			Translation:               enrichment.Field[string]{Value: response.Translation, Available: true, Provenance: provenance},
			SentenceTranslation:       enrichment.Field[string]{Value: response.SentenceTranslation, Available: true, Provenance: provenance},
			SentenceTranslationTarget: enrichment.Field[string]{Value: response.SentenceTranslationTarget, Available: true, Provenance: provenance},
		}}
	}
	renderer := &cardexport.Service{}
	serialArtifact, err := renderer.RenderManifest(context.Background(), bound, serial)
	require.NoError(t, err)
	batchArtifact, err := renderer.RenderManifest(context.Background(), bound, batchExact)
	require.NoError(t, err)
	assert.Equal(t, serialArtifact.APKG, batchArtifact.APKG)
	assert.Equal(t, serialArtifact.TSV, batchArtifact.TSV)
	assert.Equal(t, serialArtifact.Completeness, batchArtifact.Completeness)
}
