package enrichment

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestBatchJSONLIsDeterministicAndPrivacySafe(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "test-model", BaseURL: "https://api.openai.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	items := []BatchTranslationItem{
		{Ordinal: 4, Request: TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}},
		{Ordinal: 1, Request: TranslationRequest{Language: "fr", CanonicalLemma: "livre", UPOS: "NOUN"}},
	}
	var first, second bytes.Buffer
	firstStats, err := codec.WriteBatchJSONL(&first, "123e4567-e89b-12d3-a456-426614174000", 2, items)
	if err != nil {
		t.Fatal(err)
	}
	secondStats, err := codec.WriteBatchJSONL(&second, "123e4567-e89b-12d3-a456-426614174000", 2, []BatchTranslationItem{items[1], items[0]})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) || firstStats != secondStats {
		t.Fatalf("JSONL is not deterministic:\n%s\n%s", first.String(), second.String())
	}
	if firstStats != int64(first.Len()) {
		t.Fatalf("stats=%d bytes=%d", firstStats, first.Len())
	}
	t.Logf("golden JSONL SHA-256=%x", sha256.Sum256(first.Bytes()))
	if !strings.Contains(first.String(), `"url":"/v1/chat/completions"`) || !strings.Contains(first.String(), `"custom_id":"prepared-deck:123e4567-e89b-12d3-a456-426614174000:1:2"`) {
		t.Fatalf("unexpected JSONL=%s", first.String())
	}
	for _, private := range []string{"owner-123", "document-title", "user@example.com"} {
		if strings.Contains(first.String(), private) {
			t.Fatalf("JSONL leaked %q: %s", private, first.String())
		}
	}
	identity, err := ParseBatchCustomID("prepared-deck:123e4567-e89b-12d3-a456-426614174000:4:2")
	if err != nil || identity.Ordinal != 4 || identity.Generation != 2 {
		t.Fatalf("identity=%+v err=%v", identity, err)
	}
	if _, err := BatchCustomID("not-a-uuid", 1, 1); !errors.Is(err, ErrInvalidBatchCustomID) {
		t.Fatalf("invalid run ID err=%v", err)
	}
}

func TestDecodeBatchResultsCorrelatesUnorderedMixedOutputAndErrors(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model", BaseURL: "https://api.openai.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	items := []BatchTranslationItem{
		{Ordinal: 1, Request: TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}},
		{Ordinal: 2, Request: TranslationRequest{Language: "de", CanonicalLemma: "Baum", UPOS: "NOUN"}},
	}
	output, err := os.Open("testdata/openai_batch_output.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	errorsFile, err := os.Open("testdata/openai_batch_error.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer errorsFile.Close()
	results, err := codec.DecodeBatchResults("123e4567-e89b-12d3-a456-426614174000", 1, items, output, errorsFile)
	if err != nil {
		t.Fatal(err)
	}
	if results[2].Response.Translation != "tree" || results[1].ErrorClass != ProviderErrorRateLimit || results[1].Successful() {
		t.Fatalf("results=%+v", results)
	}
	if results[1].StatusCode != 429 || strings.Contains(results[1].String(), "Rate limit reached") {
		t.Fatalf("result did not preserve bounded HTTP outcome: %s", results[1].String())
	}
}

func TestDecodeBatchResultsRejectsDuplicateUnknownAndMalformedLines(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	item := BatchTranslationItem{Ordinal: 1, Request: TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}}
	id, _ := BatchCustomID("123e4567-e89b-12d3-a456-426614174000", 1, 1)
	valid := `{"custom_id":"` + id + `","error":{"code":"invalid_request_error"}}` + "\n"
	for name, fixture := range map[string]struct {
		content string
		kind    BatchResultErrorKind
	}{
		"duplicate":           {valid + valid, BatchResultDuplicate},
		"unknown":             {`{"custom_id":"prepared-deck:123e4567-e89b-12d3-a456-426614174000:9:1","error":{"code":"x"}}` + "\n", BatchResultUnknown},
		"malformed":           {"not-json\n", BatchResultMalformed},
		"empty error code":    {`{"custom_id":"` + id + `","error":{"code":""}}` + "\n", BatchResultMalformed},
		"invalid status code": {`{"custom_id":"` + id + `","response":{"status_code":0,"body":{}}}` + "\n", BatchResultMalformed},
		"contradictory":       {`{"custom_id":"` + id + `","response":{"status_code":200,"body":{}},"error":{"code":"invalid_request"}}` + "\n", BatchResultContradictory},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := codec.DecodeBatchResults("123e4567-e89b-12d3-a456-426614174000", 1, []BatchTranslationItem{item}, strings.NewReader(fixture.content), nil)
			var resultErr *BatchResultError
			if !errors.As(err, &resultErr) || resultErr.Kind != fixture.kind {
				t.Fatalf("err=%v kind=%q want=%q", err, resultErr.Kind, fixture.kind)
			}
			if err == nil {
				t.Fatal("malformed result accepted")
			}
		})
	}
}

func TestDecodeBatchResultsPartialReportsSortedMissingIDs(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	runID := "123e4567-e89b-12d3-a456-426614174000"
	items := []BatchTranslationItem{
		{Ordinal: 9, Request: TranslationRequest{Language: "de", CanonicalLemma: "neun", UPOS: "NUM"}},
		{Ordinal: 2, Request: TranslationRequest{Language: "de", CanonicalLemma: "zwei", UPOS: "NUM"}},
		{Ordinal: 5, Request: TranslationRequest{Language: "de", CanonicalLemma: "fünf", UPOS: "NUM"}},
	}
	id, _ := BatchCustomID(runID, 5, 1)
	content := `{"custom_id":"` + id + `","error":{"code":"batch_expired"}}` + "\n"
	results, missing, err := codec.DecodeBatchResultsPartial(runID, 1, items, nil, strings.NewReader(content))
	if err != nil || len(results) != 1 || results[5].ErrorCode != "expired" || len(missing) != 2 || missing[0] != 2 || missing[1] != 9 {
		t.Fatalf("results=%+v missing=%v err=%v", results, missing, err)
	}
	_, err = codec.DecodeBatchResults(runID, 1, items, nil, strings.NewReader(content))
	var resultErr *BatchResultError
	if !errors.As(err, &resultErr) || resultErr.Kind != BatchResultMissing {
		t.Fatalf("strict missing error=%v", err)
	}
}

func TestTranslationCodecPreservesReasoningCapabilityBehavior(t *testing.T) {
	known, err := NewTranslationCodec(LLMConfig{Model: "o3-mini", BaseURL: "https://api.openai.com/v1", ReasoningEffort: "medium"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := known.EncodeRequest(TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	if !strings.Contains(string(body), `"reasoning_effort":"medium"`) || strings.Contains(string(body), `"temperature"`) {
		t.Fatalf("reasoning body=%s", body)
	}
	unknown, err := NewTranslationCodec(LLMConfig{Model: "o3-mini", BaseURL: "https://custom.example/v1"})
	if err != nil {
		t.Fatal(err)
	}
	body, _ = unknown.EncodeRequest(TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	if !strings.Contains(string(body), `"temperature":0`) || strings.Contains(string(body), `"reasoning_effort"`) {
		t.Fatalf("custom endpoint body=%s", body)
	}
}
