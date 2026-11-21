//go:build integration

package cataloguesync

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/opds"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// indexReader serves a fixed language navigation feed and per-language entry
// feeds, with optional per-feed failures, so index reconciliation can be
// exercised through the worker's public seam.
type indexReader struct {
	languages opds.Feed
	feeds     map[string]opds.Feed
	fail      map[string]error
}

func (r *indexReader) Languages(context.Context, string, string) (opds.Feed, error) {
	return r.languages, nil
}

func (r *indexReader) BrowseLanguage(_ context.Context, _, _, languageID string) (opds.Feed, error) {
	if err := r.fail[languageID]; err != nil {
		return opds.Feed{}, err
	}
	return r.feeds[languageID], nil
}

func (r *indexReader) BrowseLanguageUnfiltered(ctx context.Context, owner, connection, languageID string) (opds.Feed, error) {
	return r.BrowseLanguage(ctx, owner, connection, languageID)
}

func indexEntry(id, title, author, languageType, href string) opds.Entry {
	entry := opds.Entry{ID: id, Title: title, Author: author, Links: []opds.Link{{Rel: opds.AcquisitionRel, Type: opds.EPUBMediaType, Href: href}}}
	if languageType != "" {
		entry.Links = append(entry.Links, opds.Link{Rel: opds.CoverImageRel, Type: languageType, Href: "https://catalog.example/" + id + ".png"})
	}
	return entry
}

