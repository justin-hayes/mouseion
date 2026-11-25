//go:build integration

package persistence

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// coverStoreFixture is one owner with a store and helpers that drive the
// entry-keyed cover store the way a Catalog sync does.
type coverStoreFixture struct {
	t     *testing.T
	ctx   context.Context
	store *PostgresStore
	owner string
}

func newCoverStoreFixture(t *testing.T, ownerName string) *coverStoreFixture {
	t.Helper()
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, ownerName, false)
	require.NoError(t, err)
	return &coverStoreFixture{t: t, ctx: ctx, store: store, owner: owner.ID}
}

func (f *coverStoreFixture) connection(name string) domain.OpdsConnection {
	f.t.Helper()
	return addFailureConnection(f.t, f.ctx, f.store, f.owner, name)
}

// coverEntry is one index entry; a non-empty cover advertises an image.
type coverEntry struct{ id, cover string }

// reconcile replaces the connection's index like one complete sync run and
// returns the retrieval generation the run started.
func (f *coverStoreFixture) reconcile(connectionID string, entries ...coverEntry) time.Time {
	f.t.Helper()
	indexed := make([]domain.CatalogueIndexEntry, 0, len(entries))
	for _, entry := range entries {
		indexed = append(indexed, domain.CatalogueIndexEntry{
			ConnectionID: connectionID, EntryIdentifier: entry.id, Title: "Title " + entry.id, LanguageTag: "de",
			EPUBHref: "https://catalog.example/" + entry.id + ".epub", CoverHref: entry.cover, CoverMediaType: "image/png",
		})
	}
	observedAt := time.Now()
	require.NoError(f.t, f.store.ReconcileCatalogueIndex(f.ctx, f.owner, connectionID, indexed, observedAt))
	return observedAt.Truncate(CatalogEntryCoverFence)
}

func (f *coverStoreFixture) save(connectionID, entryID string, generation time.Time, body string) {
	f.t.Helper()
	require.NoError(f.t, f.store.SaveCatalogEntryCover(f.ctx, f.owner, connectionID, entryID, generation, "image/png", 4, 6, "hash-"+body, []byte(body)))
}

// book adds the Book for an indexed entry through the production Add path.
func (f *coverStoreFixture) book(connectionID, entryID string) string {
	f.t.Helper()
	added, err := f.store.AddCatalogueEntry(f.ctx, f.owner, connectionID, entryID)
	require.NoError(f.t, err)
	return added.Book.ID
}

func (f *coverStoreFixture) alias(bookID, connectionID, entryID string) {
	f.t.Helper()
	_, err := f.store.Pool().Exec(f.ctx, `INSERT INTO book_aliases(owner_id, book_id, connection_id, alias_type, namespace, value) VALUES($1,$2,$3,$4,$5,$6)`,
		f.owner, bookID, connectionID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, entryID)
	require.NoError(f.t, err)
}

func (f *coverStoreFixture) state(bookID string) string {
	f.t.Helper()
	cover, err := f.store.GetBookCover(f.ctx, f.owner, bookID)
	require.NoError(f.t, err)
	return cover.State
}

func (f *coverStoreFixture) bytes(bookID string) string {
	f.t.Helper()
	resource, err := f.store.GetBookCoverResource(f.ctx, f.owner, bookID)
	require.NoError(f.t, err)
	return string(resource.Bytes)
}

func (f *coverStoreFixture) selected(bookID string) string {
	f.t.Helper()
	var entry string
	err := f.store.Pool().QueryRow(f.ctx, `SELECT COALESCE(max(selected_source_identifier), '') FROM book_covers WHERE owner_id=$1 AND book_id=$2`, f.owner, bookID).Scan(&entry)
	require.NoError(f.t, err)
	return entry
}

func (f *coverStoreFixture) entryRows() int {
	f.t.Helper()
	var count int
	require.NoError(f.t, f.store.Pool().QueryRow(f.ctx, `SELECT count(*) FROM catalog_entry_covers WHERE owner_id=$1`, f.owner).Scan(&count))
	return count
}

