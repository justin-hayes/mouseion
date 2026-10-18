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

// TestEvidenceAssistedPassAssessmentIsRecorded proves the #2171 disposition is
// recorded and matches the current codec, so the pass runs and its call is
// allowed.
func TestEvidenceAssistedPassAssessmentIsRecorded(t *testing.T) {
	assessments := ProviderAssessments()
	assessment := assessmentByTicket(t, "#2171")
	assert.True(t, assessment.Recorded, "Justin recorded the #2171 assessment")
	assert.Equal(t, "approved for personal use of the locally produced deck", assessment.Disposition)
	assert.True(t, EvidenceAssistedPassOffered(assessments))

	codec := passCodec(t)
	assert.Equal(t, codec.EvidenceAssistedPassPromptContract(), assessment.PromptVersion, "the recorded prompt contract is the one the codec sends; a new prompt needs a new record")
	require.NoError(t, AllowExternalProviderCall(assessments, "grc", codec.ProviderName(), codec.EvidenceAssistedPassPromptContract(), EvidenceAssistedPassPayloadFields()))
}

// TestLSJSenseSummaryAssessmentIsRecorded proves the #2172 disposition is
// recorded and matches the current codec, so the pass requests a context-free
// summary of each pinned LSJ entry.
func TestLSJSenseSummaryAssessmentIsRecorded(t *testing.T) {
	assessments := ProviderAssessments()
	assessment := assessmentByTicket(t, "#2172")
	assert.True(t, assessment.Recorded, "Justin recorded the #2172 assessment")
	assert.Equal(t, "approved for personal use of the locally produced deck", assessment.Disposition)
	assert.True(t, LSJSenseSummaryOffered(assessments))

	codec := passCodec(t)
	assert.Equal(t, codec.LSJSenseSummaryPromptContract(), assessment.PromptVersion, "the recorded prompt contract is the one the codec sends; a new prompt needs a new record")
	assert.Equal(t, LSJSenseSummaryPayloadFields(), assessment.Fields, "the recorded field set is the one the codec sends; a changed payload needs a new record")
	require.NoError(t, AllowExternalProviderCall(assessments, "grc", codec.ProviderName(), codec.LSJSenseSummaryPromptContract(), LSJSenseSummaryPayloadFields()))
}

// TestLSJSenseSummaryUnrecordedAssessmentFailsClosed proves an unrecorded
// summary assessment withholds the summary.
func TestLSJSenseSummaryUnrecordedAssessmentFailsClosed(t *testing.T) {
	unrecorded := ProviderAssessments()
	for i := range unrecorded {
		if unrecorded[i].Ticket == "#2172" {
			unrecorded[i].Recorded = false
		}
	}
	assert.False(t, LSJSenseSummaryOffered(unrecorded))

	codec := passCodec(t)
	require.ErrorIs(t, AllowExternalProviderCall(unrecorded, "grc", codec.ProviderName(), codec.LSJSenseSummaryPromptContract(), LSJSenseSummaryPayloadFields()), ErrProviderAssessmentNotRecorded)
}

// assessmentByTicket returns the compiled assessment recorded for one ticket.
func assessmentByTicket(t *testing.T, ticket string) ProviderAssessment {
	t.Helper()
	for _, assessment := range ProviderAssessments() {
		if assessment.Ticket == ticket {
			return assessment
		}
	}
	t.Fatalf("no provider assessment recorded for %s", ticket)
	return ProviderAssessment{}
}

// TestEvidenceAssistedPassUnrecordedAssessmentFailsClosed proves an unrecorded
// assessment allows no pass call.
func TestEvidenceAssistedPassUnrecordedAssessmentFailsClosed(t *testing.T) {
	unrecorded := ProviderAssessments()
	unrecorded[0].Recorded = false
	assert.False(t, EvidenceAssistedPassOffered(unrecorded))
	codec := passCodec(t)
	err := AllowExternalProviderCall(unrecorded, "grc", codec.ProviderName(), codec.EvidenceAssistedPassPromptContract(), EvidenceAssistedPassPayloadFields())
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
