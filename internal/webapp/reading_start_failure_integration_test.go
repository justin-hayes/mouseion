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

func TestAuthenticatedStartRetainsReadingWhenDeckEnqueueFailsAndRetryPreparesIt(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reading-start-enqueue-failure-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	owner := createAccount(t, ctx, store, "reading-enqueue-failure", "learner-password", false)
	book, source, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "start-enqueue-failure", "Start enqueue failure", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3}})
	var analysisRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&analysisRunID))
	var analysisJobID int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT COALESCE(MAX(river_job_id),0)+1 FROM analysis_jobs`).Scan(&analysisJobID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,corpus_id,progress,display_number,analysis_run_id) VALUES($1,$2,$3,$4,$5,100,1,$6)`, analysisJobID, owner.ID, source.ID, source.ContentHash, corpus.ID, analysisRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de','haus','NOUN',3,'["Haus"]','[]','{}')`, owner.ID, corpus.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))

	workers := river.NewWorkers()
	prepareddeck.AddPreparedDeckWorker(workers, store, cardexport.NewPresentation(nil), nil, nil, prepareddeck.BatchConfig{}, prepareddeck.PreparedDeckConfig{}, false)
	client, err := river.NewClient[pgx.Tx](riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{prepareddeck.Queue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	deckService := prepareddeck.NewService(store, client)
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		PreparedDeck: deckService, AnalysisInsights: analysisinsights.NewService(store), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")

	setEnqueueFailure := func(enabled bool) {
		t.Helper()
		statement := `DROP TRIGGER IF EXISTS reject_prepared_deck_enqueue ON river_job; DROP FUNCTION IF EXISTS reject_prepared_deck_enqueue()`
		if enabled {
			statement = `CREATE FUNCTION reject_prepared_deck_enqueue() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				IF NEW.kind = 'prepared_deck' THEN RAISE EXCEPTION 'prepared deck queue unavailable'; END IF;
				RETURN NEW;
			END $$;
			CREATE TRIGGER reject_prepared_deck_enqueue BEFORE INSERT ON river_job
			FOR EACH ROW EXECUTE FUNCTION reject_prepared_deck_enqueue()`
		}
		_, changeErr := store.Pool().Exec(ctx, statement)
		require.NoError(t, changeErr)
	}
	setEnqueueFailure(true)
	t.Cleanup(func() {
		if _, cleanupErr := store.Pool().Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_prepared_deck_enqueue ON river_job; DROP FUNCTION IF EXISTS reject_prepared_deck_enqueue()`); cleanupErr != nil {
			t.Errorf("drop prepared deck enqueue failure trigger: %v", cleanupErr)
		}
	})
	start := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "deck+preparation+could+not+be+queued")

	reading, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, reading.BookID)
	assert.NotEmpty(t, reading.SourceMaterialID)
	assert.NotEmpty(t, reading.AnalysisRunID)
	assert.NotEmpty(t, reading.SnapshotID)
	assert.Positive(t, reading.SnapshotSize)
	var reservationCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, owner.ID, reading.SnapshotID).Scan(&reservationCount))
	assert.Equal(t, reading.SnapshotSize, reservationCount)
	frozenVocabulary, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Len(t, frozenVocabulary, reading.SnapshotSize)
	readingPage := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, readingPage.Code)
	assert.Contains(t, readingPage.Body.String(), `href="/reading/books/`+book.ID+`/deck/preparations/new"`)
	assert.Contains(t, readingPage.Body.String(), `action="/reading/books/`+book.ID+`/deck/retry"`)

	setEnqueueFailure(false)
	retry := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, retry.Code)
	assert.Contains(t, retry.Header().Get("Location"), "Deck+preparation+retry+queued")

	preparation, err := deckService.GetForGoalSnapshot(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, reading.SourceMaterialID, preparation.SourceMaterialID)
	assert.Equal(t, reading.AnalysisRunID, preparation.AnalysisRunID)
	assert.Equal(t, reading.SnapshotID, preparation.GoalSnapshotID)
	assert.Equal(t, domain.DeckPreparationQueued, preparation.State)
	var consent bool
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT COALESCE((args->>'external_translation_consent')::boolean, false) FROM river_job WHERE args->>'preparation_id'=$1`, preparation.ID).Scan(&consent))
	assert.False(t, consent, "local preparation retry must not imply external translation consent")

	unchanged, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, reading, unchanged, "retry changed current-reading identity or its frozen snapshot")
	retriedVocabulary, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, frozenVocabulary, retriedVocabulary, "retry changed the frozen vocabulary snapshot")

	completedJob, err := store.Pool().Exec(ctx, `UPDATE river_job SET state='completed',finalized_at=now() WHERE args->>'preparation_id'=$1 AND state IN ('available','pending','running','retryable','scheduled')`, preparation.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, completedJob.RowsAffected())
	setEnqueueFailure(true)
	duplicateStart := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, duplicateStart.Code)
	assert.Contains(t, duplicateStart.Header().Get("Location"), "deck+preparation+could+not+be+queued")
	queuedPage := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, queuedPage.Code)
	assert.Contains(t, queuedPage.Body.String(), `action="/reading/books/`+book.ID+`/deck/retry"`)
	assert.Contains(t, queuedPage.Body.String(), "Retry deck preparation")
	setEnqueueFailure(false)
	retryExisting := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/retry", url.Values{
		"csrf_token": {csrf}, "expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, retryExisting.Code)
	assert.Contains(t, retryExisting.Header().Get("Location"), "Deck+preparation+retry+queued")
}
