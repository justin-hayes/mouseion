//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
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

func TestCurrentReadingStartAndSwitchSurviveDeckFailuresWithFocusedRetry(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reading-start-enqueue-failure-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	owner := createAccount(t, ctx, store, "reading-enqueue-failure", "learner-password", false)
	otherOwner := createAccount(t, ctx, store, "reading-provider-other", "other-learner-password", false)
	book, source, corpus, historicalDeck := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "start-enqueue-failure", "Start enqueue failure", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3}})
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
		Analysis:     analysis.NewService(store.Pool(), nil),
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
	oldJobSubmission := perform(t, h, http.MethodPost, "/jobs/"+strconv.FormatInt(analysisJobID, 10)+"/deck/preparations", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, oldJobSubmission.Code, "the old analysis-job route cannot prepare a non-current Book")
	for _, request := range []struct {
		method string
		path   string
		form   url.Values
	}{
		{method: http.MethodGet, path: "/reading/books/" + book.ID + "/deck/preparations/new"},
		{method: http.MethodPost, path: "/reading/books/" + book.ID + "/deck/preparations", form: url.Values{"csrf_token": {csrf}}},
	} {
		response := perform(t, h, request.method, request.path, request.form, cookies)
		assert.Equalf(t, http.StatusNotFound, response.Code, "pre-reading route %s %s", request.method, request.path)
	}
	start := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, start.Code)
	assert.Contains(t, start.Header().Get("Location"), "is+now+your+current+reading")

	reading, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, reading.BookID)
	assert.NotEmpty(t, reading.SourceMaterialID)
	assert.NotEmpty(t, reading.AnalysisRunID)
	assert.NotEmpty(t, reading.SnapshotID)
	assert.Positive(t, reading.SnapshotSize)
	var preparationCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1 AND book_id=$2 AND goal_snapshot_id=$3`, owner.ID, book.ID, reading.SnapshotID).Scan(&preparationCount))
	assert.Zero(t, preparationCount, "starting Reading must not submit optional deck work")
	for _, path := range []string{
		"/reading/books/" + book.ID + "/to-read",
		"/reading/books/" + book.ID + "/set-aside",
	} {
		retired := perform(t, h, http.MethodPost, path, url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equalf(t, http.StatusNotFound, retired.Code, "retired disposition endpoint %s", path)
	}
	afterRetiredPosts, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, reading, afterRetiredPosts, "retired endpoints leave the current reading and reservation intact")
	disposition, err := store.GetBookDisposition(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)
	var historyCount, knownCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&knownCount))
	assert.Zero(t, historyCount)
	assert.Zero(t, knownCount)
	var reservationCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshot_vocabulary WHERE owner_id=$1 AND snapshot_id=$2`, owner.ID, reading.SnapshotID).Scan(&reservationCount))
	assert.Equal(t, reading.SnapshotSize, reservationCount)
	frozenVocabulary, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Len(t, frozenVocabulary, reading.SnapshotSize)
	pendingBook, pendingSource, pendingCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "publication-pending", "Publication pending", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "buch", UPOS: "NOUN", OccurrenceCount: 1}})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, pendingBook.ID, domain.BookDispositionToRead))
	var pendingRunID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM corpora WHERE owner_id=$1 AND id=$2`, owner.ID, pendingCorpus.ID).Scan(&pendingRunID))
	_, err = store.Pool().Exec(ctx, `DELETE FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, pendingBook.ID)
	require.NoError(t, err)
	var pendingJobID int64
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT COALESCE(MAX(river_job_id),0)+1 FROM analysis_jobs`).Scan(&pendingJobID))
	_, err = store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash,corpus_id,progress,display_number,analysis_run_id) VALUES($1,$2,$3,$4,$5,100,(SELECT COALESCE(MAX(display_number),0)+1 FROM analysis_jobs WHERE owner_id=$2),$6)`, pendingJobID, owner.ID, pendingSource.ID, pendingSource.ContentHash, pendingCorpus.ID, pendingRunID)
	require.NoError(t, err)
	readingPage := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, readingPage.Code)
	assert.NotContains(t, readingPage.Body.String(), "Remove from To Read")
	assert.Contains(t, readingPage.Body.String(), "This Book is not shown as analyzed until its result is published")
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_jobs SET error='publication failed' WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, pendingRunID)
	require.NoError(t, err)
	failedPublicationPage := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, failedPublicationPage.Code)
	assert.Contains(t, failedPublicationPage.Body.String(), "publication is pending or failed")
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_jobs SET error='' WHERE owner_id=$1 AND analysis_run_id=$2`, owner.ID, pendingRunID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, owner.ID, pendingBook.ID, pendingSource.ID, pendingRunID)
	require.NoError(t, err)
	publishedPage := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, publishedPage.Code)
	assert.NotContains(t, publishedPage.Body.String(), "This Book is not shown as analyzed until its result is published")
	assert.Contains(t, readingPage.Body.String(), "Confirm set aside", "current-reading Set Aside remains explicit and confirmed")
	assert.Contains(t, readingPage.Body.String(), `href="/library"`, "Reading directs disposition decisions to My Books")
	assert.Contains(t, readingPage.Body.String(), `href="/reading/books/`+book.ID+`/deck/preparations/new"`)
	assert.Contains(t, readingPage.Body.String(), `action="/reading/books/`+book.ID+`/deck/retry"`, "the current reading offers explicit optional preparation")

	setEnqueueFailure(false)
	prepare := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/deck/preparations", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, prepare.Code)
	assert.Contains(t, prepare.Header().Get("Location"), "/deck-preparations/")

	preparation, err := deckService.GetForGoalSnapshot(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, reading.SourceMaterialID, preparation.SourceMaterialID)
	assert.Equal(t, reading.AnalysisRunID, preparation.AnalysisRunID)
	assert.Equal(t, reading.SnapshotID, preparation.GoalSnapshotID)
	assert.Equal(t, domain.DeckPreparationQueued, preparation.State)
	preparedSnapshot, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, preparation.GoalSnapshotID)
	require.NoError(t, err)
	assert.Equal(t, frozenVocabulary, preparedSnapshot, "preparation keeps frozen identities after live corpus and Known state change")
	var jobArgs []byte
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT args FROM river_job WHERE args->>'preparation_id'=$1`, preparation.ID).Scan(&jobArgs))
	assert.NotContains(t, string(jobArgs), "external_translation_consent", "retry jobs have no per-submission choice")

	unchanged, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, reading, unchanged, "retry changed current-reading identity or its frozen snapshot")
	retriedVocabulary, err := store.ListPrimaryGoalSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, frozenVocabulary, retriedVocabulary, "retry changed the frozen vocabulary snapshot")

	worker := &prepareddeck.Worker{
		Store: store,
		Coordinator: prepareddeck.NewDurableCoordinator(store, client, prepareddeck.NewPreparedDeckPlanner(
			prepareddeck.NewInputAssembler(store), cardexport.NewPresentation(nil), nil, false,
			prepareddeck.BatchConfig{}, prepareddeck.PreparedDeckConfig{},
		)),
	}
	require.NoError(t, worker.Work(ctx, &river.Job[prepareddeck.JobArgs]{Args: prepareddeck.JobArgs{
		PreparationID: preparation.ID, OwnerID: owner.ID, SourceMaterialID: reading.SourceMaterialID,
		ContentHash: source.ContentHash, AnalysisRunID: reading.AnalysisRunID,
	}}))
	failed, err := deckService.GetForGoalSnapshot(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, failed.State)
	assert.Contains(t, failed.Error, "configured translation provider")
	assert.Empty(t, failed.Artifact, "an unconfigured provider must not publish a local-only deck")
	failedVocabulary, err := store.ListDeckPreparationVocabulary(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Empty(t, failedVocabulary, "a failed preparation cannot reserve card vocabulary")
	_, err = deckService.GetForGoalSnapshot(ctx, otherOwner.ID, reading.SnapshotID)
	require.ErrorIs(t, err, persistence.ErrNotFound, "a failed preparation remains owner-scoped")

	focusedTask := perform(t, h, http.MethodGet, "/reading/books/"+book.ID+"/deck/preparations/new", nil, cookies)
	require.Equal(t, http.StatusOK, focusedTask.Code)
	assert.Contains(t, focusedTask.Body.String(), "Deck preparation failed")
	assert.Contains(t, focusedTask.Body.String(), "Retry deck preparation")
	assert.NotContains(t, focusedTask.Body.String(), "Download deck")
	stillReading, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, reading, stillReading, "provider configuration failure must not change current reading")
	var knownAfterFailure int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&knownAfterFailure))
	assert.Equal(t, knownCount, knownAfterFailure, "deck failure must not change Known vocabulary")

	completedJob, err := store.Pool().Exec(ctx, `UPDATE river_job SET state='completed',finalized_at=now() WHERE args->>'preparation_id'=$1 AND state IN ('available','pending','running','retryable','scheduled')`, preparation.ID)
	require.NoError(t, err)
	assert.EqualValues(t, 1, completedJob.RowsAffected())
	setEnqueueFailure(true)
	duplicateStart := perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusSeeOther, duplicateStart.Code)
	assert.Contains(t, duplicateStart.Header().Get("Location"), "is+now+your+current+reading")
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
	retriedPreparation, err := deckService.GetForGoalSnapshot(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, preparation.ID, retriedPreparation.ID, "retry must remain attached to this exact reading snapshot")
	assert.Equal(t, domain.DeckPreparationQueued, retriedPreparation.State)

	switchBook, switchSource, switchCorpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "switch-enqueue-failure", "Switch enqueue failure", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "garten", UPOS: "NOUN", OccurrenceCount: 3}})
	_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de','garten','NOUN',3,'["Garten"]','[]','{}')`, owner.ID, switchCorpus.ID)
	require.NoError(t, err)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, switchBook.ID, domain.BookDispositionToRead))
	setEnqueueFailure(true)
	switched := perform(t, h, http.MethodPost, "/reading/books/"+switchBook.ID+"/switch", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {reading.BookID},
		"expected_current_snapshot_id": {reading.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, switched.Code)
	assert.Contains(t, switched.Header().Get("Location"), "is+now+your+current+reading")
	currentAfterSwitch, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, switchBook.ID, currentAfterSwitch.BookID, "deck queue failure must not roll back switching current reading")
	assert.NotEmpty(t, currentAfterSwitch.SnapshotID)
	assert.Positive(t, currentAfterSwitch.SnapshotSize)
	assert.Equal(t, switchSource.ID, currentAfterSwitch.SourceMaterialID)
	var switchedPreparationCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1 AND book_id=$2 AND goal_snapshot_id=$3`, owner.ID, switchBook.ID, currentAfterSwitch.SnapshotID).Scan(&switchedPreparationCount))
	assert.Zero(t, switchedPreparationCount, "switching Reading must not submit optional deck work")
	oldJobAfterSwitch := perform(t, h, http.MethodPost, "/jobs/"+strconv.FormatInt(analysisJobID, 10)+"/deck/preparations", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, oldJobAfterSwitch.Code, "the former analysis-job route cannot prepare a Book after switching away")
	stillCurrent := perform(t, h, http.MethodGet, "/reading", nil, cookies)
	require.Equal(t, http.StatusOK, stillCurrent.Code)
	assert.Contains(t, stillCurrent.Body.String(), currentAfterSwitch.BookID, "a denied historical submission must not change Reading")
	finished := perform(t, h, http.MethodPost, "/reading/finish", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {switchBook.ID},
		"expected_current_snapshot_id": {currentAfterSwitch.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusOK, finished.Code)
	assert.Contains(t, finished.Body.String(), "Reading finished")
	continued, err := store.ClaimDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err, "a preparation submitted during Reading remains claimable after Reading ends")
	assert.Equal(t, preparation.GoalSnapshotID, continued.GoalSnapshotID)
	completedAfterFinish, err := store.CompleteDeckPreparation(ctx, owner.ID, preparation.ID, domain.DeckPreparation{
		Artifact: []byte("completed-after-reading"), Filename: "completed-after-reading.apkg", DeckName: "Completed after reading", TotalCards: 1,
	})
	require.NoError(t, err)
	assert.Equal(t, preparation.GoalSnapshotID, completedAfterFinish.GoalSnapshotID)
	completedDownload, err := store.DownloadDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	assert.Equal(t, []byte("completed-after-reading"), completedDownload.Artifact)
	var knownGarden int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='garten' AND upos='NOUN'`, owner.ID).Scan(&knownGarden))
	assert.Equal(t, 1, knownGarden, "finishing without preparing a deck accepts the frozen identity into Known")
	for _, request := range []struct {
		method string
		path   string
		form   url.Values
	}{
		{method: http.MethodGet, path: "/reading/books/" + switchBook.ID + "/deck/preparations/new"},
		{method: http.MethodPost, path: "/reading/books/" + switchBook.ID + "/deck/preparations", form: url.Values{"csrf_token": {csrf}}},
	} {
		response := perform(t, h, request.method, request.path, request.form, cookies)
		assert.Equalf(t, http.StatusNotFound, response.Code, "post-reading route %s %s", request.method, request.path)
	}
	var generationsBefore, generationsAfter int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&generationsBefore))
	for _, action := range []string{"retry", "reprepare", "rerender"} {
		response := perform(t, h, http.MethodPost, "/deck-preparations/"+preparation.ID+"/"+action, url.Values{"csrf_token": {csrf}}, cookies)
		assert.Equalf(t, http.StatusNotFound, response.Code, "historical preparation %s is unavailable after Reading ends", action)
	}
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&generationsAfter))
	assert.Equal(t, generationsBefore, generationsAfter, "denied actions do not create another Book generation")
	ownerDownload := perform(t, h, http.MethodGet, "/deck-preparations/"+historicalDeck.ID+"/download", nil, cookies)
	assert.Equal(t, http.StatusOK, ownerDownload.Code, "the historical pre-reading artifact remains owner-downloadable")
	assert.Equal(t, []byte("migration-start-enqueue-failure"), ownerDownload.Body.Bytes())
	otherCookies, _ := loginCookies(t, h, otherOwner.Username, "other-learner-password")
	otherDownload := perform(t, h, http.MethodGet, "/deck-preparations/"+historicalDeck.ID+"/download", nil, otherCookies)
	assert.Equal(t, http.StatusNotFound, otherDownload.Code, "historical artifact downloads remain owner-scoped")
}
