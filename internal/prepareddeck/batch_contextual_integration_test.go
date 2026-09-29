//go:build integration

package prepareddeck

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

type contextualBatchFixtureProvider struct {
	input    []byte
	output   []byte
	batch    enrichment.Batch
	metadata map[string]string
	batches  int
}

type contextualStandardFixtureProvider struct {
	name, version string
}

func (p contextualStandardFixtureProvider) Name() string    { return p.name }
func (p contextualStandardFixtureProvider) Version() string { return p.version }
func (p contextualStandardFixtureProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	if request.CanonicalLemma == "umhauen" {
		return enrichment.TranslationResponse{
			Translation: "knock over", Gloss: "knock down", ContextOnly: true,
			SentenceTranslation: "And then the motorcycle knocks Piero over.", SentenceTranslationTargets: []string{"knocks", "over"},
		}, nil
	}
	if request.CanonicalLemma == "inferred" {
		return enrichment.TranslationResponse{
			Translation: "translated", Gloss: "contextual meaning", ContextOnly: true,
			SentenceTranslation: "The complete sentence stays available without emphasis.", SentenceTranslationTargets: []string{"Piero", "knocks"},
		}, nil
	}
	if request.CanonicalLemma == "unresolved" {
		return enrichment.TranslationResponse{
			Translation: "translated", Gloss: "contextual meaning", ContextOnly: true,
			SentenceTranslation: "This complete sentence has no target alignment.",
		}, nil
	}
	return enrichment.TranslationResponse{Translation: "translated", Gloss: "contextual meaning", EvidenceIDs: []string{"wikt:context"}, SentenceTranslation: "The translated sentence.", SentenceTranslationTargets: []string{"translated"}}, nil
}

func (p *contextualBatchFixtureProvider) UploadFile(_ context.Context, _ string, content io.Reader) (enrichment.OpenAIFile, error) {
	var err error
	p.input, err = io.ReadAll(content)
	return enrichment.OpenAIFile{ID: "contextual-input-" + strconv.Itoa(p.batches+1), Bytes: int64(len(p.input))}, err
}

func (p *contextualBatchFixtureProvider) CreateBatch(_ context.Context, request enrichment.CreateBatchRequest) (enrichment.Batch, error) {
	p.batches++
	batchSuffix := strconv.Itoa(p.batches)
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
			"sentence_translation_targets": []string{"translated"}, "evidence_ids": []string{}, "context_only": true,
		}
		if identity.Ordinal == 0 {
			response["translation"] = "knock over"
			response["gloss"] = "knock down"
			response["sentence_translation"] = "And then the motorcycle knocks Piero over."
			response["sentence_translation_targets"] = []string{"knocks", "over"}
			response["unresolved_reason"] = ""
		}
		if identity.Ordinal == 1 {
			response["gloss"] = "contextual meaning"
			response["sentence_translation"] = "This complete sentence has no target alignment."
			response["sentence_translation_targets"] = []string{}
			response["unresolved_reason"] = ""
		} else if identity.Ordinal == 0 {
			response["context_only"] = true
		} else {
			response["gloss"] = "contextual meaning"
			response["unresolved_reason"] = ""
			if identity.Ordinal == 2 {
				response["sentence_translation"] = "The complete sentence stays available without emphasis."
				response["sentence_translation_targets"] = []string{"Piero", "knocks"}
			}
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
	p.batch = enrichment.Batch{ID: "contextual-batch-" + batchSuffix, InputFileID: request.InputFileID, OutputFileID: "contextual-output-" + batchSuffix, Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CompletionWindow: "24h", Status: enrichment.BatchStatusCompleted, RequestCounts: enrichment.BatchRequestCounts{Total: count, Completed: count}, Metadata: request.Metadata}
	return p.batch, nil
}

func (p *contextualBatchFixtureProvider) ListBatches(context.Context, enrichment.ListBatchesRequest) (enrichment.BatchList, error) {
	return enrichment.BatchList{}, nil
}

