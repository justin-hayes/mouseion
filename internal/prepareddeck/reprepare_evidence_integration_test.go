//go:build integration

package prepareddeck

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"sync"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// changingMeaningIndex is a pinned, in-memory stand-in for the local dictionary
// index. A test can advance its version and evidence without network access.
type changingMeaningIndex struct {
	mu       sync.Mutex
	version  string
	evidence enrichment.LexicalSense
	lookups  int
}

func (i *changingMeaningIndex) Name() string { return "kaikki" }

func (i *changingMeaningIndex) Version() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.version
}

func (i *changingMeaningIndex) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.lookups++
	sense := i.evidence
	sense.Version = i.version
	return enrichment.LexicalEntry{Senses: []enrichment.LexicalSense{sense}, CandidateSenses: []enrichment.LexicalSense{sense}}, true, nil
}

func (i *changingMeaningIndex) advance(version string, sense enrichment.LexicalSense) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.version, i.evidence = version, sense
}

func (i *changingMeaningIndex) lookupCount() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.lookups
}

type reprepareFixtureAssembler struct{}

func (reprepareFixtureAssembler) AssemblePreparedDeckInputs(_ context.Context, _ pgx.Tx, prep domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	const sentence = "Dann haut das Motorrad Piero um."
	entry := cardexport.Entry{
		Language: "de", CanonicalLemma: "umhauen", UPOS: "VERB", Sentence: sentence,
		TargetWord: "haut ... um", SourceDocument: "Re-preparation fixture", FirstEncounter: 1,
		SentenceTokens: []analyzer.Token{
			{Surface: "Dann", UPOS: "ADV", Dependency: "advmod", Head: 1},
			{Surface: "haut", UPOS: "VERB", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}},
			{Surface: "das", UPOS: "DET", Dependency: "det", Head: 3},
			{Surface: "Motorrad", UPOS: "NOUN", Dependency: "nsubj", Head: 1},
			{Surface: "Piero", UPOS: "PROPN", Dependency: "obj", Head: 1},
			{Surface: "um", UPOS: "PART", Dependency: "compound:prt", Head: 1},
		},
	}
	candidate := domain.SelectionCandidate{OwnerID: prep.OwnerID, Language: "de", CanonicalLemma: "umhauen", UPOS: "VERB", OccurrenceCount: 3, ObservedForms: []byte(`["haut","um"]`), FirstEncounter: 1}
	return []cardexport.CandidateProjection{{OwnerID: prep.OwnerID, DeckName: "Re-preparation fixture", Candidate: candidate, Entry: entry}}, "Re-preparation fixture", nil
}

type reprepareFixtureProvider struct {
	mu    sync.Mutex
	calls int
}

func (*reprepareFixtureProvider) Name() string    { return "reprepare-fixture" }
func (*reprepareFixtureProvider) Version() string { return "1" }
func (p *reprepareFixtureProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	p.calls++
	p.mu.Unlock()
	if len(request.CandidateSenses) == 0 {
		return enrichment.TranslationResponse{}, assert.AnError
	}
	sense := request.CandidateSenses[0]
	gloss := "to knock down"
	sentenceTranslation := "And then the motorcycle knocks Piero over."
	targets := []string{"knocks"}
	if sense.EvidenceID == "wikt:umhauen-v2" {
		gloss = "to knock over"
		targets = []string{"knocks", "over"}
	}
	return enrichment.TranslationResponse{
		Translation: "knock over", Gloss: gloss, EvidenceIDs: []string{sense.EvidenceID},
		SentenceTranslation: sentenceTranslation, SentenceTranslationTargets: targets,
	}, nil
}

func (p *reprepareFixtureProvider) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls
}

