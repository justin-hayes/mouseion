//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlannersAssembleEquivalentStandardAndBatchPlans(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	owner, err := store.CreateUser(ctx, "planner-equivalence", false)
	require.NoError(t, err)
	err = store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: "planner-hash", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}, nil)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "planner-book", Title: "Planner Book", MediaType: "text/plain", ContentHash: "planner-hash", Content: []byte("Das alte Haus steht heute im Garten."), FullText: "Das alte Haus steht heute im Garten."})
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, source.ContentHash)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash})
	require.NoError(t, err)
	addCandidate := func(lemma string, occurrences int) {
		t.Helper()
		text := "Heute sieht die Person " + lemma + " im großen Garten."
		refs, marshalErr := json.Marshal([]map[string]any{{"sentence_index": 0, "text": text, "location": map[string]any{"start_offset": 0}}})
		require.NoError(t, marshalErr)
		_, putErr := store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", OccurrenceCount: occurrences, ObservedForms: []byte(`["` + lemma + `"]`), SentenceReferences: refs, Provenance: []byte(`{"occurrence_count":3}`)})
		require.NoError(t, putErr)
	}
	addCandidate("haus", 3)
	addCandidate("rare", 2)
	addCandidate("known", 3)
	addCandidate("generated-other", 3)
	addCandidate("generated-same", 3)
	addCandidate("reserved", 3)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "known", "NOUN")
	require.NoError(t, err)
	otherSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "planner-other", Title: "Other", MediaType: "text/plain", ContentHash: "planner-hash-other", Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	otherDeck, err := store.PutDeck(ctx, owner.ID, "de", "Other")
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: owner.ID, Language: "de", CanonicalLemma: "generated-other", UPOS: "NOUN", FirstDeckID: otherDeck.ID, FirstSourceMaterialID: &otherSource.ID})
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: owner.ID, Language: "de", CanonicalLemma: "generated-same", UPOS: "NOUN", FirstDeckID: otherDeck.ID, FirstSourceMaterialID: &source.ID})
	require.NoError(t, err)
	_, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: owner.ID, Language: "de", CanonicalLemma: "reserved", UPOS: "NOUN", FirstDeckID: otherDeck.ID, FirstSourceMaterialID: &source.ID})
	require.NoError(t, err)
	readProvenance := func(lemma string) [3]string {
		t.Helper()
		var provenance [3]string
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT first_deck_id::text, first_source_material_id::text, first_generated_at::text FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma=$2`, owner.ID, lemma).Scan(&provenance[0], &provenance[1], &provenance[2]))
		return provenance
	}
	generatedOtherBefore := readProvenance("generated-other")
	generatedSameBefore := readProvenance("generated-same")

	assembler := NewInputAssembler(store)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "planner-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	standard := NewPreparedDeckPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{MaxRequests: 1}, PreparedDeckConfig{TranslationMode: "standard"})
	batch := NewPreparedDeckPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{MaxRequests: 1}, PreparedDeckConfig{TranslationMode: "batch"})

	standardPlan := plannerPlan(t, ctx, store, standard, preparation)
	batchPlan := plannerPlan(t, ctx, store, batch, preparation)

	standardDigest, err := standardPlan.Projection.Digest()
	require.NoError(t, err)
	batchDigest, err := batchPlan.Projection.Digest()
	require.NoError(t, err)
	assert.Equal(t, standardDigest, batchDigest, "standard and Batch must freeze the same contextual request contract")
	assert.Equal(t, string(domain.PreparedDeckExecutionStandard), standardPlan.Config.ExecutionMode)
	assert.Equal(t, string(domain.PreparedDeckExecutionBatch), batchPlan.Config.ExecutionMode)
	require.Len(t, standardPlan.Projection.Items, 4)
	require.NotNil(t, standardPlan.Projection.Items[0].CacheKey)
	assert.Equal(t, "dictionary-v1", standardPlan.Projection.Items[0].Entry.DictionaryProviderVersion)
	assert.Equal(t, int64(0), standardPlan.Projection.Items[0].SentenceOrdinal)
	assert.Equal(t, enrichment.SentenceHash(standardPlan.Projection.Items[0].Entry.Sentence), standardPlan.Projection.Items[0].CacheKey.SentenceHash)
	assert.Equal(t, standardPlan.Config.Provider, standardPlan.Projection.Items[0].CacheKey.Provider)
	assert.Equal(t, standardPlan.Config.ProviderVersion, standardPlan.Projection.Items[0].CacheKey.ProviderVersion)
	assert.NotEmpty(t, standardPlan.Projection.Items[0].CacheKey.MeaningEvidenceHash)
	assert.Equal(t, standardPlan.Projection.Items[0].CacheKey.MeaningEvidenceHash, batchPlan.Projection.Items[0].CacheKey.MeaningEvidenceHash)
	assert.Equal(t, standardPlan.Config.ProviderVersion, batchPlan.Config.ProviderVersion)
	assert.Empty(t, standardPlan.Chunks)
	assert.Len(t, batchPlan.Chunks, 4)
	assert.Equal(t, []int{0}, batchPlan.Chunks[0].Ordinals)
	assert.Equal(t, []int{1}, batchPlan.Chunks[1].Ordinals)
	assert.Equal(t, []int{2}, batchPlan.Chunks[2].Ordinals)
	assert.Equal(t, []int{3}, batchPlan.Chunks[3].Ordinals)
	assert.Equal(t, generatedOtherBefore, readProvenance("generated-other"), "historical provenance changed")
	assert.Equal(t, generatedSameBefore, readProvenance("generated-same"), "historical provenance changed")

	counted := &countingPlanner{planner: standard}
	workers := river.NewWorkers()
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, store, client, nil, PreparedDeckConfig{}, time.Second)
	coordinator := NewDurableCoordinator(store, client, counted)
	frozen, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	assert.False(t, frozen.NeedsFinalizer, "contextual translation jobs must complete before finalization")
	repeated, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	assert.True(t, repeated.Existing)
	assert.Equal(t, 1, counted.calls, "existing-run freeze must not replan")
	var translationJobs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2`, (StandardTranslationJobArgs{}).Kind(), preparation.ID).Scan(&translationJobs))
	assert.Equal(t, 4, translationJobs, "every selected item must have contextual translation work")
}

type countingPlanner struct {
	planner DurableRunPlanner
	calls   int
}

func (p *countingPlanner) PlanPreparedDeckRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation) (persistence.FreezePreparedDeckRunParams, error) {
	p.calls++
	return p.planner.PlanPreparedDeckRun(ctx, tx, preparation)
}

func plannerPlan(t *testing.T, ctx context.Context, store *persistence.PostgresStore, planner DurableRunPlanner, preparation domain.DeckPreparation) persistence.FreezePreparedDeckRunParams {
	t.Helper()
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	testutil.Cleanup(t, "transaction", func() error { return tx.Rollback(ctx) })
	plan, err := planner.PlanPreparedDeckRun(ctx, tx, preparation)
	require.NoError(t, err)
	return plan
}
