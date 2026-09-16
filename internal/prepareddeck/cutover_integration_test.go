//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"testing"

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

func TestPlannersAssembleEquivalentLocalStandardAndBatchPlans(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()

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
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at) VALUES($1,$2,'de','reserved','NOUN',now())`, owner.ID, preparation.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET studying_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID)
	require.NoError(t, err)

	assembler := NewInputAssembler(store)
	local := NewPreparedDeckPlanner(assembler, cardexport.NewPresentation(nil), nil, false, BatchConfig{}, PreparedDeckConfig{TranslationMode: "standard"})
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "planner-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	standard := NewPreparedDeckPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{MaxRequests: 1}, PreparedDeckConfig{TranslationMode: "standard"})
	batch := NewPreparedDeckPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{MaxRequests: 1}, PreparedDeckConfig{TranslationMode: "batch"})

	localPlan := plannerPlan(t, ctx, store, local, preparation, false)
	standardPlan := plannerPlan(t, ctx, store, standard, preparation, true)
	batchPlan := plannerPlan(t, ctx, store, batch, preparation, true)

	localDigest, err := localPlan.Projection.Digest()
	require.NoError(t, err)
	standardDigest, err := standardPlan.Projection.Digest()
	require.NoError(t, err)
	batchDigest, err := batchPlan.Projection.Digest()
	require.NoError(t, err)
	assert.Equal(t, standardDigest, batchDigest)
	assert.NotEqual(t, localDigest, standardDigest, "external provider identity is part of the frozen plan")
	assert.Equal(t, string(domain.PreparedDeckExecutionStandard), standardPlan.Config.ExecutionMode)
	assert.Equal(t, string(domain.PreparedDeckExecutionBatch), batchPlan.Config.ExecutionMode)
	require.Len(t, localPlan.Projection.Items, 2)
	require.Len(t, standardPlan.Projection.Items, 2)
	assert.Nil(t, localPlan.Projection.Items[0].CacheKey)
	require.NotNil(t, standardPlan.Projection.Items[0].CacheKey)
	assert.Equal(t, "dictionary-v1", standardPlan.Projection.Items[0].Entry.DictionaryProviderVersion)
	assert.Equal(t, int64(0), standardPlan.Projection.Items[0].SentenceOrdinal)
	assert.Equal(t, enrichment.SentenceHash(standardPlan.Projection.Items[0].Entry.Sentence), standardPlan.Projection.Items[0].CacheKey.SentenceHash)
	assert.Equal(t, standardPlan.Config.Provider, standardPlan.Projection.Items[0].CacheKey.Provider)
	assert.Equal(t, standardPlan.Config.ProviderVersion, standardPlan.Projection.Items[0].CacheKey.ProviderVersion)
	assert.Empty(t, standardPlan.Chunks)
	assert.Len(t, batchPlan.Chunks, 2)
	assert.Equal(t, []int{0}, batchPlan.Chunks[0].Ordinals)
	assert.Equal(t, []int{1}, batchPlan.Chunks[1].Ordinals)

	counted := &countingPlanner{planner: local}
	workers := river.NewWorkers()
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	coordinator := NewDurableCoordinator(store, client, counted)
	frozen, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	require.True(t, frozen.NeedsFinalizer)
	repeated, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	assert.True(t, repeated.Existing)
	assert.Equal(t, 1, counted.calls, "existing-run freeze must not replan")
	var finalizerJobs int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind=$1 AND args->>'preparation_id'=$2`, (FinalizeJobArgs{}).Kind(), preparation.ID).Scan(&finalizerJobs))
	assert.Equal(t, 1, finalizerJobs)
}

type countingPlanner struct {
	planner DurableRunPlanner
	calls   int
}

func (p *countingPlanner) PlanPreparedDeckRun(ctx context.Context, tx pgx.Tx, preparation domain.DeckPreparation, consent bool) (persistence.FreezePreparedDeckRunParams, error) {
	p.calls++
	return p.planner.PlanPreparedDeckRun(ctx, tx, preparation, consent)
}

func plannerPlan(t *testing.T, ctx context.Context, store *persistence.PostgresStore, planner DurableRunPlanner, preparation domain.DeckPreparation, consent bool) persistence.FreezePreparedDeckRunParams {
	t.Helper()
	tx, err := store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	plan, err := planner.PlanPreparedDeckRun(ctx, tx, preparation, consent)
	require.NoError(t, err)
	return plan
}
