package enrichment

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func passCodec(t *testing.T) *TranslationCodec {
	t.Helper()
	codec, err := NewTranslationCodec(LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	return codec
}

// TestEvidenceAssistedPassAssessmentShipsClosed proves the #2171 disposition is
// unrecorded, so the pass is not offered and no pass call is transmitted.
func TestEvidenceAssistedPassAssessmentShipsClosed(t *testing.T) {
	assessments := ProviderAssessments()
	require.Len(t, assessments, 1)
	assessment := assessments[0]
	assert.False(t, assessment.Recorded, "the #2171 assessment ships unrecorded until Justin records it")
	assert.Equal(t, "#2171", assessment.Ticket)
	assert.False(t, EvidenceAssistedPassOffered(assessments), "the pass is not offered while its assessment is unrecorded")

	codec := passCodec(t)
	err := AllowExternalProviderCall(assessments, "grc", codec.ProviderName(), codec.EvidenceAssistedPassPromptContract(), EvidenceAssistedPassPayloadFields())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrProviderAssessmentNotRecorded)
}

// TestEvidenceAssistedPassGateRequiresExactPayload proves a recorded assessment
// pins the provider, prompt contract and exact transmitted field set.
func TestEvidenceAssistedPassGateRequiresExactPayload(t *testing.T) {
	codec := passCodec(t)
	recorded := ProviderAssessment{
		Language: "grc", Provider: codec.ProviderName(),
		PromptVersion: codec.EvidenceAssistedPassPromptContract(),
		Fields:        EvidenceAssistedPassPayloadFields(),
		Disposition:   "recorded for this test", Ticket: "#2171", Recorded: true,
	}
	assessments := []ProviderAssessment{recorded}
	require.NoError(t, AllowExternalProviderCall(assessments, "grc", codec.ProviderName(), codec.EvidenceAssistedPassPromptContract(), EvidenceAssistedPassPayloadFields()))
	assert.True(t, EvidenceAssistedPassOffered(assessments))

	drifted := append(EvidenceAssistedPassPayloadFields(), "extra")
	require.ErrorIs(t, AllowExternalProviderCall(assessments, "grc", codec.ProviderName(), codec.EvidenceAssistedPassPromptContract(), drifted), ErrProviderAssessmentNotRecorded)
	require.ErrorIs(t, AllowExternalProviderCall(assessments, "grc", "other-provider", codec.EvidenceAssistedPassPromptContract(), EvidenceAssistedPassPayloadFields()), ErrProviderAssessmentNotRecorded)
	require.ErrorIs(t, AllowExternalProviderCall(assessments, "grc", codec.ProviderName(), "other-prompt", EvidenceAssistedPassPayloadFields()), ErrProviderAssessmentNotRecorded)
	assert.False(t, EvidenceAssistedPassOffered(nil), "no recorded assessment means the pass is not offered")
}

// TestProviderAssessmentLeavesOtherLanguagesUnaffected proves a language the
// installation records no assessment for is unaffected.
func TestProviderAssessmentLeavesOtherLanguagesUnaffected(t *testing.T) {
	codec := passCodec(t)
	assert.NoError(t, AllowExternalProviderCall(ProviderAssessments(), "de", codec.ProviderName(), codec.EvidenceAssistedPassPromptContract(), EvidenceAssistedPassPayloadFields()))
}
