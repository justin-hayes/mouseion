package enrichment

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAncientGreekProviderAssessmentAwaitsTheShortHandleReview(t *testing.T) {
	assessment := AncientGreekProviderAssessment
	assert.False(t, assessment.Recorded, "the short-handle payload awaits Justin's review on #2125")
	assert.Equal(t, "grc", assessment.Language)
	assert.Equal(t, "openai-compatible", assessment.Provider)
	assert.Equal(t, contextualGlossPromptVersion, assessment.PromptVersion)
	assert.Equal(t, "#2125", assessment.Ticket)
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

// TestProviderAssessmentGateRejectsUntilTheNewContractIsRecorded covers the
// #2125 human checkpoint: Greek preparation fails closed while the recorded
// assessment still describes the superseded contract.
func TestProviderAssessmentGateRejectsUntilTheNewContractIsRecorded(t *testing.T) {
	err := AllowExternalProviderCall("grc", AncientGreekProviderAssessment.Provider, contextualGlossPromptVersion, ContextualGlossPayloadFields())
	require.ErrorIs(t, err, ErrProviderAssessmentNotRecorded)
	assert.Contains(t, err.Error(), "#2125")

	// The assessment that Justin will record for the short-handle contract
	// admits the new contract.
	rerecorded := AncientGreekProviderAssessment
	rerecorded.Recorded = true
	require.NoError(t, rerecorded.Gate("grc", rerecorded.Provider, contextualGlossPromptVersion, ContextualGlossPayloadFields()))

	// Even once recorded, the superseded prompt contract stays rejected.
	err = rerecorded.Gate("grc", rerecorded.Provider, "translation-v14-structured-target-alignment", ContextualGlossPayloadFields())
	require.ErrorIs(t, err, ErrProviderAssessmentNotRecorded)
}

func TestProviderAssessmentGateFailsClosedWhenUnrecorded(t *testing.T) {
	unrecorded := AncientGreekProviderAssessment
	unrecorded.Recorded = false
	err := unrecorded.Gate("grc", unrecorded.Provider, unrecorded.PromptVersion, ContextualGlossPayloadFields())
	require.ErrorIs(t, err, ErrProviderAssessmentNotRecorded)
	assert.Contains(t, err.Error(), "#2125")
}

func TestProviderAssessmentGateFailsClosedOnContractDrift(t *testing.T) {
	recorded := AncientGreekProviderAssessment
	recorded.Recorded = true
	for _, tc := range []struct {
		name, provider, prompt string
		fields                 []string
	}{
		{name: "provider", provider: "self-hosted", prompt: recorded.PromptVersion, fields: ContextualGlossPayloadFields()},
		{name: "prompt", provider: recorded.Provider, prompt: "translation-v16", fields: ContextualGlossPayloadFields()},
		{name: "fields", provider: recorded.Provider, prompt: recorded.PromptVersion, fields: append(ContextualGlossPayloadFields(), "learner_id")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := recorded.Gate("grc", tc.provider, tc.prompt, tc.fields)
			assert.ErrorIs(t, err, ErrProviderAssessmentNotRecorded)
		})
	}
}

func TestAllowExternalProviderCallUsesTheRecordedAssessment(t *testing.T) {
	recorded := AncientGreekProviderAssessment
	recorded.Recorded = true
	require.NoError(t, recorded.Gate("grc", recorded.Provider, recorded.PromptVersion, recorded.Fields))

	unrecorded := recorded
	unrecorded.Recorded = false
	assert.ErrorIs(t, unrecorded.Gate("grc", unrecorded.Provider, unrecorded.PromptVersion, ContextualGlossPayloadFields()), ErrProviderAssessmentNotRecorded)
}
