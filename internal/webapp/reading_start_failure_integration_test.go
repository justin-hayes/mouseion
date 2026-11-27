//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentReadingStartReportsCrossBookCountReadinessAndRetries(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reading-count-readiness-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	owner := createAccount(t, ctx, store, "reading-count-readiness", "learner-password")
	book, _, corpus := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "count-readiness-target", "Count readiness target", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "crossing", UPOS: "NOUN", OccurrenceCount: 2}})
	other, _, otherCorpus := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, "count-readiness-other", "Count readiness other", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "crossing", UPOS: "NOUN", OccurrenceCount: 8}})
	testutil.SeedCorpusWords(t, ctx, store.Pool(), owner.ID, corpus.ID, "de", testutil.CorpusWord{Lemma: "crossing", UPOS: "NOUN", Occurrences: 2})
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, other.ID, domain.BookDispositionToRead))

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	start := func() *httptest.ResponseRecorder {
		t.Helper()
		return perform(t, h, http.MethodPost, "/reading/books/"+book.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	}
	showRedirect := func(response *httptest.ResponseRecorder) string {
		t.Helper()
		location := response.Header().Get("Location")
		page := perform(t, h, http.MethodGet, location, nil, cookies)
		require.Equal(t, http.StatusOK, page.Code)
		return page.Body.String()
	}

	updating := start()
	assert.Equal(t, http.StatusSeeOther, updating.Code)
	assert.Contains(t, showRedirect(updating), "Across-book vocabulary counts are updating")
	reading, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, reading.BookID, "pending counts must not create a partial Reading")

	var targetRun string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&targetRun))
	_, err = store.Pool().Exec(ctx, `INSERT INTO river_job(kind,args,queue,state,max_attempts,finalized_at)
VALUES('rebuild_vocabulary_browse_counts',jsonb_build_object('owner_id',$1::uuid,'book_id',$2::uuid,'run_id',$3::uuid),'default','discarded',1,now())`, owner.ID, book.ID, targetRun)
	require.NoError(t, err)
	unavailable := start()
	assert.Equal(t, http.StatusSeeOther, unavailable.Code)
	assert.Contains(t, showRedirect(unavailable), "unavailable after repeated rebuild failures")
	reading, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, reading.BookID, "failed count rebuilds must leave Reading unchanged")

	for _, item := range []struct {
		book   domain.Book
		corpus domain.Corpus
		count  int64
	}{
		{book: book, corpus: corpus, count: 2},
		{book: other, corpus: otherCorpus, count: 13},
	} {
		var run string
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, item.book.ID).Scan(&run))
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count) VALUES($1,$2,'de',$3,$4,'crossing','NOUN',$5)`, owner.ID, item.book.ID, run, item.corpus.ID, item.count)
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,'de',$3,$4,5)`, owner.ID, item.book.ID, run, item.corpus.ID)
		require.NoError(t, err)
	}
	_, err = store.Pool().Exec(ctx, `DELETE FROM river_job WHERE kind='rebuild_vocabulary_browse_counts' AND args->>'owner_id'=$1 AND args->>'book_id'=$2 AND args->>'run_id'=$3`, owner.ID, book.ID, targetRun)
	require.NoError(t, err)
	ready := start()
	assert.Equal(t, http.StatusSeeOther, ready.Code)
	assert.Contains(t, ready.Header().Get("Location"), "is+now+your+current+reading")
	reading, err = store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, reading.BookID)
	snapshot, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
	require.NoError(t, err)
	require.Len(t, snapshot, 1)
	assert.Equal(t, "crossing", snapshot[0].CanonicalLemma)
	assert.Equal(t, 2, snapshot[0].OccurrenceCount)
}
