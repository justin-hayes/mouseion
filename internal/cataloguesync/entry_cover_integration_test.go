//go:build integration

package cataloguesync

import (
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/riverqueue/river"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (f *coverFixture) entryCoverWorker() *EntryCoverWorker {
	return &EntryCoverWorker{Connections: f.store, Catalogue: f.store, Reader: f.opdsService}
}

func (f *coverFixture) pendingEntryCoverArgs() []EntryCoverArgs {
	f.t.Helper()
	rows, err := f.store.Pool().Query(f.ctx, `SELECT args FROM river_job WHERE kind=$1 AND args->>'owner_id'=$2 AND state::text=ANY($3::text[]) ORDER BY id`, EntryCoverKind, f.ownerID, liveRiverStates())
	require.NoError(f.t, err)
	defer rows.Close()
	var args []EntryCoverArgs
	for rows.Next() {
		var raw []byte
		require.NoError(f.t, rows.Scan(&raw))
		var decoded EntryCoverArgs
		require.NoError(f.t, json.Unmarshal(raw, &decoded))
		args = append(args, decoded)
	}
	require.NoError(f.t, rows.Err())
	return args
}

func (f *coverFixture) runEntryCoverJob(args EntryCoverArgs) error {
	return f.entryCoverWorker().Work(f.ctx, &river.Job[EntryCoverArgs]{Args: args})
}

func (f *coverFixture) indexedEntry(identifier string) domain.CatalogueIndexEntry {
	f.t.Helper()
	entries, err := f.store.ListCatalogueEntries(f.ctx, f.ownerID, f.connection.ID)
	require.NoError(f.t, err)
	for _, entry := range entries {
		if entry.EntryIdentifier == identifier {
			return entry
		}
	}
	f.t.Fatalf("entry %q is not indexed", identifier)
	return domain.CatalogueIndexEntry{}
}

func (f *coverFixture) entryCover(identifier string) (domain.CatalogEntryCoverResource, error) {
	return f.store.GetCatalogEntryCoverResource(f.ctx, f.ownerID, f.connection.ID, identifier)
}

func newestEntryCoverArgs(t *testing.T, args []EntryCoverArgs) EntryCoverArgs {
	t.Helper()
	require.NotEmpty(t, args)
	newest := args[0]
	for _, candidate := range args[1:] {
		if candidate.AdvertisedAt.After(newest.AdvertisedAt) {
			newest = candidate
		}
	}
	return newest
}

func TestCatalogueSyncRetainsEntryCoversWithoutBooksAndRefreshesOnChange(t *testing.T) {
	f := newCoverFixture(t)
	entry := coverEntry{id: "entry-1", title: "Entry One", imageHref: "/a/image/one.png"}
	f.catalog.setImage(entry.imageHref, coverImage{body: testPNG(t, 10), contentType: "image/png"})
	f.catalog.setEntries("/a", entry, coverEntry{id: "entry-2", title: "Entry Two"})
	require.NoError(t, f.sync(f.connection.ID))

	// Only the entry that advertises an image is scheduled, and no Book exists.
	args := f.pendingEntryCoverArgs()
	require.Len(t, args, 1)
	assert.Equal(t, "entry-1", args[0].SourceIdentifier)
	assert.Empty(t, f.pendingCoverArgs())
	indexed := f.indexedEntry("entry-1")
	assert.False(t, indexed.CoverRetained, "a pending cover is not retained")

	require.NoError(t, f.runEntryCoverJob(args[0]))
	indexed = f.indexedEntry("entry-1")
	assert.True(t, indexed.CoverRetained)
	assert.Positive(t, indexed.CoverWidth)
	plain := f.indexedEntry("entry-2")
	assert.False(t, plain.CoverRetained)
	first, err := f.entryCover("entry-1")
	require.NoError(t, err)
	assert.Equal(t, "image/png", first.MediaType)
	_, err = f.entryCover("entry-2")
	require.ErrorIs(t, err, persistence.ErrNotFound)

	// An unchanged advertisement keeps the retained image and schedules nothing.
	require.NoError(t, f.sync(f.connection.ID))
	assert.Len(t, f.pendingEntryCoverArgs(), 1)

	// A changed advertised image starts a new generation; the prior image stays
	// until the replacement validates, and a stale completion is dropped.
	f.catalog.setImage("/a/image/two.png", coverImage{body: testPNG(t, 200), contentType: "image/png"})
	f.catalog.setEntries("/a", coverEntry{id: "entry-1", title: "Entry One", imageHref: "/a/image/two.png"}, coverEntry{id: "entry-2", title: "Entry Two"})
	require.NoError(t, f.sync(f.connection.ID))
	all := f.pendingEntryCoverArgs()
	require.Len(t, all, 2)
	current := newestEntryCoverArgs(t, all)
	still, err := f.entryCover("entry-1")
	require.NoError(t, err)
	assert.Equal(t, first.ContentHash, still.ContentHash)
	require.NoError(t, f.runEntryCoverJob(args[0]))
	still, err = f.entryCover("entry-1")
	require.NoError(t, err)
	assert.Equal(t, first.ContentHash, still.ContentHash, "stale generation replaced the image")
	require.NoError(t, f.runEntryCoverJob(current))
	replaced, err := f.entryCover("entry-1")
	require.NoError(t, err)
	assert.NotEqual(t, first.ContentHash, replaced.ContentHash)

	// A failed refresh keeps the retained image and is retried by River.
	f.catalog.setImage("/a/image/three.png", coverImage{body: []byte("not an image"), contentType: "image/png"})
	f.catalog.setEntries("/a", coverEntry{id: "entry-1", title: "Entry One", imageHref: "/a/image/three.png"}, coverEntry{id: "entry-2", title: "Entry Two"})
	require.NoError(t, f.sync(f.connection.ID))
	failing := newestEntryCoverArgs(t, f.pendingEntryCoverArgs())
	require.Error(t, f.runEntryCoverJob(failing))
	kept, err := f.entryCover("entry-1")
	require.NoError(t, err)
	assert.Equal(t, replaced.ContentHash, kept.ContentHash)
}

