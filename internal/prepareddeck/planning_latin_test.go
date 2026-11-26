package prepareddeck

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// cutoverLatinProjection is a valid synthetic Latin projection whose sentence
// passes the imported-source sentence-boundary rule: no capitalised start.
func cutoverLatinProjection() cardexport.CandidateProjection {
	return cardexport.CandidateProjection{
		OwnerID: "alice", DeckName: "Book",
		Candidate: domain.SelectionCandidate{OwnerID: "alice", CorpusID: "corpus", Language: "la", CanonicalLemma: "domus", UPOS: "NOUN", ObservedForms: []byte(`["domum"]`), SentenceReferences: []byte(`[{"sentence_index":0,"location":{"start_offset":12}}]`), FirstEncounter: 10, OccurrenceCount: 3},
		Entry:     cardexport.Entry{OwnerID: "alice", Language: "la", CanonicalLemma: "domus", UPOS: "NOUN", Headword: "domus", SourceDocument: "Book"},
		Sentences: map[int64]analyzer.Sentence{0: {Text: "vir domum magnam videt et cum amicis in forum venit."}},
	}
}

// A Latin deck is the providerless default even when a provider is configured,
// a Greek-payload record names Latin, and the Idiom switch lists Latin: no
// provider request, no cache identity, no Idiom card.
func TestBatchPlannerPlansLatinDeckWithNoProviderRequest(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverLatinProjection()}, deckName: "Book", language: "la"}
	planner := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{})
	planner.IdiomCardLanguages = map[string]bool{"la": true}
	planner.Assessments = []enrichment.ProviderAssessment{
		{Kind: enrichment.EvidenceAssistedPassPayload, Language: "la", Provider: codec.ProviderName(), Recorded: true},
		{Kind: enrichment.IdiomCardPayload, Language: "la", Provider: codec.ProviderName(), Recorded: true},
	}

	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assert.False(t, plan.Config.ExternalTranslationConsent)
	assert.Empty(t, plan.Config.Provider)
	require.Len(t, plan.Projection.Items, 1, "one default card and no Idiom card")
	item := plan.Projection.Items[0]
	assert.Nil(t, item.CacheKey, "a Latin item has no provider cache identity")

	frozen, err := json.Marshal(plan.Projection)
	require.NoError(t, err)
	assert.NotContains(t, string(frozen), "house", "Wiktionary meaning evidence is retired for Latin")
	assert.NotContains(t, string(frozen), "dictionary-v1")

	deck, err := cardexport.NewPresentation(nil).Restore(plan.Projection)
	require.NoError(t, err)
	assert.Empty(t, deck.WorkProjection(), "a Latin deck has no provider work")
}

func TestBatchPlannerPlansLatinDeckWithNoProviderConfigured(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverLatinProjection()}, deckName: "Book", language: "la"}
	planner := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), nil, false, BatchConfig{})

	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assert.Empty(t, plan.Config.Provider)
	require.Len(t, plan.Projection.Items, 1)
}

// With the compiled assessments, which record the Latin pass (#2766), a Latin
// deck with a configured provider plans the pass and sends it.
func TestBatchPlannerPlansLatinDeckWithThePassUnderTheCompiledAssessments(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverLatinProjection()}, deckName: "Book", language: "la"}
	planner := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{})
	planner.Assessments = enrichment.ProviderAssessments()

	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	t.Run("the run is consented and uses the Latin pass contract", func(t *testing.T) {
		assert.True(t, plan.Config.ExternalTranslationConsent)
		assert.Equal(t, codec.LatinEvidenceAssistedPassProviderVersion(), plan.Config.ProviderVersion)
	})
	t.Run("the word is frozen as pass work", func(t *testing.T) {
		deck, err := cardexport.NewPresentation(nil).Restore(plan.Projection)
		require.NoError(t, err)
		work := deck.WorkProjection()
		require.Len(t, work, 1)
		assert.True(t, work[0].Request.RequireEvidenceAssistedPass)
		assert.Equal(t, "la", work[0].Request.Language)
	})
}

// With a test-recorded Latin assessment a Latin deck freezes the pass under the
// Latin contract.
func TestBatchPlannerPlansLatinEvidenceAssistedPassWithRecordedAssessment(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverLatinProjection()}, deckName: "Book", language: "la"}
	planner := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{})
	planner.Assessments = []enrichment.ProviderAssessment{{
		Kind: enrichment.LatinEvidenceAssistedPassPayload, Language: "la", Provider: codec.ProviderName(),
		PromptVersion: codec.LatinEvidenceAssistedPassPromptContract(),
		Fields:        enrichment.LatinEvidenceAssistedPassPayloadFields(),
		Disposition:   "recorded for this test", Ticket: "#test", Recorded: true,
	}}

	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assert.True(t, plan.Config.ExternalTranslationConsent)
	assert.Equal(t, codec.LatinEvidenceAssistedPassProviderVersion(), plan.Config.ProviderVersion)

	deck, err := cardexport.NewPresentation(nil).Restore(plan.Projection)
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	assert.True(t, work[0].Request.RequireEvidenceAssistedPass)
	assert.Equal(t, "la", work[0].Request.Language)
}
