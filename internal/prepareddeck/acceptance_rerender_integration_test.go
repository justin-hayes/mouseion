//go:build integration

package prepareddeck

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
	_ "modernc.org/sqlite"
)

func TestPresentationChangeReachesExistingDeckWithoutTouchingStudy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	owner, err := store.CreateUser(ctx, "acceptance-rerender-owner", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{
		OwnerID: owner.ID, Language: "de", SourceIdentifier: uuid.NewString(),
		Title: "Acceptance Book", MediaType: "text/plain", ContentHash: uuid.NewString(),
		Content: []byte("Das alte Haus steht heute am ruhigen Fluss."), FullText: "Das alte Haus steht heute am ruhigen Fluss.",
	})
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID,
		Filename: cardexport.DownloadFilename(source.Title),
		DeckName: cardexport.DeckName(source.Language, source.Title), ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de','haus','NOUN','candidate')`, owner.ID)
	require.NoError(t, err)

	deck, err := testutil.FreezePresentationDeck(ctx, owner.ID, source.Title, []cardexport.Entry{{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das alte Haus steht heute am ruhigen Fluss.", TargetWord: "Haus", SourceDocument: source.Title,
		FirstEncounter: 1, SentenceTokens: []analyzer.Token{
			{Surface: "Das", UPOS: "DET", Dependency: "det", Head: 1},
			{Surface: "alte", UPOS: "ADJ", Dependency: "amod", Head: 2},
			{Surface: "Haus", UPOS: "NOUN", Dependency: "nsubj", Head: 3},
			{Surface: "steht", UPOS: "VERB", Dependency: "root", Head: 3, Morphology: map[string]string{"VerbForm": "Fin"}},
			{Surface: "heute", UPOS: "ADV", Dependency: "advmod", Head: 3},
			{Surface: "am", UPOS: "ADP", Dependency: "case", Head: 7},
			{Surface: "ruhigen", UPOS: "ADJ", Dependency: "amod", Head: 7},
			{Surface: "Fluss", UPOS: "NOUN", Dependency: "obl", Head: 3},
		},
	}}, testutil.PresentationProvider{Name: "acceptance-provider", Version: "1", TargetLanguage: "en"})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	candidate := work[0].RequestCandidate()
	key := enrichment.CacheKey{
		Language: candidate.Language, TargetLanguage: "en", CanonicalLemma: candidate.CanonicalLemma,
		UPOS: candidate.UPOS, Provider: "acceptance-provider", ProviderVersion: "1",
		SentenceHash: enrichment.SentenceHash(candidate.ExampleSentence),
	}
	provider := &barrierTranslationProvider{}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{
		Queues:  map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}},
		Workers: workers,
	})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	config := persistence.PreparedDeckRunConfig{
		ExternalTranslationConsent: true, ExternalTranslationConfigured: true,
		ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en",
		ContextMode: string(enrichment.SentenceContext), Provider: key.Provider,
		ProviderVersion: key.ProviderVersion, Endpoint: enrichment.OpenAIChatCompletionsEndpoint, Model: "acceptance-model",
	}
	planner := fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{
		RunID: uuid.NewString(), Projection: deck.StorageProjection(), Config: config,
	}}
	result, err := NewDurableCoordinator(store, client, planner).Freeze(ctx, DurableFreezeRequest{
		OwnerID: owner.ID, PreparationID: preparation.ID, ExternalTranslationConsent: true,
	})
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	standardRun := standardIntegrationRun{store: store, owner: owner.ID, prep: preparation, run: result.Run, keys: []enrichment.CacheKey{key}}
	waitStandardOutcomes(t, ctx, standardRun, 1)
	calls, _ := provider.stats()
	assert.Equal(t, 1, calls, "initial provider calls")
	require.NoError(t, client.Stop(ctx))

	frozen, stored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	deck, err = cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, cardexport.RunFacts{Consent: result.Run.ExternalTranslationConsent, Configured: result.Run.ExternalTranslationConfigured, ExecutionMode: string(result.Run.ExecutionMode), TargetLanguage: result.Run.TargetLanguage, Provider: result.Run.Provider, ProviderVersion: result.Run.ProviderVersion})
	require.NoError(t, err)
	artifact.APKG = legacyPresentationArtifact(t, ctx, artifact.APKG)
	claimToken := uuid.NewString()
	_, err = store.ClaimPreparedDeckFinalization(ctx, owner.ID, preparation.ID, result.Run.ID, 0, claimToken, time.Now().UTC().Add(time.Minute))
	require.NoError(t, err)
	_, err = store.CompletePreparedDeckRun(ctx, owner.ID, preparation.ID, result.Run.ID, claimToken, artifact)
	require.NoError(t, err)
	other := createCompletedStaleDeck(t, ctx, store, owner.ID, "acceptance-other")
	retired := createCompletedStaleDeck(t, ctx, store, owner.ID, "acceptance-retired")
	graduated := createCompletedStaleDeck(t, ctx, store, owner.ID, "acceptance-graduated")
	var generatedDeckID string
	require.NoError(t, store.Pool().QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,'de','acceptance-graduated') RETURNING id::text`, owner.ID).Scan(&generatedDeckID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,'de','graduated','NOUN',$2,$3)`, owner.ID, generatedDeckID, graduated.SourceMaterialID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state) VALUES($1,'de','graduated','NOUN','candidate')`, owner.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at) VALUES($1,$2,'de','graduated','NOUN',now())`, owner.ID, graduated.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET total_cards=1 WHERE owner_id=$1 AND id=$2`, owner.ID, graduated.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET reviewed_at=now(),graduated_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, graduated.ID)
	require.NoError(t, err)
	graduatedBeforeRecovery, err := store.GetDeckPreparation(ctx, owner.ID, graduated.ID)
	require.NoError(t, err)
	started, err := store.GetDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	before, err := store.DownloadDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Contains(t, collectionModels(t, ctx, before.Artifact), "letter-spacing: -0.015em")
	var knownBeforeRecovery int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&knownBeforeRecovery))
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET retired_at=now() WHERE owner_id=$1 AND id=$2`, owner.ID, retired.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE cards SET front='stale projection' WHERE owner_id=$1`, owner.ID)
	require.NoError(t, err)
	oldPresentationVersion := cardexport.PresentationVersion - 1
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparations SET presentation_version=$3 WHERE owner_id=$1 AND id=$2`, owner.ID, preparation.ID, oldPresentationVersion)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE deck_preparation_runs SET presentation_version=$4 WHERE owner_id=$1 AND preparation_id=$2 AND id=$3`, owner.ID, preparation.ID, result.Run.ID, oldPresentationVersion)
	require.NoError(t, err)
	manifestBefore, digestBefore, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	var analysisRunsBefore, selectionCandidatesBefore, cacheEntriesBefore int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, owner.ID).Scan(&analysisRunsBefore))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, owner.ID).Scan(&selectionCandidatesBefore))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM enrichment_cache WHERE language='de'`).Scan(&cacheEntriesBefore))

	renderer := cardexport.NewPresentation(nil)
	rerenderWorkers := river.NewWorkers()
	rerenderClient, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{
		Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}}, Workers: rerenderWorkers,
	})
	require.NoError(t, err)
	AddRerenderWorker(rerenderWorkers, &DurableRerenderer{Store: store, Renderer: renderer})
	river.AddWorker(rerenderWorkers, &integrationFinalizeWorker{})
	recovery := &RecoveryWorker{Store: store, Client: rerenderClient, Interval: time.Hour, Limit: 10}
	var snooze *river.JobSnoozeError
	require.ErrorAs(t, recovery.Work(ctx, nil), &snooze)
	require.NoError(t, rerenderClient.Start(ctx))
	testutil.Cleanup(t, "River client", func() error { return rerenderClient.Stop(context.Background()) })

	var updated, updatedOther, updatedGraduated domain.DeckPreparation
	for {
		updated, err = store.GetDeckPreparation(ctx, owner.ID, preparation.ID)
		require.NoError(t, err)
		updatedOther, err = store.GetDeckPreparation(ctx, owner.ID, other.ID)
		require.NoError(t, err)
		updatedGraduated, err = store.GetDeckPreparation(ctx, owner.ID, graduated.ID)
		require.NoError(t, err)
		if updated.DeckRevision == 2 && updated.PresentationVersion == cardexport.PresentationVersion && updatedOther.DeckRevision == 2 && updatedOther.PresentationVersion == cardexport.PresentationVersion && updatedGraduated.DeckRevision == 2 && updatedGraduated.PresentationVersion == cardexport.PresentationVersion {
			break
		}
		select {
		case <-ctx.Done():
			require.FailNow(t, "timed out waiting for automatic deck rerender")
		case <-time.After(20 * time.Millisecond):
		}
	}
	retiredAfter, err := store.GetDeckPreparation(ctx, owner.ID, retired.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, retiredAfter.DeckRevision)
	assert.Zero(t, retiredAfter.PresentationVersion)
	after, err := store.DownloadDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.NotEqual(t, before.Artifact, after.Artifact, "rerender did not replace the old presentation")
	assert.Contains(t, collectionModels(t, ctx, after.Artifact), "letter-spacing: 0em")
	assert.Equal(t, result.Run.ID, updated.CurrentRunID)
	assert.Equal(t, started.StudyingAt, updated.StudyingAt)
	assert.Nil(t, updated.ReviewedAt)
	assert.Nil(t, updated.GraduatedAt)
	var known int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&known))
	assert.Equal(t, knownBeforeRecovery, known)
	var front string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT front FROM cards WHERE owner_id=$1`, owner.ID).Scan(&front))
	assert.NotEqual(t, "stale projection", front)
	var analysisRunsAfter, selectionCandidatesAfter, cacheEntriesAfter int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM analysis_runs WHERE owner_id=$1`, owner.ID).Scan(&analysisRunsAfter))
	assert.Equal(t, analysisRunsBefore, analysisRunsAfter)
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1`, owner.ID).Scan(&selectionCandidatesAfter))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM enrichment_cache WHERE language='de'`).Scan(&cacheEntriesAfter))
	assert.Equal(t, selectionCandidatesBefore, selectionCandidatesAfter)
	assert.Equal(t, cacheEntriesBefore, cacheEntriesAfter)
	manifestAfter, digestAfter, err := store.LoadPreparedDeckStorageProjection(ctx, owner.ID, preparation.ID, result.Run.ID)
	require.NoError(t, err)
	assert.Equal(t, manifestBefore, manifestAfter)
	assert.Equal(t, digestBefore, digestAfter)
	calls, _ = provider.stats()
	assert.Equal(t, 1, calls, "automatic rerender performed provider work")
	assert.Equal(t, graduatedBeforeRecovery.StudyingAt, updatedGraduated.StudyingAt)
	assert.Equal(t, graduatedBeforeRecovery.ReviewedAt, updatedGraduated.ReviewedAt)
	assert.Equal(t, graduatedBeforeRecovery.GraduatedAt, updatedGraduated.GraduatedAt)
	var knownAfterRecovery int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&knownAfterRecovery))
	assert.Equal(t, knownBeforeRecovery, knownAfterRecovery)
	calls, _ = provider.stats()
	assert.Equal(t, 1, calls, "rerender of graduated deck performed provider work")
}

