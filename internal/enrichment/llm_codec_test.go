package enrichment

import (
	"bytes"
	"encoding/json"
	"fmt"
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

// contextualEvidenceHandle returns the short handle a contextual Gloss request
// transmits for a stable evidence ID.
func contextualEvidenceHandle(input TranslationRequest, evidenceID string) string {
	return newContextualGlossHandles(input).toHandle[evidenceID]
}

func TestTranslationCodecAcceptsContextualGlossOnlyWithFrozenEvidenceReferences(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Bank", UPOS: "NOUN", TargetWord: "Bank", ExampleSentence: "Sie sitzt auf der Bank.", RequireContextualGloss: true, CandidateSenses: []LexicalSense{
		{EvidenceID: "wikt:seat", Gloss: "bench"},
		{EvidenceID: "wikt:finance", Gloss: "financial institution"},
	}}
	seat := contextualEvidenceHandle(input, "wikt:seat")
	content := `{"item_id":"` + ContextualGlossItemHandle + `","source_language":"de","target_language":"en","translation":"bank","sentence_translation":"She is sitting on the bench.","sentence_translation_targets":["bench"],"gloss":"bench","evidence_ids":["` + seat + `"],"context_only":false}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Equal(t, "bench", response.Gloss)
	assert.Equal(t, []string{"wikt:seat"}, response.EvidenceIDs)
	assert.False(t, response.ContextOnly)

	for name, invalid := range map[string]string{
		"unknown evidence":                  strings.Replace(content, seat, "e99", 1),
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
	seat := contextualEvidenceHandle(input, "wikt:seat")
	content := `{"item_id":"` + ContextualGlossItemHandle + `","source_language":"de","target_language":"en","translation":"bank","sentence_translation":"She sees the bank.","sentence_translation_targets":["bank"],"gloss":"","evidence_ids":[],"context_only":true,"unresolved_kind":"ambiguous","unresolved_reason":"The sentence does not provide enough context to distinguish the meanings."}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Empty(t, response.Gloss)
	assert.True(t, response.ContextOnly)
	assert.Equal(t, ContextualGlossUnresolvedAmbiguous, response.UnresolvedKind)
	assert.Equal(t, "The sentence does not provide enough context to distinguish the meanings.", response.UnresolvedReason)

	for name, invalid := range map[string]string{
		"missing reason":             strings.Replace(content, `,"unresolved_reason":"The sentence does not provide enough context to distinguish the meanings."`, "", 1),
		"missing kind":               strings.Replace(content, `"unresolved_kind":"ambiguous",`, "", 1),
		"unknown kind":               strings.Replace(content, `"unresolved_kind":"ambiguous"`, `"unresolved_kind":"different_lexeme"`, 1),
		"non-string kind":            strings.Replace(content, `"unresolved_kind":"ambiguous"`, `"unresolved_kind":1`, 1),
		"abstention with evidence":   strings.Replace(content, `"evidence_ids":[]`, `"evidence_ids":["`+seat+`"]`, 1),
		"abstention without context": strings.Replace(content, `"context_only":true`, `"context_only":false`, 1),
		"markup reason":              strings.Replace(content, "does not provide enough context", "<b>unsafe</b>", 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, decodeErr := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(invalid)+`}}]}`))
			assert.ErrorIs(t, decodeErr, ErrInvalidTranslationResponse)
		})
	}
}

func TestTranslationCodecAcceptsLemmaMismatchOmission(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "sehen", UPOS: "VERB", TargetWord: "sieht", ExampleSentence: "Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.", RequireContextualGloss: true}
	content := `{"item_id":"` + ContextualGlossItemHandle + `","source_language":"de","target_language":"en","translation":"to see","sentence_translation":"Grandmother watches television in the living room without picture or sound.","sentence_translation_targets":[],"gloss":"","evidence_ids":[],"context_only":true,"unresolved_kind":"lemma_mismatch","unresolved_reason":"The sentence uses fernsehen, not sehen."}`
	response, err := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(content)+`}}]}`))
	require.NoError(t, err)
	t.Run("omits the card as a lemma mismatch", func(t *testing.T) {
		require.Empty(t, response.Gloss)
		require.Equal(t, ContextualGlossUnresolvedLemmaMismatch, response.UnresolvedKind)
	})
	t.Run("keeps the bounded provider reason", func(t *testing.T) {
		require.Equal(t, "The sentence uses fernsehen, not sehen.", response.UnresolvedReason)
	})
}

