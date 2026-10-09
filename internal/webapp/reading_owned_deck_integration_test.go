//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentReadingPreparationSurvivesLiveEvidenceChangesAndFinish(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reading-owned-deck-integration-secret")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	owner := createAccount(t, ctx, store, "reading-owned-deck", "reading-owned-password", false)
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "reading-owned-deck", "Reading-owned deck", []domain.LemmaOccurrence{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3},
		{Language: "de", CanonicalLemma: "buch", UPOS: "NOUN", OccurrenceCount: 3},
	})
	var analysisRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&analysisRunID))
	var analysisJobID int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT COALESCE(MAX(river_job_id),0)+1 FROM analysis_jobs`).Scan(&analysisJobID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,corpus_id,progress,display_number,analysis_run_id) VALUES($1,$2,$3,$4,$5,100,1,$6)`, analysisJobID, owner.ID, source.ID, source.ContentHash, corpus.ID, analysisRunID)
	require.NoError(t, err)
	for _, lemma := range []string{"haus", "buch"} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de',$3,'NOUN',3,'[]','[]','{}')`, owner.ID, corpus.ID, lemma)
		require.NoError(t, err)
	}
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))

	workers := river.NewWorkers()
	prepareddeck.AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), nil, nil, prepareddeck.BatchConfig{}, prepareddeck.PreparedDeckConfig{}, false)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{prepareddeck.Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	deckService := prepareddeck.NewService(store, client)
	webAuth := auth.New(store, time.Hour)
	h := New(Services{
		Auth: webAuth, WebAuth: webauth.New(webAuth, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store),
		PreparedDeck: deckService, Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "reading-owned-password")

	started := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, started.Code)
	reading, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	initialSnapshot, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.Len(t, initialSnapshot, 2)
	assert.Equal(t, []string{"buch", "haus"}, []string{initialSnapshot[0].CanonicalLemma, initialSnapshot[1].CanonicalLemma})

	// Live eligibility and Known state change after the freeze. Neither is
	// allowed to reselect the identities for this Reading's deck.
	_, err = store.Pool().Exec(ctx, `UPDATE selection_candidates SET occurrence_count=1 WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,'de','haus','NOUN') ON CONFLICT(owner_id,language,canonical_lemma,upos) DO NOTHING`, owner.ID)
	require.NoError(t, err)

	missing := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/preparations", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, missing.Code)
	assert.Contains(t, missing.Header().Get("Location"), "error=", "missing commitment identity is rejected")
	submitted := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/preparations", url.Values{"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID}}, cookies)
	require.Equal(t, http.StatusSeeOther, submitted.Code)
	preparation, err := deckService.GetForGoalSnapshot(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, reading.SourceMaterialID, preparation.SourceMaterialID)
	assert.Equal(t, reading.AnalysisRunID, preparation.AnalysisRunID)
	assert.Equal(t, reading.SnapshotID, preparation.GoalSnapshotID)

	// Verify the actual preparation input reader uses the frozen rows, not the
	// now-ineligible live projection.
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	facts, err := store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback(ctx))
	assert.True(t, facts.GoalSnapshotActive)
	assert.Equal(t, initialSnapshot, facts.GoalSnapshot)

	retried := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusSeeOther, retried.Code)
	afterRetry, err := deckService.GetForGoalSnapshot(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, preparation.ID, afterRetry.ID)
	assert.Equal(t, reading.SnapshotID, afterRetry.GoalSnapshotID)

	finished := perform(t, h, http.MethodPost, "/reading/finish", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {book.ID},
		"expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	require.Equal(t, http.StatusOK, finished.Code)
	assert.Contains(t, finished.Body.String(), "Reading finished")
	for _, lemma := range []string{"haus", "buch"} {
		var known int
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma=$2 AND upos='NOUN'`, owner.ID, lemma).Scan(&known))
		assert.Equal(t, 1, known, "finishing accepts frozen %s into Known", lemma)
	}
	afterFinishTx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	afterFinishFacts, err := store.LoadPreparedDeckInputFactsTx(ctx, afterFinishTx, preparation)
	require.NoError(t, err)
	require.NoError(t, afterFinishTx.Rollback(ctx))
	assert.Equal(t, initialSnapshot, afterFinishFacts.GoalSnapshot, "the queued job still reads its historical snapshot after Reading ends")

	// The already-submitted job remains usable and retains the exact snapshot
	// even after Reading releases it and a reread submits a newer generation.
	_, err = store.Pool().Exec(ctx, `UPDATE selection_candidates SET occurrence_count=3 WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de','garten','NOUN',3,'[]','[]','{}')`, owner.ID, corpus.ID)
	require.NoError(t, err)
	var eligibleRows int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2 AND occurrence_count>=3`, owner.ID, corpus.ID).Scan(&eligibleRows))
	assert.Equal(t, 3, eligibleRows)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	restarted := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, restarted.Code)
	nextReading, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.NotEqual(t, reading.SnapshotID, nextReading.SnapshotID, "a reread freezes a new snapshot")
	require.Positive(t, nextReading.SnapshotSize, "the live corpus growth is eligible in the reread")
	staleSubmitted := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/preparations", url.Values{"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID}}, cookies)
	assert.Contains(t, staleSubmitted.Header().Get("Location"), "error=", "the previous snapshot cannot prepare the restarted reading")
	nextSubmitted := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/preparations", url.Values{"csrf_token": {csrf}, "expected_current_snapshot_id": {nextReading.SnapshotID}}, cookies)
	require.Equal(t, http.StatusSeeOther, nextSubmitted.Code)
	nextPreparation, err := deckService.GetForGoalSnapshot(ctx, owner.ID, nextReading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, nextReading.SnapshotID, nextPreparation.GoalSnapshotID)
	oldGeneration, err := store.GetDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	require.NotNil(t, oldGeneration.RetiredAt, "the new reading keeps its own deck generation")
	continued, err := store.ClaimDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err, "a submitted historical job remains claimable after a newer generation retires it")
	assert.Equal(t, reading.SnapshotID, continued.GoalSnapshotID)
	_, err = store.CompleteDeckPreparation(ctx, owner.ID, preparation.ID, domain.DeckPreparation{
		Artifact: []byte("finished-reading-deck"), Filename: "finished-reading.apkg", DeckName: "Reading-owned deck", TotalCards: 2,
	})
	require.NoError(t, err)
	artifact, err := store.DownloadDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("finished-reading-deck"), artifact.Artifact)
}
