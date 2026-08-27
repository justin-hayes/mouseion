package enrichment

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

const testBatchRunID = "018f64b6-5f2f-7e12-a7a7-832a50f68b7c"

func TestTranslationCodecBatchJSONLGoldenDeterministicAndPrivate(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-test"})
	if err != nil {
		t.Fatal(err)
	}
	items := []BatchTranslationItem{
		{Ordinal: 7, Request: TranslationRequest{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"}},
		{Ordinal: 2, Request: TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}},
	}
	var first, second bytes.Buffer
	firstBytes, err := codec.WriteBatchJSONL(&first, testBatchRunID, 3, items)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := codec.WriteBatchJSONL(&second, testBatchRunID, 3, []BatchTranslationItem{items[1], items[0]})
	if err != nil {
		t.Fatal(err)
	}
	if firstBytes != int64(first.Len()) || secondBytes != int64(second.Len()) || !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("serialization is not deterministic: first=%d/%d second=%d/%d", firstBytes, first.Len(), secondBytes, second.Len())
	}
	want, err := os.ReadFile("testdata/openai_batch_requests.golden.jsonl")
	if err != nil {
		t.Fatalf("read golden: %v\ngot:\n%s", err, first.Bytes())
	}
	if !bytes.Equal(first.Bytes(), want) {
		t.Fatalf("golden mismatch\ngot:\n%s\nwant:\n%s", first.Bytes(), want)
	}
	serialized := first.String()
	for _, forbidden := range []string{"owner_id", "document", "reading", "metadata", "book title"} {
		if strings.Contains(strings.ToLower(serialized), forbidden) {
			t.Fatalf("Batch request leaked forbidden field %q", forbidden)
		}
	}
	for _, privateValue := range []string{"Haus", "Das Haus ist groß."} {
		for _, line := range strings.Split(strings.TrimSpace(serialized), "\n") {
			customIDStart := strings.Index(line, `"custom_id":"`)
			customIDEnd := strings.Index(line[customIDStart+13:], `"`)
			customID := line[customIDStart+13 : customIDStart+13+customIDEnd]
			if strings.Contains(customID, privateValue) {
				t.Fatalf("custom ID leaked provider input: %q", customID)
			}
		}
	}
}

func TestBatchCustomIDRoundTripAndValidation(t *testing.T) {
	customID, err := BatchCustomID(strings.ToUpper(testBatchRunID), 42, 2)
	if err != nil {
		t.Fatal(err)
	}
	if customID != "prepared-deck:"+testBatchRunID+":42:2" {
		t.Fatalf("custom ID=%q", customID)
	}
	identity, err := ParseBatchCustomID(customID)
	if err != nil || identity != (BatchItemIdentity{RunID: testBatchRunID, Ordinal: 42, Generation: 2}) || identity.String() != customID {
		t.Fatalf("identity=%+v string=%q err=%v", identity, identity.String(), err)
	}
	for _, invalid := range []string{"", "owner@example.com", "prepared-deck:" + testBatchRunID + ":-1:1", "prepared-deck:" + testBatchRunID + ":1:0", "prepared-deck:" + testBatchRunID + ":01:1"} {
		if _, err := ParseBatchCustomID(invalid); err == nil {
			t.Fatalf("ParseBatchCustomID(%q) returned nil error", invalid)
		}
	}
}

func TestTranslationCodecPreservesReasoningCapability(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-5-mini", BaseURL: defaultLLMBaseURL, ReasoningEffort: "high"})
	if err != nil {
		t.Fatal(err)
	}
	body, err := codec.EncodeRequest(TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte(`"reasoning_effort":"high"`)) || bytes.Contains(body, []byte(`"temperature"`)) {
		t.Fatalf("reasoning request=%s", body)
	}
}

func TestTranslationCodecRejectsOversizedResponse(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = codec.DecodeResponse(TranslationRequest{}, bytes.Repeat([]byte("x"), maxTranslationResponseBytes+1))
	if err == nil || !strings.Contains(err.Error(), "exceeds 1 MiB") {
		t.Fatalf("err=%v", err)
	}
}
