package prepareddeck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type batchListProviderStub struct {
	pages []enrichment.BatchList
	err   error
}

func (*batchListProviderStub) UploadFile(context.Context, string, io.Reader) (enrichment.OpenAIFile, error) {
	return enrichment.OpenAIFile{}, errors.New("not used")
}

func (*batchListProviderStub) CreateBatch(context.Context, enrichment.CreateBatchRequest) (enrichment.Batch, error) {
	return enrichment.Batch{}, errors.New("not used")
}

func (p *batchListProviderStub) ListBatches(context.Context, enrichment.ListBatchesRequest) (enrichment.BatchList, error) {
	if p.err != nil {
		return enrichment.BatchList{}, p.err
	}
	if len(p.pages) == 0 {
		return enrichment.BatchList{}, nil
	}
	page := p.pages[0]
	p.pages = p.pages[1:]
	return page, nil
}

func TestPlanBatchChunksUsesFrozenOrderAndExactIdentity(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	items := []cardexport.WorkItem{
		{Ordinal: 0, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}, CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN"}},
		{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"}, CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "gehen", UPOS: "VERB"}},
		{Ordinal: 2, Request: enrichment.TranslationRequest{Language: "it", CanonicalLemma: "casa", UPOS: "NOUN", TargetWord: "casa", ExampleSentence: "La casa è grande."}, CacheKey: enrichment.CacheKey{Language: "it", TargetLanguage: "en", CanonicalLemma: "casa", UPOS: "NOUN"}},
	}
	plans, err := PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, items, BatchChunkLimits{MaxRequests: 2, MaxPromptTokens: 100000})
	require.NoError(t, err)
	require.Len(t, plans, 2)
	assert.Equal(t, "run", plans[0].SplitReason)
	assert.Equal(t, "request_limit", plans[1].SplitReason)
	assert.Equal(t, []int{0, 1}, plans[0].Ordinals)
	var encoded bytes.Buffer
	_, err = codec.WriteBatchJSONL(&encoded, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, batchTranslationItems(items[:2]))
	require.NoError(t, err)
	sum := sha256.Sum256(encoded.Bytes())
	assert.Equal(t, int64(encoded.Len()), plans[0].InputBytes)
	assert.Equal(t, hex.EncodeToString(sum[:]), plans[0].InputDigest)
}

func TestPlanBatchChunksSplitsBeforeBytesAndTokens(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	items := []cardexport.WorkItem{
		{Ordinal: 0, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "a", UPOS: "NOUN", ExampleSentence: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "a", UPOS: "NOUN"}},
		{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "b", UPOS: "NOUN", ExampleSentence: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "b", UPOS: "NOUN"}},
		{Ordinal: 2, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "c", UPOS: "NOUN", ExampleSentence: "cccccccccccccccccccccccccccccccc"}, CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "c", UPOS: "NOUN"}},
	}
	var first bytes.Buffer
	_, err = codec.WriteBatchJSONL(&first, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, batchTranslationItems(items[:1]))
	require.NoError(t, err)
	plans, err := PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, items, BatchChunkLimits{MaxRequests: 50, MaxBytes: int64(first.Len()) + 1, MaxPromptTokens: 100000})
	require.NoError(t, err)
	require.Len(t, plans, 3)
	assert.Equal(t, "byte_limit", plans[1].SplitReason)
	assert.Equal(t, "byte_limit", plans[2].SplitReason)
	plans, err = PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, items, BatchChunkLimits{MaxRequests: 50, MaxBytes: 100000, MaxPromptTokens: 500})
	require.NoError(t, err)
	require.Len(t, plans, 3)
	assert.Equal(t, "token_limit", plans[1].SplitReason)
	assert.Equal(t, "token_limit", plans[2].SplitReason)
}

func TestPlanBatchChunksRejectsReorderedItems(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	_, err = PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, []cardexport.WorkItem{{Ordinal: 1, CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "eins", UPOS: "NOUN"}}, {Ordinal: 0, CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "null", UPOS: "NOUN"}}}, BatchChunkLimits{})
	assert.Error(t, err, "reordered items were accepted")
}

