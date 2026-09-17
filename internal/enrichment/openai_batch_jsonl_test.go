package enrichment_test

import (
	"bytes"
	"crypto/sha256"
	"os"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchJSONLIsDeterministicAndPrivacySafe(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "test-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	items := []enrichment.BatchTranslationItem{
		{Ordinal: 4, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}},
		{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "fr", CanonicalLemma: "livre", UPOS: "NOUN"}},
	}
	var first, second bytes.Buffer
	firstStats, err := codec.WriteBatchJSONL(&first, "123e4567-e89b-12d3-a456-426614174000", 2, items)
	require.NoError(t, err)
	secondStats, err := codec.WriteBatchJSONL(&second, "123e4567-e89b-12d3-a456-426614174000", 2, []enrichment.BatchTranslationItem{items[1], items[0]})
	require.NoError(t, err)
	assert.Equal(t, first.Bytes(), second.Bytes(), "JSONL is not deterministic")
	assert.Equal(t, firstStats, secondStats, "JSONL is not deterministic")
	assert.Equal(t, int64(first.Len()), firstStats)
	t.Logf("golden JSONL SHA-256=%x", sha256.Sum256(first.Bytes()))
	assert.Contains(t, first.String(), `"url":"/v1/chat/completions"`)
	assert.Contains(t, first.String(), `"custom_id":"prepared-deck:123e4567-e89b-12d3-a456-426614174000:1:2"`)
	for _, private := range []string{"owner-123", "document-title", "user@example.com"} {
		assert.NotContains(t, first.String(), private, "JSONL leaked %q: %s", private, first.String())
	}
	identity, err := enrichment.ParseBatchCustomID("prepared-deck:123e4567-e89b-12d3-a456-426614174000:4:2")
	require.NoError(t, err)
	assert.Equal(t, 4, identity.Ordinal)
	assert.Equal(t, 2, identity.Generation)
	_, err = enrichment.BatchCustomID("not-a-uuid", 1, 1)
	assert.ErrorIs(t, err, enrichment.ErrInvalidBatchCustomID, "invalid run ID")
}

func TestDecodeBatchResultsCorrelatesUnorderedMixedOutputAndErrors(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	items := []enrichment.BatchTranslationItem{
		{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}},
		{Ordinal: 2, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Baum", UPOS: "NOUN"}},
	}
	output, err := os.Open("testdata/openai_batch_output.jsonl")
	require.NoError(t, err)
	testutil.Cleanup(t, "batch output file", output.Close)
	errorsFile, err := os.Open("testdata/openai_batch_error.jsonl")
	require.NoError(t, err)
	testutil.Cleanup(t, "batch errors file", errorsFile.Close)
	results, err := codec.DecodeBatchResults("123e4567-e89b-12d3-a456-426614174000", 1, items, output, errorsFile)
	require.NoError(t, err)
	assert.Equal(t, "tree", results[2].Response.Translation)
	assert.Equal(t, enrichment.ProviderErrorRateLimit, results[1].ErrorClass)
	assert.False(t, results[1].Successful())
	assert.Equal(t, 429, results[1].StatusCode)
	assert.NotContains(t, results[1].String(), "Rate limit reached", "result did not preserve bounded HTTP outcome: %s", results[1].String())
}

func TestDecodeBatchResultsRejectsDuplicateUnknownAndMalformedLines(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "model"})
	require.NoError(t, err)
	item := enrichment.BatchTranslationItem{Ordinal: 1, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}}
	id, _ := enrichment.BatchCustomID("123e4567-e89b-12d3-a456-426614174000", 1, 1)
	valid := `{"custom_id":"` + id + `","error":{"code":"invalid_request_error"}}` + "\n"
	for name, fixture := range map[string]struct {
		content string
		kind    enrichment.BatchResultErrorKind
	}{
		"duplicate":           {valid + valid, enrichment.BatchResultDuplicate},
		"unknown":             {`{"custom_id":"prepared-deck:123e4567-e89b-12d3-a456-426614174000:9:1","error":{"code":"x"}}` + "\n", enrichment.BatchResultUnknown},
		"malformed":           {"not-json\n", enrichment.BatchResultMalformed},
		"empty error code":    {`{"custom_id":"` + id + `","error":{"code":""}}` + "\n", enrichment.BatchResultMalformed},
		"invalid status code": {`{"custom_id":"` + id + `","response":{"status_code":0,"body":{}}}` + "\n", enrichment.BatchResultMalformed},
		"contradictory":       {`{"custom_id":"` + id + `","response":{"status_code":200,"body":{}},"error":{"code":"invalid_request"}}` + "\n", enrichment.BatchResultContradictory},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := codec.DecodeBatchResults("123e4567-e89b-12d3-a456-426614174000", 1, []enrichment.BatchTranslationItem{item}, strings.NewReader(fixture.content), nil)
			var resultErr *enrichment.BatchResultError
			require.ErrorAs(t, err, &resultErr)
			assert.Equal(t, fixture.kind, resultErr.Kind, "err=%v", err)
		})
	}
}

func TestDecodeBatchResultsPartialReportsSortedMissingIDs(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "model"})
	require.NoError(t, err)
	runID := "123e4567-e89b-12d3-a456-426614174000"
	items := []enrichment.BatchTranslationItem{
		{Ordinal: 9, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "neun", UPOS: "NUM"}},
		{Ordinal: 2, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "zwei", UPOS: "NUM"}},
		{Ordinal: 5, Request: enrichment.TranslationRequest{Language: "de", CanonicalLemma: "fünf", UPOS: "NUM"}},
	}
	id, _ := enrichment.BatchCustomID(runID, 5, 1)
	content := `{"custom_id":"` + id + `","error":{"code":"batch_expired"}}` + "\n"
	results, missing, err := codec.DecodeBatchResultsPartial(runID, 1, items, nil, strings.NewReader(content))
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "expired", results[5].ErrorCode)
	assert.Equal(t, []int{2, 9}, missing)
	_, err = codec.DecodeBatchResults(runID, 1, items, nil, strings.NewReader(content))
	var resultErr *enrichment.BatchResultError
	require.ErrorAs(t, err, &resultErr)
	assert.Equal(t, enrichment.BatchResultMissing, resultErr.Kind)
}

func TestTranslationCodecPreservesReasoningCapabilityBehavior(t *testing.T) {
	known, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "o3-mini", BaseURL: "https://api.openai.com/v1", ReasoningEffort: "medium"})
	require.NoError(t, err)
	body, _ := known.EncodeRequest(enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	assert.Contains(t, string(body), `"reasoning_effort":"medium"`, "reasoning body=%s", body)
	assert.NotContains(t, string(body), `"temperature"`, "reasoning body=%s", body)
	unknown, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "o3-mini", BaseURL: "https://custom.example/v1"})
	require.NoError(t, err)
	body, _ = unknown.EncodeRequest(enrichment.TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	assert.Contains(t, string(body), `"temperature":0`, "custom endpoint body=%s", body)
	assert.NotContains(t, string(body), `"reasoning_effort"`, "custom endpoint body=%s", body)
}
