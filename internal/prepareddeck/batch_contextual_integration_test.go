//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/google/uuid"
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

type contextualBatchFixtureProvider struct {
	input    []byte
	output   []byte
	batch    enrichment.Batch
	metadata map[string]string
}

type contextualStandardFixtureProvider struct {
	name, version string
}

func (p contextualStandardFixtureProvider) Name() string    { return p.name }
func (p contextualStandardFixtureProvider) Version() string { return p.version }
func (p contextualStandardFixtureProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	if request.CanonicalLemma == "unresolved" {
		return enrichment.TranslationResponse{Translation: "translated", SentenceTranslation: "The translated sentence.", ContextOnly: true, UnresolvedReason: "The sentence does not distinguish the meanings."}, nil
	}
	return enrichment.TranslationResponse{Translation: "translated", Gloss: "contextual meaning", EvidenceIDs: []string{"wikt:context"}, SentenceTranslation: "The translated sentence.", SentenceTranslationTarget: "translated"}, nil
}

func (p *contextualBatchFixtureProvider) UploadFile(_ context.Context, _ string, content io.Reader) (enrichment.OpenAIFile, error) {
	var err error
	p.input, err = io.ReadAll(content)
	return enrichment.OpenAIFile{ID: "contextual-input", Bytes: int64(len(p.input))}, err
}

func (p *contextualBatchFixtureProvider) CreateBatch(_ context.Context, request enrichment.CreateBatchRequest) (enrichment.Batch, error) {
	p.metadata = request.Metadata
	var output strings.Builder
	count := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(p.input)), "\n") {
		if line == "" {
			continue
		}
		var input struct {
			CustomID string `json:"custom_id"`
		}
		if err := json.Unmarshal([]byte(line), &input); err != nil {
			return enrichment.Batch{}, err
		}
		identity, err := enrichment.ParseBatchCustomID(input.CustomID)
		if err != nil {
			return enrichment.Batch{}, err
		}
		response := map[string]any{
			"item_id": input.CustomID, "source_language": "de", "target_language": "en",
			"translation": "translated", "sentence_translation": "The translated sentence.",
			"sentence_translation_target": "translated", "evidence_ids": []string{}, "context_only": true,
		}
		if identity.Ordinal == 1 {
			response["gloss"] = ""
			response["unresolved_reason"] = "The sentence does not distinguish the meanings."
		} else {
			response["gloss"] = "contextual meaning"
			response["unresolved_reason"] = ""
		}
		content, err := json.Marshal(response)
		if err != nil {
			return enrichment.Batch{}, err
		}
		lineBytes, err := json.Marshal(map[string]any{"custom_id": input.CustomID, "response": map[string]any{"status_code": 200, "body": map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(content)}}}}}})
		if err != nil {
			return enrichment.Batch{}, err
		}
		output.Write(lineBytes)
		output.WriteByte('\n')
		count++
	}
	p.output = []byte(output.String())
	p.batch = enrichment.Batch{ID: "contextual-batch", InputFileID: request.InputFileID, OutputFileID: "contextual-output", Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CompletionWindow: "24h", Status: enrichment.BatchStatusCompleted, RequestCounts: enrichment.BatchRequestCounts{Total: count, Completed: count}, Metadata: request.Metadata}
	return p.batch, nil
}

func (p *contextualBatchFixtureProvider) ListBatches(context.Context, enrichment.ListBatchesRequest) (enrichment.BatchList, error) {
	return enrichment.BatchList{}, nil
}

func (p *contextualBatchFixtureProvider) GetBatch(context.Context, string) (enrichment.Batch, error) {
	return p.batch, nil
}

func (p *contextualBatchFixtureProvider) FileContent(_ context.Context, id string, dst io.Writer) error {
	if id == "contextual-output" {
		_, err := dst.Write(p.output)
		return err
	}
	return nil
}

