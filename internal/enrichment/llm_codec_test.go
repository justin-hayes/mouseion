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
		for line := range strings.SplitSeq(strings.TrimSpace(serialized), "\n") {
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
	}{TranslationItemID(input), input.Language, input.TargetLanguage, TranslationResponse{Translation: "house", FallbackGloss: "building", SentenceTranslation: "The house is large.", SentenceTranslationTargets: []string{"house"}}}
	contentBytes, err := json.Marshal(content)
	require.NoError(t, err)
	body, err := json.Marshal(struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage TranslationUsage `json:"usage"`
	}{Choices: []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{{Message: struct {
		Content string `json:"content"`
	}{string(contentBytes)}}}, Usage: TranslationUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20}})
	require.NoError(t, err)
	response, usage, err := codec.DecodeResponseWithUsage(input, body)
	require.NoError(t, err)
	assert.Equal(t, "house", response.Translation)
	assert.Equal(t, TranslationUsage{PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20}, usage)
}

func TestTranslationCodecRejectsIdentityAndLanguageDrift(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	base := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"house","gloss":"dwelling","sentence_translation":"","sentence_translation_targets":[]}`
	for name, content := range map[string]string{
		"malformed JSON":       `{"item_id":`,
		"missing":              strings.Replace(base, `"item_id":"`+TranslationItemID(input)+`",`, "", 1),
		"duplicate":            strings.Replace(base, `,"source_language"`, `,"item_id":"other","source_language"`, 1),
		"unexpected":           strings.Replace(base, `,"translation"`, `,"unexpected":"x","translation"`, 1),
		"legacy single phrase": strings.Replace(base, `,"sentence_translation_targets":[]`, `,"sentence_translation_target":"house"`, 1),
		"empty translation":    strings.Replace(base, `"translation":"house"`, `"translation":""`, 1),
		"mismatched item":      strings.Replace(base, TranslationItemID(input), "translation-item-other", 1),
		"mismatched source":    strings.Replace(base, `"source_language":"de"`, `"source_language":"fr"`, 1),
		"mismatched target":    strings.Replace(base, `"target_language":"en"`, `"target_language":"de"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
			_, err := codec.DecodeResponse(input, []byte(body))
			require.Error(t, err, "invalid correlated response accepted")
			assert.ErrorIs(t, err, ErrInvalidTranslationResponse, "malformed response must be retryable by the caller")
		})
	}
}

func TestTranslationCodecCarriesFrozenSenseCandidatesAndValidatesSelectionRange(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{
		Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN",
		CandidateSenses: []LexicalSense{{Gloss: "building"}, {Gloss: "house"}},
	}
	body, err := codec.EncodeRequest(input)
	require.NoError(t, err)
	assert.Contains(t, string(body), `candidate_senses`)
	assert.Contains(t, string(body), `building`)
	assert.Contains(t, string(body), `house`)
	assert.Contains(t, string(body), `0-based`)
	assert.Contains(t, string(body), `display limit of 3`)
	assert.Contains(t, string(body), `omit it, or return an empty array`)
	assert.Contains(t, string(body), `sentence_translation_targets`)
	assert.NotContains(t, string(body), `sentence_translation_target `)

	content := struct {
		ItemID         string `json:"item_id"`
		SourceLanguage string `json:"source_language"`
		TargetLanguage string `json:"target_language"`
		TranslationResponse
	}{TranslationItemID(input), input.Language, input.TargetLanguage, TranslationResponse{Translation: "house", SenseOrder: []int{2}, FallbackGloss: "dwelling"}}
	contentBytes, marshalErr := json.Marshal(content)
	require.NoError(t, marshalErr)
	responseBody, marshalErr := json.Marshal(struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}{Choices: []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{{Message: struct {
		Content string `json:"content"`
	}{string(contentBytes)}}}})
	require.NoError(t, marshalErr)
	response, err := codec.DecodeResponse(input, responseBody)
	require.NoError(t, err)
	assert.Empty(t, response.SenseOrder)
	assert.Empty(t, response.FallbackGloss)
	assert.Contains(t, response.Warnings, "invalid sense selection; using deterministic order")
}