func TestTranslationCodecDiscardsStrayUnresolvedReasonFromResolvedGloss(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Bank", UPOS: "NOUN", TargetWord: "Bank", ExampleSentence: "Sie sitzt auf der Bank.", RequireContextualGloss: true, CandidateSenses: []LexicalSense{
		{EvidenceID: "wikt:seat", Gloss: "bench"},
		{EvidenceID: "wikt:finance", Gloss: "financial institution"},
	}}
	seat := contextualEvidenceHandle(input, "wikt:seat")
	content := `{"item_id":"` + ContextualGlossItemHandle + `","source_language":"de","target_language":"en","translation":"bank","sentence_translation":"She is sitting on the bench.","sentence_translation_targets":["bench"],"gloss":"bench","evidence_ids":["` + seat + `"],"context_only":false,"unresolved_kind":"lemma_mismatch","unresolved_reason":"The gloss may be wrong."}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Equal(t, "bench", response.Gloss)
	assert.Equal(t, []string{"wikt:seat"}, response.EvidenceIDs)
	assert.False(t, response.ContextOnly)
	assert.Empty(t, response.UnresolvedKind, "a stray kind on a resolved gloss is discarded")
	assert.Empty(t, response.UnresolvedReason)
}

func TestTranslationCodecDiscardsInvalidAlignmentWithoutLosingTranslationOrGloss(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "umhauen", UPOS: "VERB", TargetWord: "haut", ExampleSentence: "Und dann haut das Motorrad Piero um.", RequireContextualGloss: true}
	content := `{"item_id":"` + ContextualGlossItemHandle + `","source_language":"de","target_language":"en","translation":"knock over","sentence_translation":"And then the motorcycle knocks Piero over.","sentence_translation_targets":["Piero","knocks"],"gloss":"knock down","evidence_ids":[],"context_only":true,"unresolved_reason":""}`
	body := `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}`
	response, err := codec.DecodeResponse(input, []byte(body))
	require.NoError(t, err)
	assert.Equal(t, "knock over", response.Translation)
	assert.Equal(t, "knock down", response.Gloss)
	assert.Equal(t, "And then the motorcycle knocks Piero over.", response.SentenceTranslation)
	assert.Empty(t, response.SentenceTranslationTargets)
}

// TestTranslationCodecTransmitsShortHandlesAndResolvesThemBack covers the
// #2125 contract: the contextual Gloss payload carries short per-request
// handles, and decoding maps the echoed handles back to the stable evidence ID.
func TestTranslationCodecTransmitsShortHandlesAndResolvesThemBack(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	bindID := "wiktionary:" + strings.Repeat("a", 64)
	lackID := "wiktionary:" + strings.Repeat("b", 64)
	input := TranslationRequest{Language: "grc", TargetLanguage: "en", CanonicalLemma: "δέω", UPOS: "VERB", TargetWord: "δέω", ExampleSentence: "ἄνθρωπος ὁρᾷ τὸν λόγον καὶ δέω βραδέως;", RequireContextualGloss: true, CandidateSenses: []LexicalSense{
		{EvidenceID: bindID, Gloss: "bind"},
		{EvidenceID: lackID, Gloss: "lack"},
		{Gloss: "no evidence id"},
	}}
	content := `{"item_id":"` + ContextualGlossItemHandle + `","source_language":"grc","target_language":"en","translation":"I bind","sentence_translation":"A man sees the word and I bind slowly.","sentence_translation_targets":["bind"],"gloss":"bind","evidence_ids":["e1"],"context_only":false}`

	t.Run("payload carries short handles", func(t *testing.T) {
		body, err := codec.EncodeRequest(input)
		require.NoError(t, err)
		user := encodedUserContent(t, body)
		for _, want := range []string{`"item_id":"` + ContextualGlossItemHandle + `"`, `"evidence_id":"e1"`, `"evidence_id":"e2"`, `"gloss":"no evidence id"`} {
			require.Contains(t, user, want)
		}
		require.NotContains(t, user, "wiktionary:", "the stable evidence IDs are not transmitted")
		require.NotContains(t, user, TranslationItemID(input), "the stable item ID is not transmitted")
	})

	t.Run("decoded response carries the original stable IDs", func(t *testing.T) {
		response, err := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(content)+`}}]}`))
		require.NoError(t, err)
		require.Equal(t, []string{bindID}, response.EvidenceIDs)
		require.Equal(t, TranslationItemID(input), response.ItemID)
	})

	for name, invalid := range map[string]string{
		"unknown handle":    strings.Replace(content, `"e1"`, `"e3"`, 1),
		"duplicate handle":  strings.Replace(content, `"evidence_ids":["e1"]`, `"evidence_ids":["e1","e1"]`, 1),
		"stable evidence":   strings.Replace(content, `"e1"`, `"`+bindID+`"`, 1),
		"wrong item handle": strings.Replace(content, `"item_id":"`+ContextualGlossItemHandle+`"`, `"item_id":"i2"`, 1),
		"stable item ID":    strings.Replace(content, `"item_id":"`+ContextualGlossItemHandle+`"`, `"item_id":"`+TranslationItemID(input)+`"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			_, decodeErr := codec.DecodeResponse(input, []byte(`{"choices":[{"message":{"content":`+strconv.Quote(invalid)+`}}]}`))
			require.ErrorIs(t, decodeErr, ErrInvalidTranslationResponse)
		})
	}
}