// TestBookCoverFirstSuccessSelectsAndStaysStable proves a Book resolves through
// the entry whose image validated first and that later aliases and
// replacements follow ADR 0077.
func TestBookCoverFirstSuccessSelectsAndStaysStable(t *testing.T) {
	f := newCoverStoreFixture(t, "first-success-owner")
	first, second := f.connection("First"), f.connection("Second")
	genFirst := f.reconcile(first.ID, coverEntry{"entry-1", "https://first.example/1.png"})
	genSecond := f.reconcile(second.ID, coverEntry{"entry-2", "https://second.example/2.png"})
	bookID := f.book(first.ID, "entry-1")
	f.alias(bookID, second.ID, "entry-2")

	// Both entries are pending: the Book reports a pending cover and serves none.
	assert.Equal(t, domain.BookCoverPending, f.state(bookID))
	_, err := f.store.GetBookCoverResource(f.ctx, f.owner, bookID)
	require.ErrorIs(t, err, ErrNotFound)

	// The later alias validates first and wins; the earlier one cannot displace it.
	f.save(second.ID, "entry-2", genSecond, "second-image")
	assert.Equal(t, "entry-2", f.selected(bookID))
	f.save(first.ID, "entry-1", genFirst, "first-image")
	assert.Equal(t, "entry-2", f.selected(bookID))
	assert.Equal(t, "second-image", f.bytes(bookID))

	// The selected entry's refresh replaces the image; a stale generation does not.
	replacement := f.reconcile(second.ID, coverEntry{"entry-2", "https://second.example/2-v2.png"})
	f.save(second.ID, "entry-2", genSecond, "stale-image")
	assert.Equal(t, "second-image", f.bytes(bookID))
	f.save(second.ID, "entry-2", replacement, "replacement-image")
	assert.Equal(t, "replacement-image", f.bytes(bookID))

	// A failed refresh keeps the retained image.
	failing := f.reconcile(second.ID, coverEntry{"entry-2", "https://second.example/2-v3.png"})
	require.NoError(t, f.store.MarkCatalogEntryCoverUnavailable(f.ctx, f.owner, second.ID, "entry-2", failing, "fetch failed"))
	assert.Equal(t, domain.BookCoverAvailable, f.state(bookID))
	assert.Equal(t, "replacement-image", f.bytes(bookID))
}