func legacyPresentationArtifact(t *testing.T, ctx context.Context, apkg []byte) []byte {
	t.Helper()
	models := collectionModels(t, ctx, apkg)
	models = strings.Replace(models, "letter-spacing: 0em", "letter-spacing: -0.015em", 1)
	return replaceCollectionModels(t, ctx, apkg, models)
}

func collectionModels(t *testing.T, ctx context.Context, apkg []byte) string {
	t.Helper()
	collection := collectionBytes(t, apkg)
	path := t.TempDir() + "/collection.anki2"
	require.NoError(t, os.WriteFile(path, collection, 0o600))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	testutil.Cleanup(t, "SQLite database", db.Close)
	var models string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT models FROM col`).Scan(&models))
	return models
}

func replaceCollectionModels(t *testing.T, ctx context.Context, apkg []byte, models string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(apkg), int64(len(apkg)))
	require.NoError(t, err)
	collection := collectionBytes(t, apkg)
	path := t.TempDir() + "/collection.anki2"
	require.NoError(t, os.WriteFile(path, collection, 0o600))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE col SET models=?`, models)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	collection, err = os.ReadFile(path) //nolint:gosec // path is a file created in this test's TempDir.
	require.NoError(t, err)

	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	for _, file := range reader.File {
		opened, openErr := file.Open()
		require.NoError(t, openErr)
		data, readErr := io.ReadAll(opened)
		require.NoError(t, readErr)
		require.NoError(t, opened.Close())
		if file.Name == "collection.anki2" {
			data = collection
		}
		header := file.FileHeader
		header.Method = zip.Store
		entry, writeErr := writer.CreateHeader(&header)
		require.NoError(t, writeErr)
		_, writeErr = entry.Write(data)
		require.NoError(t, writeErr)
	}
	require.NoError(t, writer.Close())
	return output.Bytes()
}

func collectionBytes(t *testing.T, apkg []byte) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(apkg), int64(len(apkg)))
	require.NoError(t, err)
	for _, file := range reader.File {
		if file.Name != "collection.anki2" {
			continue
		}
		opened, openErr := file.Open()
		require.NoError(t, openErr)
		data, readErr := io.ReadAll(opened)
		require.NoError(t, readErr)
		require.NoError(t, opened.Close())
		return data
	}
	require.FailNow(t, "APKG does not contain collection.anki2")
	return nil
}
