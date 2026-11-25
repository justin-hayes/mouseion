//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The expand migration that creates the entry store, the data-only backfill,
// and the contract that drops the per-Book bytes (ADR 0038, ADR 0130).
const (
	catalogEntryCoversMigration  = 64
	catalogEntryCoverBackfill    = 65
	bookCoversContractMigration  = 66
	bookCoverMigrationOwnerLabel = "cover-migration-owner"
)

// migrateThroughCatalogEntryCovers builds a database at the expand step, where
// book_covers still holds per-Book image bytes.
func migrateThroughCatalogEntryCovers(databaseURL string) (err error) {
	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	if err != nil {
		return err
	}
	defer func() {
		sourceErr, databaseErr := m.Close()
		err = errors.Join(err, sourceErr, databaseErr)
	}()
	return m.Migrate(catalogEntryCoversMigration)
}

// TestBookCoverBackfillCarriesExistingCoversThroughTheContract seeds per-Book
// covers in every state, runs the backfill and the contract, and proves each
// Book still resolves to what it showed before without any retrieval.
func TestBookCoverBackfillCarriesExistingCoversThroughTheContract(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, migrateThroughCatalogEntryCovers)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, bookCoverMigrationOwnerLabel, false)
	require.NoError(t, err)
	connection := addFailureConnection(t, ctx, store, owner.ID, "Catalog")

	newBook := func(title string) string {
		book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
		require.NoError(t, err)
		return book.ID
	}
	available, pending, unavailable, none := newBook("Available"), newBook("Pending"), newBook("Unavailable"), newBook("None")
	_, err = pool.Exec(ctx, `
		INSERT INTO book_covers(owner_id, book_id, state, selected_connection_id, selected_source_identifier, media_type, width, height, content_hash, bytes, advertised_at, fetched_at, failure_reason)
		VALUES ($1, $2, 'available', $5, 'entry-available', 'image/png', 4, 6, 'hash-available', '\x696d616765', now(), now(), NULL),
		       ($1, $3, 'pending', $5, 'entry-pending', NULL, NULL, NULL, NULL, NULL, now(), NULL, NULL),
		       ($1, $4, 'unavailable', $5, 'entry-unavailable', NULL, NULL, NULL, NULL, NULL, now(), NULL, 'fetch failed'),
		       ($1, $6, 'none', NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL, NULL)`,
		owner.ID, available, pending, unavailable, connection.ID, none)
	require.NoError(t, err)

	moveApplicationMigrationsTo(t, databaseURL, catalogEntryCoverBackfill)
	countEntryRows := func() (rows, availableRows int) {
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE state = 'available') FROM catalog_entry_covers WHERE owner_id = $1`, owner.ID).Scan(&rows, &availableRows))
		return rows, availableRows
	}
	rows, availableRows := countEntryRows()
	assert.Equal(t, 3, rows, "one entry row per selected pair; a Book with no selection has none")
	assert.Equal(t, 1, availableRows)

	// The backfill is idempotent: running its statement again changes nothing.
	statement, err := migrations.FS.ReadFile("000065_catalog_entry_cover_backfill.up.sql")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, string(statement))
	require.NoError(t, err)
	rowsAgain, availableAgain := countEntryRows()
	assert.Equal(t, rows, rowsAgain)
	assert.Equal(t, availableRows, availableAgain)

	moveApplicationMigrationsTo(t, databaseURL, bookCoversContractMigration)
	cover, err := store.GetBookCover(ctx, owner.ID, available)
	require.NoError(t, err)
	assert.Equal(t, domain.BookCoverAvailable, cover.State)
	assert.Equal(t, 4, cover.Width)
	resource, err := store.GetBookCoverResource(ctx, owner.ID, available)
	require.NoError(t, err)
	assert.Equal(t, "image", string(resource.Bytes))
	assert.Equal(t, "hash-available", resource.ContentHash)
	for book, want := range map[string]string{pending: domain.BookCoverPending, unavailable: domain.BookCoverUnavailable, none: domain.BookCoverNone} {
		cover, err := store.GetBookCover(ctx, owner.ID, book)
		require.NoError(t, err)
		assert.Equal(t, want, cover.State)
	}
	var selections int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM book_covers WHERE owner_id = $1`, owner.ID).Scan(&selections))
	assert.Equal(t, 3, selections, "the Book with no selection keeps no pointer row")
	var tableExists bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT to_regclass('public.book_cover_candidates') IS NOT NULL`).Scan(&tableExists))
	assert.False(t, tableExists)
}

// TestBookCoverContractRefusesToDropImagesWithNoSelectedEntry proves the
// contract does not silently lose an available image that has nowhere to move.
func TestBookCoverContractRefusesToDropImagesWithNoSelectedEntry(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, migrateThroughCatalogEntryCovers)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, bookCoverMigrationOwnerLabel, false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Orphan", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		INSERT INTO book_covers(owner_id, book_id, state, media_type, width, height, content_hash, bytes)
		VALUES ($1, $2, 'available', 'image/png', 4, 6, 'hash', '\x696d616765')`, owner.ID, book.ID)
	require.NoError(t, err)

	moveApplicationMigrationsTo(t, databaseURL, catalogEntryCoverBackfill)
	source, err := iofs.New(migrations.FS, ".")
	require.NoError(t, err)
	migrator, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	require.NoError(t, err)
	err = migrator.Migrate(bookCoversContractMigration)
	sourceErr, databaseErr := migrator.Close()
	if closeErr := errors.Join(sourceErr, databaseErr); closeErr != nil {
		t.Errorf("close migrator: %v", closeErr)
	}
	require.Error(t, err)
	var remaining int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM book_covers WHERE state = 'available'`).Scan(&remaining))
	assert.Equal(t, 1, remaining, "the failed contract changed nothing")
}