func TestExplicitRepreparePublishesCurrentMeaningEvidenceAndKeepsHistoryFrozen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "reprepare-meaning-owner", false)
	require.NoError(t, err)
	other, err := store.CreateUser(ctx, "reprepare-meaning-other", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Re-preparation fixture",
		MediaType: "application/epub+zip", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4}}})
	require.NoError(t, err)
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_content_revision_id::text,current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID).Scan(&source.ContentRevisionID, &source.ContentSnapshotID))
	var analysisRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'fixture','1','reprepare-evidence','completed',now()) RETURNING id::text`, owner.ID, source.ID, source.ContentRevisionID, source.ContentSnapshotID).Scan(&analysisRunID))
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: source.Title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	require.NoError(t, store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: source.ContentHash, Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "fixture", AnalyzerVersion: "1"}, nil))
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, analysisRunID, owner.ID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, owner.ID, analysisRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner.ID, book.ID, source.ID, analysisRunID)
	require.NoError(t, err)
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: "umhauen", UPOS: "VERB", OccurrenceCount: 3, ObservedForms: []byte(`["haut","um"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{"min_occurrences":3}`)})
	require.NoError(t, err)
	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.NotEmpty(t, goal.SnapshotID)
	require.Equal(t, 1, goal.SnapshotSize)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRunID, GoalSnapshotID: goal.SnapshotID,
		Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)

	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	index := &changingMeaningIndex{version: "fixture-v1", evidence: enrichment.LexicalSense{EvidenceID: "wikt:umhauen-v1", Gloss: "to knock down", Source: "wiktionary", Kind: "meaning", Origin: "fixture"}}
	planner := NewStandardPlanner(reprepareFixtureAssembler{}, cardexport.NewPresentation(index), codec, true, BatchConfig{}, PreparedDeckConfig{StandardMaxAttempts: 1})
	workers := river.NewWorkers()
	provider := &reprepareFixtureProvider{}
	translation := &StandardTranslationWorker{Store: store, Provider: provider}
	preparationWorker := &Worker{Store: store}
	river.AddWorker(workers, translation)
	river.AddWorker(workers, preparationWorker)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	translation.Client = client
	coordinator := NewDurableCoordinator(store, client, planner)
	preparationWorker.Coordinator = coordinator
	finalizer := &DurableFinalizer{Store: store, Renderer: cardexport.NewPresentation(nil)}

	prepare := func(prep domain.DeckPreparation) domain.DeckPreparation {
		t.Helper()
		_, claimErr := store.ClaimDeckPreparation(ctx, prep.OwnerID, prep.ID)
		require.NoError(t, claimErr)
		frozen, freezeErr := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: prep.OwnerID, PreparationID: prep.ID})
		require.NoError(t, freezeErr)
		snapshot, _, loadErr := store.LoadPreparedDeckStorageProjection(ctx, prep.OwnerID, prep.ID, frozen.Run.ID)
		require.NoError(t, loadErr)
		deck, restoreErr := cardexport.NewPresentation(nil).Restore(snapshot)
		require.NoError(t, restoreErr)
		for _, work := range deck.WorkProjection() {
			if prep.ID == preparation.ID {
				// Seed the historical immutable cache shape: the original singular
				// phrase field is populated and the structured target list is absent.
				_, cacheErr := store.Put(ctx, enrichment.CacheEntry{
					CacheKey: work.CacheKey, Translation: "knock over", FallbackGloss: "to knock down", SenseSelection: []int{0},
					SentenceTranslation: "And then the motorcycle knocks Piero over.", SentenceTranslationTarget: "knocks Piero over", CachedAt: time.Now().UTC(),
				})
				require.NoError(t, cacheErr)
			}
			require.NoError(t, translation.execute(ctx, StandardTranslationJobArgs{OwnerID: prep.OwnerID, PreparationID: prep.ID, RunID: frozen.Run.ID, Ordinal: work.Ordinal, Generation: 0}))
		}
		run, runErr := store.GetPreparedDeckRun(ctx, prep.OwnerID, prep.ID, frozen.Run.ID)
		require.NoError(t, runErr)
		ready, finalizeErr := finalizer.Finalize(ctx, prep.OwnerID, prep.ID, frozen.Run.ID, run.FinalizationDispatchGeneration)
		require.NoError(t, finalizeErr)
		require.Equal(t, domain.DeckPreparationReady, ready.State)
		return ready
	}

	first := prepare(preparation)
	firstArtifact, err := store.DownloadDeckPreparation(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	firstRun, err := store.GetCurrentPreparedDeckRun(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	assert.Equal(t, codec.ContextualGlossProviderVersion(), firstRun.ProviderVersion)
	firstProjection, firstDigest, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, first.ID, firstRun.ID)
	require.NoError(t, err)
	require.Equal(t, "fixture-v1", firstProjection.Items[0].Entry.DictionaryProviderVersion)
	require.Equal(t, "wikt:umhauen-v1", firstProjection.Items[0].Entry.CandidateSenses[0].EvidenceID)
	firstKey := firstProjection.Items[0].CacheKey
	require.NotNil(t, firstKey)
	assert.Contains(t, artifactNoteFields(t, ctx, firstArtifact.Artifact), "to knock down")
	assert.Contains(t, artifactNoteFields(t, ctx, firstArtifact.Artifact), "And then the motorcycle <b>knocks Piero over</b>.")
	lookupsBeforeRerender := index.lookupCount()
	callsBeforeRerender := provider.callCount()
	assert.Zero(t, callsBeforeRerender, "legacy cached generation should not call the provider")

	// A presentation-only rerender consumes the frozen specification. It must
	// neither query the now-current local dictionary nor translate again.
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, first.ID, cardexport.PresentationVersion-1)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=$4 WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner.ID, first.ID, firstRun.ID, cardexport.PresentationVersion-1)
	require.NoError(t, err)
	rerendered, err := (&DurableRerenderer{Store: store, Renderer: cardexport.NewPresentation(nil)}).Rerender(ctx, owner.ID, first.ID, firstRun.ID, cardexport.PresentationVersion)
	require.NoError(t, err)
	assert.Equal(t, cardexport.PresentationVersion, rerendered.PresentationVersion)
	assert.Equal(t, 2, rerendered.DeckRevision)
	_, rerenderDigest, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, first.ID, firstRun.ID)
	require.NoError(t, err)
	assert.Equal(t, firstDigest, rerenderDigest)
	assert.Equal(t, lookupsBeforeRerender, index.lookupCount(), "rerender queried local lexical data")
	assert.Equal(t, callsBeforeRerender, provider.callCount(), "rerender invoked the translation provider")
	firstAfterRerender, err := store.DownloadDeckPreparation(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	assert.Contains(t, artifactNoteFields(t, ctx, firstAfterRerender.Artifact), "to knock down")
	index.advance("fixture-v2", enrichment.LexicalSense{EvidenceID: "wikt:umhauen-v2", Gloss: "to knock over", Source: "wiktionary", Kind: "meaning", Origin: "fixture"})
	service := NewService(store, client)
	refreshed, err := service.Reprepare(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, refreshed.Preparation.ID)
	require.Equal(t, domain.DeckPreparationQueued, refreshed.Preparation.State)
	assert.Equal(t, analysisRunID, refreshed.Preparation.AnalysisRunID)
	assert.Equal(t, goal.SnapshotID, refreshed.Preparation.GoalSnapshotID, "explicit re-preparation must retain the frozen Reading snapshot")
	second := prepare(refreshed.Preparation)
	secondRun, err := store.GetCurrentPreparedDeckRun(ctx, owner.ID, second.ID)
	require.NoError(t, err)
	assert.Equal(t, codec.ContextualGlossProviderVersion(), secondRun.ProviderVersion)
	assert.NotEqual(t, firstRun.ID, secondRun.ID, "each generation must have its own durable run identity")
	secondProjection, _, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, second.ID, secondRun.ID)
	require.NoError(t, err)
	assert.NotEqual(t, firstKey.MeaningEvidenceHash, secondProjection.Items[0].CacheKey.MeaningEvidenceHash)
	assert.Equal(t, "fixture-v2", secondProjection.Items[0].Entry.DictionaryProviderVersion)
	assert.Equal(t, "wikt:umhauen-v2", secondProjection.Items[0].Entry.CandidateSenses[0].EvidenceID)
	secondArtifact, err := store.DownloadDeckPreparation(ctx, owner.ID, second.ID)
	require.NoError(t, err)
	assert.Contains(t, artifactNoteFields(t, ctx, secondArtifact.Artifact), "to knock over")
	assert.Contains(t, artifactNoteFields(t, ctx, secondArtifact.Artifact), "And then the motorcycle <b>knocks</b> Piero <b>over</b>.")
	assert.Equal(t, callsBeforeRerender+1, provider.callCount(), "new generation must translate using refreshed evidence")
	oldAgain, err := store.DownloadDeckPreparation(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	assert.Equal(t, firstAfterRerender.Artifact, oldAgain.Artifact, "re-preparation changed the old artifact after rerender")
	assert.Equal(t, artifactNoteFields(t, ctx, firstAfterRerender.Artifact), artifactNoteFields(t, ctx, oldAgain.Artifact), "re-preparation changed historical card content")
	_, err = store.DownloadDeckPreparation(ctx, other.ID, first.ID)
	require.ErrorIs(t, err, persistence.ErrNotFound, "historical artifact must remain owner-scoped")
	generated, err := store.ListGeneratedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, generated, 1, "re-preparing the same analysis must not duplicate generated vocabulary identities")
	assert.Equal(t, "umhauen", generated[0].CanonicalLemma)
	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, known, "preparing cards must not mark vocabulary Known")
	otherGenerated, err := store.ListGeneratedVocabulary(ctx, other.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, otherGenerated, "generated vocabulary must remain owner-scoped")
}

func artifactNoteFields(t *testing.T, ctx context.Context, artifact []byte) string {
	t.Helper()
	path := t.TempDir() + "/collection.anki2"
	require.NoError(t, os.WriteFile(path, collectionBytes(t, artifact), 0o600))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	var fields string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT flds FROM notes`).Scan(&fields))
	return strings.ReplaceAll(fields, "\x1f", " ")
}
