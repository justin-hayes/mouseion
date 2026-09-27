//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedMyBooksDispositionFiltersAndTransitions(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "my-books-disposition-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "alice", "alice-password", false)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})

	create := func(title string) domain.Book {
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, createErr)
		return book
	}
	inbox := create("Inbox book")
	toReadBook := create("To Read book")
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, toReadBook.ID, domain.BookDispositionToRead))
	setAside := create("Set aside book")
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, setAside.ID, domain.BookDispositionSetAside))
	otherInbox := create("Another inbox book")
	expectedRevision := func(bookID string) string {
		detail, detailErr := store.GetBookDetail(ctx, alice.ID, bookID)
		require.NoError(t, detailErr)
		return strconv.FormatInt(detail.DispositionRevision, 10)
	}

	cookies, csrf := loginCookies(t, h, "alice", "alice-password")
	inboxPage := perform(t, h, http.MethodGet, "/library?disposition=inbox", nil, cookies)
	assert.Equal(t, http.StatusOK, inboxPage.Code)
	assert.Contains(t, inboxPage.Body.String(), "Inbox book")
	assert.NotContains(t, inboxPage.Body.String(), "To Read book")
	assert.Contains(t, inboxPage.Body.String(), "Inbox (2)")

	toRead := perform(t, h, http.MethodPost, "/library/books/"+inbox.ID+"/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {expectedRevision(inbox.ID)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, toRead.Code)
	assert.Contains(t, toRead.Header().Get("Location"), "disposition=to_read")
	actualDisposition, err := store.GetBookDisposition(ctx, alice.ID, inbox.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, actualDisposition)

	toReadPage := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Equal(t, http.StatusOK, toReadPage.Code)
	assert.Contains(t, toReadPage.Body.String(), "Inbox book")
	assert.Contains(t, toReadPage.Body.String(), "To Read book")
	assert.NotContains(t, toReadPage.Body.String(), "Set aside book")
	assert.Contains(t, toReadPage.Body.String(), "Set Aside")

	setAsideResponse := perform(t, h, http.MethodPost, "/library/books/"+otherInbox.ID+"/set-aside", url.Values{
		"csrf_token": {csrf}, "expected_revision": {expectedRevision(otherInbox.ID)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, setAsideResponse.Code)
	assert.Contains(t, setAsideResponse.Header().Get("Location"), "disposition=set_aside")
	actualDisposition, err = store.GetBookDisposition(ctx, alice.ID, otherInbox.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, actualDisposition)
	setAsidePage := perform(t, h, http.MethodGet, "/library?disposition=set_aside", nil, cookies)
	assert.Equal(t, http.StatusOK, setAsidePage.Code)
	assert.Contains(t, setAsidePage.Body.String(), "Another inbox book")
	assert.Contains(t, setAsidePage.Body.String(), "Set Aside")
	allBooksPage := perform(t, h, http.MethodGet, "/library", nil, cookies)
	assert.Equal(t, http.StatusOK, allBooksPage.Code)
	assert.Contains(t, allBooksPage.Body.String(), "Another inbox book", "Set Aside Books remain visible in All")
	moveBack := perform(t, h, http.MethodPost, "/library/books/"+otherInbox.ID+"/to-read", url.Values{"csrf_token": {csrf}, "expected_revision": {expectedRevision(otherInbox.ID)}}, cookies)
	assert.Equal(t, http.StatusSeeOther, moveBack.Code)
	actualDisposition, err = store.GetBookDisposition(ctx, alice.ID, otherInbox.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, actualDisposition)
	removedBook := create("Removed from My Books")
	removedRevision := expectedRevision(removedBook.ID)
	require.NoError(t, store.RemoveBookFromMyBooks(ctx, alice.ID, removedBook.ID))
	staleMove := perform(t, h, http.MethodPost, "/library/books/"+removedBook.ID+"/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {removedRevision},
	}, cookies)
	assert.Equal(t, http.StatusNotFound, staleMove.Code, "a stale Inbox form must not restore removed My Books membership")
	actualDisposition, err = store.GetBookDisposition(ctx, alice.ID, removedBook.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, actualDisposition)
}

func TestAuthenticatedMyBooksStaleDispositionFormsConflictAcrossTabs(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "my-books-stale-decision-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "stale-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "stale-bob", "bob-password", false)
	assert.NotEqual(t, alice.ID, bob.ID)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Concurrent decisions", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, "stale-alice", "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "stale-bob", "bob-password")
	initial, err := store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	form := func(csrf string, revision int64) url.Values {
		return url.Values{"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(revision, 10)}}
	}

	first := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/to-read", form(aliceCSRF, initial.DispositionRevision), aliceCookies)
	assert.Equal(t, http.StatusSeeOther, first.Code)
	replayed := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/to-read", form(aliceCSRF, initial.DispositionRevision), aliceCookies)
	assert.NotContains(t, replayed.Header().Get("Location"), "error=", "retrying the accepted form remains safe")
	afterFirst, err := store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	require.Equal(t, initial.DispositionRevision+1, afterFirst.DispositionRevision)

	competing := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/set-aside", form(aliceCSRF, afterFirst.DispositionRevision), aliceCookies)
	assert.NotContains(t, competing.Header().Get("Location"), "error=")
	afterAside, err := store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	returned := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/to-read", form(aliceCSRF, afterAside.DispositionRevision), aliceCookies)
	assert.NotContains(t, returned.Header().Get("Location"), "error=")

	stale := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/set-aside", form(aliceCSRF, initial.DispositionRevision), aliceCookies)
	assert.Contains(t, stale.Header().Get("Location"), "error=")
	assert.Contains(t, stale.Header().Get("Location"), "changed+in+another+tab")
	staleErrorPage := perform(t, h, http.MethodGet, stale.Header().Get("Location"), nil, aliceCookies)
	assert.Equal(t, http.StatusOK, staleErrorPage.Code)
	assert.Contains(t, staleErrorPage.Body.String(), `role="alert"`)
	assert.Contains(t, staleErrorPage.Body.String(), "changed in another tab")
	final, err := store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, final.Disposition, "the stale decision must not overwrite the later To Read decision")
	assert.Equal(t, afterAside.DispositionRevision+1, final.DispositionRevision)

	foreign := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/to-read", form(bobCSRF, final.DispositionRevision), bobCookies)
	assert.Equal(t, http.StatusNotFound, foreign.Code, "a revision token does not cross owner boundaries")
	for _, path := range []string{
		"/reading/books/" + book.ID + "/to-read",
		"/reading/books/" + book.ID + "/set-aside",
	} {
		retired := perform(t, h, http.MethodPost, path, url.Values{"csrf_token": {bobCSRF}}, bobCookies)
		assert.Equalf(t, http.StatusNotFound, retired.Code, "retired disposition route is unavailable to another owner: %s", path)
	}
	var historyRows, reservedRows, knownRows int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&historyRows))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM primary_goal_snapshots WHERE owner_id=$1 AND book_id=$2 AND released_at IS NULL`, alice.ID, book.ID).Scan(&reservedRows))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, alice.ID).Scan(&knownRows))
	assert.Zero(t, historyRows)
	assert.Zero(t, reservedRows)
	assert.Zero(t, knownRows)
}

func TestRetiredRemoveRequestCannotChangeMyBooksData(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "retired-remove-preservation-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	alice := createAccount(t, ctx, store, "retired-remove-owner", "owner-password", false)
	book, _, _, preparation := seedMigrationAnalyzedBook(t, ctx, store, alice.ID, "retired-remove", "Retained book", []domain.LemmaOccurrence{{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", OccurrenceCount: 1}})
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, book.ID, domain.BookDispositionSetAside))
	_, err = store.ImportPreviouslyRead(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	authService := auth.New(store, time.Hour)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	cookies, csrf := loginCookies(t, h, "retired-remove-owner", "owner-password")

	response := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/remove", url.Values{"csrf_token": {csrf}}, cookies)
	assert.Equal(t, http.StatusNotFound, response.Code)

	retained, err := store.GetBook(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, "Retained book", retained.Title)
	disposition, err := store.GetBookDisposition(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, disposition)
	sources, err := store.ListSourceMaterials(ctx, alice.ID)
	require.NoError(t, err)
	require.Len(t, sources, 1)
	assert.Equal(t, book.ID, sources[0].BookID)
	preparations, err := store.ListDeckPreparationsForSourceMaterial(ctx, alice.ID, sources[0].Source.ID)
	require.NoError(t, err)
	require.Len(t, preparations, 1)
	assert.Equal(t, preparation.ID, preparations[0].ID)
	var analyses, histories int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&analyses))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&histories))
	assert.Equal(t, 1, analyses)
	assert.Equal(t, 1, histories)
	readPage := perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
	assert.Contains(t, readPage.Body.String(), "Retained book")
}

func TestAuthenticatedPreviouslyReadHistoryAndRereading(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "my-books-previously-read-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "previously-read-alice", "alice-password", false)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), SessionLifetime: time.Hour})
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Previously read book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	cookies, csrf := loginCookies(t, h, "previously-read-alice", "alice-password")
	initialPage := perform(t, h, http.MethodGet, "/library", nil, cookies)
	assert.Contains(t, initialPage.Body.String(), "Mark as previously read")

	mark := url.Values{"csrf_token": {csrf}}
	first := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/previously-read", mark, cookies)
	assert.Equal(t, http.StatusSeeOther, first.Code)
	assert.Contains(t, first.Header().Get("Location"), "disposition=inbox", "the redirect should preserve Inbox visible-bucket precedence")
	assert.Contains(t, first.Header().Get("Location"), "Previously+read+history+recorded")
	redirected := perform(t, h, http.MethodGet, first.Header().Get("Location"), nil, cookies)
	assert.Equal(t, http.StatusOK, redirected.Code)
	assert.Contains(t, redirected.Body.String(), "Previously read book")
	assert.Contains(t, redirected.Body.String(), "Workflow</strong>: Inbox")
	assert.Contains(t, redirected.Body.String(), "Previously read history recorded. Vocabulary was not changed.")
	retry := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/previously-read", mark, cookies)
	assert.Equal(t, http.StatusSeeOther, retry.Code)
	assert.Contains(t, retry.Header().Get("Location"), "disposition=inbox")

	inboxPage := perform(t, h, http.MethodGet, "/library?disposition=inbox", nil, cookies)
	assert.Equal(t, http.StatusOK, inboxPage.Code)
	assert.Contains(t, inboxPage.Body.String(), "Previously read book")
	assert.Contains(t, inboxPage.Body.String(), "Read before Mouseion")
	assert.Contains(t, inboxPage.Body.String(), "1 completion")
	assert.Contains(t, inboxPage.Body.String(), "Read again")
	assert.Contains(t, inboxPage.Body.String(), "Workflow</strong>: Inbox")
	readPage := perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
	assert.NotContains(t, readPage.Body.String(), "Previously read book", "Read filter follows the visible bucket, not history alone")
	assert.Contains(t, inboxPage.Body.String(), "Read (0)", "Inbox history is not counted in Read")

	var historyCount, knownCount, snapshotCount, eligibleCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*), max(snapshot_vocabulary_count), max(eligible_vocabulary_count) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&historyCount, &snapshotCount, &eligibleCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, alice.ID).Scan(&knownCount))
	assert.Equal(t, 1, historyCount, "retries should leave one imported history record")
	assert.Zero(t, snapshotCount, "an imported completion must not invent a vocabulary snapshot")
	assert.Zero(t, eligibleCount, "an imported completion must not invent eligible vocabulary")
	assert.Zero(t, knownCount, "importing reading history must not mark vocabulary known")

	detail, err := store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	reread := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/read-again", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, reread.Code)
	assert.Contains(t, reread.Header().Get("Location"), "disposition=to_read")
	readDisposition, err := store.GetBookDisposition(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, readDisposition)
	readPage = perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Contains(t, readPage.Body.String(), "Previously read book")
	assert.Contains(t, readPage.Body.String(), "Workflow</strong>: To Read")
	assert.Contains(t, readPage.Body.String(), "To Read (1)")
	assert.Contains(t, readPage.Body.String(), "Read (0)", "To Read history is not counted in Read")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&historyCount))
	assert.Equal(t, 1, historyCount, "rereading must preserve prior history")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, alice.ID).Scan(&knownCount))
	assert.Zero(t, knownCount, "rereading must not mark vocabulary Known")

	// A historical Set Aside Book projects into Read. Moving it back to To Read
	// and setting it aside again changes only the visible bucket, not history.
	detail, err = store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	setAside := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/set-aside", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, setAside.Code)
	assert.Contains(t, setAside.Header().Get("Location"), "history=read", "setting aside a historical Book returns to its visible Read bucket")
	previouslyReadAgain := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/previously-read", mark, cookies)
	assert.Equal(t, http.StatusSeeOther, previouslyReadAgain.Code)
	assert.Contains(t, previouslyReadAgain.Header().Get("Location"), "history=read", "an idempotent assertion for a Book already in Read stays in its visible bucket")
	readPage = perform(t, h, http.MethodGet, previouslyReadAgain.Header().Get("Location"), nil, cookies)
	assert.Contains(t, readPage.Body.String(), "Previously read book")
	assert.Contains(t, readPage.Body.String(), "Workflow</strong>: Read")
	assert.Contains(t, readPage.Body.String(), "Read (1)")
	assert.Contains(t, readPage.Body.String(), "Reading history</strong>: 1 completion")
	assert.Contains(t, readPage.Body.String(), "Previously read history recorded. Vocabulary was not changed.")
	assert.NotContains(t, readPage.Body.String(), "Set Aside (1)", "history has one visible bucket")

	detail, err = store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	reconsider := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/read-again", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, reconsider.Code)
	toReadPage := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Contains(t, toReadPage.Body.String(), "Workflow</strong>: To Read")
	assert.Contains(t, toReadPage.Body.String(), "To Read (1)")
	assert.Contains(t, toReadPage.Body.String(), "Read (0)")

	detail, err = store.GetBookDetail(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	setAside = perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/set-aside", url.Values{
		"csrf_token": {csrf}, "expected_revision": {strconv.FormatInt(detail.DispositionRevision, 10)},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, setAside.Code)
	readPage = perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
	assert.Contains(t, readPage.Body.String(), "Workflow</strong>: Read")
	assert.Contains(t, readPage.Body.String(), "Read (1)")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&historyCount))
	assert.Equal(t, 1, historyCount, "visible bucket transitions must not rewrite append-only history")
}
