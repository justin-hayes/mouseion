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
	assert.NoError(t, validatePolledBatch(chunk, completed), "completed Batch with exact counts was rejected") //nolint:testifylint // This is an independent validation case after the table cases.
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

func TestTerminalBatchResultsPreserveProviderCountsAndExpiredItems(t *testing.T) {
	chunk := domain.PreparedDeckBatchChunk{RequestCount: 2}
	decoded := map[int]enrichment.BatchTranslationOutcome{
		0: {StatusCode: 200, ErrorClass: enrichment.ProviderErrorNone},
		1: {StatusCode: 500, ErrorClass: enrichment.ProviderErrorExpired},
	}
	counts, code := validateTerminalBatchResults(decoded, nil, enrichment.Batch{
		Status:        enrichment.BatchStatusCompleted,
		RequestCounts: enrichment.BatchRequestCounts{Completed: 1, Failed: 1},
	}, chunk)
	assert.Equal(t, "", code)
	assert.Equal(t, 1, counts.successes)
	assert.Equal(t, 1, counts.expiredFailures)

	counts, code = validateTerminalBatchResults(map[int]enrichment.BatchTranslationOutcome{
		0: {StatusCode: 200, ErrorClass: enrichment.ProviderErrorNone},
	}, []int{1}, enrichment.Batch{
		Status:        enrichment.BatchStatusExpired,
		RequestCounts: enrichment.BatchRequestCounts{Completed: 1},
	}, chunk)
	assert.Equal(t, "", code)
	assert.Equal(t, 1, counts.successes)
}

func TestFailedBatchItemRetainsProviderErrorAndExhaustsRetryGeneration(t *testing.T) {
	outcome := enrichment.BatchTranslationOutcome{ErrorClass: enrichment.ProviderErrorRateLimit, ErrorCode: "token_limit_exceeded"}
	chunk := domain.PreparedDeckBatchChunk{Generation: 1}
	run := domain.PreparedDeckRun{MaxBatchGenerations: 2}
	update, retry := failedBatchItem(4, outcome, true, enrichment.Batch{Status: enrichment.BatchStatusCompleted}, chunk, run)
	assert.True(t, retry)
	assert.Equal(t, domain.PreparedDeckOutcomePending, update.State)
	assert.Equal(t, "rate_limit", update.ErrorClass)
	assert.Equal(t, "token_limit_exceeded", update.ErrorCode)

	chunk.Generation = 2
	update, retry = failedBatchItem(4, outcome, true, enrichment.Batch{Status: enrichment.BatchStatusCompleted}, chunk, run)
	assert.False(t, retry)
	assert.Equal(t, domain.PreparedDeckOutcomeFailed, update.State)
	assert.Equal(t, "retry_exhausted", update.ErrorClass)
	assert.Equal(t, "token_limit_exceeded", update.ErrorCode)
}

func TestCompletedBatchItemRecordsUnresolvedMeaningWithoutCacheEntry(t *testing.T) {
	item := enrichment.BatchTranslationItem{Ordinal: 3}
	work := cardexport.WorkItem{Ordinal: 3, CacheKey: enrichment.CacheKey{CanonicalLemma: "bank"}}
	outcome := enrichment.BatchTranslationOutcome{Response: enrichment.TranslationResponse{
		Translation: "bank", SentenceTranslation: "She sat on the bank.",
		UnresolvedReason: "The sentence does not distinguish the meanings.",
	}}

	update := completedBatchItem(&BatchPollWorker{}, item, work, outcome)

	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, update.State)
	assert.Equal(t, "The sentence does not distinguish the meanings.", update.OmissionReason)
	assert.Nil(t, update.CacheEntry, "an unresolved result must not become a dictionary-only cache entry")
}

