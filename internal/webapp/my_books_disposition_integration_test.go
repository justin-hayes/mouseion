//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
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
	journeyBook := create("Journey book")
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", journeyBook.ID, journey.Revision)
	require.NoError(t, err)
	setAside := create("Set aside book")
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, setAside.ID, domain.BookDispositionSetAside))
	otherInbox := create("Another inbox book")

	cookies, csrf := loginCookies(t, h, "alice", "alice-password")
	inboxPage := perform(t, h, http.MethodGet, "/library?disposition=inbox", nil, cookies)
	assert.Equal(t, http.StatusOK, inboxPage.Code)
	assert.Contains(t, inboxPage.Body.String(), "Inbox book")
	assert.NotContains(t, inboxPage.Body.String(), "Journey book")
	assert.Contains(t, inboxPage.Body.String(), "Inbox (2)")

	toRead := perform(t, h, http.MethodPost, "/library/books/"+inbox.ID+"/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"1"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, toRead.Code)
	assert.Contains(t, toRead.Header().Get("Location"), "disposition=to_read")
	actualDisposition, err := store.GetBookDisposition(ctx, alice.ID, inbox.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, actualDisposition)

	toReadPage := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.Equal(t, http.StatusOK, toReadPage.Code)
	assert.Contains(t, toReadPage.Body.String(), "Inbox book")
	assert.Contains(t, toReadPage.Body.String(), "Journey book")
	assert.NotContains(t, toReadPage.Body.String(), "Set aside book")
	assert.Contains(t, toReadPage.Body.String(), "Set Aside")

	setAsideResponse := perform(t, h, http.MethodPost, "/library/books/"+otherInbox.ID+"/set-aside", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"2"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, setAsideResponse.Code)
	assert.Contains(t, setAsideResponse.Header().Get("Location"), "disposition=set_aside")
	actualDisposition, err = store.GetBookDisposition(ctx, alice.ID, otherInbox.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, actualDisposition)
	removedBook := create("Removed from My Books")
	require.NoError(t, store.RemoveBookFromMyBooks(ctx, alice.ID, removedBook.ID))
	staleMove := perform(t, h, http.MethodPost, "/library/books/"+removedBook.ID+"/to-read", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"2"},
	}, cookies)
	assert.Equal(t, http.StatusNotFound, staleMove.Code, "a stale Inbox form must not restore removed My Books membership")
	actualDisposition, err = store.GetBookDisposition(ctx, alice.ID, removedBook.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionSetAside, actualDisposition)
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
	retry := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/previously-read", mark, cookies)
	assert.Equal(t, http.StatusSeeOther, retry.Code)

	readPage := perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
	assert.Equal(t, http.StatusOK, readPage.Code)
	assert.Contains(t, readPage.Body.String(), "Previously read book")
	assert.Contains(t, readPage.Body.String(), "Read before Mouseion")
	assert.Contains(t, readPage.Body.String(), "1 completion")
	assert.Contains(t, readPage.Body.String(), "Read again")
	assert.Contains(t, readPage.Body.String(), "Workflow</strong>: Inbox")

	var historyCount, knownCount, snapshotCount, eligibleCount int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*), max(snapshot_vocabulary_count), max(eligible_vocabulary_count) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&historyCount, &snapshotCount, &eligibleCount))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, alice.ID).Scan(&knownCount))
	assert.Equal(t, 1, historyCount, "retries should leave one imported history record")
	assert.Zero(t, snapshotCount, "an imported completion must not invent a vocabulary snapshot")
	assert.Zero(t, eligibleCount, "an imported completion must not invent eligible vocabulary")
	assert.Zero(t, knownCount, "importing reading history must not mark vocabulary known")

	reread := perform(t, h, http.MethodPost, "/library/books/"+book.ID+"/read-again", url.Values{
		"csrf_token": {csrf}, "expected_revision": {"0"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, reread.Code)
	assert.Contains(t, reread.Header().Get("Location"), "disposition=to_read")
	readDisposition, err := store.GetBookDisposition(ctx, alice.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, readDisposition)
	readPage = perform(t, h, http.MethodGet, "/library?history=read", nil, cookies)
	assert.Contains(t, readPage.Body.String(), "Previously read book", "Read remains an independent history projection")
	assert.Contains(t, readPage.Body.String(), "Workflow</strong>: To Read")
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, alice.ID, book.ID).Scan(&historyCount))
	assert.Equal(t, 1, historyCount, "rereading must preserve prior history")
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	require.Len(t, journey.Entries, 1, "reading again should return the Book to the Reading")
	assert.Equal(t, book.ID, journey.Entries[0].BookID)
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, alice.ID).Scan(&knownCount))
	assert.Zero(t, knownCount, "rereading must not mark vocabulary Known")
}
