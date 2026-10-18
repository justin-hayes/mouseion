//go:build integration

package prepareddeck

import (
	"context"
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

// fixedMeaningIndex is a pinned in-memory meaning index for the retry fixture.
type fixedMeaningIndex struct{ version string }

func (i *fixedMeaningIndex) Name() string    { return "kaikki" }
func (i *fixedMeaningIndex) Version() string { return i.version }

func (i *fixedMeaningIndex) Lookup(_ context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	sense := enrichment.LexicalSense{EvidenceID: "wikt:" + request.CanonicalLemma, Source: "wiktionary", Kind: "meaning", Origin: "fixture", Version: i.version, Gloss: "a fixture gloss"}
	return enrichment.LexicalEntry{Senses: []enrichment.LexicalSense{sense}, CandidateSenses: []enrichment.LexicalSense{sense}}, true, nil
}

// omissionFixtureAssembler freezes two candidates whose identities the fake
// provider answers differently.
type omissionFixtureAssembler struct{}

func (omissionFixtureAssembler) AssemblePreparedDeckInputs(_ context.Context, _ pgx.Tx, prep domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	const sentence = "Dann haut das Motorrad Piero um."
	tokens := []analyzer.Token{
		{Surface: "Dann", UPOS: "ADV", Dependency: "advmod", Head: 1},
		{Surface: "haut", UPOS: "VERB", Dependency: "root", Head: 1, Morphology: map[string]string{"VerbForm": "Fin"}},
		{Surface: "das", UPOS: "DET", Dependency: "det", Head: 3},
		{Surface: "Motorrad", UPOS: "NOUN", Dependency: "nsubj", Head: 1},
		{Surface: "Piero", UPOS: "PROPN", Dependency: "obj", Head: 1},
		{Surface: "um", UPOS: "PART", Dependency: "compound:prt", Head: 1},
	}
	candidate := func(lemma, upos, target string, encounter int64) cardexport.CandidateProjection {
		return cardexport.CandidateProjection{
			OwnerID: prep.OwnerID, DeckName: "Omissions fixture",
			Candidate: domain.SelectionCandidate{OwnerID: prep.OwnerID, Language: "de", CanonicalLemma: lemma, UPOS: upos, OccurrenceCount: 3, ObservedForms: []byte(`["` + target + `"]`), FirstEncounter: encounter},
			Entry:     cardexport.Entry{Language: "de", CanonicalLemma: lemma, UPOS: upos, Sentence: sentence, TargetWord: target, SourceDocument: "Omissions fixture", FirstEncounter: encounter, SentenceTokens: tokens},
		}
	}
	return []cardexport.CandidateProjection{candidate("umhauen", "VERB", "haut", 1), candidate("motorrad", "NOUN", "Motorrad", 2)}, "Omissions fixture", nil
}

type omissionFixtureProvider struct {
	mu    sync.Mutex
	calls map[string]int
}

func (*omissionFixtureProvider) Name() string    { return "omission-fixture" }
func (*omissionFixtureProvider) Version() string { return "1" }
func (*omissionFixtureProvider) Enabled() bool   { return true }

func (p *omissionFixtureProvider) Translate(_ context.Context, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.mu.Lock()
	if p.calls == nil {
		p.calls = make(map[string]int)
	}
	p.calls[request.CanonicalLemma]++
	attempt := p.calls[request.CanonicalLemma]
	p.mu.Unlock()
	// The first attempt at umhauen returns an unusable answer; the retry run
	// answers it successfully. Every other identity is answered once and cached.
	if request.CanonicalLemma == "umhauen" && attempt == 1 {
		return enrichment.TranslationResponse{}, nil
	}
	evidence := ""
	if len(request.CandidateSenses) > 0 {
		evidence = request.CandidateSenses[0].EvidenceID
	}
	return enrichment.TranslationResponse{
		Translation:                request.CanonicalLemma + "-translated",
		Gloss:                      "contextual gloss for " + request.CanonicalLemma,
		EvidenceIDs:                []string{evidence},
		SentenceTranslation:        "And then the motorcycle knocks Piero over.",
		SentenceTranslationTargets: []string{"knocks"},
	}, nil
}

func (p *omissionFixtureProvider) callsFor(lemma string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.calls[lemma]
}

func TestRetryingOmittedItemsReusesCacheAndRecoversCards(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "retry-omissions-owner", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(), Title: "Omissions fixture",
		MediaType: "application/epub+zip", ContentHash: uuid.NewString(), Content: []byte("text"), FullText: "text",
	}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{{ID: domain.EPUBUnitID(0, "unit"), Order: 0, SpineIndex: 0, ManifestID: "unit", Text: "text", EndOffset: 4}}})
	require.NoError(t, err)
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT current_content_revision_id::text,current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, owner.ID, source.ID).Scan(&source.ContentRevisionID, &source.ContentSnapshotID))
	var analysisRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'fixture','1','retry-omissions','completed',now()) RETURNING id::text`, owner.ID, source.ID, source.ContentRevisionID, source.ContentSnapshotID).Scan(&analysisRunID))
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
	for _, seeded := range []struct{ lemma, upos string }{{"umhauen", "VERB"}, {"motorrad", "NOUN"}} {
		testutil.SeedCorpusWords(t, ctx, store.Pool(), owner.ID, corpus.ID, "de", testutil.CorpusWord{Lemma: seeded.lemma, UPOS: seeded.upos, Occurrences: 3})
	}
	reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.Positive(t, reading.SnapshotSize)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: analysisRunID, SnapshotID: reading.SnapshotID,
		Filename: cardexport.DownloadFilename(source.Title), DeckName: source.Title, ContentHash: source.ContentHash,
	})
	require.NoError(t, err)

	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	index := &fixedMeaningIndex{version: "fixture-v1"}
	planner := NewStandardPlanner(omissionFixtureAssembler{}, cardexport.NewPresentation(index), codec, true, BatchConfig{}, PreparedDeckConfig{StandardMaxAttempts: 1})
	workers := river.NewWorkers()
	provider := &omissionFixtureProvider{}
	translation := &StandardTranslationWorker{Store: store, Provider: provider, AttemptTimeout: time.Second}
	preparationWorker := &Worker{Store: store}
	river.AddWorker(workers, translation)
	river.AddWorker(workers, preparationWorker)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
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
	firstStatus, err := store.GetDeckPreparationStatus(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	require.Equal(t, domain.DeckPreparationReady, firstStatus.State)
	assert.Equal(t, 1, firstStatus.ProviderOmissions, "the unusable answer must be reported as an omission")
	assert.Equal(t, 1, firstStatus.TotalCards)
	firstGenerated, err := store.ListGeneratedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, firstGenerated, 1)
	assert.Equal(t, "motorrad", firstGenerated[0].CanonicalLemma, "the omitted identity must not be recorded as generated vocabulary")

	service := NewService(store, client)
	refreshed, err := service.Reprepare(ctx, owner.ID, first.ID, first.SnapshotID)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, refreshed.Preparation.ID)
	require.Equal(t, domain.DeckPreparationQueued, refreshed.Preparation.State)

	second := prepare(refreshed.Preparation)
	secondStatus, err := store.GetDeckPreparationStatus(ctx, owner.ID, second.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, secondStatus.State)
	assert.Zero(t, secondStatus.ProviderOmissions, "the recovered identity must no longer be omitted")
	assert.Equal(t, 2, secondStatus.TotalCards, "the retried deck must include the recovered card")
	assert.Equal(t, 2, provider.callsFor("umhauen"), "the omitted identity must be re-requested once per run")
	assert.Equal(t, 1, provider.callsFor("motorrad"), "a cached result must not be requested again")
	secondGenerated, err := store.ListGeneratedVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, secondGenerated, 2, "the recovered card must be recorded as generated vocabulary")
}
