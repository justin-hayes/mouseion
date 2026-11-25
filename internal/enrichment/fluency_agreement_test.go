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

func fluencyEnvelope(content string) []byte {
	body, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": content}}}})
	return body
}

func TestFluencyAgreementRequestSendsOnlyTheAssessedFields(t *testing.T) {
	body, err := passCodec(t).EncodeRequest(fluencyRequest())
	require.NoError(t, err)
	var request struct {
		Messages []struct{ Role, Content string } `json:"messages"`
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
	cases := []struct {
		name    string
		content string
		agrees  bool
		wantErr string
	}{
		{"agrees", `{"item_id":"f1","uses_learned_meaning":true,"gloss":"to go","sentence_translation":"He went home.","sentence_translation_targets":["went"]}`, true, ""},
		{"disagrees", `{"item_id":"f1","uses_learned_meaning":false,"gloss":"to work","sentence_translation":"It works.","sentence_translation_targets":[]}`, false, ""},
		{"agrees without a gloss", `{"item_id":"f1","uses_learned_meaning":true,"gloss":"","sentence_translation":"He went home.","sentence_translation_targets":[]}`, false, "needs a gloss"},
		{"missing agreement", `{"item_id":"f1","gloss":"to go","sentence_translation":"x","sentence_translation_targets":[]}`, false, "uses_learned_meaning is required"},
		{"wrong item", `{"item_id":"c1","uses_learned_meaning":true,"gloss":"to go","sentence_translation":"x","sentence_translation_targets":[]}`, false, "item_id mismatch"},
		{"unknown field", `{"item_id":"f1","uses_learned_meaning":true,"gloss":"to go","sentence_translation":"x","sentence_translation_targets":[],"note":"y"}`, false, "unknown field"},
		{"duplicate field", `{"item_id":"f1","item_id":"f1","uses_learned_meaning":true,"gloss":"to go","sentence_translation":"x","sentence_translation_targets":[]}`, false, "duplicate"},
		{"markup", `{"item_id":"f1","uses_learned_meaning":true,"gloss":"<b>to go</b>","sentence_translation":"x","sentence_translation_targets":[]}`, false, "markup"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			response, _, err := codec.DecodeResponseWithUsage(fluencyRequest(), fluencyEnvelope(tc.content))
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.ErrorIs(t, err, ErrInvalidTranslationResponse)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.agrees, response.UsesLearnedMeaning)
		})
	}
}

func TestFluencyAgreementAssessmentFailsClosedUntilRecorded(t *testing.T) {
	err := ProviderAssessments().MaySend(FluencyAgreementPayload, "de", passCodec(t))
	require.ErrorIs(t, err, ErrProviderAssessmentNotRecorded, "the payload sends the learned meaning and has not been reviewed")
}

func TestEnrichFluencyAgreement(t *testing.T) {
	candidate := Candidate{Language: "de", CanonicalLemma: "gehen", UPOS: "verb", TargetWord: "ging", ExampleSentence: "Er ging nach Hause."}
	t.Run("an agreeing answer keeps the occurrence's own Gloss and translation", func(t *testing.T) {
		provider := &contextualProvider{enabled: true, response: TranslationResponse{UsesLearnedMeaning: true, Gloss: "to go", SentenceTranslation: "He went home.", SentenceTranslationTargets: []string{"went"}}}
		service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
		agreement, err := service.EnrichFluencyAgreement(context.Background(), candidate, "to go")
		require.NoError(t, err)
		assert.True(t, agreement.Agrees)
		assert.Equal(t, "to go", agreement.Gloss)
		assert.Equal(t, []string{"went"}, agreement.SentenceTranslationTargets)
		require.Len(t, provider.requests, 1)
		assert.True(t, provider.requests[0].RequireFluencyAgreement)
		assert.Equal(t, "to go", provider.requests[0].LearnedMeaning)
		assert.Equal(t, "VERB", provider.requests[0].UPOS)
	})
	t.Run("a disagreement is an answer, not an error", func(t *testing.T) {
		provider := &contextualProvider{enabled: true, response: TranslationResponse{Gloss: "to work"}}
		service := NewService(Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 1}, nil, nil, nil, provider, nil)
		agreement, err := service.EnrichFluencyAgreement(context.Background(), candidate, "to go")
		require.NoError(t, err)
		assert.False(t, agreement.Agrees)
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
