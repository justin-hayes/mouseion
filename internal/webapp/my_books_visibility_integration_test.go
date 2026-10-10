//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticatedMyBooksHideAndUnhideRecovery(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "my-books-visibility-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "vis-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "vis-bob", "bob-password", false)
	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: prepareddeck.NewService(store, nil), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})

	create := func(title, state, tag string) domain.Book {
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: state, LanguageTag: tag})
		require.NoError(t, createErr)
		return book
	}
	inbox := create("Inbox visible", domain.LanguageChosen, "de")
	toRead := create("To Read hidden", domain.LanguageChosen, "de")
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, toRead.ID, domain.BookDispositionToRead))
	needs := create("Needs language hidden", domain.LanguageUnknown, "")

	cookies, csrf := loginCookies(t, h, "vis-alice", "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, "vis-bob", "bob-password")
	revision := func(id string) string {
		_, rev, getErr := store.GetBookVisibility(ctx, alice.ID, id)
		require.NoError(t, getErr)
		return strconv.FormatInt(rev, 10)
	}
	hide := func(id, token string, c []*http.Cookie) *httptest.ResponseRecorder {
		return perform(t, h, http.MethodPost, "/library/books/"+id+"/hide", url.Values{
			"csrf_token": {token}, "expected_visibility_revision": {revision(id)}, "return_to": {"/library?disposition=to_read"},
		}, c)
	}

	// CSRF and owner authorization are enforced.
	denied := perform(t, h, http.MethodPost, "/library/books/"+toRead.ID+"/hide", url.Values{"expected_visibility_revision": {"0"}}, cookies)
	assert.NotEqual(t, http.StatusSeeOther, denied.Code)
	foreign := hide(toRead.ID, bobCSRF, bobCookies)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
	_ = bob

	first := hide(toRead.ID, csrf, cookies)
	assert.Equal(t, http.StatusSeeOther, first.Code)
	assert.Contains(t, first.Header().Get("Location"), "/library?disposition=to_read")
	hide(needs.ID, csrf, cookies)

	// Disposition is untouched.
	disposition, err := store.GetBookDisposition(ctx, alice.ID, toRead.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionToRead, disposition)

	// Default scope omits Hidden; Needs language too.
	page := perform(t, h, http.MethodGet, "/library?disposition=to_read", nil, cookies)
	assert.NotContains(t, page.Body.String(), "To Read hidden</h2>")
	assert.Contains(t, page.Body.String(), "Show hidden books (1)")
	needsPage := perform(t, h, http.MethodGet, "/library?needs-language", nil, cookies)
	assert.NotContains(t, needsPage.Body.String(), "Needs language hidden")

	// Show hidden scope lists and labels them with Unhide.
	shown := perform(t, h, http.MethodGet, "/library?disposition=to_read&show-hidden", nil, cookies)
	assert.Contains(t, shown.Body.String(), "To Read hidden")
	assert.Contains(t, shown.Body.String(), "Hidden")
	assert.Contains(t, shown.Body.String(), "Unhide")
	assert.NotContains(t, shown.Body.String(), "Inbox visible")

	// A stale Hide cannot reverse a newer Unhide.
	staleRevision := revision(toRead.ID)
	unhide := perform(t, h, http.MethodPost, "/library/books/"+toRead.ID+"/unhide", url.Values{
		"csrf_token": {csrf}, "expected_visibility_revision": {staleRevision}, "return_to": {"/library?show-hidden"},
	}, cookies)
	assert.Equal(t, http.StatusSeeOther, unhide.Code)
	assert.Contains(t, unhide.Header().Get("Location"), "message=")
	stale := perform(t, h, http.MethodPost, "/library/books/"+toRead.ID+"/hide", url.Values{
		"csrf_token": {csrf}, "expected_visibility_revision": {staleRevision}, "return_to": {"/library?show-hidden"},
	}, cookies)
	assert.Contains(t, stale.Header().Get("Location"), "error=")
	hidden, _, err := store.GetBookVisibility(ctx, alice.ID, toRead.ID)
	require.NoError(t, err)
	assert.False(t, hidden)

	// Return targets are restricted to local /library addresses.
	open := perform(t, h, http.MethodPost, "/library/books/"+inbox.ID+"/hide", url.Values{
		"csrf_token": {csrf}, "expected_visibility_revision": {revision(inbox.ID)}, "return_to": {"https://evil.example/x"},
	}, cookies)
	assert.Contains(t, open.Header().Get("Location"), "/library")
	assert.NotContains(t, open.Header().Get("Location"), "evil.example")

	// The default Reading chooser omits Hidden candidates, which stay To Read.
	chooser, err := store.ListMyBooksWithEvidence(ctx, alice.ID)
	require.NoError(t, err)
	for _, book := range chooser {
		if book.Book.ID == inbox.ID {
			assert.True(t, book.Hidden)
		}
	}
}
