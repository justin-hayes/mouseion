//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookCoverEndpointRequiresActiveMyBooksMembership(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "book-cover-endpoint-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	alice := createAccount(t, ctx, store, "cover-alice", "alice-password", false)
	bob := createAccount(t, ctx, store, "cover-bob", "bob-password", false)
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Cover catalog", URL: "https://cover.example/opds"})
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Retained cover", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	_, advertisedAt, err := store.RecordBookCoverAdvertisement(ctx, alice.ID, book.ID, connection.ID, "cover-entry", true)
	require.NoError(t, err)
	require.NoError(t, store.SaveBookCover(ctx, alice.ID, book.ID, connection.ID, "cover-entry", advertisedAt, "image/png", 1, 1, "cover-v1", []byte("cover bytes")))

	unavailable, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Unavailable cover", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	_, unavailableAt, err := store.RecordBookCoverAdvertisement(ctx, alice.ID, unavailable.ID, connection.ID, "unavailable-entry", true)
	require.NoError(t, err)
	require.NoError(t, store.MarkBookCoverUnavailable(ctx, alice.ID, unavailable.ID, connection.ID, "unavailable-entry", unavailableAt, "fetch failed"))

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		Capabilities: readyGerman(), SessionLifetime: time.Hour,
	})
	aliceCookies, _ := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, _ := loginCookies(t, h, bob.Username, "bob-password")

	active := perform(t, h, http.MethodGet, "/books/"+book.ID+"/cover", nil, aliceCookies)
	require.Equal(t, http.StatusOK, active.Code)
	assert.Equal(t, []byte("cover bytes"), active.Body.Bytes())
	assert.Equal(t, `"cover-v1"`, active.Header().Get("ETag"))
	assert.Equal(t, "image/png", active.Header().Get("Content-Type"))
	assert.Equal(t, "private, no-cache", active.Header().Get("Cache-Control"))

	conditionalRequest := httptest.NewRequestWithContext(ctx, http.MethodGet, "/books/"+book.ID+"/cover", nil)
	conditionalRequest.Header.Set("If-None-Match", `"cover-v1"`)
	for _, cookie := range aliceCookies {
		conditionalRequest.AddCookie(cookie)
	}
	conditionalResponse := httptest.NewRecorder()
	h.ServeHTTP(conditionalResponse, conditionalRequest)
	assert.Equal(t, http.StatusNotModified, conditionalResponse.Code)
	assert.Empty(t, conditionalResponse.Body.Bytes())

	require.NoError(t, store.RemoveBookFromMyBooks(ctx, alice.ID, book.ID))
	removed := perform(t, h, http.MethodGet, "/books/"+book.ID+"/cover", nil, aliceCookies)
	assert.Equal(t, http.StatusNotFound, removed.Code)
	assert.Equal(t, []byte("cover bytes"), mustRetainedCoverBytes(t, store, ctx, alice.ID, book.ID))
	removedConditional := coverRequest(t, h, book.ID, aliceCookies, `"cover-v1"`)
	assert.Equal(t, http.StatusNotFound, removedConditional.Code)

	missing := perform(t, h, http.MethodGet, "/books/"+uuid.NewString()+"/cover", nil, aliceCookies)
	unavailableResponse := perform(t, h, http.MethodGet, "/books/"+unavailable.ID+"/cover", nil, aliceCookies)
	crossOwner := perform(t, h, http.MethodGet, "/books/"+book.ID+"/cover", nil, bobCookies)
	for _, response := range []*httptest.ResponseRecorder{missing, unavailableResponse, crossOwner} {
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(t, removed.Body.String(), response.Body.String())
	}

	require.NoError(t, store.AddBookToMyBooks(ctx, alice.ID, book.ID))
	reactivated := perform(t, h, http.MethodGet, "/books/"+book.ID+"/cover", nil, aliceCookies)
	assert.Equal(t, http.StatusOK, reactivated.Code)
	assert.Equal(t, []byte("cover bytes"), reactivated.Body.Bytes())
	assert.Equal(t, `"cover-v1"`, reactivated.Header().Get("ETag"))
}

func coverRequest(t *testing.T, h http.Handler, bookID string, cookies []*http.Cookie, etag string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/books/"+bookID+"/cover", nil)
	r.Header.Set("If-None-Match", etag)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func mustRetainedCoverBytes(t *testing.T, store *persistence.PostgresStore, ctx context.Context, owner, bookID string) []byte {
	t.Helper()
	resource, err := store.GetBookCoverResource(ctx, owner, bookID)
	require.NoError(t, err)
	return resource.Bytes
}
