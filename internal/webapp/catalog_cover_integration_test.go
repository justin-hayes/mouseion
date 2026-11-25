//go:build integration

package webapp

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/internal/webauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCatalogEntryCoverEndpointAndCardAreOwnerScopedAndHideUpstreamURL(t *testing.T) {
	t.Setenv("MOUSEION_SECRET", "catalog-cover-endpoint-integration-secret-0123456789")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	alice := createAccount(t, ctx, store, "entry-cover-alice", "alice-password")
	bob := createAccount(t, ctx, store, "entry-cover-bob", "bob-password")
	connection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Cover catalog", URL: "https://cover.example/opds"})
	require.NoError(t, err)

	// The identifier holds "/" and ":" so the route must not carry it as a path.
	const entryID = "urn:uuid:a/b"
	observed := time.Now()
	require.NoError(t, store.ReconcileCatalogueIndex(ctx, alice.ID, connection.ID, []domain.CatalogueIndexEntry{
		{ConnectionID: connection.ID, EntryIdentifier: entryID, Title: "Covered", LanguageTag: "de", EPUBHref: "https://cover.example/a.epub", CoverHref: "https://upstream.example/secret-cover.png", CoverMediaType: "image/png"},
		{ConnectionID: connection.ID, EntryIdentifier: "plain", Title: "Plain", LanguageTag: "de", EPUBHref: "https://cover.example/b.epub"},
	}, observed))
	require.NoError(t, store.SaveCatalogEntryCover(ctx, alice.ID, connection.ID, entryID, observed.Truncate(persistence.CatalogEntryCoverFence), "image/png", 2, 3, "entry-v1", []byte("entry cover bytes")))

	authService := auth.New(store, time.Hour)
	h := New(Services{
		Auth: authService, WebAuth: webauth.New(authService, false, time.Hour), Store: storeDependencies(store),
		CatalogueSync: cataloguesync.NewService(cataloguesync.StoreDependencies{Connections: store, Catalogue: store, Statuses: store}, nil, nil, readyGerman()),
		Capabilities:  readyGerman(), SessionLifetime: time.Hour,
	})
	aliceCookies, _ := loginCookies(t, h, alice.Username, "alice-password")
	bobCookies, _ := loginCookies(t, h, bob.Username, "bob-password")
	coverPath := "/catalogs/sources/" + connection.ID + "/cover?entry=" + url.QueryEscape(entryID)

	served := perform(t, h, http.MethodGet, coverPath, nil, aliceCookies)
	require.Equal(t, http.StatusOK, served.Code)
	assert.Equal(t, []byte("entry cover bytes"), served.Body.Bytes())
	assert.Equal(t, `"entry-v1"`, served.Header().Get("ETag"))
	assert.Equal(t, "image/png", served.Header().Get("Content-Type"))
	assert.Equal(t, "private, no-cache", served.Header().Get("Cache-Control"))

	conditional := performWithHeader(t, h, http.MethodGet, coverPath, nil, aliceCookies, "If-None-Match", `"entry-v1"`)
	assert.Equal(t, http.StatusNotModified, conditional.Code)

	missing := perform(t, h, http.MethodGet, "/catalogs/sources/"+connection.ID+"/cover?entry=plain", nil, aliceCookies)
	crossOwner := perform(t, h, http.MethodGet, coverPath, nil, bobCookies)
	crossConditional := performWithHeader(t, h, http.MethodGet, coverPath, nil, bobCookies, "If-None-Match", `"entry-v1"`)
	badSource := perform(t, h, http.MethodGet, "/catalogs/sources/not-a-uuid/cover?entry=x", nil, aliceCookies)
	noEntry := perform(t, h, http.MethodGet, "/catalogs/sources/"+connection.ID+"/cover", nil, aliceCookies)
	for _, response := range []int{missing.Code, crossOwner.Code, crossConditional.Code, noEntry.Code, badSource.Code} {
		assert.Equal(t, http.StatusNotFound, response)
	}
	assert.Equal(t, missing.Body.String(), crossOwner.Body.String())

	page := perform(t, h, http.MethodGet, "/catalogs", nil, aliceCookies)
	require.Equal(t, http.StatusOK, page.Code)
	html := page.Body.String()
	assert.Contains(t, html, `class="book-cover-media__image"`)
	assert.Contains(t, html, "/catalogs/sources/"+connection.ID+"/cover?entry=urn%3Auuid%3Aa%2Fb")
	assert.Contains(t, html, `width="2" height="3"`)
	assert.NotContains(t, html, "upstream.example")
	assert.NotContains(t, html, "secret-cover")
	assert.Contains(t, html, "title-plate__title", "the entry without a retained cover keeps its Title plate")
}
