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

// readNextHarness is a Postgres-backed Handler with a signed-in learner who
// owns analysed German Books.
type readNextHarness struct {
	t       *testing.T
	ctx     context.Context
	store   *persistence.PostgresStore
	h       http.Handler
	owner   domain.User
	cookies []*http.Cookie
	csrf    string
}

func newReadNextHarness(t *testing.T) *readNextHarness {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "reading-read-next-secret-0123456789abcdef")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	owner := createAccount(t, ctx, store, "reading-read-next", "learner-password")
	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Analysis: analysis.NewService(store.Pool(), nil), AnalysisInsights: analysisinsights.NewService(store), SessionLifetime: time.Hour,
	})
	cookies, csrf := loginCookies(t, h, owner.Username, "learner-password")
	return &readNextHarness{t: t, ctx: ctx, store: store, h: h, owner: owner, cookies: cookies, csrf: csrf}
}

// toReadBook seeds an analysed German To Read Book.
func (s *readNextHarness) toReadBook(suffix string) domain.Book {
	s.t.Helper()
	book, _, _ := seedMigrationAnalyzedBook(s.t, s.ctx, s.store, s.owner.ID, suffix, "Read next "+suffix, []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3}})
	require.NoError(s.t, s.store.SetBookDisposition(s.ctx, s.owner.ID, book.ID, domain.BookDispositionToRead))
	return book
}

// post submits the form with no commitment fields: Read next, Finish and End
// take only the CSRF token.
func (s *readNextHarness) post(path string) *httptest.ResponseRecorder {
	s.t.Helper()
	return perform(s.t, s.h, http.MethodPost, path, url.Values{"csrf_token": {s.csrf}}, s.cookies)
}

func (s *readNextHarness) current() domain.CurrentReading {
	s.t.Helper()
	reading, err := s.store.GetCurrentReading(s.ctx, s.owner.ID, "de")
	require.NoError(s.t, err)
	return reading
}

func (s *readNextHarness) count(query string, args ...any) int {
	s.t.Helper()
	var n int
	require.NoError(s.t, s.store.Pool().QueryRow(s.ctx, query, args...).Scan(&n))
	return n
}

