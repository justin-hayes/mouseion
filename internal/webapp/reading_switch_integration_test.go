//go:build integration

package webapp

import (
	"context"
	"net/http"
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

func TestCurrentReadingSwitchIsAtomicAndReturnFreezesFreshSnapshot(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reading-switch-secret-0123456789abcdef")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))

	owner := createAccount(t, ctx, store, "reading-switch", "learner-password", false)
	other := createAccount(t, ctx, store, "reading-switch-other", "other-learner-password", false)
	seed := func(suffix string, lemmas ...string) domain.Book {
		t.Helper()
		occurrences := make([]domain.LemmaOccurrence, 0, len(lemmas))
		for _, lemma := range lemmas {
			occurrences = append(occurrences, domain.LemmaOccurrence{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", OccurrenceCount: 3})
		}
		book, _, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, suffix, "Switch "+suffix, occurrences)
		for _, lemma := range lemmas {
			_, execErr := store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de',$3,'NOUN',3,'[]','[]','{}')`, owner.ID, corpus.ID, lemma)
			require.NoError(t, execErr)
		}
		require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
		return book
	}
	bookA := seed("switch-a", "haus", "baum")
	bookB := seed("switch-b", "garten")

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	otherCookies, otherCSRF := loginCookies(t, h, other.Username, "other-learner-password")

	snapshotLemmas := func(snapshotID string) []string {
		t.Helper()
		rows, listErr := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, snapshotID)
		require.NoError(t, listErr)
		lemmas := make([]string, 0, len(rows))
		for _, row := range rows {
			lemmas = append(lemmas, row.CanonicalLemma)
		}
		return lemmas
	}
	switchTo := func(target domain.Book, expected domain.CurrentReading) string {
		t.Helper()
		response := perform(t, h, http.MethodPost, "/reading/books/"+target.ID+"/switch", url.Values{
			"csrf_token": {csrf}, "expected_current_book_id": {expected.BookID},
			"expected_current_snapshot_id": {expected.SnapshotID},
		}, cookies)
		require.Equal(t, http.StatusSeeOther, response.Code)
		return response.Header().Get("Location")
	}
	current := func() domain.CurrentReading {
		t.Helper()
		reading, getErr := store.GetCurrentReading(ctx, owner.ID, "de")
		require.NoError(t, getErr)
		return reading
	}
	releasedAt := func(snapshotID string) bool {
		t.Helper()
		var released bool
		require.NoError(t, store.Pool().QueryRow(ctx, `SELECT released_at IS NOT NULL FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2`, owner.ID, snapshotID).Scan(&released))
		return released
	}

	start := perform(t, h, http.MethodPost, "/reading/books/"+bookA.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	first := current()
	require.Equal(t, bookA.ID, first.BookID)
	assert.ElementsMatch(t, []string{"haus", "baum"}, snapshotLemmas(first.SnapshotID))

	// Rejected switches make no change: CSRF, ownership, stale expectation.
	noCSRF := perform(t, h, http.MethodPost, "/reading/books/"+bookB.ID+"/switch", url.Values{
		"expected_current_book_id": {first.BookID}, "expected_current_snapshot_id": {first.SnapshotID},
	}, cookies)
	assert.GreaterOrEqual(t, noCSRF.Code, 400, "a switch without CSRF is rejected")
	foreign := perform(t, h, http.MethodPost, "/reading/books/"+bookB.ID+"/switch", url.Values{
		"csrf_token": {otherCSRF}, "expected_current_book_id": {first.BookID},
		"expected_current_snapshot_id": {first.SnapshotID},
	}, otherCookies)
	assert.NotContains(t, foreign.Header().Get("Location"), "now+your+current+reading", "another owner cannot switch to this Book")
	stale := switchTo(bookB, domain.CurrentReading{BookID: first.BookID, SnapshotID: "00000000-0000-0000-0000-000000000001"})
	assert.Contains(t, stale, "The+current+book+changed")
	assert.Equal(t, first, current(), "rejected switches leave the commitment untouched")
	assert.False(t, releasedAt(first.SnapshotID))

	// Known vocabulary changes before the next freeze must show up only in fresh snapshots.
	switched := switchTo(bookB, first)
	assert.Contains(t, switched, "is+now+your+current+reading")
	second := current()
	assert.Equal(t, bookB.ID, second.BookID)
	assert.NotEqual(t, first.SnapshotID, second.SnapshotID)
	assert.True(t, releasedAt(first.SnapshotID), "the former snapshot is released")
	assert.False(t, releasedAt(second.SnapshotID))
	assert.ElementsMatch(t, []string{"haus", "baum"}, snapshotLemmas(first.SnapshotID), "the released snapshot is preserved")
	detail, err := store.GetBookDetail(ctx, owner.ID, bookA.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition, "the former Book remains To Read")
	var completions, activeReservations int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&completions))
	assert.Zero(t, completions, "switching accepts nothing as Known")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goals WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&activeReservations))
	assert.Equal(t, 1, activeReservations, "at most one Current reading exists")

	// A replay of the successful switch changes nothing newer.
	replay := switchTo(bookB, first)
	assert.Contains(t, replay, "reading")
	assert.Equal(t, second, current())

	// Known import between releases and the return: the return is a fresh freeze.
	_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,'de','haus','NOUN')`, owner.ID)
	require.NoError(t, err)
	returned := switchTo(bookA, second)
	assert.Contains(t, returned, "is+now+your+current+reading")
	third := current()
	assert.Equal(t, bookA.ID, third.BookID)
	assert.NotEqual(t, first.SnapshotID, third.SnapshotID, "return never adopts the released snapshot")
	assert.Equal(t, []string{"baum"}, snapshotLemmas(third.SnapshotID), "the fresh freeze excludes newly Known vocabulary")
	assert.ElementsMatch(t, []string{"haus", "baum"}, snapshotLemmas(first.SnapshotID), "the historical snapshot is never rewritten")
	assert.True(t, releasedAt(second.SnapshotID))
}

func TestCurrentReadingSwitchBlocksOnUnresolvedLemmaReview(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "reading-switch-review-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	owner := createAccount(t, ctx, store, "reading-switch-review", "learner-password", false)
	seed := func(suffix, lemma string) domain.Book {
		t.Helper()
		book, _, corpus, _ := seedMigrationAnalyzedBook(t, ctx, store, owner.ID, suffix, "Review "+suffix, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN", OccurrenceCount: 3}})
		_, execErr := store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,$2,'de',$3,'NOUN',3,'[]','[]','{}')`, owner.ID, corpus.ID, lemma)
		require.NoError(t, execErr)
		require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
		return book
	}
	bookA := seed("review-a", "haus")
	bookB := seed("review-b", "garten")
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	start := perform(t, h, http.MethodPost, "/reading/books/"+bookA.ID+"/start", url.Values{"csrf_token": {csrf}}, cookies)
	require.Equal(t, http.StatusSeeOther, start.Code)
	first, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Equal(t, bookA.ID, first.BookID)

	// An unresolved review flag on the target blocks the freeze, whatever its source.
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_review_flags(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,reason,evidence_provenance)
SELECT owner_id,book_id,corpus_id,analysis_run_id,'unit',0,5,'Needs review','{}'::jsonb
FROM (SELECT a.owner_id, a.book_id, c.id AS corpus_id, a.analysis_run_id FROM book_current_analyses a JOIN corpora c ON c.owner_id=a.owner_id AND c.analysis_run_id=a.analysis_run_id WHERE a.owner_id=$1 AND a.book_id=$2) x`, owner.ID, bookB.ID)
	require.NoError(t, err)
	blocked := perform(t, h, http.MethodPost, "/reading/books/"+bookB.ID+"/switch", url.Values{
		"csrf_token": {csrf}, "expected_current_book_id": {first.BookID},
		"expected_current_snapshot_id": {first.SnapshotID},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, blocked.Code)
	assert.Contains(t, blocked.Header().Get("Location"), "/lemma-review")
	after, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, first, after, "a review-gated switch makes no partial change")
	var released bool
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT released_at IS NOT NULL FROM primary_goal_snapshots WHERE owner_id=$1 AND id=$2`, owner.ID, first.SnapshotID).Scan(&released))
	assert.False(t, released, "the former reservation stays intact")
}