func TestBookCoverStateIsDerivedFromEntryRows(t *testing.T) {
	f := newCoverStoreFixture(t, "derived-state-owner")
	conn := f.connection("Catalog")
	gen := f.reconcile(conn.ID, coverEntry{"with-cover", "https://c.example/1.png"}, coverEntry{"bare", ""})
	covered, bare := f.book(conn.ID, "with-cover"), f.book(conn.ID, "bare")

	assert.Equal(t, domain.BookCoverPending, f.state(covered))
	assert.Equal(t, domain.BookCoverNone, f.state(bare))
	require.NoError(t, f.store.MarkCatalogEntryCoverUnavailable(f.ctx, f.owner, conn.ID, "with-cover", gen, "fetch failed"))
	assert.Equal(t, domain.BookCoverUnavailable, f.state(covered))
	f.save(conn.ID, "with-cover", gen, "late-image")
	assert.Equal(t, domain.BookCoverAvailable, f.state(covered), "a retried success replaces the failure")

	cover, err := f.store.GetBookCover(f.ctx, f.owner, covered)
	require.NoError(t, err)
	assert.Equal(t, 4, cover.Width)
	assert.Equal(t, 6, cover.Height)
	_, err = f.store.GetBookCover(f.ctx, f.owner, "00000000-0000-0000-0000-000000000000")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestBookCoverConcurrentFirstSuccessChoosesOneEntry(t *testing.T) {
	f := newCoverStoreFixture(t, "concurrent-cover-owner")
	first, second := f.connection("First"), f.connection("Second")
	genFirst := f.reconcile(first.ID, coverEntry{"entry-1", "https://first.example/1.png"})
	genSecond := f.reconcile(second.ID, coverEntry{"entry-2", "https://second.example/2.png"})
	bookID := f.book(first.ID, "entry-1")
	f.alias(bookID, second.ID, "entry-2")

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, candidate := range []struct {
		connection, entry string
		generation        time.Time
	}{{first.ID, "entry-1", genFirst}, {second.ID, "entry-2", genSecond}} {
		wg.Go(func() {
			errs <- f.store.SaveCatalogEntryCover(f.ctx, f.owner, candidate.connection, candidate.entry, candidate.generation, "image/png", 4, 6, "hash-"+candidate.entry, []byte("image-"+candidate.entry))
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var selections int
	require.NoError(t, f.store.Pool().QueryRow(f.ctx, `SELECT count(*) FROM book_covers WHERE owner_id=$1 AND book_id=$2`, f.owner, bookID).Scan(&selections))
	assert.Equal(t, 1, selections)
	assert.Equal(t, "image-"+f.selected(bookID), f.bytes(bookID))
}

// TestBookCoverExplicitAbsenceDemotesBeforeDeletingTheEntryRow covers ADR 0077
// explicit absence under the foreign key: the Book's selection is demoted (to
// another alias when one has an image) and then the entry row is deleted.
func TestBookCoverExplicitAbsenceDemotesBeforeDeletingTheEntryRow(t *testing.T) {
	f := newCoverStoreFixture(t, "absence-owner")
	first, second := f.connection("First"), f.connection("Second")
	genFirst := f.reconcile(first.ID, coverEntry{"entry-1", "https://first.example/1.png"}, coverEntry{"solo", "https://first.example/solo.png"})
	genSecond := f.reconcile(second.ID, coverEntry{"entry-2", "https://second.example/2.png"})
	aliased, solo := f.book(first.ID, "entry-1"), f.book(first.ID, "solo")
	f.alias(aliased, second.ID, "entry-2")
	f.save(first.ID, "entry-1", genFirst, "first-image")
	f.save(first.ID, "solo", genFirst, "solo-image")
	f.save(second.ID, "entry-2", genSecond, "second-image")
	require.Equal(t, "entry-1", f.selected(aliased))

	// Both selected entries stop advertising an image in a complete run.
	f.reconcile(first.ID, coverEntry{"entry-1", ""}, coverEntry{"solo", ""})
	assert.Equal(t, "entry-2", f.selected(aliased), "falls back to the other alias's image")
	assert.Equal(t, "second-image", f.bytes(aliased))
	assert.Equal(t, domain.BookCoverNone, f.state(solo))
	assert.Empty(t, f.selected(solo))
	assert.Equal(t, 1, f.entryRows(), "the absent entries' rows are gone")
}

func TestBookCoverSurvivesEntryLeavingIndexUnusedOneDoesNot(t *testing.T) {
	f := newCoverStoreFixture(t, "leaves-index-owner")
	conn := f.connection("Catalog")
	gen := f.reconcile(conn.ID, coverEntry{"used", "https://c.example/used.png"}, coverEntry{"unused", "https://c.example/unused.png"})
	used := f.book(conn.ID, "used")
	f.save(conn.ID, "used", gen, "used-image")
	f.save(conn.ID, "unused", gen, "unused-image")

	f.reconcile(conn.ID)
	assert.Equal(t, "used-image", f.bytes(used))
	assert.Equal(t, 1, f.entryRows())
	_, err := f.store.GetCatalogEntryCoverResource(f.ctx, f.owner, conn.ID, "unused")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestBookCoverConnectionDeletionKeepsUsedCoversOnly(t *testing.T) {
	f := newCoverStoreFixture(t, "delete-connection-owner")
	conn := f.connection("Catalog")
	gen := f.reconcile(conn.ID,
		coverEntry{"used", "https://c.example/used.png"}, coverEntry{"unused", "https://c.example/unused.png"}, coverEntry{"waiting", "https://c.example/waiting.png"})
	used, waiting := f.book(conn.ID, "used"), f.book(conn.ID, "waiting")
	f.save(conn.ID, "used", gen, "used-image")
	f.save(conn.ID, "unused", gen, "unused-image")
	// "waiting" is still pending when the connection goes away.
	_, err := f.store.Pool().Exec(f.ctx, `INSERT INTO book_covers(owner_id, book_id, selected_connection_id, selected_source_identifier) VALUES($1,$2,$3,'waiting')`, f.owner, waiting, conn.ID)
	require.NoError(t, err)

	require.NoError(t, f.store.DeleteOpdsConnection(f.ctx, f.owner, conn.ID))

	assert.Equal(t, "used-image", f.bytes(used), "a cover a Book uses survives its connection")
	assert.Equal(t, 2, f.entryRows(), "only the unused cover was removed")
	assert.Equal(t, domain.BookCoverUnavailable, f.state(waiting), "a retrieval that can never run is unavailable")
	_, err = f.store.GetCatalogEntryCoverResource(f.ctx, f.owner, conn.ID, "unused")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestBookCoverEndpointReadRequiresActiveMembershipAndOwner(t *testing.T) {
	f := newCoverStoreFixture(t, "membership-owner")
	conn := f.connection("Catalog")
	gen := f.reconcile(conn.ID, coverEntry{"entry-1", "https://c.example/1.png"})
	bookID := f.book(conn.ID, "entry-1")
	f.save(conn.ID, "entry-1", gen, "image")

	resource, err := f.store.GetActiveBookCoverResource(f.ctx, f.owner, bookID)
	require.NoError(t, err)
	assert.Equal(t, "image", string(resource.Bytes))
	other, err := f.store.CreateUser(f.ctx, "other-cover-owner", false)
	require.NoError(t, err)
	_, err = f.store.GetActiveBookCoverResource(f.ctx, other.ID, bookID)
	require.ErrorIs(t, err, ErrNotFound)
	require.NoError(t, f.store.RemoveBookFromMyBooks(f.ctx, f.owner, bookID))
	_, err = f.store.GetActiveBookCoverResource(f.ctx, f.owner, bookID)
	require.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, "image", f.bytes(bookID), "removal keeps the retained cover")
}

func TestSaveCatalogEntryCoverPropagatesSelectionFailure(t *testing.T) {
	f := newCoverStoreFixture(t, "selection-failure-owner")
	conn := f.connection("Catalog")
	gen := f.reconcile(conn.ID, coverEntry{"entry-1", "https://c.example/1.png"})
	f.book(conn.ID, "entry-1")
	raiseOnWrite(t, f.ctx, f.store, "book_covers", "INSERT", "")

	err := f.store.SaveCatalogEntryCover(f.ctx, f.owner, conn.ID, "entry-1", gen, "image/png", 4, 6, "hash", []byte("image"))
	require.Error(t, err)
}