func TestTranslationCodecCapsOverLimitSenseSelectionInModelOrder(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{
		Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN",
		CandidateSenses: []LexicalSense{{Gloss: "building"}, {Gloss: "house"}, {Gloss: "home"}, {Gloss: "dwelling"}},
	}
	content := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"house","sense_order":[3,1,0,2],"sentence_translation":"","sentence_translation_targets":[]}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`

	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Equal(t, []int{3, 1, 0}, response.SenseOrder)
	assert.Empty(t, response.Warnings)
}

func TestTranslationCodecAcceptsFallbackWhenSenseSelectionIsOmitted(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{
		Language: "de", TargetLanguage: "en", CanonicalLemma: "laufen", UPOS: "VERB",
		CandidateSenses: []LexicalSense{{Gloss: "run"}, {Gloss: "walk"}},
	}
	content := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"run","fallback_gloss":"operate","sentence_translation":"","sentence_translation_targets":[]}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Equal(t, "operate", response.FallbackGloss)
	assert.NotContains(t, response.Warnings, "invalid sense selection; using deterministic order")
}

func TestTranslationCodecRequestsFallbackWhenCandidateSensesAreEmpty(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "seltenes-wort", UPOS: "NOUN", ExampleSentence: "Das Seltene ist heute wichtig."}
	body, err := codec.EncodeRequest(input)
	require.NoError(t, err)
	var request chatRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	assert.NotContains(t, request.Messages[1].Content, `candidate_senses`)
	assert.Contains(t, string(body), "fallback_gloss")
	assert.Contains(t, string(body), "candidate list is empty")

	content := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"rare word","fallback_gloss":"something uncommon","sentence_translation":"The rare thing is important today.","sentence_translation_targets":["rare"]}`
	responseBody := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(responseBody))
	require.NoError(t, err)
	assert.Equal(t, "something uncommon", response.FallbackGloss)
	assert.Empty(t, response.SenseOrder)
	assert.NotContains(t, response.Warnings, "invalid sense selection; using deterministic order")
}

func TestTranslationCodecAcceptsContextualGlossOnlyWithFrozenEvidenceReferences(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Bank", UPOS: "NOUN", TargetWord: "Bank", ExampleSentence: "Sie sitzt auf der Bank.", RequireContextualGloss: true, CandidateSenses: []LexicalSense{
		{EvidenceID: "wikt:seat", Gloss: "bench"},
		{EvidenceID: "wikt:finance", Gloss: "financial institution"},
	}}
	content := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"bank","sentence_translation":"She is sitting on the bench.","sentence_translation_targets":["bench"],"gloss":"bench","evidence_ids":["wikt:seat"],"context_only":false}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Equal(t, "bench", response.Gloss)
	assert.Equal(t, []string{"wikt:seat"}, response.EvidenceIDs)
	assert.False(t, response.ContextOnly)

	for name, invalid := range map[string]string{
		"unknown evidence":                  strings.Replace(content, `wikt:seat`, `wikt:unknown`, 1),
		"context flag contradicts evidence": strings.Replace(content, `"context_only":false`, `"context_only":true`, 1),
		"missing contextual gloss":          strings.Replace(content, `"gloss":"bench",`, ``, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, decodeErr := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(invalid)+`}}]}`))
			assert.ErrorIs(t, decodeErr, ErrInvalidTranslationResponse)
		})
	}
}

func TestTranslationCodecAcceptsExplicitlyUnresolvedMeaningAndRejectsMalformedOmission(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Bank", UPOS: "NOUN", TargetWord: "Bank", ExampleSentence: "Sie sieht die Bank.", RequireContextualGloss: true, CandidateSenses: []LexicalSense{{EvidenceID: "wikt:seat", Gloss: "bench"}}}
	content := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"bank","sentence_translation":"She sees the bank.","sentence_translation_targets":["bank"],"gloss":"","evidence_ids":[],"context_only":true,"unresolved_reason":"The sentence does not provide enough context to distinguish the meanings."}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Empty(t, response.Gloss)
	assert.True(t, response.ContextOnly)
	assert.Equal(t, "The sentence does not provide enough context to distinguish the meanings.", response.UnresolvedReason)

	for name, invalid := range map[string]string{
		"missing reason":             strings.Replace(content, `,"unresolved_reason":"The sentence does not provide enough context to distinguish the meanings."`, "", 1),
		"resolved gloss with reason": strings.Replace(strings.Replace(content, `"gloss":""`, `"gloss":"bench"`, 1), `"evidence_ids":[]`, `"evidence_ids":["wikt:seat"]`, 1),
		"markup reason":              strings.Replace(content, "does not provide enough context", "<b>unsafe</b>", 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, decodeErr := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(invalid)+`}}]}`))
			assert.ErrorIs(t, decodeErr, ErrInvalidTranslationResponse)
		})
	}
}

func TestTranslationCodecDiscardsInvalidAlignmentWithoutLosingTranslationOrGloss(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "umhauen", UPOS: "VERB", TargetWord: "haut", ExampleSentence: "Und dann haut das Motorrad Piero um.", RequireContextualGloss: true}
	content := `{"item_id":"` + TranslationItemID(input) + `","source_language":"de","target_language":"en","translation":"knock over","sentence_translation":"And then the motorcycle knocks Piero over.","sentence_translation_targets":["Piero","knocks"],"gloss":"knock down","evidence_ids":[],"context_only":true,"unresolved_reason":""}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Equal(t, "knock over", response.Translation)
	assert.Equal(t, "knock down", response.Gloss)
	assert.Equal(t, "And then the motorcycle knocks Piero over.", response.SentenceTranslation)
	assert.Empty(t, response.SentenceTranslationTargets)
}