func newCatalogueIndexFixture(t *testing.T) (*persistence.PostgresStore, domain.User, domain.OpdsConnection, *indexReader, *Worker) {
	t.Helper()
	t.Setenv("MOUSEION_SECRET", "catalogue-index-integration-secret")
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "catalogue index store", store.Close)
	owner, err := store.CreateUser(ctx, "catalogue-index-owner", false)
	require.NoError(t, err)
	connection, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Index catalog", URL: "https://catalog.example/opds"})
	require.NoError(t, err)
	reader := &indexReader{
		languages: opds.Feed{Entries: []opds.Entry{
			{Title: "German", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/7"}}},
			{Title: "Greek", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/11"}}},
			{Title: "French", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/10"}}},
			{Title: "Italian", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/9"}}},
			{Title: "English", Links: []opds.Link{{Rel: "subsection", Href: "https://catalog.example/language/8"}}},
		}},
		feeds: map[string]opds.Feed{
			"7":  {Entries: []opds.Entry{indexEntry("entry-de", "German title", "German author", "image/png", "https://catalog.example/de.epub")}},
			"11": {Entries: []opds.Entry{indexEntry("entry-el", "Greek title", "", "", "https://catalog.example/el.epub")}},
			"10": {Entries: []opds.Entry{indexEntry("entry-fr", "French title", "French author", "image/jpeg", "https://catalog.example/fr.epub")}},
			"9":  {Entries: []opds.Entry{indexEntry("entry-it", "Italian title", "", "", "https://catalog.example/it.epub")}},
			"8":  {Entries: []opds.Entry{indexEntry("entry-en", "English title", "", "", "https://catalog.example/en.epub")}},
		},
	}
	capabilities := fakeCapabilities{value: analyzer.Capabilities{Languages: []analyzer.LanguageCapability{
		{Language: "de", DisplayName: "German", Ready: true},
		{Language: "el", DisplayName: "Greek", Ready: true},
		{Language: "fr", DisplayName: "French", Ready: true},
		{Language: "it", DisplayName: "Italian", Ready: false},
		{Language: "en", DisplayName: "English", Ready: true},
	}}}
	worker := &Worker{Connections: store, Catalogue: store, Statuses: store, Reader: reader, Capabilities: capabilities}
	return store, owner, connection, reader, worker
}

func syncJob(ownerID, connectionID string) *river.Job[SyncArgs] {
	return &river.Job[SyncArgs]{Args: SyncArgs{OwnerID: ownerID, ConnectionID: connectionID}}
}

func TestSyncWorkerMaintainsTheCatalogueIndex(t *testing.T) {
	ctx := context.Background()
	store, owner, connection, reader, worker := newCatalogueIndexFixture(t)
	job := syncJob(owner.ID, connection.ID)
	require.NoError(t, worker.Work(ctx, job))

	entries, err := store.ListCatalogueEntries(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	require.Len(t, entries, 3, "index did not hold exactly the ready non-English entries: %+v", entries)
	byIdentifier := make(map[string]domain.CatalogueIndexEntry, len(entries))
	for _, entry := range entries {
		byIdentifier[entry.EntryIdentifier] = entry
	}
	german, ok := byIdentifier["entry-de"]
	require.True(t, ok)
	assert.Equal(t, "German title", german.Title)
	assert.Equal(t, "German author", german.Author)
	assert.Equal(t, "de", german.LanguageTag)
	assert.Equal(t, "https://catalog.example/entry-de.png", german.CoverHref)
	assert.Equal(t, "image/png", german.CoverMediaType)
	assert.Equal(t, "https://catalog.example/de.epub", german.EPUBHref)
	assert.False(t, german.LastSeenSync.IsZero())
	greek, ok := byIdentifier["entry-el"]
	require.True(t, ok)
	assert.Empty(t, greek.CoverHref, "entry without a cover advertised one")
	assert.Equal(t, "el", greek.LanguageTag)
	assert.NotContains(t, byIdentifier, "entry-it", "not-ready language entered the index")
	assert.NotContains(t, byIdentifier, "entry-en", "English entered the index")

	books, err := store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, books, 3, "index sync changed existing Book creation")

	// Repeating the run is idempotent.
	require.NoError(t, worker.Work(ctx, job))
	again, err := store.ListCatalogueEntries(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	assert.Len(t, again, 3)
	for _, entry := range again {
		assert.False(t, entry.LastSeenSync.Before(byIdentifier[entry.EntryIdentifier].LastSeenSync), "resync moved last-seen backwards")
	}

	// Upstream metadata changes update the entry.
	reader.feeds["7"] = opds.Feed{Entries: []opds.Entry{indexEntry("entry-de", "Renamed title", "Renamed author", "", "https://catalog.example/de-new.epub")}}
	require.NoError(t, worker.Work(ctx, job))
	renamed, err := store.GetCatalogueEntry(ctx, owner.ID, connection.ID, "entry-de")
	require.NoError(t, err)
	assert.Equal(t, "Renamed title", renamed.Title)
	assert.Equal(t, "Renamed author", renamed.Author)
	assert.Empty(t, renamed.CoverHref, "cover removal did not update the index")
	assert.Equal(t, "https://catalog.example/de-new.epub", renamed.EPUBHref)

	// A complete run deletes entries it no longer saw; the Book stays.
	reader.feeds["7"] = opds.Feed{}
	require.NoError(t, worker.Work(ctx, job))
	entries, err = store.ListCatalogueEntries(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	for _, entry := range entries {
		assert.NotEqual(t, "entry-de", entry.EntryIdentifier, "entry missing from a complete run was not deleted")
	}
	books, err = store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, books, 3, "index deletion changed Books")
}

func TestSyncWorkerFailedRunLeavesTheCatalogueIndexUnchanged(t *testing.T) {
	ctx := context.Background()
	store, owner, connection, reader, worker := newCatalogueIndexFixture(t)
	job := syncJob(owner.ID, connection.ID)
	require.NoError(t, worker.Work(ctx, job))
	before, err := store.ListCatalogueEntries(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	require.Len(t, before, 3)

	// The German feed offers new metadata, but the French feed fails after the
	// traversal has already reconciled German Books. The index must not move.
	reader.feeds["7"] = opds.Feed{Entries: []opds.Entry{indexEntry("entry-de", "Partially seen title", "", "", "https://catalog.example/de.epub"), indexEntry("entry-new", "New title", "", "", "https://catalog.example/new.epub")}}
	reader.feeds["10"] = opds.Feed{}
	reader.fail = map[string]error{"10": errors.New("feed unavailable")}
	err = worker.Work(ctx, job)
	require.Error(t, err)
	after, listErr := store.ListCatalogueEntries(ctx, owner.ID, connection.ID)
	require.NoError(t, listErr)
	assert.Equal(t, before, after, "a failed run changed the index")
}

func TestSyncWorkerScopesSameEntryIDPerConnection(t *testing.T) {
	ctx := context.Background()
	store, owner, first, _, worker := newCatalogueIndexFixture(t)
	second, err := store.CreateOpdsConnection(ctx, owner.ID, domain.OpdsConnection{Name: "Second index catalog", URL: "https://second.example/opds"})
	require.NoError(t, err)
	require.NoError(t, worker.Work(ctx, syncJob(owner.ID, first.ID)))
	require.NoError(t, worker.Work(ctx, syncJob(owner.ID, second.ID)))

	firstEntries, err := store.ListCatalogueEntries(ctx, owner.ID, first.ID)
	require.NoError(t, err)
	secondEntries, err := store.ListCatalogueEntries(ctx, owner.ID, second.ID)
	require.NoError(t, err)
	require.Len(t, firstEntries, len(secondEntries))
	for i := range firstEntries {
		assert.Equal(t, firstEntries[i].EntryIdentifier, secondEntries[i].EntryIdentifier)
		assert.Equal(t, first.ID, firstEntries[i].ConnectionID)
		assert.Equal(t, second.ID, secondEntries[i].ConnectionID)
	}
	assert.NotEqual(t, firstEntries[0].ConnectionID, secondEntries[0].ConnectionID, "same entry ID was not connection-scoped")
}

func TestDeletingConnectionRemovesItsCatalogueIndexAndKeepsBooks(t *testing.T) {
	ctx := context.Background()
	store, owner, connection, _, worker := newCatalogueIndexFixture(t)
	require.NoError(t, worker.Work(ctx, syncJob(owner.ID, connection.ID)))
	books, err := store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	require.Len(t, books, 3)
	require.NoError(t, store.DeleteOpdsConnection(ctx, owner.ID, connection.ID))

	entries, err := store.ListCatalogueEntries(ctx, owner.ID, connection.ID)
	require.NoError(t, err)
	assert.Empty(t, entries)
	var indexRows int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM catalogue_entries WHERE connection_id=$1`, connection.ID).Scan(&indexRows)
	require.NoError(t, err)
	assert.Zero(t, indexRows, "deleting a connection left its index rows behind")
	books, err = store.ListMyBooks(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, books, 3, "deleting a connection removed Books")
}