// TestContextualGlossPromptRequiresLemmaMismatchOutcome covers the #2328
// contract: the prompt names the new unresolved kind and its contract version
// changes so cached v15 results are never reused.
func TestContextualGlossPromptRequiresLemmaMismatchOutcome(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	body, err := codec.EncodeRequest(TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "sehen", UPOS: "VERB", TargetWord: "sieht", ExampleSentence: "Großmutter sieht im Wohnzimmer fern.", RequireContextualGloss: true})
	require.NoError(t, err)
	var request chatRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	prompt := request.Messages[0].Content
	t.Run("names the unresolved kind", func(t *testing.T) {
		require.Contains(t, prompt, "unresolved_kind")
		require.Contains(t, prompt, "lemma_mismatch")
	})
	t.Run("requires a different-lexeme outcome", func(t *testing.T) {
		require.Contains(t, prompt, "different lexeme")
	})
	t.Run("bumps the contract version", func(t *testing.T) {
		require.Equal(t, "translation-v16-lemma-mismatch", codec.ContextualGlossPromptContract())
	})
}

// TestTranslationCodecKeepsStableIdentifiersOutsideContextualGloss guards the
// other provider path: only the contextual Gloss payload shortens IDs.
func TestTranslationCodecKeepsStableIdentifiersOutsideContextualGloss(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", CandidateSenses: []LexicalSense{{EvidenceID: "wikt:house", Gloss: "building"}}}
	body, err := codec.EncodeRequest(input)
	require.NoError(t, err)
	user := encodedUserContent(t, body)
	assert.Contains(t, user, `"item_id":"`+TranslationItemID(input)+`"`)
	assert.Contains(t, user, `"evidence_id":"wikt:house"`)
}

// encodedUserContent returns the user message content of an encoded request.
func encodedUserContent(t *testing.T, body []byte) string {
	t.Helper()
	var request chatRequest
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	return request.Messages[1].Content
}

