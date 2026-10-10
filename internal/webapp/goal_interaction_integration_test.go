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
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoalInteractionIntegrationKeepsReadingOnlyBooksAndOwnerBoundaries(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "goal-interaction-integration-secret-0123456789")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	authService := auth.New(store, time.Hour)
	alice := createAccount(t, ctx, store, "goal-integration-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "goal-integration-bob", "bob-password", false)
	newBook := func(owner domain.User, title string) domain.Book {
		t.Helper()
		book, createErr := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, createErr)
		return book
	}
	readingOnly := newBook(alice, "Reading-only integration book")
	replacement := newBook(alice, "Replacement integration book")

	h := New(Services{Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store), PreparedDeck: prepareddeck.NewService(store, nil), CatalogueSync: fixtures.NewCatalogueSync(fixtures.NewStore()), Analysis: fixtures.Analysis{}, SessionLifetime: time.Hour})
	aliceCookies, aliceCSRF := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, bobCSRF := loginCookies(t, h, bob.Username, "bob-password")

	for _, test := range []struct {
		path string
		form url.Values
	}{
		{"/goal/books/" + readingOnly.ID, url.Values{"csrf_token": {aliceCSRF}, "expected_goal_book_id": {""}}},
		{"/goal/books/" + replacement.ID, url.Values{"csrf_token": {aliceCSRF}, "expected_goal_book_id": {"stale"}}},
		{"/goal/clear", url.Values{"csrf_token": {aliceCSRF}, "expected_goal_book_id": {readingOnly.ID}}},
		{"/goal/finish", url.Values{"csrf_token": {aliceCSRF}, "expected_goal_book_id": {readingOnly.ID}, "expected_goal_snapshot_id": {"stale"}}},
	} {
		response := perform(t, h, http.MethodPost, test.path, test.form, aliceCookies)
		assert.Equalf(t, http.StatusNotFound, response.Code, "retired Goal mutation %s", test.path)
	}
	foreign := perform(t, h, http.MethodPost, "/goal/books/"+readingOnly.ID, url.Values{"csrf_token": {bobCSRF}}, bobCookies)
	assert.Equal(t, http.StatusNotFound, foreign.Code)
	for _, account := range []domain.User{alice, bob} {
		current, currentErr := store.GetCurrentReading(ctx, account.ID, "de")
		require.NoError(t, currentErr)
		assert.Empty(t, current.BookID)
	}
	for _, book := range []domain.Book{readingOnly, replacement} {
		disposition, dispositionErr := store.GetBookDisposition(ctx, alice.ID, book.ID)
		require.NoError(t, dispositionErr)
		assert.Equal(t, domain.BookDispositionInbox, disposition)
	}
}
