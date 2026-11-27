package prepareddeck

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

func idiomAssessmentFor(codec *enrichment.TranslationCodec, language string) enrichment.ProviderAssessmentSet {
	return enrichment.ProviderAssessmentSet{{
		Kind: enrichment.IdiomCardPayload, Language: language, Provider: codec.ProviderName(),
		PromptVersion: codec.IdiomCardPromptContract(), Fields: enrichment.IdiomCardPayloadFields(),
		Disposition: "recorded for this test", Ticket: "#2751", Recorded: true,
	}}
}

func TestIdiomSwitchNeedsTheOperatorSwitchAndARecordedAssessment(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	recorded := idiomAssessmentFor(codec, "de")

	require.True(t, IdiomSwitch(map[string]bool{"de": true}, recorded, codec)("de"))
	require.True(t, IdiomSwitch(map[string]bool{"de": true}, recorded, codec)("DE"), "the language is normalized")
	require.False(t, IdiomSwitch(map[string]bool{}, recorded, codec)("de"), "the operator switch is off")
	require.False(t, IdiomSwitch(map[string]bool{"de": true}, idiomAssessmentFor(codec, "it"), codec)("de"), "no assessment is recorded for German")
	require.False(t, IdiomSwitch(map[string]bool{"de": true}, recorded, nil)("de"), "no codec sends nothing")
	require.False(t, IdiomSwitch(map[string]bool{"de": true}, nil, codec)("de"), "the production set records Italian only")
	require.True(t, IdiomSwitch(map[string]bool{"it": true}, nil, codec)("it"), "the production set records Italian")
}