func TestPlanBatchChunksRejectsWorkWithoutFrozenIdentity(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	_, err = PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, []cardexport.WorkItem{{Ordinal: 0}}, BatchChunkLimits{})
	assert.Error(t, err, "work without a frozen cache identity was accepted")
}

func TestPlanBatchChunksUsesFrozenRequestFieldsAtCodecBoundary(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	work := cardexport.WorkItem{
		Ordinal: 0,
		Request: enrichment.TranslationRequest{
			Language: "de", TargetLanguage: "en", CanonicalLemma: "aufstehen", UPOS: "VERB",
			TargetWord: "steht auf", ExampleSentence: "Er steht heute auf.",
			CandidateSenses: []enrichment.LexicalSense{{Gloss: "to get up"}},
		},
		CacheKey: enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "aufstehen", UPOS: "VERB", Provider: "openai", ProviderVersion: "v1"},
	}
	plans, err := PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, []cardexport.WorkItem{work}, BatchChunkLimits{})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	var encoded bytes.Buffer
	_, err = codec.WriteBatchJSONL(&encoded, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, []enrichment.BatchTranslationItem{{Ordinal: work.Ordinal, Request: work.Request}})
	require.NoError(t, err)
	assert.Equal(t, int64(encoded.Len()), plans[0].InputBytes)
	assert.Equal(t, []int{work.Ordinal}, plans[0].Ordinals)
}

func TestPlanBatchChunksRoundTripsRestoredFrozenWorkThroughGoldenRequests(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	runID := "018f64b6-5f2f-7e12-a7a7-832a50f68b7c"
	snapshot := cardexport.ManifestSnapshot{
		SchemaVersion: cardexport.ManifestSchemaVersion,
		Owner:         "owner-1",
		DeckName:      "Book",
		Filename:      cardexport.DownloadFilename("Book"),
		Items: []cardexport.ManifestItem{
			{Ordinal: 0, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", TargetWord: "Haus", Sentence: "Das Haus ist groß."}, Quality: cardexport.SentenceQuality{Accepted: true}, CacheKey: &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "v1"}},
			{Ordinal: 1, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", TargetWord: "gehen"}, Quality: cardexport.SentenceQuality{Accepted: true}, CacheKey: &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "gehen", UPOS: "VERB", Provider: "openai", ProviderVersion: "v1"}},
		},
	}
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	work := deck.WorkProjection()
	plans, err := PlanBatchChunks(codec, runID, 3, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, work, BatchChunkLimits{})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	chunk := domain.PreparedDeckBatchChunk{InputBytes: plans[0].InputBytes, InputDigest: plans[0].InputDigest, RequestCount: len(work)}
	reader, err := writeChunkJSONL(codec, runID, 3, batchTranslationItems(work), chunk)
	require.NoError(t, err)
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	want, err := os.ReadFile("../enrichment/testdata/openai_batch_requests.golden.jsonl")
	require.NoError(t, err)
	want = []byte(strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(string(want), ":2:3", ":0:3"), ":7:3", ":1:3"), `\"upos\":\"VERB\"}`, `\"upos\":\"VERB\",\"target_word\":\"gehen\"}`))
	assert.Equal(t, want, got)
}

