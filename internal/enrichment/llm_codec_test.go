package enrichment

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testBatchRunID = "018f64b6-5f2f-7e12-a7a7-832a50f68b7c"

func TestTranslationCodecBatchJSONLGoldenDeterministicAndPrivate(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	items := []BatchTranslationItem{
		{Ordinal: 7, Request: TranslationRequest{Language: "de", CanonicalLemma: "gehen", UPOS: "VERB"}},
		{Ordinal: 2, Request: TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", TargetWord: "Haus", ExampleSentence: "Das Haus ist groß."}},
	}
	var first, second bytes.Buffer
	firstBytes, err := codec.WriteBatchJSONL(&first, testBatchRunID, 3, items)
	require.NoError(t, err)
	secondBytes, err := codec.WriteBatchJSONL(&second, testBatchRunID, 3, []BatchTranslationItem{items[1], items[0]})
	require.NoError(t, err)
	assert.Equal(t, int64(first.Len()), firstBytes)
	assert.Equal(t, int64(second.Len()), secondBytes)
	assert.Equal(t, first.Bytes(), second.Bytes(), "serialization is not deterministic: first=%d/%d second=%d/%d", firstBytes, first.Len(), secondBytes, second.Len())
	want, err := os.ReadFile("testdata/openai_batch_requests.golden.jsonl")
	require.NoError(t, err, "read golden")
	assert.Equal(t, want, first.Bytes(), "golden mismatch")
	serialized := first.String()
	for _, forbidden := range []string{"owner_id", "document", "reading", "metadata", "book title"} {
		assert.NotContains(t, strings.ToLower(serialized), forbidden, "Batch request leaked forbidden field %q", forbidden)
	}
	for _, privateValue := range []string{"Haus", "Das Haus ist groß."} {
		for _, line := range strings.Split(strings.TrimSpace(serialized), "\n") {
			customIDStart := strings.Index(line, `"custom_id":"`)
			customIDEnd := strings.Index(line[customIDStart+13:], `"`)
			customID := line[customIDStart+13 : customIDStart+13+customIDEnd]
			assert.NotContains(t, customID, privateValue, "custom ID leaked provider input: %q", customID)
		}
	}
}

func TestBatchCustomIDRoundTripAndValidation(t *testing.T) {
	customID, err := BatchCustomID(strings.ToUpper(testBatchRunID), 42, 2)
	require.NoError(t, err)
	assert.Equal(t, "prepared-deck:"+testBatchRunID+":42:2", customID)
	identity, err := ParseBatchCustomID(customID)
	require.NoError(t, err)
	assert.Equal(t, BatchItemIdentity{RunID: testBatchRunID, Ordinal: 42, Generation: 2}, identity)
	assert.Equal(t, customID, identity.String())
	for _, invalid := range []string{"", "owner@example.com", "prepared-deck:" + testBatchRunID + ":-1:1", "prepared-deck:" + testBatchRunID + ":1:0", "prepared-deck:" + testBatchRunID + ":01:1"} {
		_, err := ParseBatchCustomID(invalid)
		assert.Error(t, err, "ParseBatchCustomID(%q)", invalid)
	}
}

func TestTranslationCodecPreservesReasoningCapability(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-5-mini", BaseURL: defaultLLMBaseURL, ReasoningEffort: "high"})
	require.NoError(t, err)
	body, err := codec.EncodeRequest(TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"})
	require.NoError(t, err)
	assert.True(t, bytes.Contains(body, []byte(`"reasoning_effort":"high"`)), "reasoning request=%s", body)
	assert.False(t, bytes.Contains(body, []byte(`"temperature"`)), "reasoning request=%s", body)
}

func TestTranslationCodecRejectsOversizedResponse(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	_, err = codec.DecodeResponse(TranslationRequest{}, bytes.Repeat([]byte("x"), maxTranslationResponseBytes+1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds 1 MiB")
}

func TestTranslationCodecUsageObservationKeepsSharedValidation(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist groß."}
	content := struct {
		ItemID         string `json:"item_id"`
		SourceLanguage string `json:"source_language"`
		TargetLanguage string `json:"target_language"`
		TranslationResponse
	}{TranslationItemID(input), input.Language, input.TargetLanguage, TranslationResponse{Translation: "house", Gloss: "building", SentenceTranslation: "The house is large.", SentenceTranslationTarget: "house"}}
	contentBytes, _ := json.Marshal(content)
	body, _ := json.Marshal(struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		Usage TranslationUsage `json:"usage"`
	}{Choices: []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{{Message: struct {
		Content string `json:"content"`
	}{string(contentBytes)}}}, Usage: TranslationUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20}})
	response, usage, err := codec.DecodeResponseWithUsage(input, body)
	require.NoError(t, err)
	assert.Equal(t, "house", response.Translation)
	assert.Equal(t, TranslationUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20}, usage)
}

func TestTranslationCodecRejectsIdentityAndLanguageDrift(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	base := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"house","gloss":"dwelling","sentence_translation":"","sentence_translation_target":""}`
	for name, content := range map[string]string{
		"missing":           strings.Replace(base, `"item_id":"`+TranslationItemID(input)+`",`, "", 1),
		"duplicate":         strings.Replace(base, `,"source_language"`, `,"item_id":"other","source_language"`, 1),
		"unexpected":        strings.Replace(base, `,"translation"`, `,"unexpected":"x","translation"`, 1),
		"mismatched item":   strings.Replace(base, TranslationItemID(input), "translation-item-other", 1),
		"mismatched source": strings.Replace(base, `"source_language":"de"`, `"source_language":"fr"`, 1),
		"mismatched target": strings.Replace(base, `"target_language":"en"`, `"target_language":"de"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
			_, err := codec.DecodeResponse(input, []byte(body))
			assert.Error(t, err, "invalid correlated response accepted")
		})
	}
}