// TestTranslationCodecBatchContextualHandlesKeepTheCustomIDCorrelation covers
// the Batch acceptance criterion: correlation still uses the request's custom
// ID while the body carries short handles that decode back to stable IDs.
func TestTranslationCodecBatchContextualHandlesKeepTheCustomIDCorrelation(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	item := BatchTranslationItem{Ordinal: 2, Request: TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Bank", UPOS: "NOUN", TargetWord: "Bank", ExampleSentence: "Sie sitzt auf der Bank.", RequireContextualGloss: true, CandidateSenses: []LexicalSense{{EvidenceID: "wikt:seat", Gloss: "bench"}}}}
	customID, err := BatchCustomID(testBatchRunID, item.Ordinal, 1)
	require.NoError(t, err)
	var buf bytes.Buffer
	_, err = codec.WriteBatchJSONL(&buf, testBatchRunID, 1, []BatchTranslationItem{item})
	require.NoError(t, err)

	var line struct {
		CustomID string `json:"custom_id"`
		Body     struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		} `json:"body"`
	}
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &line))
	assert.Equal(t, customID, line.CustomID)
	var payload struct {
		ItemID          string         `json:"item_id"`
		CandidateSenses []LexicalSense `json:"candidate_senses"`
	}
	require.NoError(t, json.Unmarshal([]byte(line.Body.Messages[1].Content), &payload))
	assert.Equal(t, ContextualGlossItemHandle, payload.ItemID)
	require.Len(t, payload.CandidateSenses, 1)
	assert.Equal(t, "e1", payload.CandidateSenses[0].EvidenceID)

	content := `{"item_id":"` + ContextualGlossItemHandle + `","source_language":"de","target_language":"en","translation":"bank","sentence_translation":"She is sitting on the bench.","sentence_translation_targets":["bench"],"gloss":"bench","evidence_ids":["e1"],"context_only":false}`
	output := fmt.Sprintf(`{"custom_id":%q,"response":{"status_code":200,"body":{"choices":[{"message":{"content":%q}}]}}}`+"\n", customID, content)
	outcomes, err := codec.DecodeBatchResults(testBatchRunID, 1, []BatchTranslationItem{item}, strings.NewReader(output), nil)
	require.NoError(t, err)
	require.True(t, outcomes[item.Ordinal].Successful())
	assert.Equal(t, []string{"wikt:seat"}, outcomes[item.Ordinal].Response.EvidenceIDs)
	assert.Equal(t, TranslationItemID(item.Request), outcomes[item.Ordinal].Response.ItemID)
}

// codecTranslationContent builds the strict provider content object from the
// fields that follow the echoed identity and languages.
func codecTranslationContent(input TranslationRequest, fields string) string {
	return `{"item_id":"` + TranslationItemID(input) + `","source_language":"` + input.Language + `","target_language":"` + translationTargetLanguage(input) + `",` + fields + `}`
}

// decodeCodecContent runs one content object through the shared decoder,
// exercising the public DecodeResponse seam.
func decodeCodecContent(t *testing.T, codec *TranslationCodec, input TranslationRequest, fields string) (TranslationResponse, error) {
	t.Helper()
	body := `{"choices":[{"message":{"content":` + strconv.Quote(codecTranslationContent(input, fields)) + `}}]}`
	return codec.DecodeResponse(input, []byte(body))
}

