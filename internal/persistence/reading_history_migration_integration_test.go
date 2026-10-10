//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingHistoryBackfillPreservesOwnerLanguageAndKnownState(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	// Return to the pre-feature schema so this test exercises the shipped
	// backfill and cleanup migrations rather than reproducing their SQL.
	// The re-preparation and cover migrations are newer than this historical
	// scenario; go back to the same pre-reading-history version before
	// replaying its steps.
	// Rewind to version 3 before replaying the historical migrations. The
	// disposition backfill is a later successor and must not run before the
	// legacy state is seeded.
	moveApplicationMigrationsTo(t, databaseURL, 3)
	owner, err := store.CreateUser(ctx, "history-migration-owner", false)
	require.NoError(t, err)
	otherOwner, err := store.CreateUser(ctx, "history-migration-other", false)
	require.NoError(t, err)
	finishedBook := insertLegacyBook(t, ctx, store, owner.ID, "Migrated finished book", "de")
	activeBook := insertLegacyBook(t, ctx, store, owner.ID, "Unfinished Italian book", "it")
	otherBook := insertLegacyBook(t, ctx, store, otherOwner.ID, "Other owner's book", "de")
	completedAt := time.Date(2026, time.January, 20, 12, 0, 0, 0, time.UTC)
	_, err = pool.Exec(ctx, `INSERT INTO reading_journeys(owner_id, language) VALUES ($1, 'de'), ($1, 'it'), ($2, 'de')`, owner.ID, otherOwner.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id, language, book_id, position) VALUES ($1, 'de', $3, 1), ($1, 'it', $4, 1), ($2, 'de', $5, 1)`, owner.ID, otherOwner.ID, finishedBook.ID, activeBook.ID, otherBook.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id, language, book_id, reading_finished_at) VALUES ($1, 'de', $3, $6), ($1, 'it', $4, NULL), ($2, 'de', $5, NULL)`, owner.ID, otherOwner.ID, finishedBook.ID, activeBook.ID, otherBook.ID, completedAt)
	require.NoError(t, err)
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "Haus", "NOUN")
	require.NoError(t, err)

	moveApplicationMigrationsTo(t, databaseURL, 5)
	forceApplicationMigration(t, databaseURL, 4)
	moveApplicationMigrationsTo(t, databaseURL, 5)
	moveApplicationMigrationsTo(t, databaseURL, 10)
	var historyCount int
	var migratedAt time.Time
	err = pool.QueryRow(ctx, `SELECT count(*), max(completed_at) FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&historyCount, &migratedAt)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount)
	assert.True(t, completedAt.Equal(migratedAt), "completion timestamp changed: got %s want %s", migratedAt, completedAt)
	// The historical assertions above stop at version 10. Restore the current
	// schema before exercising the current persistence methods below.
	// Set Aside is retired and migration 33 refuses legacy rows rather than
	// converting them, so retire the ones this historical backfill created.
	moveApplicationMigrationsTo(t, databaseURL, 32)
	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='inbox' WHERE disposition='set_aside'`)
	require.NoError(t, err)
	migrateApplicationMigrationsToLatest(t, databaseURL)
	goal, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, goal.BookID, "finished Goal survived backfill")
	italianGoal, err := store.GetCurrentReading(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Equal(t, activeBook.ID, italianGoal.BookID, "language-isolated active Goal was changed")
	otherGoal, err := store.GetCurrentReading(ctx, otherOwner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, otherBook.ID, otherGoal.BookID, "owner-isolated active Goal was changed")
	finishedDisposition, err := store.GetBookDisposition(ctx, owner.ID, finishedBook.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.BookDispositionInbox, finishedDisposition, "finished Book disposition changed during history backfill")
	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, known, 1, "backfill changed Known vocabulary")
}

// insertLegacyBook seeds the pre-author schema used by migration backfill
// scenarios. The current application write path intentionally targets the
// latest schema and cannot create rows while those tests are rolled back.
func insertLegacyBook(t *testing.T, ctx context.Context, store *PostgresStore, owner, title, language string) domain.Book {
	t.Helper()
	var bookID string
	err := store.Pool().QueryRow(ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag) VALUES ($1, $2, $3, $4, $5) RETURNING id::text`, owner, title, domain.MetadataProvenanceCatalogueSync, domain.LanguageChosen, language).Scan(&bookID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_membership(owner_id, book_id, state, activated_at) VALUES ($1, $2, 'active', now())`, owner, bookID)
	require.NoError(t, err)
	return domain.Book{ID: bookID, OwnerID: owner, Title: title, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language}
}

func moveApplicationMigrationsTo(t *testing.T, databaseURL string, version uint) {
	t.Helper()
	source, err := iofs.New(migrations.FS, ".")
	require.NoError(t, err)
	migrator, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	require.NoError(t, err)
	err = migrator.Migrate(version)
	sourceErr, databaseErr := migrator.Close()
	require.NoError(t, err)
	require.NoError(t, sourceErr)
	require.NoError(t, databaseErr)
}

func migrateApplicationMigrationsToLatest(t *testing.T, databaseURL string) {
	t.Helper()
	source, err := iofs.New(migrations.FS, ".")
	require.NoError(t, err)
	migrator, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	require.NoError(t, err)
	err = migrator.Up()
	sourceErr, databaseErr := migrator.Close()
	require.NoError(t, err)
	require.NoError(t, sourceErr)
	require.NoError(t, databaseErr)
}

func forceApplicationMigration(t *testing.T, databaseURL string, version int) {
	t.Helper()
	source, err := iofs.New(migrations.FS, ".")
	require.NoError(t, err)
	migrator, err := migrate.NewWithSourceInstance("iofs", source, databaseURL)
	require.NoError(t, err)
	err = migrator.Force(version)
	sourceErr, databaseErr := migrator.Close()
	require.NoError(t, err)
	require.NoError(t, sourceErr)
	require.NoError(t, databaseErr)
}
