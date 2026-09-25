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