func TestBatchMetadataIsOpaqueAndSubmissionKeepsImmutableChunkMembers(t *testing.T) {
	metadata := batchMetadata("018f64b6-5f2f-7e12-a7a7-832a50f68b7c", "118f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1)
	assert.False(t, metadataMatches(map[string]string{"mouseion_run": metadata["mouseion_run"], "mouseion_chunk": metadata["mouseion_chunk"], "mouseion_generation": "1", "provider_extra": "ignored"}, metadata), "opaque metadata did not round-trip")
	assert.True(t, metadataMatches(metadata, metadata), "opaque metadata did not round-trip")
	for _, value := range metadata {
		for _, forbidden := range []string{"owner", "lemma", "title", "prompt", "sentence"} {
			assert.False(t, bytes.Contains([]byte(value), []byte(forbidden)), "metadata value %q contains forbidden marker %q", value, forbidden)
		}
	}
	snapshot := cardexport.ManifestSnapshot{SchemaVersion: cardexport.ManifestSchemaVersion, Owner: "owner", DeckName: "deck", Filename: cardexport.DownloadFilename("deck"), Items: []cardexport.ManifestItem{
		{Ordinal: 0, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", TargetWord: "Haus", Sentence: "Das Haus ist groß."}, Quality: cardexport.SentenceQuality{Accepted: true}, CacheKey: &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}},
		{Ordinal: 1, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", TargetWord: "gehen"}, Quality: cardexport.SentenceQuality{Accepted: true}, CacheKey: &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "gehen", UPOS: "VERB", Provider: "openai", ProviderVersion: "v1"}},
	}}
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	items, err := submissionBatchItems(deck, []int{0, 1}, map[int]domain.PreparedDeckTranslationOutcome{
		0: {Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted},
		1: {Ordinal: 1, State: domain.PreparedDeckOutcomePending},
	})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 0, items[0].Ordinal)
	assert.Equal(t, 1, items[1].Ordinal)
}

func TestBatchSubmissionRecoveryRequiresOneRecentExactMatch(t *testing.T) {
	metadata := batchMetadata("018f64b6-5f2f-7e12-a7a7-832a50f68b7c", "118f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1)
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	provider := &batchListProviderStub{pages: []enrichment.BatchList{
		{Data: []enrichment.Batch{{ID: "old", InputFileID: "file", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CreatedAt: now.Add(-8 * 24 * time.Hour).Unix(), Metadata: metadata}}, FirstID: "old", LastID: "old", HasMore: true},
		{Data: []enrichment.Batch{{ID: "found", InputFileID: "file", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CreatedAt: now.Unix(), Metadata: metadata}}, FirstID: "found", LastID: "found"},
	}}
	worker := &BatchSubmitWorker{Provider: provider, Now: func() time.Time { return now }}
	found, err := worker.findExistingBatch(context.Background(), metadata, "file")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "found", found.ID)
	provider.pages = []enrichment.BatchList{{Data: []enrichment.Batch{
		{ID: "one", InputFileID: "file", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CreatedAt: now.Unix(), Metadata: metadata},
		{ID: "two", InputFileID: "file", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CreatedAt: now.Unix(), Metadata: metadata},
	}}}
	_, err = worker.findExistingBatch(context.Background(), metadata, "file")
	assert.Error(t, err, "multiple matching Batches were accepted") //nolint:testifylint // Multiple-match and provider-error cases are independent lookup failures.
	provider.err = errors.New("list unavailable")
	_, err = worker.findExistingBatch(context.Background(), metadata, "file")
	assert.Error(t, err, "provider listing failure was not surfaced")
}

func TestValidCreatedBatchAcceptsInitialProviderCounts(t *testing.T) {
	metadata := batchMetadata("018f64b6-5f2f-7e12-a7a7-832a50f68b7c", "118f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1)
	created := enrichment.Batch{
		ID:               "batch_accepted",
		InputFileID:      "file_input",
		Endpoint:         enrichment.OpenAIChatCompletionsEndpoint,
		CompletionWindow: "24h",
		Status:           enrichment.BatchStatusValidating,
		RequestCounts:    enrichment.BatchRequestCounts{Total: 0, Completed: 0, Failed: 0},
		Metadata:         metadata,
	}

	assert.True(t, validCreatedBatch(created, "file_input", metadata), "accepted validating Batch with initially empty request counts was rejected")
}

func TestBoundedProviderCodePreservesOnlyApprovedDiagnostics(t *testing.T) {
	for _, code := range []string{"batch_identity", "contradictory_counts", "contradictory_result", "missing_provider_file", "provider_5xx", "create_response_lost"} {
		assert.Equal(t, code, boundedProviderCode(code), "boundedProviderCode(%q)", code)
	}
	for _, untrusted := range []string{"private provider message", "req-secret-123", "batch_identity/private"} {
		assert.Equal(t, "provider_error", boundedProviderCode(untrusted), "untrusted code %q was retained as %q", untrusted, boundedProviderCode(untrusted))
	}
}