func TestTranslationCodecRejectsMalformedResponseEnvelope(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	content := codecTranslationContent(input, `"translation":"house","sentence_translation":"","sentence_translation_targets":[]`)

	t.Run("nil codec", func(t *testing.T) {
		var nilCodec *TranslationCodec
		_, err := nilCodec.DecodeResponse(input, []byte(`{}`))
		require.Error(t, err)
		require.ErrorIs(t, err, ErrInvalidTranslationResponse)
		assert.Contains(t, err.Error(), "nil translation codec")
	})

	for name, body := range map[string]string{
		"malformed envelope": `not json`,
		"trailing envelope":  `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}]}{}`,
		"no choices":         `{"choices":[]}`,
		"invalid usage":      `{"choices":[{"message":{"content":` + strconv.Quote(content) + `}}],"usage":{"prompt_tokens":-1,"completion_tokens":0,"total_tokens":0}}`,
		"trailing content":   `{"choices":[{"message":{"content":` + strconv.Quote(content+" {}") + `}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := codec.DecodeResponse(input, []byte(body))
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidTranslationResponse)
		})
	}
}

func TestTranslationCodecRejectsMalformedOptionalMeaningFields(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "model"})
	require.NoError(t, err)
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	base := `"translation":"house","sentence_translation":"","sentence_translation_targets":[]`

	for name, fields := range map[string]string{
		"markup translation":   `"translation":"<b>house</b>","sentence_translation":"","sentence_translation_targets":[]`,
		"invalid gloss":        base + `,"gloss":123`,
		"invalid evidence ids": base + `,"gloss":"bench","evidence_ids":123`,
		"invalid context only": base + `,"gloss":"bench","evidence_ids":[],"context_only":"x"`,
		"invalid reason":       base + `,"gloss":"bench","evidence_ids":[],"context_only":false,"unresolved_reason":123`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := decodeCodecContent(t, codec, input, fields)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidTranslationResponse)
		})
	}

	for _, test := range []struct {
		name, fields, warning string
	}{
		{"invalid sense order", base + `,"sense_order":"x"`, "invalid sense selection; using deterministic order"},
		{"invalid fallback gloss", base + `,"fallback_gloss":123`, "invalid fallback gloss; ignoring it"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response, err := decodeCodecContent(t, codec, input, test.fields)
			require.NoError(t, err)
			assert.Contains(t, response.Warnings, test.warning)
		})
	}

	t.Run("non-array targets are dropped", func(t *testing.T) {
		response, err := decodeCodecContent(t, codec, input, `"translation":"house","sentence_translation":"","sentence_translation_targets":123`)
		require.NoError(t, err)
		assert.Empty(t, response.SentenceTranslationTargets)
	})
}

func TestNormalizeTranslationResponseRejectsMalformedMeaningFields(t *testing.T) {
	lemmaOnly := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN"}
	withSentence := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Haus", UPOS: "NOUN", ExampleSentence: "Das Haus ist groß."}
	contextual := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Bank", UPOS: "NOUN", RequireContextualGloss: true, CandidateSenses: []LexicalSense{{EvidenceID: "wikt:seat", Gloss: "bench"}}}
	contextualResponse := func() TranslationResponse {
		return TranslationResponse{Translation: "bank", SentenceTranslation: "She sits on the bench.", Gloss: "bench", EvidenceIDs: []string{"wikt:seat"}}
	}

	for _, test := range []struct {
		name    string
		input   TranslationRequest
		build   func() TranslationResponse
		message string
	}{
		{"empty translation", lemmaOnly, func() TranslationResponse { return TranslationResponse{} }, "translation is empty"},
		{"missing sentence translation", withSentence, func() TranslationResponse { return TranslationResponse{Translation: "house"} }, "sentence_translation is empty"},
		{"markup translation", lemmaOnly, func() TranslationResponse { return TranslationResponse{Translation: "<b>house</b>"} }, "HTML or markup"},
		{"overlong contextual gloss", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.Gloss = strings.Repeat("g", MaxFallbackGlossRunes+1)
			return response
		}, "invalid contextual gloss"},
		{"markup contextual gloss", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.Gloss = "<b>bench</b>"
			return response
		}, "invalid contextual gloss"},
		{"overlong unresolved reason", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.Gloss, response.EvidenceIDs, response.ContextOnly = "", nil, true
			response.UnresolvedKind = ContextualGlossUnresolvedAmbiguous
			response.UnresolvedReason = strings.Repeat("r", 201)
			return response
		}, "invalid unresolved reason"},
		{"empty evidence id", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.EvidenceIDs = []string{""}
			return response
		}, "empty evidence ID"},
		{"duplicate evidence id", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.EvidenceIDs = []string{"wikt:seat", "wikt:seat"}
			return response
		}, "duplicate evidence ID"},
		{"unknown evidence id", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.EvidenceIDs = []string{"wikt:unknown"}
			return response
		}, "unknown evidence ID"},
		{"context only contradicts evidence", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.ContextOnly = true
			return response
		}, "context_only contradicts evidence IDs"},
		{"unresolved meaning incomplete", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.Gloss, response.EvidenceIDs, response.ContextOnly = "", nil, true
			response.UnresolvedKind = ContextualGlossUnresolvedAmbiguous
			return response
		}, "unresolved meaning is incomplete"},
		{"missing unresolved kind", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.Gloss, response.EvidenceIDs, response.ContextOnly = "", nil, true
			response.UnresolvedReason = "The sentence does not distinguish the meanings."
			return response
		}, "unresolved meaning is incomplete"},
		{"unknown unresolved kind", contextual, func() TranslationResponse {
			response := contextualResponse()
			response.Gloss, response.EvidenceIDs, response.ContextOnly = "", nil, true
			response.UnresolvedKind = "different_lexeme"
			response.UnresolvedReason = "The sentence does not distinguish the meanings."
			return response
		}, "invalid unresolved kind"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeTranslationResponse(test.input, test.build())
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.message)
		})
	}
}

func TestNormalizeTranslationResponseDiscardsStrayUnresolvedReasonFromResolvedGloss(t *testing.T) {
	contextual := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "Bank", UPOS: "NOUN", RequireContextualGloss: true, CandidateSenses: []LexicalSense{{EvidenceID: "wikt:seat", Gloss: "bench"}}}
	response, err := NormalizeTranslationResponse(contextual, TranslationResponse{
		Translation: "bank", SentenceTranslation: "She sits on the bench.", Gloss: "bench",
		EvidenceIDs: []string{"wikt:seat"}, UnresolvedReason: "The gloss may be wrong.",
	})
	require.NoError(t, err)
	assert.Equal(t, "bench", response.Gloss)
	assert.False(t, response.ContextOnly)
	assert.Empty(t, response.UnresolvedReason)
}

func TestNormalizeTranslationResponseAppliesSelectionAndFallbackRules(t *testing.T) {
	input := TranslationRequest{Language: "de", TargetLanguage: "en", CanonicalLemma: "laufen", UPOS: "VERB", CandidateSenses: []LexicalSense{{Gloss: "run"}, {Gloss: "walk"}}}

	t.Run("resets a selection flagged by an earlier warning", func(t *testing.T) {
		response := TranslationResponse{Translation: "run", SentenceTranslation: "It runs.", SenseOrder: []int{0}, FallbackGloss: "operate", Warnings: []string{"invalid sense selection; using deterministic order"}}
		got, err := NormalizeTranslationResponse(input, response)
		require.NoError(t, err)
		assert.Empty(t, got.SenseOrder)
		assert.Empty(t, got.FallbackGloss)
		assert.Equal(t, []string{"invalid sense selection; using deterministic order"}, got.Warnings)
	})

	t.Run("ignores an invalid fallback gloss", func(t *testing.T) {
		got, err := NormalizeTranslationResponse(input, TranslationResponse{Translation: "run", SentenceTranslation: "It runs.", FallbackGloss: "<b>operate</b>"})
		require.NoError(t, err)
		assert.Empty(t, got.FallbackGloss)
		assert.Contains(t, got.Warnings, "invalid fallback gloss; ignoring it")
	})

	t.Run("drops a fallback gloss supplied with a sense selection", func(t *testing.T) {
		got, err := NormalizeTranslationResponse(input, TranslationResponse{Translation: "run", SentenceTranslation: "It runs.", SenseOrder: []int{0}, FallbackGloss: "operate"})
		require.NoError(t, err)
		assert.Equal(t, []int{0}, got.SenseOrder)
		assert.Empty(t, got.FallbackGloss)
		assert.Contains(t, got.Warnings, "fallback gloss supplied with a sense selection; ignoring it")
	})
}
