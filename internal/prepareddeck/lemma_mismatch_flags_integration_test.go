//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"
)

const lemmaFlagSentence = "Dann haut das Motorrad Piero um."

type lemmaFlagFixture struct {
	ctx           context.Context
	store         *persistence.PostgresStore
	client        *river.Client[pgx.Tx]
	codec         *enrichment.TranslationCodec
	owner         string
	source        domain.SourceMaterial
	bookID        string
	corpusID      string
	analysisRunID string
	preparation   domain.DeckPreparation
	deck          cardexport.FrozenDeck
	target        cardexport.TargetOccurrence
}

func newLemmaFlagFixture(t *testing.T, ctx context.Context) *lemmaFlagFixture {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "lemma-flag-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Lemma flag Book",
		MediaType: "application/epub+zip", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersionV1, Units: []domain.ExtractedUnit{{
		ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4,
	}}})
	require.NoError(t, err)
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_content_revision_id::text,current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID).Scan(&source.ContentRevisionID, &source.ContentSnapshotID))
	var analysisRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'fixture','1',$5,'completed',now()) RETURNING id::text`, owner.ID, source.ID, source.ContentRevisionID, source.ContentSnapshotID, "lemma-flag-"+uuid.NewString()).Scan(&analysisRunID))
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: source.Title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "fixture", AnalyzerVersion: "1"}, nil))
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, analysisRunID, owner.ID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, owner.ID, analysisRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner.ID, book.ID, source.ID, analysisRunID)
	require.NoError(t, err)

	unitID := domain.EPUBUnitID(0, "unit")
	tokens := lemmaFlagTokens(unitID)
	testutil.SeedCorpusSentence(t, ctx, store.Pool(), owner.ID, corpus.ID, "de", analysisRunID, unitID, 0, 0, testutil.SeededSentence{Text: lemmaFlagSentence, Tokens: tokens})

	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	entry := cardexport.Entry{
		Language: "de", CanonicalLemma: "motorrad", UPOS: "NOUN", Sentence: lemmaFlagSentence, TargetWord: "Motorrad",
		CorpusID: corpus.ID, SentenceOrdinal: 0, SentenceTokens: tokens, SourceDocument: source.Title, FirstEncounter: 1,
		CandidateSenses: []enrichment.LexicalSense{{EvidenceID: "wikt:motorcycle", Source: "wiktionary", Origin: "fixture", Version: "fixture-v1", Gloss: "motorcycle"}},
	}
	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{entry}, testutil.PresentationProvider{
		Name: codec.ProviderName(), Version: codec.ContextualGlossProviderVersion(), TargetLanguage: "en", RequireContextualGloss: true,
	})
	require.NoError(t, err)
	target, ok := deck.TargetOccurrence(0)
	require.True(t, ok, "the fixture sentence must resolve its target token")

	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRunID,
		Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)

	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, store, client, nil, PreparedDeckConfig{}, time.Second)
	river.AddWorker(workers, &BatchSubmitWorker{})
	river.AddWorker(workers, &BatchPollWorker{})
	river.AddWorker(workers, &integrationFinalizeWorker{})
	testutil.StopOnCleanup(t, "River client", client.Stop)

	return &lemmaFlagFixture{
		ctx: ctx, store: store, client: client, codec: codec, owner: owner.ID, source: source, bookID: book.ID,
		corpusID: corpus.ID, analysisRunID: analysisRunID, preparation: preparation, deck: deck, target: target,
	}
}

// lemmaFlagTokens builds the fixture's dependency parse. Offsets are relative
// to the sentence and also document-absolute because the sentence starts at 0.
func lemmaFlagTokens(unitID string) []analyzer.Token {
	tokens := []analyzer.Token{
		{Surface: "Dann", RawLemma: "dann", CanonicalLemma: "dann", UPOS: "ADV", Dependency: "advmod", Head: 1, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 0, EndOffset: 4}},
		{Surface: "haut", RawLemma: "hauen", CanonicalLemma: "hauen", UPOS: "VERB", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 5, EndOffset: 9}},
		{Surface: "das", RawLemma: "der", CanonicalLemma: "der", UPOS: "DET", Dependency: "det", Head: 3, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 10, EndOffset: 13}},
		{Surface: "Motorrad", RawLemma: "motorrad", CanonicalLemma: "motorrad", UPOS: "NOUN", Dependency: "nsubj", Head: 1, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 14, EndOffset: 22}},
		{Surface: "Piero", RawLemma: "piero", CanonicalLemma: "piero", UPOS: "PROPN", Dependency: "obj", Head: 1, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 23, EndOffset: 28}},
		{Surface: "um", RawLemma: "um", CanonicalLemma: "um", UPOS: "PART", Dependency: "compound:prt", Head: 1, Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: 29, EndOffset: 31}},
	}
	for i := range tokens {
		if tokens[i].Morphology == nil {
			tokens[i].Morphology = map[string]string{}
		}
	}
	return tokens
}

func (f *lemmaFlagFixture) freezeStandard(t *testing.T) domain.PreparedDeckRun {
	t.Helper()
	config := persistence.PreparedDeckRunConfig{
		ExternalTranslationConsent: true, ExternalTranslationConfigured: true,
		ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext),
		Provider: f.codec.ProviderName(), ProviderVersion: f.codec.ContextualGlossProviderVersion(),
		Endpoint: enrichment.OpenAIChatCompletionsEndpoint, Model: f.codec.Model(), MaxProviderAttempts: 1,
	}
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: uuid.NewString(), Projection: f.deck.StorageProjection(), Config: config}}
	result, err := NewDurableCoordinator(f.store, f.client, planner).Freeze(f.ctx, DurableFreezeRequest{OwnerID: f.owner, PreparationID: f.preparation.ID})
	require.NoError(t, err)
	return result.Run
}

func (f *lemmaFlagFixture) freezeBatch(t *testing.T) (domain.PreparedDeckRun, domain.PreparedDeckBatchChunk) {
	t.Helper()
	runID := uuid.NewString()
	chunks, err := PlanBatchChunks(f.codec, runID, 1, f.codec.Model(), enrichment.OpenAIChatCompletionsEndpoint, f.deck.WorkProjection(), BatchChunkLimits{MaxRequests: 10})
	require.NoError(t, err)
	config := persistence.PreparedDeckRunConfig{
		ExternalTranslationConsent: true, ExternalTranslationConfigured: true,
		ExecutionMode: string(domain.PreparedDeckExecutionBatch), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext),
		Provider: f.codec.ProviderName(), ProviderVersion: f.codec.ContextualGlossProviderVersion(),
		Endpoint: enrichment.OpenAIChatCompletionsEndpoint, Model: f.codec.Model(),
		BatchMaxRequests: 10, BatchMaxBytes: persistence.DefaultBatchMaxBytes,
	}
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: f.deck.StorageProjection(), Config: config, Chunks: chunks}}
	result, err := NewDurableCoordinator(f.store, f.client, planner).Freeze(f.ctx, DurableFreezeRequest{OwnerID: f.owner, PreparationID: f.preparation.ID})
	require.NoError(t, err)
	require.Len(t, result.Chunks, 1)
	return result.Run, result.Chunks[0]
}

type lemmaMismatchStandardProvider struct{ enabled bool }

func (p lemmaMismatchStandardProvider) Name() string { return "fixture-provider" }
func (p lemmaMismatchStandardProvider) Version() string {
	return "fixture-model/translation-v16-citation-form-gloss"
}
func (p lemmaMismatchStandardProvider) Enabled() bool { return p.enabled }

func (p lemmaMismatchStandardProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	return enrichment.TranslationResponse{
		Translation: "translated", SentenceTranslation: "The sentence.",
		ContextOnly: true, UnresolvedKind: enrichment.ContextualGlossUnresolvedLemmaMismatch,
		UnresolvedReason: "  The sentence uses a different word than this lemma.  ",
	}, nil
}

func (f *lemmaFlagFixture) occurrences(t *testing.T) []domain.LemmaReviewOccurrence {
	t.Helper()
	occurrences, err := f.store.ListLemmaReviewOccurrences(f.ctx, f.owner, f.bookID, "Motorrad")
	require.NoError(t, err)
	return occurrences
}

func (f *lemmaFlagFixture) flagCount(t *testing.T) int {
	t.Helper()
	var count int
	require.NoError(t, f.store.Pool().QueryRow(f.ctx, `SELECT count(*) FROM occurrence_lemma_review_flags WHERE owner_id=$1 AND book_id=$2`, f.owner, f.bookID).Scan(&count))
	return count
}

func requireLemmaFlagProvenance(t *testing.T, occurrence domain.LemmaReviewOccurrence, codec *enrichment.TranslationCodec, reason string) {
	t.Helper()
	require.Equal(t, domain.DeckPreparationLemmaMismatchLabel, occurrence.ReviewFlagReason)
	require.True(t, domain.LemmaReviewFlagProviderSourced(occurrence.ReviewFlagProvenance))
	require.Equal(t, domain.LemmaReviewFlagProviderSource, occurrence.ReviewFlagProvenance["source"])
	require.Equal(t, codec.ProviderName(), occurrence.ReviewFlagProvenance["provider"])
	require.Equal(t, codec.Model(), occurrence.ReviewFlagProvenance["model"])
	require.Equal(t, codec.ContextualGlossPromptContract(), occurrence.ReviewFlagProvenance["prompt_version"])
	require.Equal(t, reason, occurrence.ReviewFlagProvenance["unresolved_reason"])
	require.NotContains(t, occurrence.ReviewFlagProvenance, "proposed_lemma")
	require.NotContains(t, occurrence.ReviewFlagProvenance, "alternative_lemma")
}

func TestStandardLemmaMismatchPersistsOneAdvisoryFlag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	fixture := newLemmaFlagFixture(t, ctx)
	run := fixture.freezeStandard(t)
	worker := &StandardTranslationWorker{Store: fixture.store, Client: fixture.client, Provider: lemmaMismatchStandardProvider{enabled: true}, AttemptTimeout: time.Second}
	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: fixture.owner, PreparationID: fixture.preparation.ID, RunID: run.ID, Ordinal: 0, Generation: 0}))

	outcomes, err := fixture.store.ListPreparedDeckTranslationOutcomes(ctx, fixture.owner, fixture.preparation.ID, run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	occurrences := fixture.occurrences(t)
	require.Len(t, occurrences, 1)
	occurrence := occurrences[0]

	t.Run("records the terminal omission", func(t *testing.T) {
		require.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
		require.True(t, domain.IsDeckPreparationLemmaMismatchOmission(outcomes[0].OmissionReason))
	})
	t.Run("persists the exact target occurrence with provider evidence", func(t *testing.T) {
		requireLemmaFlagProvenance(t, occurrence, fixture.codec, "The sentence uses a different word than this lemma.")
		require.Equal(t, fixture.target.CorpusID, occurrence.CorpusID)
		require.Equal(t, fixture.analysisRunID, occurrence.AnalysisRunID)
		require.Equal(t, fixture.target.SourceDocumentID, occurrence.SourceDocumentID)
		require.Equal(t, fixture.target.StartOffset, occurrence.StartOffset)
		require.Equal(t, fixture.target.EndOffset, occurrence.EndOffset)
	})
	t.Run("does not duplicate on reprocessing", func(t *testing.T) {
		saveContextualGlossLemmaMismatchFlags(ctx, fixture.store, fixture.owner, fixture.preparation.ID, run.ID, []contextualGlossLemmaMismatchOutcome{{
			Ordinal: 0, Target: fixture.target, TargetFound: true, UnresolvedReason: "The sentence uses a different word than this lemma.",
		}})
		require.Equal(t, 1, fixture.flagCount(t))
	})
	t.Run("never reopens a learner-resolved flag", func(t *testing.T) {
		require.NoError(t, fixture.store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{
			Occurrence: occurrence, CanonicalLemma: occurrence.CanonicalLemma,
		}}))
		resolved := fixture.occurrences(t)
		require.Len(t, resolved, 1)
		require.Equal(t, "keep", resolved[0].ReviewFlagResolution)
		saveContextualGlossLemmaMismatchFlags(ctx, fixture.store, fixture.owner, fixture.preparation.ID, run.ID, []contextualGlossLemmaMismatchOutcome{{
			Ordinal: 0, Target: fixture.target, TargetFound: true, UnresolvedReason: "The sentence uses a different word than this lemma.",
		}})
		resolved = fixture.occurrences(t)
		require.Len(t, resolved, 1)
		require.Equal(t, "keep", resolved[0].ReviewFlagResolution, "a resolved flag must never be reopened")
		require.Equal(t, 1, fixture.flagCount(t))
	})
}

func TestStandardLemmaMismatchSkipsAStaleOccurrence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	fixture := newLemmaFlagFixture(t, ctx)
	run := fixture.freezeStandard(t)
	_, err := fixture.store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, fixture.owner, fixture.bookID)
	require.NoError(t, err)
	worker := &StandardTranslationWorker{Store: fixture.store, Client: fixture.client, Provider: lemmaMismatchStandardProvider{enabled: true}, AttemptTimeout: time.Second}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: fixture.owner, PreparationID: fixture.preparation.ID, RunID: run.ID, Ordinal: 0, Generation: 0}))

	outcomes, err := fixture.store.ListPreparedDeckTranslationOutcomes(ctx, fixture.owner, fixture.preparation.ID, run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)

	t.Run("keeps the item completed", func(t *testing.T) {
		require.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State, "a stale occurrence must not fail the item")
	})
	t.Run("keeps the omission reported", func(t *testing.T) {
		require.True(t, domain.IsDeckPreparationLemmaMismatchOmission(outcomes[0].OmissionReason), "the omission must still be reported")
	})
	t.Run("writes no flag", func(t *testing.T) {
		require.Zero(t, fixture.flagCount(t))
	})
}

func TestStandardAmbiguousOmissionWritesNoFlag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	fixture := newLemmaFlagFixture(t, ctx)
	run := fixture.freezeStandard(t)
	provider := ambiguousStandardProvider{name: fixture.codec.ProviderName(), version: fixture.codec.ContextualGlossProviderVersion()}
	worker := &StandardTranslationWorker{Store: fixture.store, Client: fixture.client, Provider: provider, AttemptTimeout: time.Second}

	require.NoError(t, worker.execute(ctx, StandardTranslationJobArgs{OwnerID: fixture.owner, PreparationID: fixture.preparation.ID, RunID: run.ID, Ordinal: 0, Generation: 0}))

	t.Run("writes no advisory flag", func(t *testing.T) {
		require.Zero(t, fixture.flagCount(t))
	})
}

type ambiguousStandardProvider struct{ name, version string }

func (p ambiguousStandardProvider) Name() string    { return p.name }
func (p ambiguousStandardProvider) Version() string { return p.version }
func (p ambiguousStandardProvider) Enabled() bool   { return true }

func (p ambiguousStandardProvider) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	return enrichment.TranslationResponse{
		Translation: "translated", SentenceTranslation: "The sentence.",
		ContextOnly: true, UnresolvedKind: enrichment.ContextualGlossUnresolvedAmbiguous,
		UnresolvedReason: "The sentence does not distinguish the meanings.",
	}, nil
}

type lemmaMismatchBatchProvider struct {
	input    []byte
	output   []byte
	batch    enrichment.Batch
	metadata map[string]string
}

func (p *lemmaMismatchBatchProvider) UploadFile(_ context.Context, _ string, content io.Reader) (enrichment.OpenAIFile, error) {
	var err error
	p.input, err = io.ReadAll(content)
	return enrichment.OpenAIFile{ID: "lemma-input", Bytes: int64(len(p.input))}, err
}

func (p *lemmaMismatchBatchProvider) CreateBatch(_ context.Context, request enrichment.CreateBatchRequest) (enrichment.Batch, error) {
	p.metadata = request.Metadata
	var output strings.Builder
	count := 0
	for line := range strings.SplitSeq(strings.TrimSpace(string(p.input)), "\n") {
		if line == "" {
			continue
		}
		var input struct {
			CustomID string `json:"custom_id"`
			Body     struct {
				Messages []struct {
					Content string `json:"content"`
				} `json:"messages"`
			} `json:"body"`
		}
		if err := json.Unmarshal([]byte(line), &input); err != nil {
			return enrichment.Batch{}, err
		}
		if len(input.Body.Messages) < 2 {
			return enrichment.Batch{}, errors.New("lemma flag Batch request has no user message")
		}
		var payload struct {
			ItemID string `json:"item_id"`
		}
		if err := json.Unmarshal([]byte(input.Body.Messages[1].Content), &payload); err != nil {
			return enrichment.Batch{}, err
		}
		response := map[string]any{
			"item_id": payload.ItemID, "source_language": "de", "target_language": "en",
			"translation": "translated", "sentence_translation": "The sentence.",
			"sentence_translation_targets": []string{}, "gloss": "", "evidence_ids": []string{}, "context_only": true,
			"unresolved_kind": "lemma_mismatch", "unresolved_reason": "The sentence uses a different word than this lemma.",
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
	p.batch = enrichment.Batch{
		ID: "lemma-batch", InputFileID: request.InputFileID, OutputFileID: "lemma-output",
		Endpoint: enrichment.OpenAIChatCompletionsEndpoint, CompletionWindow: "24h", Status: enrichment.BatchStatusCompleted,
		RequestCounts: enrichment.BatchRequestCounts{Total: count, Completed: count}, Metadata: request.Metadata,
	}
	return p.batch, nil
}

func (p *lemmaMismatchBatchProvider) ListBatches(context.Context, enrichment.ListBatchesRequest) (enrichment.BatchList, error) {
	return enrichment.BatchList{}, nil
}

func (p *lemmaMismatchBatchProvider) GetBatch(context.Context, string) (enrichment.Batch, error) {
	return p.batch, nil
}

func (p *lemmaMismatchBatchProvider) FileContent(_ context.Context, id string, dst io.Writer) error {
	if id != "lemma-output" {
		return nil
	}
	_, err := dst.Write(p.output)
	return err
}

func TestBatchLemmaMismatchPersistsOneAdvisoryFlag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	fixture := newLemmaFlagFixture(t, ctx)
	run, chunk := fixture.freezeBatch(t)
	provider := &lemmaMismatchBatchProvider{}
	submitter := &BatchSubmitWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}
	poller := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}

	require.NoError(t, submitter.Submit(ctx, BatchSubmitJobArgs{OwnerID: fixture.owner, PreparationID: fixture.preparation.ID, RunID: run.ID, ChunkID: chunk.ID, Generation: chunk.Generation}))
	require.NoError(t, poller.Poll(ctx, BatchPollJobArgs{OwnerID: fixture.owner, PreparationID: fixture.preparation.ID, RunID: run.ID, ChunkID: chunk.ID, Generation: chunk.ReconciliationGeneration}))

	outcomes, err := fixture.store.ListPreparedDeckTranslationOutcomes(ctx, fixture.owner, fixture.preparation.ID, run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)
	occurrences := fixture.occurrences(t)
	require.Len(t, occurrences, 1)

	t.Run("records the terminal omission", func(t *testing.T) {
		require.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
		require.True(t, domain.IsDeckPreparationLemmaMismatchOmission(outcomes[0].OmissionReason))
	})
	t.Run("persists one flag for the exact target occurrence", func(t *testing.T) {
		require.Equal(t, 1, fixture.flagCount(t))
		require.Equal(t, fixture.target.StartOffset, occurrences[0].StartOffset)
		require.Equal(t, fixture.target.EndOffset, occurrences[0].EndOffset)
	})
	t.Run("records provider provenance", func(t *testing.T) {
		requireLemmaFlagProvenance(t, occurrences[0], fixture.codec, "The sentence uses a different word than this lemma.")
	})
}

func TestBatchLemmaMismatchSkipsAStaleOccurrence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	fixture := newLemmaFlagFixture(t, ctx)
	run, chunk := fixture.freezeBatch(t)
	_, err := fixture.store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, fixture.owner, fixture.bookID)
	require.NoError(t, err)
	provider := &lemmaMismatchBatchProvider{}
	submitter := &BatchSubmitWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}
	poller := &BatchPollWorker{Store: fixture.store, Client: fixture.client, Provider: provider, Codec: fixture.codec}
	require.NoError(t, submitter.Submit(ctx, BatchSubmitJobArgs{OwnerID: fixture.owner, PreparationID: fixture.preparation.ID, RunID: run.ID, ChunkID: chunk.ID, Generation: chunk.Generation}))
	require.NoError(t, poller.Poll(ctx, BatchPollJobArgs{OwnerID: fixture.owner, PreparationID: fixture.preparation.ID, RunID: run.ID, ChunkID: chunk.ID, Generation: chunk.ReconciliationGeneration}))

	outcomes, err := fixture.store.ListPreparedDeckTranslationOutcomes(ctx, fixture.owner, fixture.preparation.ID, run.ID)
	require.NoError(t, err)
	require.Len(t, outcomes, 1)

	t.Run("keeps the item completed", func(t *testing.T) {
		require.Equal(t, domain.PreparedDeckOutcomeCompleted, outcomes[0].State)
	})
	t.Run("keeps the omission reported", func(t *testing.T) {
		require.True(t, domain.IsDeckPreparationLemmaMismatchOmission(outcomes[0].OmissionReason))
	})
	t.Run("writes no flag", func(t *testing.T) {
		require.Zero(t, fixture.flagCount(t))
	})
}
