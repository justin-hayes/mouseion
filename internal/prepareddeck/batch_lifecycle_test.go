package prepareddeck

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/riverqueue/river"
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
			if terminal != test.terminal {
				t.Fatalf("terminalBatchStatus(%q)=%t want %t", test.status, terminal, test.terminal)
			}
			state := domain.PreparedDeckBatchPolling
			if terminal {
				state = domain.PreparedDeckBatchCompleted
			}
			update := batchReconciliationUpdate(enrichment.Batch{Status: test.status}, state, terminalChunkErrorClass(test.status), terminalChunkErrorCode(test.status), 0)
			if update.ProviderStatus != string(test.status) || update.State != state {
				t.Fatalf("update=%+v", update)
			}
		})
	}
}

func TestUnknownBatchStatusIsNotMappedToPolling(t *testing.T) {
	if knownBatchStatus(enrichment.BatchStatus("future_status")) {
		t.Fatal("unknown provider status accepted")
	}
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
			if err := validatePolledBatch(chunk, batch); err != nil {
				t.Fatalf("normal %s Batch with unavailable counts was rejected: %v", status, err)
			}
		})
	}
	for _, status := range []enrichment.BatchStatus{enrichment.BatchStatusCompleted, enrichment.BatchStatusExpired} {
		t.Run(string(status), func(t *testing.T) {
			batch := base
			batch.Status = status
			if err := validatePolledBatch(chunk, batch); err == nil {
				t.Fatalf("terminal result-bearing %s Batch with missing counts was accepted", status)
			}
		})
	}
	completed := base
	completed.Status = enrichment.BatchStatusCompleted
	completed.RequestCounts = enrichment.BatchRequestCounts{Total: chunk.RequestCount, Completed: chunk.RequestCount}
	if err := validatePolledBatch(chunk, completed); err != nil {
		t.Fatalf("completed Batch with exact counts was rejected: %v", err)
	}
	completed.RequestCounts = enrichment.BatchRequestCounts{Total: chunk.RequestCount + 1, Completed: chunk.RequestCount + 1}
	if err := validatePolledBatch(chunk, completed); err == nil {
		t.Fatal("contradictory completed Batch count was accepted")
	}
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
			if got := retryableBatchFailure(test.class); got != test.retryable {
				t.Fatalf("retryable=%t want %t", got, test.retryable)
			}
			if got := outcomeErrorClass(test.class); got != test.itemClass {
				t.Fatalf("outcome class=%q want %q", got, test.itemClass)
			}
		})
	}
}

func TestRetryableFailedBatchUsesBoundedItemReconciliation(t *testing.T) {
	batch := enrichment.Batch{
		Status: enrichment.BatchStatusFailed,
		Errors: []enrichment.BatchIssue{{Class: enrichment.ProviderErrorRateLimit, Code: "token_limit_exceeded"}},
	}
	if terminalBatchFailsRun(batch) {
		t.Fatal("retryable provider Batch validation failure would fail the whole run")
	}
	if got := terminalBatchErrorCode(batch); got != "token_limit_exceeded" {
		t.Fatalf("durable diagnostic=%q", got)
	}
	batch.Errors[0] = enrichment.BatchIssue{Class: enrichment.ProviderErrorInvalidRequest, Code: "invalid_request"}
	if !terminalBatchFailsRun(batch) {
		t.Fatal("permanent Batch validation failure was made retryable")
	}
}

