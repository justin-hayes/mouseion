package prepareddeck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
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
	if err != nil {
		t.Fatal(err)
	}
	items := []enrichment.BatchTranslationItem{
		{Ordinal: 0, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}},
		{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"}},
		{Ordinal: 2, Request: enrichment.TranslationRequest{Language: "it", CanonicalLemma: "casa", UPOS: "NOUN", TargetWord: "casa", ExampleSentence: "La casa è grande."}},
	}
	plans, err := PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, items, BatchChunkLimits{MaxRequests: 2, MaxPromptTokens: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 || plans[0].SplitReason != "run" || plans[1].SplitReason != "request_limit" {
		t.Fatalf("plans=%+v", plans)
	}
	if got, want := plans[0].Ordinals, []int{0, 1}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("first ordinals=%v", got)
	}
	var encoded bytes.Buffer
	if _, err = codec.WriteBatchJSONL(&encoded, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, items[:2]); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded.Bytes())
	if plans[0].InputBytes != int64(encoded.Len()) || plans[0].InputDigest != hex.EncodeToString(sum[:]) {
		t.Fatalf("identity bytes=%d/%d digest=%q/%x", plans[0].InputBytes, encoded.Len(), plans[0].InputDigest, sum)
	}
}

func TestPlanBatchChunksSplitsBeforeBytesAndTokens(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	items := []enrichment.BatchTranslationItem{
		{Ordinal: 0, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "a", UPOS: "NOUN", ExampleSentence: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
		{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "b", UPOS: "NOUN", ExampleSentence: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		{Ordinal: 2, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "c", UPOS: "NOUN", ExampleSentence: "cccccccccccccccccccccccccccccccc"}},
	}
	var first bytes.Buffer
	if _, err = codec.WriteBatchJSONL(&first, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, items[:1]); err != nil {
		t.Fatal(err)
	}
	plans, err := PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, items, BatchChunkLimits{MaxRequests: 50, MaxBytes: int64(first.Len()) + 1, MaxPromptTokens: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 || plans[1].SplitReason != "byte_limit" || plans[2].SplitReason != "byte_limit" {
		t.Fatalf("byte plans=%+v", plans)
	}
	plans, err = PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, items, BatchChunkLimits{MaxRequests: 50, MaxBytes: 100000, MaxPromptTokens: 300})
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 3 || plans[1].SplitReason != "token_limit" || plans[2].SplitReason != "token_limit" {
		t.Fatalf("token plans=%+v", plans)
	}
}

func TestPlanBatchChunksRejectsReorderedItems(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = PlanBatchChunks(codec, "018f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1, "gpt-test", enrichment.OpenAIChatCompletionsEndpoint, []enrichment.BatchTranslationItem{{Ordinal: 1}, {Ordinal: 0}}, BatchChunkLimits{})
	if err == nil {
		t.Fatal("reordered items were accepted")
	}
}

func TestBatchMetadataIsOpaqueAndActiveItemsExcludeTerminalOutcomes(t *testing.T) {
	metadata := batchMetadata("018f64b6-5f2f-7e12-a7a7-832a50f68b7c", "118f64b6-5f2f-7e12-a7a7-832a50f68b7c", 1)
	if metadataMatches(map[string]string{"mouseion_run": metadata["mouseion_run"], "mouseion_chunk": metadata["mouseion_chunk"], "mouseion_generation": "1", "provider_extra": "ignored"}, metadata) || !metadataMatches(metadata, metadata) {
		t.Fatal("opaque metadata did not round-trip")
	}
	for _, value := range metadata {
		for _, forbidden := range []string{"owner", "lemma", "title", "prompt", "sentence"} {
			if bytes.Contains([]byte(value), []byte(forbidden)) {
				t.Fatalf("metadata value %q contains forbidden marker %q", value, forbidden)
			}
		}
	}
	snapshot := cardexport.ManifestSnapshot{SchemaVersion: cardexport.ManifestSchemaVersion, Owner: "owner", DeckName: "deck", Filename: cardexport.DownloadFilename("deck"), Items: []cardexport.ManifestItem{
		{Ordinal: 0, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", TargetWord: "Haus", Sentence: "Das Haus ist groß."}, Quality: cardexport.SentenceQuality{Accepted: true}, CacheKey: &enrichment.CacheKey{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "openai", ProviderVersion: "v1", SentenceHash: enrichment.SentenceHash("Das Haus ist groß.")}},
		{Ordinal: 1, Disposition: cardexport.ManifestAccepted, Entry: cardexport.Entry{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", TargetWord: "gehen"}, Quality: cardexport.SentenceQuality{Accepted: true}, CacheKey: &enrichment.CacheKey{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB", Provider: "openai", ProviderVersion: "v1"}},
	}}
	items, err := activeBatchItems(snapshot, []int{0, 1}, map[int]domain.PreparedDeckTranslationOutcome{
		0: {Ordinal: 0, State: domain.PreparedDeckOutcomeCompleted},
		1: {Ordinal: 1, State: domain.PreparedDeckOutcomePending},
	})
	if err != nil || len(items) != 1 || items[0].Ordinal != 1 {
		t.Fatalf("active items=%+v err=%v", items, err)
	}
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
	if err != nil || found == nil || found.ID != "found" {
		t.Fatalf("found=%+v err=%v", found, err)
	}
	provider.pages = []enrichment.BatchList{{Data: []enrichment.Batch{
		{ID: "one", InputFileID: "file", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CreatedAt: now.Unix(), Metadata: metadata},
		{ID: "two", InputFileID: "file", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CreatedAt: now.Unix(), Metadata: metadata},
	}}}
	if _, err = worker.findExistingBatch(context.Background(), metadata, "file"); err == nil {
		t.Fatal("multiple matching Batches were accepted")
	}
	provider.err = errors.New("list unavailable")
	if _, err = worker.findExistingBatch(context.Background(), metadata, "file"); err == nil {
		t.Fatal("provider listing failure was not surfaced")
	}
}
