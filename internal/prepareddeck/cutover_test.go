package prepareddeck

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cutoverBuilder struct{ manifest cardexport.Manifest }

func (b *cutoverBuilder) PrepareCoverage(context.Context, string, string) (cardexport.Manifest, error) {
	return b.manifest, nil
}

func (b *cutoverBuilder) RenderManifest(context.Context, cardexport.Manifest, []cardexport.ExactEnrichment) (cardexport.Artifact, error) {
	return cardexport.Artifact{}, nil
}

func TestBatchPlannerKeepsDisabledAndNoConsentRunsProviderFree(t *testing.T) {
	builder := &cutoverBuilder{manifest: cardexport.NewManifest("alice", "Book", []cardexport.Entry{{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", FirstEncounter: 10}})}
	cases := []struct {
		name, wantProvider string
		enabled, consent   bool
	}{
		{name: "disabled", enabled: false, consent: true},
		{name: "no consent", enabled: true, consent: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			plan, err := NewBatchPlanner(builder, nil, test.enabled, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, test.consent)
			require.NoError(t, err)
			assert.False(t, plan.Config.ExternalTranslationConfigured)
			assert.Equal(t, test.wantProvider, plan.Config.Provider)
			assert.Len(t, plan.Chunks, 0)
			assert.Nil(t, plan.Projection.Items[0].CacheKey, "disabled/no-consent projection has a cache identity")
		})
	}
}

func TestBatchPlannerBuildsExactEligibleBatchContract(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	builder := &cutoverBuilder{manifest: cardexport.NewManifest("alice", "Book", []cardexport.Entry{{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", FirstEncounter: 10}})}
	plan, err := NewBatchPlanner(builder, codec, true, BatchConfig{MaxRequests: 1}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, true)
	require.NoError(t, err)
	assert.True(t, plan.Config.ExternalTranslationConfigured)
	assert.Equal(t, enrichment.OpenAIChatCompletionsEndpoint, plan.Config.Endpoint)
	assert.Equal(t, codec.Model(), plan.Config.Model)
	assert.Equal(t, codec.ProviderName(), plan.Config.Provider)
	assert.Equal(t, codec.ProviderVersion(), plan.Config.ProviderVersion)
	require.Len(t, plan.Chunks, 1)
	require.Len(t, plan.Chunks[0].Ordinals, 1)
	assert.Equal(t, 1, plan.Chunks[0].Generation)
	assert.Equal(t, "run", plan.Chunks[0].SplitReason)
	require.NotNil(t, plan.Projection.Items[0].CacheKey)
	assert.Equal(t, enrichment.SentenceHash("Das alte Haus ist überraschend groß."), plan.Projection.Items[0].CacheKey.SentenceHash)
	assert.NotEmpty(t, plan.RunID)
	assert.Equal(t, 1, plan.Config.BatchMaxRequests)
	assert.Equal(t, persistence.DefaultBatchMaxBytes, plan.Config.BatchMaxBytes)
}

func TestPreparedDeckPlannerDefaultsToStandardWithoutBatchChunks(t *testing.T) {
	builder := &cutoverBuilder{manifest: cardexport.NewManifest("alice", "Book", []cardexport.Entry{{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", FirstEncounter: 10}})}
	planner := NewPreparedDeckPlanner(builder, nil, false, BatchConfig{}, PreparedDeckConfig{TranslationMode: DefaultTranslationMode})
	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, true)
	require.NoError(t, err)
	assert.Equal(t, string(domain.PreparedDeckExecutionStandard), plan.Config.ExecutionMode)
	assert.Len(t, plan.Chunks, 0)
	assert.False(t, plan.Config.ExternalTranslationConfigured)
}