func TestCatalogueSyncRemovesEntryCoverOnAbsenceAndWhenEntryLeavesIndex(t *testing.T) {
	f := newCoverFixture(t)
	for _, id := range []string{"keep", "bare", "gone"} {
		f.catalog.setImage("/a/image/"+id+".png", coverImage{body: testPNG(t, 30), contentType: "image/png"})
	}
	withImages := []coverEntry{
		{id: "keep", title: "Keep", imageHref: "/a/image/keep.png"},
		{id: "bare", title: "Bare", imageHref: "/a/image/bare.png"},
		{id: "gone", title: "Gone", imageHref: "/a/image/gone.png"},
	}
	f.catalog.setEntries("/a", withImages...)
	require.NoError(t, f.sync(f.connection.ID))
	for _, args := range f.pendingEntryCoverArgs() {
		require.NoError(t, f.runEntryCoverJob(args))
	}
	for _, id := range []string{"keep", "bare", "gone"} {
		_, err := f.entryCover(id)
		require.NoError(t, err, id)
	}

	// An incomplete run (feed error) is not explicit absence and changes nothing.
	f.catalog.setFeedError(true)
	require.Error(t, f.sync(f.connection.ID))
	_, err := f.entryCover("bare")
	require.NoError(t, err)
	f.catalog.setFeedError(false)

	// Explicit absence on a complete run removes that image; an entry that left
	// the index loses its image too; the other keeps its own.
	f.catalog.setEntries("/a", withImages[0], coverEntry{id: "bare", title: "Bare"})
	require.NoError(t, f.sync(f.connection.ID))
	_, err = f.entryCover("keep")
	require.NoError(t, err)
	_, err = f.entryCover("bare")
	require.ErrorIs(t, err, persistence.ErrNotFound)
	_, err = f.entryCover("gone")
	require.ErrorIs(t, err, persistence.ErrNotFound)

	// A late job for a removed entry changes nothing and does not error.
	for _, args := range f.pendingEntryCoverArgs() {
		if args.SourceIdentifier == "gone" {
			require.NoError(t, f.runEntryCoverJob(args))
		}
	}
	_, err = f.entryCover("gone")
	require.ErrorIs(t, err, persistence.ErrNotFound)

	// Deleting the connection removes the covers of its entries.
	require.NoError(t, f.store.DeleteOpdsConnection(f.ctx, f.ownerID, f.connection.ID))
	var remaining int
	require.NoError(t, f.store.Pool().QueryRow(f.ctx, `SELECT count(*) FROM catalog_entry_covers WHERE owner_id=$1`, f.ownerID).Scan(&remaining))
	assert.Zero(t, remaining)
}

func TestEntryCoverIsOwnerScoped(t *testing.T) {
	f := newCoverFixture(t)
	f.catalog.setImage("/a/image/one.png", coverImage{body: testPNG(t, 10), contentType: "image/png"})
	f.catalog.setEntries("/a", coverEntry{id: "entry-1", title: "Entry One", imageHref: "/a/image/one.png"})
	require.NoError(t, f.sync(f.connection.ID))
	require.NoError(t, f.runEntryCoverJob(f.pendingEntryCoverArgs()[0]))
	other, err := f.store.CreateUser(f.ctx, "other-owner", false)
	require.NoError(t, err)
	_, err = f.store.GetCatalogEntryCoverResource(f.ctx, other.ID, f.connection.ID, "entry-1")
	require.ErrorIs(t, err, persistence.ErrNotFound)
}
