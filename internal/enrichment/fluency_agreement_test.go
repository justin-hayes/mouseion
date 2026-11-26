package enrichment

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fluencyRequest() TranslationRequest {
	return TranslationRequest{
		Language: "de", TargetLanguage: "en", CanonicalLemma: "gehen", UPOS: "VERB",
		TargetWord: "ging", ExampleSentence: "Er ging nach Hause.", LearnedMeaning: "to go",
		RequireFluencyAgreement: true,
	}
}

func fluencyEnvelope(t *testing.T, content string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}}})
	require.NoError(t, err)
	return body
}

func TestFluencyAgreementRequestSendsOnlyTheAssessedFields(t *testing.T) {
	body, err := passCodec(t).EncodeRequest(fluencyRequest())
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	assert.Equal(t, fluencyAgreementSystemPrompt, request.Messages[0].Content)

	var payload map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &payload))
	var fields []string
	for field := range payload {
		fields = append(fields, field)
	}
	assert.ElementsMatch(t, FluencyAgreementPayloadFields(), fields, "the payload is exactly the reviewed field set")
	assert.JSONEq(t, `"to go"`, string(payload["learned_meaning"]))
	assert.JSONEq(t, `"f1"`, string(payload["item_id"]))
}

func TestFluencyAgreementRequestNeedsTheLearnedMeaning(t *testing.T) {
	request := fluencyRequest()
	request.LearnedMeaning = " "
	_, err := passCodec(t).EncodeRequest(request)
	require.Error(t, err)
}

func TestDecodeFluencyAgreement(t *testing.T) {
	codec := passCodec(t)
	body := func(reason string) string {
		return `{"item_id":"f1","reason":"` + reason + `","gloss":"to go","sentence_translation":"He went home.","sentence_translation_targets":["went"]}`
	}
	for _, reason := range fluencyReasons {
		t.Run("reason "+reason, func(t *testing.T) {
			response, _, err := codec.DecodeResponseWithUsage(fluencyRequest(), fluencyEnvelope(t, body(reason)))
			require.NoError(t, err)
			assert.Equal(t, reason, response.FluencyReason)
			assert.Equal(t, "to go", response.Gloss, "every reason keeps its Gloss")
			assert.Equal(t, "He went home.", response.SentenceTranslation)
			assert.Equal(t, []string{"went"}, response.SentenceTranslationTargets)
		})
	}
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"unknown reason", body("maybe"), "reason"},
		{"missing reason", `{"item_id":"f1","gloss":"to go","sentence_translation":"x","sentence_translation_targets":[]}`, "reason"},
		{"the v1 boolean", `{"item_id":"f1","uses_learned_meaning":true,"gloss":"to go","sentence_translation":"x","sentence_translation_targets":[]}`, "unknown field"},
		{"agreeing without a gloss", `{"item_id":"f1","reason":"same-meaning","gloss":"","sentence_translation":"He went home.","sentence_translation_targets":[]}`, "needs a gloss"},
		{"disagreeing without a gloss", `{"item_id":"f1","reason":"idiomatic","gloss":"","sentence_translation":"He went home.","sentence_translation_targets":[]}`, "needs a gloss"},
		{"disagreeing without a translation", `{"item_id":"f1","reason":"idiomatic","gloss":"to go","sentence_translation":"","sentence_translation_targets":[]}`, "needs a gloss"},
		{"wrong item", `{"item_id":"c1","reason":"same-meaning","gloss":"to go","sentence_translation":"x","sentence_translation_targets":[]}`, "item_id mismatch"},
		{"unknown field", `{"item_id":"f1","reason":"same-meaning","gloss":"to go","sentence_translation":"x","sentence_translation_targets":[],"note":"y"}`, "unknown field"},
		{"duplicate field", `{"item_id":"f1","item_id":"f1","reason":"same-meaning","gloss":"to go","sentence_translation":"x","sentence_translation_targets":[]}`, "duplicate"},
		{"markup", `{"item_id":"f1","reason":"same-meaning","gloss":"<b>to go</b>","sentence_translation":"x","sentence_translation_targets":[]}`, "markup"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := codec.DecodeResponseWithUsage(fluencyRequest(), fluencyEnvelope(t, tc.content))
			require.ErrorIs(t, err, ErrInvalidTranslationResponse)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestFluencyAgreementAssessmentIsRecordedForGermanOnly(t *testing.T) {
	assertRecordedAssessment(t, "#2702", FluencyAgreementPayload, "de",
		(*TranslationCodec).FluencyAgreementPromptContract, FluencyAgreementPayloadFields)

	t.Run("another language is refused", func(t *testing.T) {
		require.ErrorIs(t, ProviderAssessments().MaySend(FluencyAgreementPayload, "it", passCodec(t)), ErrProviderAssessmentNotRecorded, "a language without a Fluency agreement assessment is not rolled out by implication")
	})
}

func TestEnrichFluencyAgreement(t *testing.T) {
	candidate := Candidate{Language: "de", CanonicalLemma: "gehen", UPOS: "verb", TargetWord: "ging", ExampleSentence: "Er ging nach Hause."}
	t.Run("an agreeing answer keeps the occurrence's own Gloss and translation", func(t *testing.T) {
		provider := &contextualProvider{enabled: true, response: TranslationResponse{FluencyReason: FluencyReasonSameMeaning, Gloss: "to go", SentenceTranslation: "He went home.", SentenceTranslationTargets: []string{"went"}}}
		service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
		agreement, err := service.EnrichFluencyAgreement(context.Background(), candidate, "to go")
		require.NoError(t, err)
		assert.True(t, agreement.Agrees())
		assert.Equal(t, FluencyReasonSameMeaning, agreement.Reason)
		assert.Equal(t, "to go", agreement.Gloss)
		assert.Equal(t, []string{"went"}, agreement.SentenceTranslationTargets)
		require.Len(t, provider.requests, 1)
		assert.True(t, provider.requests[0].RequireFluencyAgreement)
		assert.Equal(t, "to go", provider.requests[0].LearnedMeaning)
		assert.Equal(t, "VERB", provider.requests[0].UPOS)
	})
	t.Run("a disagreement is an answer with a reason, not an error", func(t *testing.T) {
		provider := &contextualProvider{enabled: true, response: TranslationResponse{FluencyReason: FluencyReasonNotThisLemma, Gloss: "to work", SentenceTranslation: "It works.", SentenceTranslationTargets: []string{"works"}}}
		service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
		agreement, err := service.EnrichFluencyAgreement(context.Background(), candidate, "to go")
		require.NoError(t, err)
		assert.False(t, agreement.Agrees())
		assert.Equal(t, FluencyReasonNotThisLemma, agreement.Reason)
		assert.Equal(t, "to work", agreement.Gloss, "a disagreeing answer keeps its Gloss as evidence")
		assert.Equal(t, "It works.", agreement.SentenceTranslation)
		assert.Equal(t, []string{"works"}, agreement.SentenceTranslationTargets)
	})
	t.Run("not configured is not an answer", func(t *testing.T) {
		service := NewService(Config{ExternalEnabled: true, UserOptIn: true}, nil, nil, nil, &contextualProvider{enabled: false}, nil)
		_, err := service.EnrichFluencyAgreement(context.Background(), candidate, "to go")
		require.ErrorIs(t, err, ErrExternalNotConfigured)
	})
	t.Run("a provider failure is retryable", func(t *testing.T) {
		provider := &contextualProvider{enabled: true, err: errors.New("down")}
		service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
		_, err := service.EnrichFluencyAgreement(context.Background(), candidate, "to go")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrExternalNotConfigured)
	})
}