func TestBatchProviderCountsTreatHTTP200InvalidTranslationAsCompleted(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	runID := "123e4567-e89b-12d3-a456-426614174000"
	item := enrichment.BatchTranslationItem{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist groß."}}
	customID, err := enrichment.BatchCustomID(runID, item.Ordinal, 1)
	if err != nil {
		t.Fatal(err)
	}
	output := fmt.Sprintf(`{"id":"batch_req_1","custom_id":%q,"response":{"status_code":200,"request_id":"req_1","body":{"choices":[{"index":0,"message":{"role":"assistant","content":"{\"translation\":\"house\",\"gloss\":\"building\",\"sentence_translation\":\"\",\"sentence_translation_target\":\"\"}"}}]}},"error":null}`+"\n", customID)
	outcomes, missing, err := codec.DecodeBatchResultsPartial(runID, 1, []enrichment.BatchTranslationItem{item}, strings.NewReader(output), nil)
	if err != nil || len(missing) != 0 || outcomes[item.Ordinal].StatusCode != 200 || outcomes[item.Ordinal].ErrorClass != enrichment.ProviderErrorInvalidResponse {
		t.Fatalf("outcomes=%+v missing=%v err=%v", outcomes, missing, err)
	}
	completed, failed := batchProviderResultCounts(outcomes)
	if completed != 1 || failed != 0 {
		t.Fatalf("provider counts=%d completed, %d failed; want 1 completed, 0 failed", completed, failed)
	}
}

func TestBatchPollingUsesConfiguredBoundedJitter(t *testing.T) {
	worker := &BatchPollWorker{PollInterval: 40 * time.Second, Jitter: func(interval time.Duration) time.Duration { return interval + interval/10 }}
	if got := worker.pollDelay(); got != 44*time.Second {
		t.Fatalf("poll delay=%s", got)
	}
	worker.Jitter = func(time.Duration) time.Duration { return time.Hour }
	if got := worker.pollDelay(); got != 40*time.Second {
		t.Fatalf("out-of-bounds jitter delay=%s", got)
	}
	err := river.JobSnooze(worker.pollDelay())
	var snooze *river.JobSnoozeError
	if !errors.As(err, &snooze) || snooze.Duration != 40*time.Second {
		t.Fatalf("snooze=%v", err)
	}
}

func TestBatchResultFailureClassIsPrivacySafeAndConstrained(t *testing.T) {
	for kind, want := range map[enrichment.BatchResultErrorKind]string{
		enrichment.BatchResultMalformed:     "malformed_result",
		enrichment.BatchResultUnknown:       "unknown_result",
		enrichment.BatchResultDuplicate:     "duplicate_result",
		enrichment.BatchResultContradictory: "malformed_result",
		enrichment.BatchResultMissing:       "missing_result",
	} {
		if got := batchResultFailureClass(kind); got != want {
			t.Fatalf("kind=%q got=%q want=%q", kind, got, want)
		}
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
		{Translation: "house", Gloss: "building", SentenceTranslation: "The old house is surprisingly large.", SentenceTranslationTarget: "house"},
		{Translation: "tree", Gloss: "woody plant", SentenceTranslation: "The old tree has many green leaves today.", SentenceTranslationTarget: "tree"},
	}
	serial := make([]cardexport.ExactEnrichment, len(candidates))
	for i, candidate := range candidates {
		keys[i] = enrichment.CacheKey{Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS, Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence)}
		provenance := enrichment.Provenance{Provider: "openai", ProviderVersion: "v1", External: true}
		serial[i] = cardexport.ExactEnrichment{CacheKey: keys[i], Result: enrichment.Result{Candidate: candidate,
			Translation:               enrichment.Field[string]{Value: responses[i].Translation, Available: true, Provenance: provenance},
			SentenceTranslation:       enrichment.Field[string]{Value: responses[i].SentenceTranslation, Available: true, Provenance: provenance},
			SentenceTranslationTarget: enrichment.Field[string]{Value: responses[i].SentenceTranslationTarget, Available: true, Provenance: provenance},
		}}
	}
	bound, err := manifest.BindCacheKeys(keys)
	if err != nil {
		t.Fatal(err)
	}
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	items := []enrichment.BatchTranslationItem{{Ordinal: 0, Request: enrichment.TranslationRequest{Language: candidates[0].Language, CanonicalLemma: candidates[0].CanonicalLemma, UPOS: candidates[0].UPOS, TargetWord: candidates[0].TargetWord, ExampleSentence: candidates[0].ExampleSentence}}, {Ordinal: 1, Request: enrichment.TranslationRequest{Language: candidates[1].Language, CanonicalLemma: candidates[1].CanonicalLemma, UPOS: candidates[1].UPOS, TargetWord: candidates[1].TargetWord, ExampleSentence: candidates[1].ExampleSentence}}}
	var output strings.Builder
	for _, ordinal := range []int{1, 0} {
		customID, _ := enrichment.BatchCustomID(runID, ordinal, 1)
		fmt.Fprintf(&output, `{"custom_id":%q,"response":{"status_code":200,"body":{"choices":[{"message":{"content":%q}}]}}}`+"\n", customID, fmt.Sprintf(`{"translation":%q,"gloss":%q,"sentence_translation":%q,"sentence_translation_target":%q}`, responses[ordinal].Translation, responses[ordinal].Gloss, responses[ordinal].SentenceTranslation, responses[ordinal].SentenceTranslationTarget))
	}
	decoded, err := codec.DecodeBatchResults(runID, 1, items, strings.NewReader(output.String()), nil)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	batchArtifact, err := renderer.RenderManifest(context.Background(), bound, batchExact)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(serialArtifact.APKG, batchArtifact.APKG) || serialArtifact.TSV != batchArtifact.TSV || serialArtifact.Completeness != batchArtifact.Completeness {
		t.Fatalf("serial and Batch artifacts differ\nserial=%+v\nbatch=%+v", serialArtifact, batchArtifact)
	}
}