func TestCompletedBatchItemPersistsContextualGlossAndFrozenEvidenceSelection(t *testing.T) {
	item := enrichment.BatchTranslationItem{Ordinal: 2, Request: enrichment.TranslationRequest{CandidateSenses: []enrichment.LexicalSense{
		{EvidenceID: "wikt:bank-financial", Gloss: "financial institution"},
		{EvidenceID: "wikt:bank-river", Gloss: "river edge"},
	}}}
	work := cardexport.WorkItem{Ordinal: 2, CacheKey: enrichment.CacheKey{CanonicalLemma: "bank"}}
	outcome := enrichment.BatchTranslationOutcome{Response: enrichment.TranslationResponse{
		Translation: "bank", Gloss: "river edge", EvidenceIDs: []string{"wikt:bank-river"},
		SentenceTranslation: "She sat on the river bank.", SentenceTranslationTarget: "bank",
	}}

	update := completedBatchItem(&BatchPollWorker{}, item, work, outcome)

	require.NotNil(t, update.CacheEntry)
	assert.Equal(t, "river edge", update.CacheEntry.FallbackGloss)
	assert.Equal(t, []int{1}, update.CacheEntry.SenseSelection)
	assert.Equal(t, "She sat on the river bank.", update.CacheEntry.SentenceTranslation)
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
	projections := []cardexport.CandidateProjection{
		{OwnerID: "owner", DeckName: "Frozen Book", Provider: "openai", ProviderVersion: "v1", TargetLanguage: "en", Candidate: domain.SelectionCandidate{OwnerID: "owner", CorpusID: "corpus-1", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", FirstEncounter: 1}, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", SourceDocument: "Frozen Book", FirstEncounter: 1}},
		{OwnerID: "owner", DeckName: "Frozen Book", Provider: "openai", ProviderVersion: "v1", TargetLanguage: "en", Candidate: domain.SelectionCandidate{OwnerID: "owner", CorpusID: "corpus-1", Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", FirstEncounter: 2}, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "baum", UPOS: "NOUN", Sentence: "Der alte Baum trägt heute viele grüne Blätter.", TargetWord: "Baum", SourceDocument: "Frozen Book", FirstEncounter: 2}},
	}
	responses := []enrichment.TranslationResponse{
		{Translation: "house", FallbackGloss: "building", SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house"},
		{Translation: "tree", FallbackGloss: "woody plant", SentenceTranslation: "The old tree has many green leaves today.", SentenceTranslationTarget: "tree"},
	}
	deck, _, err := cardexport.NewPresentation(nil).Freeze(context.Background(), "owner", "Frozen Book", projections)
	require.NoError(t, err)
	deck, err = cardexport.NewPresentation(nil).Restore(deck.StorageProjection())
	require.NoError(t, err)
	items, workByOrdinal, err := batchResultItems(deck, []int{0, 1})
	require.NoError(t, err)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	var output strings.Builder
	for _, ordinal := range []int{1, 0} {
		customID, err := enrichment.BatchCustomID(runID, ordinal, 1)
		require.NoError(t, err)
		fmt.Fprintf(&output, `{"custom_id":%q,"response":{"status_code":200,"body":{"choices":[{"message":{"content":%q}}]}}}`+"\n", customID, fmt.Sprintf(`{"item_id":%q,"source_language":"de","target_language":"en","translation":%q,"fallback_gloss":%q,"sentence_translation":%q,"sentence_translation_target":%q}`, customID, responses[ordinal].Translation, responses[ordinal].FallbackGloss, responses[ordinal].SentenceTranslation, responses[ordinal].SentenceTranslationTarget))
	}
	decoded, err := codec.DecodeBatchResults(runID, 1, items, strings.NewReader(output.String()), nil)
	require.NoError(t, err)
	makeStoredResults := func(ordinals []int, result func(int) enrichment.TranslationResponse) []cardexport.StoredResult {
		stored := make([]cardexport.StoredResult, 0, len(ordinals))
		for _, ordinal := range ordinals {
			response := result(ordinal)
			stored = append(stored, cardexport.StoredResult{CacheKey: workByOrdinal[ordinal].CacheKey, Record: enrichment.CacheEntry{CacheKey: workByOrdinal[ordinal].CacheKey, Translation: response.Translation, FallbackGloss: response.FallbackGloss, SentenceTranslation: response.SentenceTranslation, SentenceTranslationTarget: response.SentenceTranslationTarget}})
		}
		return stored
	}
	facts := cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "openai", ProviderVersion: "v1"}
	serialArtifact, _, err := cardexport.NewPresentation(nil).Finalize(context.Background(), deck, makeStoredResults([]int{0, 1}, func(ordinal int) enrichment.TranslationResponse { return responses[ordinal] }), facts)
	require.NoError(t, err)
	batchArtifact, _, err := cardexport.NewPresentation(nil).Finalize(context.Background(), deck, makeStoredResults([]int{1, 0}, func(ordinal int) enrichment.TranslationResponse { return decoded[ordinal].Response }), facts)
	require.NoError(t, err)
	assert.Equal(t, serialArtifact.APKG, batchArtifact.APKG)
	assert.Equal(t, serialArtifact.TSV, batchArtifact.TSV)
	assert.Equal(t, serialArtifact.Completeness, batchArtifact.Completeness)
}
