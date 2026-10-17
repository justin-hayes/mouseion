package enrichment

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAncientGreekProviderAssessmentIsRecorded(t *testing.T) {
	assessment := AncientGreekProviderAssessment
	assert.True(t, assessment.Recorded, "the reviewed personal-use disposition is recorded on #2110")
	assert.Equal(t, "grc", assessment.Language)
	assert.Equal(t, "openai-compatible", assessment.Provider)
	assert.Equal(t, contextualGlossPromptVersion, assessment.PromptVersion)
	assert.Equal(t, "#2110", assessment.Ticket)
	assert.Equal(t, ContextualGlossPayloadFields(), assessment.Fields, "the recorded field set is the codec's contextual Gloss contract")
}

// The recorded field set must be exactly what a contextual Gloss request
// transmits, so a field change is caught here.
func TestRecordedFieldSetMatchesTheContextualGlossRequest(t *testing.T) {
	codec, err := NewTranslationCodec(LLMConfig{Model: "greek-assessment-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	body, err := codec.EncodeRequest(TranslationRequest{
		Language: "grc", TargetLanguage: "en", CanonicalLemma: "λέξις", UPOS: "NOUN", TargetWord: "λέξις",
		ExampleSentence: "ἡ λέξις αὕτη μακρά ἐστιν.", RequireContextualGloss: true,
		CandidateSenses: []LexicalSense{{EvidenceID: "wikt:lexis", Gloss: "word"}},
	})
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &request))
	require.Len(t, request.Messages, 2)
	var user map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(request.Messages[1].Content), &user))
	keys := make([]string, 0, len(user))
	for _, field := range ContextualGlossPayloadFields() {
		_, ok := user[field]
		assert.True(t, ok, "contextual Gloss request is missing recorded field %q", field)
		keys = append(keys, field)
	}
	assert.Len(t, user, len(keys), "contextual Gloss request carries an unrecorded field: %v", user)
}

func TestProviderAssessmentGateAllowsOtherLanguages(t *testing.T) {
	require.NoError(t, AllowExternalProviderCall("de", "anything", "anything", []string{"anything"}))
	require.NoError(t, AllowExternalProviderCall("el", "anything", "anything", []string{"anything"}))
}

func TestProviderAssessmentGateAllowsTheRecordedContract(t *testing.T) {
	require.NoError(t, AllowExternalProviderCall(AncientGreekProviderAssessment.Language, AncientGreekProviderAssessment.Provider, AncientGreekProviderAssessment.PromptVersion, ContextualGlossPayloadFields()))
}

func TestProviderAssessmentGateFailsClosedWhenUnrecorded(t *testing.T) {
	unrecorded := AncientGreekProviderAssessment
	unrecorded.Recorded = false
	err := unrecorded.Gate("grc", AncientGreekProviderAssessment.Provider, AncientGreekProviderAssessment.PromptVersion, ContextualGlossPayloadFields())
	require.ErrorIs(t, err, ErrProviderAssessmentNotRecorded)
	assert.Contains(t, err.Error(), "#2110")
}

func TestProviderAssessmentGateFailsClosedOnContractDrift(t *testing.T) {
	assessment := AncientGreekProviderAssessment
	for _, tc := range []struct {
		name, provider, prompt string
		fields                 []string
	}{
		{name: "provider", provider: "self-hosted", prompt: assessment.PromptVersion, fields: ContextualGlossPayloadFields()},
		{name: "prompt", provider: assessment.Provider, prompt: "translation-v15", fields: ContextualGlossPayloadFields()},
		{name: "fields", provider: assessment.Provider, prompt: assessment.PromptVersion, fields: append(ContextualGlossPayloadFields(), "learner_id")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := assessment.Gate("grc", tc.provider, tc.prompt, tc.fields)
			assert.ErrorIs(t, err, ErrProviderAssessmentNotRecorded)
		})
	}
}

func TestAllowExternalProviderCallUsesTheRecordedAssessment(t *testing.T) {
	err := AllowExternalProviderCall("grc", AncientGreekProviderAssessment.Provider, AncientGreekProviderAssessment.PromptVersion, ContextualGlossPayloadFields())
	require.NotErrorIs(t, err, ErrProviderAssessmentNotRecorded)

	unrecorded := AncientGreekProviderAssessment
	unrecorded.Recorded = false
	assert.ErrorIs(t, unrecorded.Gate("grc", unrecorded.Provider, unrecorded.PromptVersion, ContextualGlossPayloadFields()), ErrProviderAssessmentNotRecorded)
}
