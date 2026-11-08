package prepareddeck

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnabledIdiomLanguagesFollowsTheSwitch(t *testing.T) {
	planner := &BatchPlanner{IdiomCardLanguages: map[string]bool{"it": true}}
	projections := []cardexport.CandidateProjection{
		{Candidate: domain.SelectionCandidate{Language: "it"}},
		{Candidate: domain.SelectionCandidate{Language: "IT"}},
		{Candidate: domain.SelectionCandidate{Language: "de"}},
	}
	assert.Equal(t, []string{"it"}, planner.enabledIdiomLanguages(projections))

	planner.IdiomCardLanguages = map[string]bool{"de": true}
	assert.Empty(t, planner.enabledIdiomLanguages(projections[:1]), "a language the deck does not use stays off")
	assert.Empty(t, (&BatchPlanner{}).enabledIdiomLanguages(projections), "an unset switch enables nothing")
}

func TestIdiomAssessmentBlocksACoveredLanguageBeforeFreeze(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test"})
	require.NoError(t, err)
	planner := &BatchPlanner{Codec: codec}
	require.ErrorIs(t, planner.validateIdiomAssessment([]string{"it"}), enrichment.ErrProviderAssessmentNotRecorded, "the compiled Italian assessment awaits its review report")

	recorded := enrichment.ProviderAssessment{
		Language: "it", Provider: codec.ProviderName(), PromptVersion: codec.IdiomCardPromptContract(),
		Fields: enrichment.IdiomCardPayloadFields(), Disposition: "recorded for this test", Ticket: "#2362", Recorded: true,
	}
	planner.Assessments = []enrichment.ProviderAssessment{recorded}
	require.NoError(t, planner.validateIdiomAssessment([]string{"it"}))
	require.ErrorIs(t, planner.validateIdiomAssessment([]string{"de"}), enrichment.ErrProviderAssessmentNotRecorded, "an unreviewed language is not enabled by the switch alone")

	drifted := recorded
	drifted.Fields = append(drifted.Fields, "book_title")
	planner.Assessments = []enrichment.ProviderAssessment{drifted}
	require.ErrorIs(t, planner.validateIdiomAssessment([]string{"it"}), enrichment.ErrProviderAssessmentNotRecorded)
}
