//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookDispositionBackfillUsesDeterministicLegacyPrecedence(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	// Seed legacy state at version 15 so migration 16 is exercised as a real
	// successor data migration; migrations 17 and 18 are then applied afterward.
	moveApplicationMigrationsTo(t, databaseURL, 15)
	owner, err := store.CreateUser(ctx, "disposition-backfill-owner", false)
	require.NoError(t, err)
	otherOwner, err := store.CreateUser(ctx, "disposition-backfill-other", false)
	require.NoError(t, err)

	goalBook := insertLegacyBook(t, ctx, store, owner.ID, "Goal book", "de")
	journeyBook := insertLegacyBook(t, ctx, store, owner.ID, "Journey book", "de")
	historyBook := insertLegacyBook(t, ctx, store, owner.ID, "History book", "de")
	removedBook := insertLegacyBook(t, ctx, store, owner.ID, "Removed book", "de")
	inboxBook := insertLegacyBook(t, ctx, store, owner.ID, "Inbox book", "de")
	otherBook := insertLegacyBook(t, ctx, store, otherOwner.ID, "Other owner's book", "de")

	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id, language, book_id) VALUES ($1, 'de', $2)`, owner.ID, goalBook.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_journeys(owner_id, language) VALUES ($1, 'de'), ($2, 'de')`, owner.ID, otherOwner.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id, language, book_id, position) VALUES ($1, 'de', $2, 1), ($1, 'de', $3, 2), ($4, 'de', $5, 1)`, owner.ID, goalBook.ID, journeyBook.ID, otherOwner.ID, otherBook.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO reading_history(owner_id, language, book_id, completed_at) VALUES ($1, 'de', $2, now()), ($1, 'de', $3, now()), ($1, 'de', $4, now())`, owner.ID, goalBook.ID, journeyBook.ID, historyBook.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE book_membership SET state = 'removed', removed_at = now() WHERE owner_id=$1 AND book_id=$2`, owner.ID, removedBook.ID)
	require.NoError(t, err)

	moveApplicationMigrationsTo(t, databaseURL, 16)

	assertMigrationDisposition(t, pool, owner.ID, goalBook.ID, domain.BookDispositionToRead)
	assertMigrationDisposition(t, pool, owner.ID, journeyBook.ID, domain.BookDispositionToRead)
	assertMigrationDisposition(t, pool, owner.ID, historyBook.ID, domain.BookDisposition("set_aside"))
	assertMigrationDisposition(t, pool, owner.ID, removedBook.ID, domain.BookDisposition("set_aside"))
	assertMigrationDisposition(t, pool, owner.ID, inboxBook.ID, domain.BookDispositionInbox)
	assertMigrationDisposition(t, pool, otherOwner.ID, otherBook.ID, domain.BookDispositionToRead)

	// Reapplying after the no-op down migration must not reinterpret or change
	// an existing learner decision, proving retry safety independently of the
	// migration runner's normal all-or-nothing transaction.
	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='set_aside' WHERE owner_id=$1 AND book_id=$2`, owner.ID, inboxBook.ID)
	require.NoError(t, err)
	moveApplicationMigrationsTo(t, databaseURL, 15)
	moveApplicationMigrationsTo(t, databaseURL, 16)
	assertMigrationDisposition(t, pool, owner.ID, inboxBook.ID, domain.BookDisposition("set_aside"))

	// Set Aside is retired and migration 33 refuses legacy rows rather than
	// converting them, so retire the ones this historical backfill created.
	moveApplicationMigrationsTo(t, databaseURL, 32)
	_, err = pool.Exec(ctx, `UPDATE book_dispositions SET disposition='inbox' WHERE disposition='set_aside'`)
	require.NoError(t, err)
	migrateApplicationMigrationsToLatest(t, databaseURL)
	toRead, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 2, toRead.Total)
	assert.Equal(t, 2, dispositionCountForBucket(toRead.DispositionCounts, domain.BookDispositionToRead))
	for _, item := range toRead.Items {
		if item.Book.ID == goalBook.ID {
			assert.Equal(t, domain.MyBookBucketCurrentReading, item.WorkflowBucket(), "migrated primary goal remains visible in To Read")
			return
		}
	}
	assert.Fail(t, "migrated primary goal missing from To Read")
}

func assertMigrationDisposition(t *testing.T, pool *pgxpool.Pool, owner, book string, want domain.BookDisposition) {
	t.Helper()
	var got string
	err := pool.QueryRow(context.Background(), `SELECT disposition FROM book_dispositions WHERE owner_id=$1 AND book_id=$2`, owner, book).Scan(&got)
	require.NoError(t, err)
	assert.Equal(t, string(want), got)
}