func TestDurableBatchPublishesContextualCardAndReportsUnresolvedOmission(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "batch-contextual-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Contextual Batch", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash})
	require.NoError(t, err)
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	entries := []cardexport.Entry{
		{Language: "de", CanonicalLemma: "resolved", UPOS: "NOUN", Sentence: "Der erste Kontext steht heute im Buch.", TargetWord: "Kontext", SourceDocument: source.Title, FirstEncounter: 1, CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "wikt:context", Gloss: "context"}}},
		{Language: "de", CanonicalLemma: "unresolved", UPOS: "NOUN", Sentence: "Der zweite Kontext bleibt heute unklar.", TargetWord: "Kontext", SourceDocument: source.Title, FirstEncounter: 2, CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "wikt:unclear", Gloss: "unclear"}}},
	}
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, entries, testutil.PresentationProvider{Name: codec.ProviderName(), Version: codec.ContextualGlossProviderVersion(), TargetLanguage: "en", RequireContextualGloss: true})
	require.NoError(t, err)
	runID := uuid.NewString()
	chunks, err := PlanBatchChunks(codec, runID, 1, codec.Model(), enrichment.OpenAIChatCompletionsEndpoint, deck.WorkProjection(), BatchChunkLimits{MaxRequests: 10})
	require.NoError(t, err)
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionBatch), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(), Endpoint: enrichment.OpenAIChatCompletionsEndpoint, Model: codec.Model(), BatchMaxRequests: 10, BatchMaxBytes: persistence.DefaultBatchMaxBytes}
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: deck.StorageProjection(), Config: config, Chunks: chunks}}
	provider := &contextualBatchFixtureProvider{}
	submitter := &BatchSubmitWorker{Store: store, Provider: provider, Codec: codec}
	poller := &BatchPollWorker{Store: store, Provider: provider, Codec: codec}
	workers := river.NewWorkers()
	river.AddWorker(workers, submitter)
	river.AddWorker(workers, poller)
	river.AddWorker(workers, &StandardTranslationWorker{})
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	submitter.Client = client
	poller.Client = client
	coordinator := NewDurableCoordinator(store, client, planner)
	run, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: preparation.ID})
	require.NoError(t, err)
	require.Len(t, run.Chunks, 1)
	chunk := run.Chunks[0]
	args := BatchSubmitJobArgs{OwnerID: owner.ID, PreparationID: preparation.ID, RunID: runID, ChunkID: chunk.ID, Generation: chunk.Generation}
	require.NoError(t, submitter.Submit(ctx, args))
	require.NoError(t, poller.Poll(ctx, BatchPollJobArgs{OwnerID: owner.ID, PreparationID: preparation.ID, RunID: runID, ChunkID: chunk.ID, Generation: chunk.ReconciliationGeneration}))
	current, err := store.GetPreparedDeckRun(ctx, owner.ID, preparation.ID, runID)
	require.NoError(t, err)
	ready, err := (&DurableFinalizer{Store: store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, owner.ID, preparation.ID, runID, current.FinalizationDispatchGeneration)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, ready.State)
	assert.Equal(t, 1, ready.TotalCards)
	frozen, stored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, runID)
	require.NoError(t, err)
	standardDeck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	standardFacts := preparedDeckRunFacts(current)
	standardFacts.ExecutionMode = string(domain.PreparedDeckExecutionStandard)
	standardArtifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, standardDeck, stored, standardFacts)
	require.NoError(t, err)
	assert.Equal(t, standardArtifact.APKG, ready.Artifact, "Batch publication must match the standard-mode artifact for the same frozen outcomes")
	assert.Equal(t, 1, standardArtifact.Completeness.TotalCards)
	status, err := store.GetDeckPreparationStatus(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []domain.DeckPreparationMeaningOmission{{TargetWord: "Kontext", Reason: "The sentence does not distinguish the meanings."}}, status.MeaningOmissions)
	assert.NotEmpty(t, status.Artifact)
	results, err := store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, preparation.ID, runID)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, results[0].State)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, results[1].State)
	assert.Equal(t, "The sentence does not distinguish the meanings.", results[1].OmissionReason)
	_, found, err := store.Get(ctx, deck.WorkProjection()[0].CacheKey)
	require.NoError(t, err)
	assert.True(t, found)
	_, found, err = store.Get(ctx, deck.WorkProjection()[1].CacheKey)
	require.NoError(t, err)
	assert.False(t, found, "unresolved meanings must not be cached")

	// Run the same frozen requests through standard execution after clearing only
	// their shared cache rows. The fake standard provider returns the identical
	// semantic outcomes, allowing the durable artifacts and statuses to be
	// compared without a live LLM request.
	_, err = store.Pool().Exec(ctx, `DELETE FROM enrichment_cache WHERE canonical_lemma IN ('resolved', 'unresolved')`)
	require.NoError(t, err)
	standardSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: source.Title, MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	standardPreparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: standardSource.ID, Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: standardSource.ContentHash})
	require.NoError(t, err)
	standardRunID := uuid.NewString()
	standardConfig := config
	standardConfig.ExecutionMode = string(domain.PreparedDeckExecutionStandard)
	standardConfig.MaxProviderAttempts = 1
	standardPlanner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: standardRunID, Projection: deck.StorageProjection(), Config: standardConfig}}
	standardCoordinator := NewDurableCoordinator(store, client, standardPlanner)
	_, err = standardCoordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: standardPreparation.ID})
	require.NoError(t, err)
	standardProvider := contextualStandardFixtureProvider{name: codec.ProviderName(), version: codec.ContextualGlossProviderVersion()}
	standardWorker := &StandardTranslationWorker{Store: store, Client: client, Provider: standardProvider}
	standardOutcomes, err := store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, standardPreparation.ID, standardRunID)
	require.NoError(t, err)
	for _, outcome := range standardOutcomes {
		if outcome.State == domain.PreparedDeckOutcomePending {
			require.NoError(t, standardWorker.execute(ctx, StandardTranslationJobArgs{OwnerID: owner.ID, PreparationID: standardPreparation.ID, RunID: standardRunID, Ordinal: outcome.Ordinal, Generation: 0}))
		}
	}
	standardRun, err := store.GetPreparedDeckRun(ctx, owner.ID, standardPreparation.ID, standardRunID)
	require.NoError(t, err)
	standardReady, err := (&DurableFinalizer{Store: store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, owner.ID, standardPreparation.ID, standardRunID, standardRun.FinalizationDispatchGeneration)
	require.NoError(t, err)
	assert.Equal(t, ready.Artifact, standardReady.Artifact)
	assert.Equal(t, ready.TotalCards, standardReady.TotalCards)
	assert.Equal(t, ready.MeaningOmissions, standardReady.MeaningOmissions)
	standardOutcomes, err = store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, standardPreparation.ID, standardRunID)
	require.NoError(t, err)
	require.Len(t, standardOutcomes, len(results))
	for index := range results {
		assert.Equal(t, results[index].State, standardOutcomes[index].State)
		assert.Equal(t, results[index].OmissionReason, standardOutcomes[index].OmissionReason)
	}
}