func TestReadNextStartsAndReplacesTheCurrentReadingWithoutConfirmation(t *testing.T) {
	s := newReadNextHarness(t)
	first, second := s.toReadBook("first"), s.toReadBook("second")

	started := s.post("/reading/books/" + first.ID + "/start")
	require.Equal(t, http.StatusSeeOther, started.Code)
	assert.Contains(t, started.Header().Get("Location"), "is+now+your+current+reading")
	assert.Equal(t, first.ID, s.current().BookID)

	again := s.post("/reading/books/" + first.ID + "/start")
	assert.Contains(t, again.Header().Get("Location"), "is+already+your+current+reading", "reading the current Book again is a repeat, not a conflict")

	switched := s.post("/reading/books/" + second.ID + "/switch")
	require.Equal(t, http.StatusSeeOther, switched.Code)
	assert.Contains(t, switched.Header().Get("Location"), "is+now+your+current+reading")
	assert.Equal(t, second.ID, s.current().BookID, "Read next replaces the current reading with no token")

	detail, err := s.store.GetBookDetail(s.ctx, s.owner.ID, first.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition, "the former Book stays To Read")
	assert.Zero(t, s.count(`SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1`, s.owner.ID), "Read next freezes no snapshot")
	assert.Zero(t, s.count(`SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, s.owner.ID), "Read next touches no Known vocabulary")
}

func TestReadNextIsNotGatedByLemmaReviewFlags(t *testing.T) {
	s := newReadNextHarness(t)
	book := s.toReadBook("flagged")
	_, err := s.store.Pool().Exec(s.ctx, `INSERT INTO occurrence_lemma_review_flags(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,reason,evidence_provenance)
SELECT owner_id,book_id,corpus_id,analysis_run_id,'unit',0,5,'Needs review','{}'::jsonb
FROM (SELECT a.owner_id, a.book_id, c.id AS corpus_id, a.analysis_run_id FROM book_current_analyses a JOIN corpora c ON c.owner_id=a.owner_id AND c.analysis_run_id=a.analysis_run_id WHERE a.owner_id=$1 AND a.book_id=$2) x`, s.owner.ID, book.ID)
	require.NoError(t, err)

	response := s.post("/reading/books/" + book.ID + "/start")
	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.NotContains(t, response.Header().Get("Location"), "lemma-review", "an unresolved flag no longer gates the Current reading")
	assert.Equal(t, book.ID, s.current().BookID)
}

func TestReadNextRefusalReasonsLeaveTheCurrentReadingUnchanged(t *testing.T) {
	s := newReadNextHarness(t)
	current := s.toReadBook("current")
	require.Equal(t, http.StatusSeeOther, s.post("/reading/books/"+current.ID+"/start").Code)

	notToRead, _, _ := seedMigrationAnalyzedBook(t, s.ctx, s.store, s.owner.ID, "inbox", "Read next inbox", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 3}})
	require.NoError(t, s.store.MarkBookPreviouslyRead(s.ctx, s.owner.ID, notToRead.ID))
	notAnalysed, err := s.store.CreateBook(s.ctx, domain.Book{OwnerID: s.owner.ID, Title: "Read next unanalysed", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	require.NoError(t, s.store.SetBookDisposition(s.ctx, s.owner.ID, notAnalysed.ID, domain.BookDispositionToRead))
	noLanguage, err := s.store.CreateBook(s.ctx, domain.Book{OwnerID: s.owner.ID, Title: "Read next no language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	require.NoError(t, s.store.SetBookDisposition(s.ctx, s.owner.ID, noLanguage.ID, domain.BookDispositionToRead))

	tests := []struct {
		name, bookID, message string
	}{
		{"not To Read", notToRead.ID, "no+longer+an+eligible+To+Read+candidate"},
		{"not analysed", notAnalysed.ID, "no+longer+has+trustworthy+current+analysis"},
		{"no Book language", noLanguage.ID, "has+no+language+yet"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := s.post("/reading/books/" + tt.bookID + "/start")
			require.Equal(t, http.StatusSeeOther, response.Code)
			assert.Contains(t, response.Header().Get("Location"), tt.message)
			assert.Equal(t, current.ID, s.current().BookID, "a refusal changes nothing")
		})
	}
	t.Run("unknown Book", func(t *testing.T) {
		response := s.post("/reading/books/00000000-0000-0000-0000-000000000000/start")
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(t, current.ID, s.current().BookID)
	})
}

func TestFinishAppendsACompletionAndClearsTheRoleWithNoVocabularyEffect(t *testing.T) {
	s := newReadNextHarness(t)
	book := s.toReadBook("finish")

	noReading := s.post("/reading/finish")
	require.Equal(t, http.StatusSeeOther, noReading.Code)
	assert.Contains(t, noReading.Header().Get("Location"), "error=", "Finish with nothing to finish says so")
	assert.Zero(t, s.count(`SELECT count(*) FROM reading_history WHERE owner_id=$1`, s.owner.ID))

	require.Equal(t, http.StatusSeeOther, s.post("/reading/books/"+book.ID+"/start").Code)
	finished := s.post("/reading/finish")
	require.Equal(t, http.StatusOK, finished.Code, finished.Body.String())
	assert.Contains(t, finished.Body.String(), "Reading finished for")
	assert.NotContains(t, finished.Body.String(), "Known", "Finish reports no vocabulary effect")
	assert.False(t, s.current().IsActive())
	detail, err := s.store.GetBookDetail(s.ctx, s.owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, detail.Disposition, "Finish clears To Read")
	assert.Equal(t, 1, detail.CompletionCount)
	assert.Zero(t, s.count(`SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, s.owner.ID))
	assert.Zero(t, s.count(`SELECT count(*) FROM study_progress WHERE owner_id=$1`, s.owner.ID), "Finish changes no Study progress")

	again := s.post("/reading/finish")
	assert.Contains(t, again.Header().Get("Location"), "error=")
	assert.Equal(t, 1, s.count(`SELECT count(*) FROM reading_history WHERE owner_id=$1`, s.owner.ID), "a second Finish appends nothing")
}

func TestEndClearsTheRoleAndKeepsTheBookToRead(t *testing.T) {
	s := newReadNextHarness(t)
	book := s.toReadBook("end")

	none := s.post("/reading/end")
	require.Equal(t, http.StatusSeeOther, none.Code)
	assert.Contains(t, none.Header().Get("Location"), "error=", "End with nothing to end says so")

	require.Equal(t, http.StatusSeeOther, s.post("/reading/books/"+book.ID+"/start").Code)
	ended := s.post("/reading/end")
	require.Equal(t, http.StatusSeeOther, ended.Code)
	assert.Contains(t, ended.Header().Get("Location"), "Reading+ended")
	assert.False(t, s.current().IsActive())
	detail, err := s.store.GetBookDetail(s.ctx, s.owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, detail.Disposition)
	assert.Zero(t, detail.CompletionCount, "End records no completion")
}
