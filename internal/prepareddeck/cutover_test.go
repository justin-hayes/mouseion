package prepareddeck

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
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
			if err != nil {
				t.Fatal(err)
			}
			if plan.Config.ExternalTranslationConfigured || plan.Config.Provider != test.wantProvider || len(plan.Chunks) != 0 {
				t.Fatalf("provider work was planned: config=%+v chunks=%+v", plan.Config, plan.Chunks)
			}
			if plan.Manifest.Items[0].CacheKey != nil {
				t.Fatal("disabled/no-consent manifest has a cache identity")
			}
		})
	}
}

func TestBatchPlannerBuildsExactEligibleBatchContract(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	if err != nil {
		t.Fatal(err)
	}
	builder := &cutoverBuilder{manifest: cardexport.NewManifest("alice", "Book", []cardexport.Entry{{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", Sentence: "Das alte Haus ist überraschend groß.", TargetWord: "Haus", FirstEncounter: 10}})}
	plan, err := NewBatchPlanner(builder, codec, true, BatchConfig{MaxRequests: 1}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Config.ExternalTranslationConfigured || plan.Config.Endpoint != enrichment.OpenAIChatCompletionsEndpoint || plan.Config.Model != codec.Model() || plan.Config.Provider != codec.ProviderName() || plan.Config.ProviderVersion != codec.ProviderVersion() {
		t.Fatalf("config=%+v", plan.Config)
	}
	if len(plan.Chunks) != 1 || len(plan.Chunks[0].Ordinals) != 1 || plan.Chunks[0].Generation != 1 || plan.Chunks[0].SplitReason != "run" {
		t.Fatalf("chunks=%+v manifest=%+v", plan.Chunks, plan.Manifest.Items)
	}
	if plan.Manifest.Items[0].CacheKey == nil || plan.Manifest.Items[0].CacheKey.SentenceHash != enrichment.SentenceHash("Das alte Haus ist überraschend groß.") {
		t.Fatalf("manifest=%+v", plan.Manifest.Items[0])
	}
	if plan.RunID == "" || plan.Config.BatchMaxRequests != 1 || plan.Config.BatchMaxBytes != persistence.DefaultBatchMaxBytes {
		t.Fatalf("plan=%+v", plan)
	}
}