func (p *contextualBatchFixtureProvider) GetBatch(context.Context, string) (enrichment.Batch, error) {
	return p.batch, nil
}

func (p *contextualBatchFixtureProvider) FileContent(_ context.Context, id string, dst io.Writer) error {
	if strings.HasPrefix(id, "contextual-output-") {
		_, err := dst.Write(p.output)
		return err
	}
	return nil
}

func TestDurableBatchRendersStructuredEnglishTargetsAndPreservesOptionalTranslation(t *testing.T) {
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
	indexPath := filepath.Join(t.TempDir(), "dictionary.sqlite")
	indexDB, err := sql.Open("sqlite", indexPath)
	require.NoError(t, err)
	_, err = indexDB.ExecContext(ctx, `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1')`)
	require.NoError(t, err)
	weakFixture := make([]enrichment.LexicalSense, 9)
	for index := range weakFixture {
		evidenceID := "wikt:weak-" + strconv.Itoa(index)
		if index == 0 {
			evidenceID = "wikt:context"
		}
		weakFixture[index] = enrichment.LexicalSense{EvidenceID: evidenceID, Gloss: "context"}
	}
	encodedFixture, err := json.Marshal(weakFixture)
	require.NoError(t, err)
	_, err = indexDB.ExecContext(ctx, `INSERT INTO entries VALUES ('de', 'resolved', '', ?, '', '', '', '', ''), ('de', 'umhauen', '', ?, '', '', '', '', '')`, string(encodedFixture), string(encodedFixture))
	require.NoError(t, err)
	require.NoError(t, indexDB.Close())
	index, err := dictionary.OpenIndex(ctx, indexPath)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, index.Close()) })
	weak, found, err := index.Lookup(ctx, enrichment.LexicalLookupRequest{Language: "de", CanonicalLemma: "resolved", UPOS: "NOUN"})
	require.NoError(t, err)
	require.True(t, found)
	require.Len(t, weak.CandidateSenses, 9)
	require.Equal(t, "lemma_only_missing_pos", weak.CandidateSenses[0].MatchStrength)
	entries := []cardexport.Entry{
		{Language: "de", CanonicalLemma: "umhauen", UPOS: "VERB", Sentence: "Dann haut das Motorrad Piero um.", TargetWord: "Piero", SourceDocument: source.Title, FirstEncounter: 1, DictionaryProviderVersion: index.Version(), CandidateSenses: weak.CandidateSenses},
		{Language: "de", CanonicalLemma: "unresolved", UPOS: "NOUN", Sentence: "Der zweite Kontext bleibt heute unklar.", TargetWord: "Kontext", SourceDocument: source.Title, FirstEncounter: 2, CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "wikt:unclear", Gloss: "unclear"}}},
		{Language: "de", CanonicalLemma: "inferred", UPOS: "NOUN", Sentence: "Der dritte Kontext beschreibt eine neue Situation.", TargetWord: "Situation", SourceDocument: source.Title, FirstEncounter: 3, CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "wikt:missing", Gloss: "scene"}}},
	}
	deck, err := testutil.FreezePresentationDeckWithLexical(ctx, owner.ID, source.Title, entries, testutil.PresentationProvider{Name: codec.ProviderName(), Version: codec.ContextualGlossProviderVersion(), TargetLanguage: "en", RequireContextualGloss: true}, index)
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
	assert.Equal(t, 3, ready.TotalCards)
	frozen, stored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, runID)
	require.NoError(t, err)
	require.Len(t, frozen.Items[0].Entry.CandidateSenses, enrichment.DefaultMaxCandidateSenses)
	require.Equal(t, "wikt:context", frozen.Items[0].Entry.CandidateSenses[0].EvidenceID)
	require.Equal(t, "wiktionary", frozen.Items[0].Entry.CandidateSenses[0].Source)
	require.Equal(t, "Kaikki.org Wiktextract enwiktionary", frozen.Items[0].Entry.CandidateSenses[0].Origin)
	require.Equal(t, "fixture-v1", frozen.Items[0].Entry.CandidateSenses[0].Version)
	require.Equal(t, "lemma_only_missing_pos", frozen.Items[0].Entry.CandidateSenses[0].MatchStrength, "durable finalization retains the weaker lexical match label")
	require.Equal(t, 1, frozen.Items[0].Entry.OmittedEvidenceCount, "durable finalization retains candidate-bound truncation")
	standardDeck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	standardFacts := preparedDeckRunFacts(current)
	standardFacts.ExecutionMode = string(domain.PreparedDeckExecutionStandard)
	standardArtifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, standardDeck, stored, standardFacts)
	require.NoError(t, err)
	assert.Equal(t, standardArtifact.APKG, ready.Artifact, "Batch publication must match the standard-mode artifact for the same frozen outcomes")
	assert.Equal(t, 3, standardArtifact.Completeness.TotalCards)
	require.Len(t, standardArtifact.Generated, 3)
	assert.Equal(t, "And then the motorcycle <b>knocks</b> Piero <b>over</b>.", standardArtifact.Generated[0].Note.EnglishSentence)
	assert.Equal(t, "This complete sentence has no target alignment.", standardArtifact.Generated[1].Note.EnglishSentence, "absent optional alignment must not discard the translated sentence")
	assert.Equal(t, "contextual meaning", standardArtifact.Generated[1].Note.Gloss, "absent optional alignment must not discard the contextual Gloss")
	assert.Equal(t, "The complete sentence stays available without emphasis.", standardArtifact.Generated[2].Note.EnglishSentence, "invalid optional alignment must not discard the translated sentence")
	assert.Equal(t, "contextual meaning", standardArtifact.Generated[2].Note.Gloss, "invalid optional alignment must not discard the contextual Gloss")
	status, err := store.GetDeckPreparationStatus(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []domain.DeckPreparationEvidenceCoverage{{Source: "wiktionary", Configured: true, Selected: 3, Matched: 3, Candidates: 10, Omitted: 1}}, status.EvidenceCoverage)
	assert.Empty(t, status.MeaningOmissions)
	assert.True(t, status.ContextualGlossesReported)
	assert.Equal(t, 3, status.ContextualGlosses)
	assert.Equal(t, 3, status.ContextOnlyGlosses)
	assert.NotEmpty(t, status.Artifact)
	results, err := store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, preparation.ID, runID)
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, results[0].State)
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, results[1].State)
	_, found, err = store.Get(ctx, deck.WorkProjection()[0].CacheKey)
	require.NoError(t, err)
	assert.True(t, found)
	_, found, err = store.Get(ctx, deck.WorkProjection()[1].CacheKey)
	require.NoError(t, err)
	assert.True(t, found, "sentence translation without alignment remains cacheable")
	_, found, err = store.Get(ctx, deck.WorkProjection()[2].CacheKey)
	require.NoError(t, err)
	assert.True(t, found, "context-only gloss must remain distinguishable in the durable cache record")

	// A later Batch preparation consumes the structured results from cache.
	_, err = store.Pool().Exec(ctx, `DELETE FROM enrichment_cache WHERE canonical_lemma='unresolved'`)
	require.NoError(t, err)
	cachedSource, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: source.Title, MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text"})
	require.NoError(t, err)
	cachedPreparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: cachedSource.ID, Filename: cardexport.DownloadFilename(cachedSource.Title), DeckName: cachedSource.Title, ContentHash: cachedSource.ContentHash})
	require.NoError(t, err)
	cachedRunID := uuid.NewString()
	cachedChunks, err := PlanBatchChunks(codec, cachedRunID, 1, codec.Model(), enrichment.OpenAIChatCompletionsEndpoint, []cardexport.WorkItem{deck.WorkProjection()[1]}, BatchChunkLimits{MaxRequests: 10})
	require.NoError(t, err)
	cachedPlanner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: cachedRunID, Projection: deck.StorageProjection(), Config: config, Chunks: cachedChunks}}
	cachedCoordinator := NewDurableCoordinator(store, client, cachedPlanner)
	cachedRun, err := cachedCoordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: cachedPreparation.ID})
	require.NoError(t, err)
	cachedOutcomes, err := store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, cachedPreparation.ID, cachedRunID)
	require.NoError(t, err)
	require.Len(t, cachedOutcomes, 3)
	assert.Equal(t, 1, cachedOutcomes[0].CacheHitCount)
	assert.Equal(t, 1, cachedOutcomes[2].CacheHitCount)
	require.Len(t, cachedRun.Chunks, 1)
	cachedChunk := cachedRun.Chunks[0]
	require.NoError(t, submitter.Submit(ctx, BatchSubmitJobArgs{OwnerID: owner.ID, PreparationID: cachedPreparation.ID, RunID: cachedRunID, ChunkID: cachedChunk.ID, Generation: cachedChunk.Generation}))
	require.NoError(t, poller.Poll(ctx, BatchPollJobArgs{OwnerID: owner.ID, PreparationID: cachedPreparation.ID, RunID: cachedRunID, ChunkID: cachedChunk.ID, Generation: cachedChunk.ReconciliationGeneration}))
	cachedCurrent, err := store.GetPreparedDeckRun(ctx, owner.ID, cachedPreparation.ID, cachedRunID)
	require.NoError(t, err)
	cachedReady, err := (&DurableFinalizer{Store: store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, owner.ID, cachedPreparation.ID, cachedRunID, cachedCurrent.FinalizationDispatchGeneration)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, cachedReady.State)
	cachedStatus, err := store.GetDeckPreparationStatus(ctx, owner.ID, cachedPreparation.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, cachedStatus.ContextualGlosses)
	assert.Equal(t, 3, cachedStatus.ContextOnlyGlosses)
	cachedFrozen, cachedStored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, cachedPreparation.ID, cachedRunID)
	require.NoError(t, err)
	cachedDeck, err := cardexport.NewPresentation(nil).Restore(cachedFrozen)
	require.NoError(t, err)
	cachedArtifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, cachedDeck, cachedStored, cardexport.RunFacts{
		Consent: true, Configured: true, ExecutionMode: string(domain.PreparedDeckExecutionBatch),
		TargetLanguage: "en", Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(),
	})
	require.NoError(t, err)
	assert.Equal(t, cachedReady.Artifact, cachedArtifact.APKG)
	require.Len(t, cachedArtifact.Generated, 3)
	assert.Equal(t, "And then the motorcycle <b>knocks</b> Piero <b>over</b>.", cachedArtifact.Generated[0].Note.EnglishSentence, "multi-span alignment must survive cache reuse through final rendering")

	// Run the same frozen requests through standard execution after clearing only
	// their shared cache rows. The fake standard provider returns the identical
	// semantic outcomes, allowing the durable artifacts and statuses to be
	// compared without a live LLM request.
	_, err = store.Pool().Exec(ctx, `DELETE FROM enrichment_cache WHERE canonical_lemma IN ('umhauen', 'unresolved', 'inferred')`)
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
	standardStatus, err := store.GetDeckPreparationStatus(ctx, owner.ID, standardPreparation.ID)
	require.NoError(t, err)
	assert.True(t, standardStatus.ContextualGlossesReported)
	assert.Equal(t, 3, standardStatus.ContextualGlosses)
	assert.Equal(t, 3, standardStatus.ContextOnlyGlosses)
	standardOutcomes, err = store.ListPreparedDeckTranslationOutcomes(ctx, owner.ID, standardPreparation.ID, standardRunID)
	require.NoError(t, err)
	require.Len(t, standardOutcomes, len(results))
	for index := range results {
		assert.Equal(t, results[index].State, standardOutcomes[index].State)
		assert.Equal(t, results[index].OmissionReason, standardOutcomes[index].OmissionReason)
	}
}
